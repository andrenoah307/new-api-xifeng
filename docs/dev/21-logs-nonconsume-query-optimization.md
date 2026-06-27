# 超大 logs 表「非消费」类型查询优化

> 状态：**代码已改，迁移 SQL 待执行确认**（dev-20260512，2026-06-27）
> 关联坑点：[`99-pitfalls.md#130`](99-pitfalls.md)
> ⚠️ **上线顺序硬约束：先在线建索引，再部署本代码**（见 §4）

---

## 1. 背景与根因

管理员按**非「消费」类型**（topup/manage/system/error/refund）筛选日志时慢，「消费」类型快。

`logs` 超大、写密集。查询每次（首页/换筛选）跑两条：
1. **SELECT**：`WHERE type=? AND created_at∈[t0,t1] [+filters] ORDER BY id DESC LIMIT n OFFSET k`
2. **COUNT**：管理员=全量 `COUNT(*)`（坑点 #69 故意移除封顶以求精确）。

原有索引**无以 `type` 打头者**：`idx_created_at_type=(created_at,type)` 的 type 是第二列。
- 消费(type=2)占全表 ~99%：PK 倒序扫几下就凑满一页 → 快。
- 非消费**稀疏**：PK 倒序要扫过海量消费行才凑够一页（SELECT 慢）；`(created_at,type)` 上 COUNT 也要扫遍窗内全部 created_at 条目逐行验 type（COUNT 慢）。

## 2. 方案（Path A：在线加索引 + 排序对齐）

**新增复合索引 `idx_logs_type_created_at (type, created_at)`**：type 打头 → `WHERE type=X AND created_at∈窗` 直接 seek 到该类型再按时间范围扫，**只碰匹配行** → COUNT/SELECT 均**精确且快**，无需封顶、不丢页码跳转、前端零改动。

代码侧（`model/log.go`，已改）：
- `Log` 结构体加该复合索引的 gorm tag（索引名 `idx_logs_type_created_at`，列序 `(type, created_at)`，priority Type=1/CreatedAt=2）。
- `GetAllLogs` / `GetUserLogs` 的 `ORDER BY logs.id desc` → **`ORDER BY logs.created_at desc, logs.id desc`**：created_at 与 id 对 append-only 日志单调同向，结果序不变；但让 `(type,created_at)`（含 type 过滤）/`(created_at,type)`（全类型）索引直接提供有序扫描，免 filesort/PK 全扫。
- **COUNT 保持精确全量**（加索引后已快），不封顶——不反转坑点 #69。

## 3. 迁移 SQL（待你确认后由 DBA 执行，在线不锁表）

> 在**日志库**执行：若设置了 `LOG_SQL_DSN` 则在该库，否则与主库相同。索引名**必须**为 `idx_logs_type_created_at`（与 gorm tag 一致，AutoMigrate 才会识别为已存在而跳过）。

**MySQL 5.7.8+/8.0**（InnoDB online DDL，加二级索引不重建表，并发读写）：
```sql
ALTER TABLE logs
  ADD INDEX idx_logs_type_created_at (type, created_at),
  ALGORITHM=INPLACE, LOCK=NONE;
```

**PostgreSQL 9.6+**（不锁写；**必须在事务外**单独执行，勿包进 BEGIN/COMMIT）：
```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_type_created_at
  ON logs (type, created_at);
-- 若中途失败会留下 INVALID 索引，清理后重试：
-- DROP INDEX IF EXISTS idx_logs_type_created_at;
```

**SQLite**（仅开发/小库，短锁可接受；或交给 AutoMigrate 自动建）：
```sql
CREATE INDEX IF NOT EXISTS idx_logs_type_created_at ON logs (type, created_at);
```

> 代价：超大表建索引耗时/IO/临时空间（不阻塞流量但占资源，建议低峰执行）；`logs` 写密集，多一个 `(int,bigint)` 索引带来少量插入开销（可控）。MySQL 如对原生 INPLACE 不放心，可用 `pt-online-schema-change` / `gh-ost`。

## 4. 上线顺序（D2：DBA 手动 + gorm tag）⚠️

1. **先**在生产日志库执行上面对应库的在线 DDL，确认索引 `idx_logs_type_created_at` 建成：
   - MySQL：`SHOW INDEX FROM logs;`
   - PG：`\d logs` 或 `SELECT indexname FROM pg_indexes WHERE tablename='logs';`
2. **再**部署本代码。AutoMigrate(`migrateLOGDB`→`AutoMigrate(&Log{})`) 见同名索引已存在 → **no-op**，不会发阻塞 DDL。
3. 全新/小型安装：无需手动，AutoMigrate 直接建（表小、瞬时）。

> ❌ **若先部署代码、后建索引**：启动时 AutoMigrate 会对超大表发**阻塞** `CREATE INDEX` → 线上卡顿。务必"索引先行"。

## 5. 验证（EXPLAIN 应命中新索引）
```sql
-- MySQL，X 取某稀疏类型如 3(manage)
EXPLAIN SELECT * FROM logs
  WHERE type=3 AND created_at >= UNIX_TIMESTAMP()-86400
  ORDER BY created_at DESC, id DESC LIMIT 10;
-- key 期望 = idx_logs_type_created_at
```

## 6. 回滚
- 代码：撤销 `model/log.go` 的 tag 与 ORDER BY 改动。
- 索引：`ALTER TABLE logs DROP INDEX idx_logs_type_created_at;`（MySQL）/ `DROP INDEX CONCURRENTLY IF EXISTS idx_logs_type_created_at;`（PG）。

## 7. 范围说明（已评审收敛）
- **stat 无需改**：`SumUsedQuota`(`/api/log/stat`) 恒 `WHERE type=LogTypeConsume`，本就是消费聚合、在快路径，无非消费场景。
- **keyset 游标(O2)/服务端时间窗护栏(O4) 在加索引后已非必要**：索引使非消费 COUNT/SELECT（含 OFFSET，因匹配行少）都快且保留页码跳转；keyset 仅对「消费/全类型」深翻页有边际收益，却要改双前端 + 丢「跳第 N 页」。**本批不做**，如需深翻页通用优化再单独评估。
- 因此本特性**前端零改动**。

## 8. 参考
- 坑点：`docs/dev/99-pitfalls.md` #130、#69（日志 COUNT 上限策略）
- 代码：`model/log.go`（Log 结构体索引、`GetAllLogs`/`GetUserLogs`）、`model/main.go:migrateLOGDB`
