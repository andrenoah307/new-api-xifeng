# 风控中心分组化二次开发指南（DEV_GUIDE）

> 分支：`dev/risk-control-group-scoping-20260502`
> 适用范围：将风控中心从"作用域级（token / user）"升级到"分组级"评估、存储、解封、审计与 UI

---

## 0. 阅读建议

先读第 1～4 节（领域知识与现状链路），再读第 5 节（变更方案）与第 6 节（场景对比矩阵）。第 7～8 节是验证清单与踩坑表。

---

## 1. 领域名词与底线

### 1.1 名词

| 名词 | 含义 | 来源 |
|---|---|---|
| `user.Group` | 用户所属分组（一对一） | `model/user.go:43` |
| `token.Group` | API key 所属分组（可空，空表示用 `user.Group`） | `model/token.go:29` |
| `token.CrossGroupRetry` | 仅 `auto` 组下生效，跨可用分组重试 | `model/token.go:30` |
| `ContextKeyUsingGroup` | 实际使用的分组（auto 跨组重试时会变化） | `constant/context_key.go:52`，键名是字面量 `"group"` |
| `ContextKeyUserGroup` | 用户分组的上下文映射 | `constant/context_key.go:51` |
| `ContextKeyTokenGroup` | Token 分组的上下文映射 | `constant/context_key.go:17` |
| `ContextKeyAutoGroup` | auto 组下当前回退命中的实际分组 | `constant/context_key.go:41` |
| `RelayInfo.UsingGroup` | relay 阶段的"当前分组"（auto 重试会变） | `relay/common/relay_info.go:92` |
| `RelayInfo.UserGroup` | 用户分组（不变） | `relay/common/relay_info.go:93` |
| `RelayInfo.TokenGroup` | Token 配置分组（空时回退到用户分组） | `relay/common/relay_info.go:90, 432-436` |
| `Channel.GetGroups()` | 渠道挂载的分组集合 | `model/channel.go:203` |
| `setting.GetAutoGroups()` | 全局 `auto` 命中顺序 | 见 `setting/group.go` 系列 |
| `service.GetUserUsableGroups(userGroup)` | 用户可用分组字典 | `service/group.go:10` |

### 1.2 底线（来自 CLAUDE.md & 仓库惯例）

1. JSON 操作只走 `common.Marshal/Unmarshal/UnmarshalJsonStr/DecodeJson`
2. DB 同时兼容 SQLite / MySQL 5.7.8+ / PostgreSQL 9.6+；`group` 是关键字，手写 SQL 必须 `commonGroupCol`
3. Modal 必加 `centered + bodyStyle.overflowY:'auto' + getPopupContainer`
4. Upstream relay 请求 DTO 用指针 + `omitempty` 保留显式零值
5. 受保护标识符：`new-api`、`QuantumNous`，绝不能改

---

## 2. 分组体系全景

### 2.1 层级关系

```
user.Group  ──┐                         ┌── auto:
              │                         │     setting.GetAutoGroups() = [g1, g2, ...]
              ├─→ token.Group(可空) ────┤      首次按用户视角"挑首个可用 g_i"
              │                         │      失败后按 cross_group_retry 切下一个
              │                         └── 显式分组：直接 = token.Group
              │
              └─→ ContextKeyUserGroup（不变，用于权限校验）

ContextKeyUsingGroup = 真正用于：渠道选择 / 计费倍率 / 速率限制 / 风控
auto 跨组重试会改写它
```

### 2.2 分组身份解析路径（请求生命周期）

1. **TokenAuth** `middleware/auth.go:330+`
   - 从 `Authorization` 解析 token → 加载 `user`
   - 校验 `token.Group` 是否在 `service.GetUserUsableGroups(user.Group)`（auth.go:440）
   - `c.SetContextKey(UsingGroup, userGroup)`（auth.go:453；若 token.Group 非空，userGroup 已被覆盖）
   - `c.SetContextKey(TokenGroup, token.Group)`（auth.go:481）
   - 写 `id / token_id / token_key / token_name / username` 等
2. **Distribute** `middleware/distributor.go:30+`
   - 读模型与（playground 下）请求体里的 `group`（仅 playground 路径允许覆盖）
   - 用 `ContextKeyUsingGroup` 选 channel：`service.CacheGetRandomSatisfiedChannel`
   - 若 `auto`：走 `GetUserAutoGroup` 序列，遇可用 channel 写 `ContextKeyAutoGroup` 并把 `selectGroup` 用作"实际生效分组"
   - `SetupContextForSelectedChannel(c, channel, modelName)`：写 `channel_id/channel_name/channel_type/...`
3. **Relay handler** `controller/relay.go:119`
   - `relaycommon.GenRelayInfo` 读出 `UsingGroup/UserGroup/TokenGroup/ChannelId/...`
   - **风控 BeforeRelay**：以 `info.UsingGroup` 为分组维度（注意：auto 跨组重试后已经是回退后的真实分组）
   - 业务执行
   - defer **风控 AfterRelay**：finish 事件入队
4. **错误日志 / 成功日志**
   - `controller/relay.go:443-446` 异常分支：`other.risk_control = audit`
   - `service/log_info_generate.go:82` 正常分支：同上
   - log 表已带 `group` 字段（业务沿用 `c.GetString("group")` 即 `ContextKeyUsingGroup`）

### 2.3 分组列表来源（前端可复用）

- **管理员视角**：`GET /api/group/` → `controller/group.go:14 GetGroups` → `ratio_setting.GetGroupRatioCopy()` 全部 group key
- **普通用户视角**：`GET /api/user/groups` → `GetUserGroups` → `service.GetUserUsableGroups(userGroup)` 过滤后的字典
- 前端已有大量复用：`web/classic/src/components/table/users/modals/EditUserModal.jsx:101` 等

### 2.4 渠道与分组

- 一个 channel 可挂多个 group：`Channel.GetGroups() []string`
- `model.IsChannelEnabledForGroupModel(group, model, channelId)` 校验某 channel 是否对该 group+model 启用
- `auto` 组下：`service.GetUserAutoGroup(userGroup)` 给出回退序列
- 风控**不感知 channel**：风控的主体只有 token/user，channel 信息仅用于审计 `other.admin_info.use_channel` 字段，不参与决策

---

## 3. 风控中心当前状态全景

### 3.1 数据流

```
[TokenAuth → Distribute → relay handler]
                                │
              ┌─────────────────┴─────────────────┐
              ▼                                   ▼
  RiskControlBeforeRelay(c, info)      defer RiskControlAfterRelay
  (enforce 模式查闸门，命中即拒)        (入队 finish 事件)
              │
              ├─ 入队 start 事件（含 group=info.UsingGroup）
              │  → 后台 worker:
              │     RecordStart(scope, subjectID, ipHash, uaHash)
              │     evaluateRiskRules(rules, scope, metrics)   ← 当前不含 group 过滤
              │     buildRiskDecision → Snapshot upsert / Incident insert
              │
              └─ enforce 命中：types.NewErrorWithStatusCode(ErrorCodeRiskControlBlocked, SkipRetry)
```

### 3.2 风控事件载荷（`service.RiskEvent`）

来自 `c` 与 `info`：
- `RequestID`、`RequestPath`
- `UserID/Username`、`TokenID/TokenName/TokenMaskedKey`
- `Group`：取 `info.UsingGroup`（注意：可能在 auto 重试后已变）
- `ClientIPHash`：`HMAC(requestip.GetClientIP(c))`
- `UserAgentHash`：`HMAC(normalize(UA))`，仅 token scope 有意义
- `StatusCode`：finish 事件用 writer.Status，闸门拒绝用 decision.StatusCode

### 3.3 当前规则模型字段

- `Scope`：`token` / `user`（**没有** group）
- `Conditions`：JSON `[]types.RiskCondition`
- `MatchMode`：`all` / `any`
- `Action`：`observe` / `block`
- `AutoBlock`/`AutoRecover`/`RecoverMode`/`RecoverAfterSeconds`
- 决策逻辑只在 enforce 模式才真正写 SetBlock 与本地缓存

### 3.4 当前指标维度

`(scope, subjectID)` 二元组：
- `distinct_ip_10m / distinct_ip_1h`：HyperLogLog
- `distinct_ua_10m`：仅 token
- `tokens_per_ip_10m`：仅 token；按 `ipHash` 维度 bucket（不区分 group）
- `request_count_1m / 10m`、`inflight_now`、`rule_hit_count_24h`、`risk_score`

### 3.5 闸门缓存 `localRiskGateCacheEntry`

- key：`scope:subjectID`（**不含 group**）
- TTL = `RiskControlSetting.LocalCacheSeconds`（默认 2s）
- 所有副本各自维护，没有 pub/sub 同步

### 3.6 已知行为

- enforce 模式以外，命中规则只产生 observe（决策被强降级）
- `tokens_per_ip_10m` **跨组聚合**（同一 IP 的所有 token 在所有组都进同一桶）
- 同一 token 在 vip/free 组的请求**指标互相污染**
- 解封作用于 `(scope, subjectID)`，没有"按组单独解封"的概念

---

## 4. 详细文件链路（事实记录，便于改动定位）

| 关注点 | 文件 / 行 |
|---|---|
| 风控启动 | `main.go:115-116` |
| 风控前置 | `controller/relay.go:124-134`、`controller/relay.go:443-446` |
| 风控后置 | `controller/relay.go:131-134`（defer） |
| 风控事件构造 | `service/risk_control.go:371-404 buildEvent` |
| 规则解析 / 重载 | `service/risk_control.go:304-324 reloadRules` |
| 规则评估 | `service/risk_control.go:528-573` |
| 决策构造 | `service/risk_control.go:642-726` |
| 主体快照 / Incident 落库 | `service/risk_control.go:485-526, 728-859` |
| Recovery loop | `service/risk_control.go:1092-1140` |
| 阻断访问异步更新 | `service/risk_control.go:1181-1206 RecordRiskBlockedAccess` |
| Redis store | `service/risk_control_store.go:36-303` |
| 内存 store | `service/risk_control_store.go:329-650` |
| Rule 表 | `model/risk_rule.go` |
| Snapshot 表 | `model/risk_subject_snapshot.go` |
| Incident 表 | `model/risk_incident.go` |
| 控制器 | `controller/risk.go` |
| 路由 | `router/api-router.go:259-273` |
| 配置 | `setting/operation_setting/risk_control_setting.go` |
| 错误码 | `types/error.go:44 ErrorCodeRiskControlBlocked` |
| 客户端 IP 提取 | `pkg/requestip/requestip.go:68 GetClientIP` |
| 前端入口 | `web/classic/src/App.jsx:165`、`web/classic/src/components/layout/SiderBar.jsx:191-195` |
| 前端页面 | `web/classic/src/pages/Risk/index.jsx`（≈2000 行） |
| 分组数据源（前端） | `GET /api/group/`（管理员）、`GET /api/user/groups`（普通用户） |
| 用户分组解析 | `service/group.go:10 GetUserUsableGroups` |
| auto 序列 | `service/group.go:45 GetUserAutoGroup` |
| 渠道分组 | `model/channel.go:203 GetGroups()`、`model/channel.go IsChannelEnabledForGroupModel` |

---

## 5. 设计原则（本次改造不可动摇的红线）

1. **未配置分组 = 不启用**：`groups` 为空（数组或空串）的规则视为未配置；`reloadRules` 跳过加载；`validateRiskRule` 在 `enabled=true` 时强制要求 `len(groups)>=1`
2. **规则评估按当前 `info.UsingGroup`**：auto 跨组重试后落到的真实分组才是评估维度，不用 `TokenGroup`（保留原始分组）也不用 `UserGroup`
3. **指标三元组隔离**：`(scope, subjectID, group)`；同一 token 在 vip/free 互不污染
4. **空分组事件直接跳过**：`event.Group == ""` 不入队、不评估、不记录（避免空字符串污染）
5. **解封粒度对齐评估粒度**：解封必须带 group；同主体在 A 组解封不影响 B 组封禁
6. **`tokens_per_ip_10m` 默认按组隔离**：与"分组级风控"主旨一致；如需跨组聚合，后续加配置开关 `cross_group_ip_token_metric`（本期不做）
7. **跨库 DDL 兼容**：`group` 列名一律用 `commonGroupCol`；唯一索引重建分库写
8. **审计可追溯**：`RiskAudit` / `RiskDecision` 增 `Group`，落入 log `other.risk_control` 中

---

## 6. 改造蓝图（本期范围）

### 6.1 数据模型

#### `model/risk_rule.go`
- 新增 `Groups string gorm:"type:text"`（JSON 数组字符串）
- `UpdateRiskRule.Updates` 加 `"groups"`
- 增 `(*RiskRule).ParsedGroups() []string`（trim/去空/去重；返回 `nil` 表示未配置）
- 不增数据库索引（应用层过滤即可）

#### `model/risk_subject_snapshot.go`
- 唯一索引扩展为 `(subject_type, subject_id, group)`
- `OnConflict.Columns` 加 `{Name: "group"}`
- `GetRiskSubjectSnapshot(scope, subjectID, group)` 签名扩展
- `RiskSubjectQuery` / `ListRiskSubjectSnapshots` 增 `Group` 过滤

#### `model/risk_incident.go`
- 增 `Group string gorm:"type:varchar(64);index;default:''"`
- `RiskIncidentQuery` 增 `Group`、`ListRiskIncidents` 加过滤
- `buildRiskIncident` 写入 `event.Group`

#### `model/main.go`（迁移段）
- `risk_rules.groups` → `ALTER TABLE ... ADD COLUMN groups TEXT DEFAULT ''`
- `risk_incident.group` → `ALTER TABLE ... ADD COLUMN <commonGroupCol> VARCHAR(64) DEFAULT ''` + 单列索引
- `risk_subject_snapshot` 唯一索引重建
  - PG/SQLite：`DROP INDEX IF EXISTS idx_risk_subject_unique; CREATE UNIQUE INDEX idx_risk_subject_unique ON risk_subject_snapshot (subject_type, subject_id, "group");`（SQLite 用反引号）
  - MySQL 5.7：先查 `information_schema.statistics` 判断索引存在再决定 DROP；CREATE 用反引号
- 三库分支：`common.UsingPostgreSQL/UsingMySQL/UsingSQLite`

### 6.2 类型 & 服务

#### `types/risk.go`
- `RiskDecision.Group string` 新增

#### `service/risk_control_types.go`
- `compiledRiskRule.Groups map[string]struct{}`

#### `service/risk_control.go`
- `reloadRules`：解析 `rule.Groups`；空 → 跳过加载 + `common.SysLog`
- 新增 `ruleAppliesToGroup(rule, group)`：rule.Groups 必须非空且包含 group
- `evaluateRiskRules(rules, scope, group, metrics)`
- `evaluateAndPersistSubject(event, scope, subjectID, group, metrics)`
- `handleStartEvent`：`event.Group == ""` 直接 return
- `handleFinishEvent`：`RecordFinish` 传 group
- `RiskControlBeforeRelay`：`group := info.UsingGroup`，传入 cache 查询
- `getCachedDecision/updateGateCache/riskMemoryKey`：签名加 group
- `UnblockRiskSubject(scope, subjectID, group, operator)`
- `validateRiskRule`：`enabled=true && parsedGroups 空` → 报错 "启用规则前必须至少选择一个分组"
- `seedDefaultRiskRules`：6 条默认规则全部 `Enabled: false` 且 `Groups: ""`，注释说明
- `RecordRiskBlockedAccess`：从 `decision.Group` 读取
- `runRecoveryLoop`：`ClearBlock(snapshot.SubjectType, snapshot.SubjectID, snapshot.Group)`
- `GetRiskOverview`：增 `unconfigured_rule_count`（统计 enabled=true 但 groups 为空的规则数）

#### `service/risk_control_store.go`
- `riskMetricStore` 接口签名全部加 `group string`
- Redis key 命名：
  ```
  rc:block:{scope}:{group}:{subjectID}
  rc:inflight:{scope}:{group}:{subjectID}
  rc:req:{scope}:{group}:{subjectID}:{bucket}
  rc:ip:min:{scope}:{group}:{subjectID}:{bucket}
  rc:ip:hour:{scope}:{group}:{subjectID}:{bucket}
  rc:ua:min:{scope}:{group}:{subjectID}:{bucket}
  rc:ip-tkn:{group}:{ipHash}:{bucket}
  rc:rule-hit:{scope}:{group}:{subjectID}
  ```
- 内存实现 `riskMemoryKey(scope, subjectID, group)`；`ipTokens` 升一层 `map[group]map[ipHash]map[bucket]map[tokenID]struct{}`
- 防御：`group == ""` 返回零值
- sweep 同步遍历最外层 group

### 6.3 控制器与路由

#### `controller/risk.go`
- `riskRuleUpsertRequest` 增 `Groups []string`
- `bindRiskRuleRequest`：trim+去重+去空+`common.Marshal` → `rule.Groups`
- `GetRiskSubjects` / `GetRiskIncidents` 读 `c.Query("group")`
- `UnblockRiskSubject` 读 `c.Query("group")`，空则返回 400 "解封必须指定分组"
- `GetRiskCenterOverview` 透传 `unconfigured_rule_count`

#### `router/api-router.go`
- 解封路径保持 `/subjects/:scope/:id/unblock` 不变
- 前端调用统一加 `?group=xxx`（**方案 B**：兼容性更好）

### 6.4 前端 `web/classic/src/pages/Risk/index.jsx`

- 拉取分组列表：复用 `GET /api/group/`（管理员视角，与现有 EditUserModal/EditChannelModal 同源）
- `RuleEditorModal`：
  - 新增 `Form.Select` 多选 `groups`，提示文案"为空时该规则不生效（仅未配置可保存为草稿）"
  - 启用 switch 旁加红字提示
  - 提交前预校验：`enabled === true && groups.length === 0` → 阻止 + Toast
- 规则列表：
  - 新增"适用分组"列：分组 Tag 集合 / 红色 Tag `未配置（已停用）`
  - 启用 Switch 行内切换：前端预校验
- subjects/incidents Tab：
  - 过滤栏增"分组"下拉
  - 表格增"分组"列
  - 解封请求拼 `?group=`
- 概览：新增"未配置分组的规则数"卡片
- i18n key（zh-CN 主，其他 6 语言用 `bun run i18n:extract` 同步）

### 6.5 测试

#### `service/risk_control_test.go`
- `groups=[]` → reload 后不加载
- `groups=[A]` + event.Group=A → 命中
- `groups=[A]` + event.Group=B → 不命中
- `validateRiskRule(enabled=true, groups=[])` → 报错

#### `service/risk_control_store_test.go`
- `RecordStart(token,1,A,...)` 与 `(token,1,B,...)` 互不污染
- `SetBlock(token,1,A)` 不影响 `GetBlock(token,1,B)`
- 内存 sweep 后跨组数据正确清理

---

## 7. 场景对比矩阵（改造前后行为）

> 表中 ✅=新行为 / ❌=旧行为可能引发的问题

| 场景 | 改造前 | 改造后 |
|---|---|---|
| 同一 token 在 vip+free 组活跃 | ❌ 指标合并：可能因 free 组的高频被 vip 组规则误判 | ✅ 三元组隔离：vip 组指标只反映 vip 流量 |
| 创建规则未选分组就启用 | ❌ 直接生效（"全局规则"） | ✅ 校验拒绝；管理员必须先选组 |
| 升级后存量规则 | ❌ 升级前规则升级后变成"全局" | ✅ 升级后存量规则 groups 空 → 自动停用，UI 标红，管理员显式补齐 |
| `auto` 跨组重试 | 风控按 `info.UsingGroup`（已是回退后的组） | ✅ 与渠道选择/计费保持同分组维度 |
| 同 IP 共用多 token（跨组） | ❌ `tokens_per_ip_10m` 跨组累计，可能误伤 vip 组 token | ✅ 默认按组隔离；vip 内的 token 数自成一桶 |
| 解封操作 | ❌ `(scope, subjectID)` 全组解封 | ✅ 必须指定 group；同 token 在 A 组解封不影响 B 组 |
| 闸门缓存命中 | ❌ key 不含 group：vip 用户被 free 组的封禁波及 | ✅ key 三元组：vip/free 各自独立 |
| Recovery loop 自动恢复 | 全表 update（凭 block_until） | 行为不变；ClearBlock 调用补 group |
| Snapshot 浏览 | 一个 subject 一行，混合所有组活动 | ✅ 一个 subject × 一个 group 一行；可按组过滤 |
| Incident 流水 | 无 group 字段 | ✅ 增 group 列，可按分组追溯 |
| 多副本一致性 | 解封后 `LocalCacheSeconds` 内仍可能拦截 | 行为不变（不引入新跨节点同步） |
| 旧 Redis key | n/a | 改造后旧 key 自然 TTL 过期；不主动清理 |
| 空 group 请求（`info.UsingGroup==""`） | 进风控并以空字符串入桶，污染数据 | ✅ 直接跳过，不入队 |
| 默认 seed 6 条规则 | enabled=true，立刻生效 | ✅ enabled=false + groups=""；管理员显式启用 |
| 概览 metrics | 4 项：观察/封禁/高风险/规则数 | ✅ 增 `unconfigured_rule_count`：醒目提示哪些规则未生效 |

### 决策点（你选定的）

1. ✅ **未配置分组 = 不启用**（启用前强制校验 groups 非空）
2. ✅ **空 `info.UsingGroup` 请求跳过风控**
3. ✅ **解封路由保留旧路径，加 `?group=` query 参数**（兼容性）
4. ✅ **`tokens_per_ip_10m` 默认按组隔离**（不引入额外配置开关）

---

## 8. 升级与兼容

| 项 | 处理 |
|---|---|
| 存量 RiskRule | groups 默认空 → 自动停用；UI 标红，管理员补齐分组后启用 |
| 存量 RiskSubjectSnapshot | group 默认 `""` → 旧数据保留；新规则不会命中空组，仅作历史展示 |
| 存量 RiskIncident | group 默认 `""` → 历史事件保留 |
| Redis 旧 key | 改造后 key 不再写入；自然 TTL 过期，**不主动 DEL** |
| 唯一索引重建 | 启动期单实例完成；多副本部署建议先停一个实例做迁移 |
| Recovery loop 兼容 | 旧 snapshot.Group="" 时 ClearBlock 传空串：store 层 defensive return；不影响逻辑 |

---

## 9. 验证清单

- [ ] `go vet ./...` 通过
- [ ] `go build ./...` 通过
- [ ] `go test ./service/... ./model/... -run Risk -count=1` 通过
- [ ] SQLite/MySQL 5.7+/PostgreSQL 启动迁移幂等（重复启动不报错）
- [ ] 创建 `groups=[]` 规则尝试启用 → 后端 4xx + 友好消息
- [ ] 规则 A 仅含 group=vip：vip 用户请求命中 / free 用户请求不命中
- [ ] enforce 模式：vip 组封禁后 free 组同 token 仍可正常请求
- [ ] 同 token 在 vip/free 两组分别累计指标互不污染（用 `redis-cli keys 'rc:*'` 抽查）
- [ ] 解封：`POST /api/risk/subjects/token/123/unblock?group=vip` 仅清 vip 组封禁
- [ ] 前端 Risk 页面：未配置分组规则红色 Tag + 启用按钮被拦
- [ ] subjects/incidents Tab 分组过滤可用
- [ ] i18n key 在 zh-CN/en 两份均补齐

---

## 10. 踩坑预案 / 经验沉淀（持续更新）

| 场景 | 现象 | 处理 |
|---|---|---|
| ratio_setting 与 operation_setting 互导致循环 | `import cycle not allowed` | operation_setting 内不引用 ratio_setting；"过滤未知分组"由 controller 层兜底（已在 controller.UpdateRiskCenterConfig 实现） |
| 前端 Bun 不可用 / npm legacy-peer-deps 拉到不一致版本 | `vite build` 报 Semi UI CSS 路径缺失；`i18next-cli extract` 报 keyFromSelector 缺失 | 二次开发使用仓库要求的 Bun 工具链；Claude 实施过程中以 `npx prettier --check` 校验 JSX 风格、`@babel/parser` 校验 JSX 语法；i18n key 后续在干净环境用 `bun run i18n:extract` 同步即可，运行时 i18next 会自动以 Chinese key 兜底 |
| `group`/`groups` 是 SQL 关键字 | PG/MySQL 报语法错误 | 手写 SQL 用 `commonGroupCol`；GORM tag 写 `gorm:"column:group"`；`OnConflict.Columns` 用 `clause.Column{Name:"group"}` GORM 自动转义。`groups` 列的 WHERE 条件**禁止**用字符串拼接（如 `Where("groups IS NULL")`），必须用 GORM map 条件（`Where(map[string]interface{}{"groups": nil})`）自动转义。已踩坑：`risk_rule.go` 和 `moderation_rule.go` 的 `CountEnabled*WithoutGroups` 均因此在 MySQL 8.0 触发 Error 1064 |
| MySQL 5.7 `DROP INDEX IF EXISTS` 不支持 | 迁移幂等失败 | 先查 `information_schema.statistics` 再决定 DROP（参考 `model/main.go` 现有模式） |
| HyperLogLog 跨组合并 | PFCOUNT 误差累积 | 不跨组合并；概览的"全表统计"由应用层近似累加 |
| `info.UsingGroup` 空 | 用户匿名 / 未授权 | 引擎跳过，避免空字符串污染存储桶 |
| 解封多副本一致性 | 其他实例本地缓存未失效 | 由 `LocalCacheSeconds` 自然过期；本期不引入跨节点 pub/sub |
| `tokens_per_ip_10m` 跨组语义 | 同 IP 跨组滥用 token | 默认按组隔离；如需全局聚合，后续加 `cross_group_ip_token_metric` 配置 |
| auto 重试期间 group 切换 | 风控用的 group 变了 | 这是预期行为：与渠道/计费保持同维度 |
| Modal 滚动 | 弹窗内部下拉被裁 | 遵守 CLAUDE.md Rule 6：`centered` + `bodyStyle.overflowY` + `getPopupContainer` |
| 指针类型 omitempty | 零值丢失 | 本期新增字段非可空 0 值（数组/字符串/Bool），无需指针；保持警惕 Rule 7 |
| 前端单文件 ≈2000 行 | 风险集中 | 改动以小函数 / 子组件粒度切分，避免大块重写 |

---

## 11. 启用判定（v4 双层模型）

风控不再"全局启用即对所有请求生效"，而是按 `(EnabledGroups, GroupModes[group])` 双层判定：

```go
// 是否对当前请求启用风控
func isRiskControlEnabledForGroup(cfg, group) bool {
    if cfg == nil || !cfg.Enabled || cfg.Mode == off { return false }
    if group == "" || group == "auto" { return false }
    if !slices.Contains(cfg.EnabledGroups, group) { return false }
    return effectiveRiskModeForGroup(cfg, group) != off
}

// 该组的有效模式：键缺失=关闭；空字符串=回退全局
func effectiveRiskModeForGroup(cfg, group) string {
    if cfg == nil { return off }
    if v, ok := cfg.GroupModes[group]; ok {
        if v == "" { return cfg.Mode }
        return v
    }
    return off
}
```

**真值表**：

| EnabledGroups 含 group | GroupModes[group] | 行为 |
|---|---|---|
| ✗ | 任意 | 跳过风控 |
| ✓ | 不存在 | **跳过**（默认关闭） |
| ✓ | `""` | 走全局 `Mode` |
| ✓ | `"observe_only"` | 仅观察 |
| ✓ | `"enforce"` | 闸门生效 |
| ✓ | `"off"` | 跳过 |

`auto` 永不进 `EnabledGroups`：后端 `Normalize` 与前端 UI 都过滤。

**RiskGroup 快照**：`relay/common/relay_info.go` 增 `RiskGroup string`。`RiskControlBeforeRelay` 入口处一次性赋值 `info.RiskGroup = info.UsingGroup`；`RiskControlAfterRelay` 与事件构造统一用 `RiskGroup`，避免 `auto` 跨组重试时 start/finish 落到不同 group 桶。

---

## 12. API 契约与版本演进

### 12.1 新接口 `GET /api/risk/groups`

**用途**：供前端"分组启用矩阵"渲染；列出所有候选分组（不含 auto）的启用状态、有效模式与聚合指标。

**响应 schema（v1）**：

```json
{
  "success": true,
  "data": {
    "schema_version": 1,
    "global_mode": "observe_only",
    "items": [
      {
        "name": "vip",
        "enabled": true,
        "mode": "enforce",
        "effective_mode": "enforce",
        "rule_count_total": 3,
        "rule_count_enabled": 2,
        "active_subject_count": 5,
        "blocked_subject_count": 2,
        "high_risk_subject_count": 1
      }
    ]
  }
}
```

**字段语义**：
- `schema_version`：从 1 开始；新增字段保持向后兼容；删字段或语义变更必须升版本
- `global_mode`：`RiskControlSetting.Mode`
- `items[].name`：分组名；不含 `auto`
- `items[].enabled`：是否在 `EnabledGroups` 白名单
- `items[].mode`：`GroupModes[name]` 原始值（可能为空字符串）
- `items[].effective_mode`：经 `effectiveRiskModeForGroup` 解析后的实际生效模式（`off` / `observe_only` / `enforce`）；前端不必复刻 fallback 逻辑
- `items[].rule_count_total`：所有 `groups` 包含 `name` 的规则总数
- `items[].rule_count_enabled`：上一项中 `Enabled=true` 的子集
- `items[].active_subject_count` / `blocked_subject_count` / `high_risk_subject_count`：基于 `risk_subject_snapshot` 在该 group 下的统计

**升级空间**：未来新增字段（如 `last_block_at`、`per_metric_score`）只在不破坏 schema_version=1 的契约下追加；前端读取应做 `?? 0` / `?? ""` 兜底。

### 12.2 解封接口扩展

`POST /api/risk/subjects/:scope/:id/unblock?group={group}`

- `?group=` **必填**；缺省返回 400 `"解封必须指定分组"`
- 允许 group 不在白名单（用于清理白名单变更后的历史封禁；`UnblockRiskSubject` 注释中标注此意图）

### 12.3 配置接口扩展

`GET/PUT /api/risk/config` 响应/入参增：
- `enabled_groups: []string`
- `group_modes: map[string]string`

后端 `Normalize` 行为：
- 去重 / trim / 移除空字符串 / 移除 `auto` / 移除不在 `ratio_setting.GetGroupRatioCopy()` 中的项
- `GroupModes` 中无效 mode（不在 `{off, observe_only, enforce, ""}`）的 entry 删除
- `GroupModes` 中 key 不在 `EnabledGroups` 时**保留**（允许预配置；前端用警告色提示"未在白名单中"）

### 12.4 概览接口扩展

`GET /api/risk/overview` 响应增：
- `enabled_group_count`：`EnabledGroups` 长度
- `unconfigured_rule_count`：`enabled=true && groups=""` 的规则数
- `group_unlisted_rule_count`：`enabled=true && groups 全部都不在 EnabledGroups` 的规则数

### 12.5 规则 CRUD 入参扩展

`POST/PUT /api/risk/rules` 增 `groups: []string`；后端 trim/去重/去空后 `common.Marshal` 持久化为 `risk_rules.groups`（TEXT，JSON 数组字符串）。

后端校验：仅 `enabled=true && groups 空` 报错；**不**校验 group 是否在白名单（与决策点 3 一致）。

---

## 14. 测试驱动开发（TDD）规范

本次改造**必须**遵循 TDD 流程，先写测试再写实现，红 → 绿 → 重构。

### 11.1 TDD 工作流

每个改动单元（一个公共函数 / 一个 HTTP handler / 一个前端校验逻辑）都按下述顺序：

1. **红（Red）**：先写一个或多个失败的单元测试，覆盖：
   - 正常路径（happy path）
   - 边界条件（空值、零值、最大/最小）
   - 异常路径（错误返回、panic 防护）
   - 与既有调用链的契约（被调用者签名、上下文键、返回 error 类型）
2. **运行测试**：确认测试因目标功能尚未实现而失败（不是因编译错误失败，避免假红）
3. **绿（Green）**：写**最少**的实现代码让测试通过，不做额外抽象
4. **重构（Refactor）**：在测试保护下整理命名、消除重复、提取公共函数；每次重构后再次跑测试
5. **提交**：测试 + 实现 + 重构在同一个 commit 内（不允许"先 commit 实现再补测试"）

### 11.2 调用链完整性的测试约束

由于本期改造把"二元 (scope, subject)"扩展到"三元 (scope, subject, group)"，调用链穿透多层（store ↔ engine ↔ controller ↔ HTTP），**必须先写"调用链契约测试"再写实现**：

#### 调用链契约测试清单（先写、先失败）

- [ ] `riskMetricStore` 接口签名变更：每个方法的 group 参数被传递到 key 构造（用 fake store 断言收到的 key）
- [ ] `evaluateRiskRules(rules, scope, group, metrics)`：
  - groups=[A] + group=A → 返回该 rule
  - groups=[A] + group=B → 不返回
  - groups=[] → 该 rule 不出现在 reload 后的 compiled 列表里
- [ ] `evaluateAndPersistSubject(event, scope, subjectID, group, metrics)`：
  - snapshot 的唯一键是 (scope, subjectID, group)：写入 A 组不影响 B 组同主体的 snapshot
- [ ] `RiskControlBeforeRelay`：`info.UsingGroup` 直接被用作 cache 查询的 group；空时跳过缓存查询
- [ ] `UnblockRiskSubject(scope, subjectID, group, operator)`：清 A 组 block 不影响 B 组
- [ ] `validateRiskRule`：`enabled=true && groups=[]` 必须返回 error
- [ ] `controller.UnblockRiskSubject`：`?group=` 缺失时返回 400
- [ ] `controller.GetRiskOverview`：返回 `unconfigured_rule_count` 字段
- [ ] DB 迁移幂等：在内存 SQLite 上跑两次 `migrateRiskTables`（或现有迁移入口）不报错；唯一索引为 `(subject_type, subject_id, group)`

#### 单元测试组织

- 后端：`service/risk_control_test.go`、`service/risk_control_store_test.go`、`controller/risk_test.go`（新建）、`model/risk_*_test.go`（新建必要的）
- 前端：`web/classic/src/pages/Risk/__tests__/*.test.jsx`（若仓库已有 vitest/jest 基础设施则沿用；若无则只在后端补 + 手测前端）—— 启动前先检查 `web/package.json` 看是否有 test 脚本；若无则 **不引入** 新测试框架，避免范围蔓延
- 表驱动测试优先（Go 风格），用 subtests `t.Run(name, ...)`
- 不 mock 数据库：用 in-memory SQLite 跑真实 GORM；遵循仓库现有 `risk_control_test.go` 的模式
- store 层用真实内存实现 + 抽象接口注入做契约测试；Redis 实现仅在显式 `REDIS_TEST_URL` 环境变量存在时跑（默认 skip）

### 11.3 阶段 → 测试映射

| 阶段 | 先写的测试文件 | 测试目标 |
|---|---|---|
| 1 模型层 | `model/risk_subject_snapshot_test.go`（必要时新建） | upsert 三元组唯一；group 列加列后查询可用 |
| 2 服务核心 | `service/risk_control_test.go` | reloadRules 跳过空 groups；ruleAppliesToGroup；evaluateRiskRules |
| 3 存储层 | `service/risk_control_store_test.go` | RecordStart/SetBlock/GetBlock/Clear 三元组隔离；sweep 跨组清理 |
| 4 控制器 | `controller/risk_test.go`（新建） | upsert 携带 groups；unblock 必须带 ?group=；overview 多字段 |
| 5 前端 | 手测 + 现有 lint | （如无既有测试基础设施则不新引入） |
| 6 集成 | `service/risk_control_test.go` 增"模拟一次 BeforeRelay 命中流程" | end-to-end：事件入队 → 评估 → 落库 → 闸门缓存 |

### 11.4 推送前自检（强制门禁）

每次 `git push` 前必须按顺序通过以下检查，任一失败立即修复后再推：

```bash
# 1. 语法 / 编译
go vet ./...
go build ./...

# 2. 测试（必须全部绿，且与本次改动相关的新增/修改测试都跑过）
go test ./... -count=1

# 3. 风控相关聚焦回归（更快发现 group 维度回归）
go test ./service/... ./model/... ./controller/... -run Risk -count=1 -v

# 4. 前端语法 / lint（若 web/ 改动）
cd web && bun run lint   # 若仓库未配 lint 脚本，跳过该步并在 commit message 注明
cd web && bun run build  # 至少能通过编译

# 5. i18n 同步（若改了 t('...') key）
cd web && bun run i18n:extract && bun run i18n:sync && bun run i18n:lint
```

任何一步失败 → 不准 push。pre-push 钩子未配置时由开发者人工执行，不得跳过。

### 11.5 提交规范（重要）

- **commit message 不携带 `Co-Authored-By` 行**（包括 `Co-Authored-By: Claude` / `Co-Authored-By: Codex` 等任何形式）
- **commit message 严禁出现 `.claude` 目录、`.claude/DEV_GUIDE.md`、`.claude/TODO.md` 等任何对该目录或其内容的引用**。原因：`.claude` 是仓库 `.gitignore` 忽略的本地协作记录，不应在公共 git 历史中暴露其存在；引用它会让 PR 评审者看到"不存在于代码库的文件"，造成困惑
- 每次 commit 标题遵循仓库历史风格：`feat:` / `fix:` / `refactor:` / `test:` / `docs:` / `chore:`
- 每个改动单元的 commit 必须包含：测试 + 实现，二者一起进；不允许"先 commit 实现再补测试"
- 推送前 `git log -1` 双重确认：**既不含** `Co-Authored-By` 行，**也不含** `.claude` / `claude` 字样（grep 一次即可）
- 最终聚合到一个 PR（仓库偏好 single bundled PR）

### 11.6 失败处理

- 测试失败 → 先理解失败本质，再决定是改测试还是改实现；不准为了让测试过而把断言放宽
- pre-commit hook 失败 → 修源码再 NEW commit（**不 amend**），与 CLAUDE.md 全局指令一致
- 三库迁移测试失败 → 检查 `commonGroupCol` 用法、唯一索引名一致性、`information_schema` 查询大小写

---

## 14. 内容审核引擎容量与持久化决策

### 14.1 持久化层选型

**选 Redis LIST 简单方案，不引入 asynq**。原因：

- 当前规模下队列吞吐处于两位数 req/s 量级；asynq 的 ~600 行依赖、独立 web UI、死信队列等能力是过度工程
- Redis 容器项目早已部署（`REDIS_CONN_STRING` 已配置可用），LIST + LPUSH/BRPOPLPUSH/LREM 三命令即可满足"重启不丢失 + ring buffer + 处理中标记"
- Redis 不可用时自动 fallback 到内存队列，relay 主路径永远不阻塞
- 升级到 asynq 的临界点：稳态 > 5000 req/s 持续 / 需要重试策略可视化 / 需要死信队列与运维 UI

### 14.2 容量配置推算（基于实测）

实测 OpenAI moderation RTT ≈ 140ms，单 worker 吞吐 ≈ 7.1 req/s。

| 配置项 | 默认值 | 推导 |
|---|---|---|
| `WorkerCount` | **16** | 5x 峰值并发下仍有 6.5x 头空间；保留与 OpenAI tier1 RPM 限额的余量 |
| `EventQueueSize` | **32768** | 配合 ring buffer，足够 buffer OpenAI 短时不可达的尖峰，旧事件被淘汰新事件优先 |
| `HTTPTimeoutMS` | **3000** | 实测 p50 = 140ms，3s 给 p99 留余量；OpenAI 偶发慢请求超过 3s 直接放弃避免 worker 阻塞 |
| `IncidentFlushIntervalMs` | **500** | admin 看到 incident 的延迟可控；批次大小天然受时间触发约束 |
| `IncidentMaxBatchSize` | **100** | 防单批事务过大；流量突发时按行数上限触发保护 |

### 14.3 失败请求过滤

`EnqueueModerationFromRelay` 在入队前判定：
- `relayErr == nil` → upstream 2xx 成功，入队
- `info.SendResponseCount > 0` → SSE 已交付至少一个 chunk，用户实际看到了内容，入队
- 其余情况（4xx/5xx/timeout/channel error） → **跳过**

理由：moderation 的目的是审计"用户实际收到了什么内容"。上游 500/404 的请求用户已看到错误返回，没有 AI 生成内容供审核；如果仍入队会浪费 OpenAI tokens、错误累计 enforcement 计数、误触发邮件提醒。

### 14.4 Ring buffer 语义

队列满时**丢最老**而不是丢最新。新事件代表"刚发生的内容"，比 5 分钟前的旧事件更值得审核；OpenAI 不可达期间堆积的旧事件可以被牺牲。

### 14.5 邮件双桶节流

命中（hit）邮件与封禁（auto_ban）邮件**独立窗口**：
- 命中：`HitEmailWindowMinutes=10` × `HitEmailMaxPerWindow=3`
- 封禁：`BanEmailWindowMinutes=60` × `BanEmailMaxPerWindow=3`

封禁邮件用独立 user 字段 `EnforcementBanEmailWindowStartAt` / `EnforcementBanEmailCountInWindow` 跟踪；命中邮件刷屏不会挤掉封禁邮件。封禁邮件自身有 `already_banned` 短路保护，60min/3 是兜底防代码 bug。

### 14.6 Incident 批量写库

worker 处理完调用 `batcher.submit(incident)`，独立 goroutine 按 500ms / 100 条聚合 `CreateInBatches`。worker 池吞吐与 PG 写入解耦。批次未 flush 时进程崩溃只丢失审计行（事件本身已通过 Redis 队列持久化）。

### 14.7 升级到 asynq 的判定标准

满足任一即考虑迁移：
- 稳态 throughput > 5000 req/s 持续超过 1 小时
- 运营要求看到队列重试策略 / 死信队列 / web UI
- 需要分布式 worker（多节点共享队列）
- 单进程 Redis LIST 已成为吞吐瓶颈（PFCOUNT 慢、LREM 退化）

---

## 15. 流异常计费与重试策略（方案 B）

### 15.1 问题背景

官方分支对"首字 -1"流异常计费的修复存在缺陷：

1. 修复条件 `usage == nil && !IsNormalEnd() && SendResponseCount == 0` 对主流流处理器（OaiStreamHandler / ClaudeStreamHandler）是死代码——它们总是返回非 nil usage
2. 流中途断开（timeout / scanner_error + 已发部分 chunks）时，用户收到不完整输出仍被按估算 token 扣费
3. 流异常后 handler 返回 `(usage, nil)` 而非 error，不触发重试

### 15.2 分层策略

| 条件 | 行为 |
|---|---|
| 异常 + 0 chunks 已发 + 非 client_gone | **返回 503 error → 重试其他渠道** |
| 异常 + N chunks 已发 + 非 client_gone | **零计费**（HTTP 已提交，无法重试） |
| client_gone（用户主动断开） | 正常计费 |
| 正常结束 | 正常计费 |

### 15.3 关键判定函数

`StreamStatus.IsServerSideError()` (`relay/common/stream_status.go`)：
- `!IsNormalEnd() && EndReason != client_gone`
- 覆盖：timeout / scanner_error / panic / ping_fail / none

`StreamAbortRetryError(info)` (`service/stream_abort.go`)：
- `IsStream && IsServerSideError() && SendResponseCount == 0` → 返回 503 error
- 其他情况返回 nil

### 15.4 变更点

| 文件 | 变更 |
|---|---|
| `relay/common/stream_status.go` | 新增 `IsServerSideError()` |
| `service/stream_abort.go` | 新增 `StreamAbortRetryError()` |
| `service/text_quota.go:calculateTextQuotaSummary` | `IsStream && IsServerSideError()` → 强制 `usage = &dto.Usage{}`（零 token → 零 quota） |
| `relay/compatible_handler.go` TextHelper | DoResponse 后 + chatCompletionsViaResponses 后插入 `StreamAbortRetryError` 检查 |
| `relay/claude_handler.go` ClaudeHelper | 同上两处 |
| `relay/gemini_handler.go` GeminiHelper | DoResponse 后插入检查 |

### 15.5 重试可行性

流式响应的 HTTP 生命周期：
```
SetEventStreamHeaders() → 设置 header（内存未提交）
PingData()（若开启） → c.Render + Flush → HTTP 200 已提交
HandleStreamFormat()  → SSE data 已发送
```

- `SendResponseCount == 0` 且 ping 发送前超时：header 未提交，可安全重试
- `SendResponseCount == 0` 但 ping 已发：HTTP 200 已提交，但只有 SSE comment（`: ping`），SSE 客户端忽略此行，重试的数据追加在后面仍然合法
- `SendResponseCount > 0`：已发业务数据，不可重试，仅零计费

### 15.6 503 与重试配置

503 已在默认重试范围 `AutomaticRetryStatusCodeRanges`（`setting/operation_setting/status_code_ranges.go`）的 `{500, 503}` 区间内，无需额外配置。重试次数受 `common.RetryTimes` 控制。

---

## 16. 分组监控系统（Redis 计数器驱动）

### 16.1 架构概述

```
请求热路径
  ├── RecordConsumeLog (type=2)  ──┐
  └── RecordErrorLog   (type=5)  ──┤
                                    ↓
                       common.GroupMonitoringHook
                       → service.RecordMonitoringMetric
                       → Redis Pipeline (HINCRBY × 8 + EXPIRE)
                       → key: gm:b:{bucket}:{group}:{channelId}
                                    │
                       后台聚合循环 (master, 每N分钟)
                       ├── SCAN gm:b:* → 收集桶
                       ├── HGETALL → 内存聚合
                       ├── UPSERT ChannelMonitoringStat (主DB)
                       ├── UPSERT GroupMonitoringStat (主DB)
                       ├── INSERT MonitoringHistory (主DB)
                       └── 清理 30天前历史

                       Redis 不可用时降级:
                       └── 查 LOG_DB 简化聚合 (无缓存率)
```

### 16.2 Redis 计数器 key 设计

```
gm:b:{bucket_ts}:{group}:{channel_id}
```
- `bucket_ts` = `floor(now / interval_sec) * interval_sec`
- Hash 字段: `t`(总数), `s`(成功), `e`(错误), `ct`(缓存tokens), `pt`(prompt tokens), `rt`(响应时间ms), `fs`(FRT总和ms), `fc`(FRT计数)
- TTL = `max(可用率周期, 缓存率周期) × 2 + 桶间隔`，自动过期无需清理

### 16.3 数据库表（3张，无 RequestStat）

| 表 | 用途 | 增长模式 |
|---|---|---|
| `ChannelMonitoringStat` | 渠道级快照 | 固定（UPSERT） |
| `GroupMonitoringStat` | 分组级快照 | 固定（UPSERT） |
| `MonitoringHistory` | 时间线图表 | ~288行/天/组，保留30天 |

### 16.4 Hook 机制

`common.GroupMonitoringHook` 函数变量（`common/monitoring_hook.go`）避免 model→service 循环依赖。`service.StartGroupMonitoringAggregation()` 初始化时赋值。

调用位置：
- `model/log.go:RecordConsumeLog` — 成功路径，从 `other` map 提取 `frt`、`cache_tokens`
- `model/log.go:RecordErrorLog` — 失败路径，frt/cache 为 0

### 16.5 配置

`setting/operation_setting/group_monitoring_setting.go`：
- `MonitoringGroups []string` — 监控的分组列表
- `AvailabilityPeriodMinutes int` (默认60) — 可用率统计窗口
- `CacheHitPeriodMinutes int` (默认60) — 缓存率统计窗口
- `AggregationIntervalMinutes int` (默认5) — 聚合间隔
- `CacheTokensSeparateGroups []string` — Claude 系分组（prompt 不含 cache）
- `AvailabilityExcludeModels/CacheHitExcludeModels/AvailabilityExcludeKeywords` — 排除规则

### 16.6 API 端点

| 路由 | 权限 | 用途 |
|---|---|---|
| `GET /api/monitoring/admin/groups` | AdminAuth | 分组概览 |
| `GET /api/monitoring/admin/groups/:group` | AdminAuth | 分组详情+渠道 |
| `GET /api/monitoring/admin/groups/:group/history` | AdminAuth | 历史图表 |
| `POST /api/monitoring/admin/refresh` | AdminAuth+限流 | 手动刷新 |
| `DELETE /api/monitoring/admin/groups/:group/records` | AdminAuth | 清空数据 |
| `GET /api/monitoring/public/groups` | TryUserAuth | 脱敏概览 |
| `GET /api/monitoring/public/groups/:group/history` | TryUserAuth | 公开历史 |

### 16.7 前端文件

| 文件 | 功能 |
|---|---|
| `pages/GroupMonitoring/index.jsx` | 页面入口 |
| `components/monitoring/GroupMonitoringDashboard.jsx` | 主面板：状态栏+卡片网格+历史获取 |
| `components/monitoring/GroupStatusCard.jsx` | 卡片：指标+进度条+迷你历史图 |
| `components/monitoring/GroupDetailPanel.jsx` | 侧边详情：仅管理员渠道表（无历史图） |
| `components/monitoring/AvailabilityCacheChart.jsx` | VChart 折线图（详情面板用） |
| `components/monitoring/MiniHistoryChart.jsx` | 迷你历史图（嵌入卡片内） |
| `components/settings/GroupMonitoringSetting.jsx` | 设置包装器 |
| `pages/Setting/Operation/SettingsGroupMonitoring.jsx` | 设置表单 |

### 16.9 前端踩坑记录

| 坑点 | 现象 | 修复 |
|---|---|---|
| `Object.keys(array)` 返回索引 | 监控设置分组列表显示 "0 1 2" 而非分组名 | `Array.isArray(data) ? data : Object.keys(data)` |
| `MonitoringHistory` 字段名 | 前端读 `h.timestamp`，后端返回 `recorded_at`（unix秒） | 统一改为 `h.recorded_at * 1000` |
| 历史响应层级 | Dashboard 从 `hRes.data.data.aggregation_interval_minutes` 读 | 实际在 `hRes.data.aggregation_interval_minutes` |
| Admin groups 无 `is_online` | Admin 端返回 `online_channels`（int），Public 端返回 `is_online`（bool） | `group.is_online ?? (group.online_channels > 0)` |
| 可用率 -1 表示无数据 | `availRate.toFixed(1)` 对 null 崩溃 | null guard：显示 "N/A"，Progress percent=0 |
| 普通用户抽屉无意义 | 点击卡片弹空侧边栏（无渠道详情权限） | `onClick` 仅 admin 传入；非 admin cursor=default |
| Overview 卡片读错数据源 | 内容审核/处置 Tab 的"启用状态"卡片从 overview API 读 enabled，overview 失败时显示"未启用" | 改为从 config state 读取；overview 仅用于动态统计指标（命中数、丢弃数等） |
| loadConfig/loadOverview 无 try/catch | API 失败时 unhandled rejection，state 留初始值，卡片全部显示 0/"未启用" | 所有 load 函数加 try/catch，静默处理异常 |

### 16.8 RPM 1500 性能指标

- 热路径延迟增加: <0.3ms/请求（1次 Redis pipeline）
- Redis 内存: ~50KB（600 key × 80 bytes）
- 聚合周期耗时: <50ms（SCAN + HGETALL 600 key + 内存聚合）
- LOG_DB 查询: 0次（Redis 主路径）/ 仅降级时使用

---

## 17. 渠道压力冷却（Pressure Cooling）

### 17.1 设计目标

当渠道首字延迟（FRT）持续过高时，自动禁用渠道并清除亲和性缓存，冷却一段时间后自动恢复并进入新的观察窗口。所有阈值均支持全局默认 + 渠道级覆盖。

### 17.2 状态机

```
OBSERVING ──(violations >= trigger_count)──► COOLING ──(cooldown expired)──► OBSERVING
                                              │
                                   (consecutive >= max)
                                              │
                                              ▼
                                          SUSPENDED（需手动恢复）
```

- **OBSERVING**: 观察期内统计 FRT 超阈值次数（`violations`），窗口到期重置
- **COOLING**: 渠道被 `ChannelStatusAutoDisabled`，冷却期满后 recovery loop 自动恢复
- **SUSPENDED**: 连续冷却次数达上限，停止自动恢复，需管理员手动启用

### 17.3 关键文件

| 文件 | 职责 |
|---|---|
| `setting/operation_setting/pressure_cooling_setting.go` | 全局配置（`config.GlobalConfig.Register`），含 11 个参数 |
| `dto/channel_settings.go` | `PressureCoolingOverride` 结构体（指针字段，nil=跟随全局） |
| `service/pressure_cooling.go` | 核心引擎：`CheckPressureCooling`、`executePressureCooling`、`canCoolChannel`、recovery loop |
| `service/pressure_cooling_store.go` | 状态存储：Redis hash（`pc:state:{channelId}`）+ 内存回退 |
| `model/channel_cache.go` | `CountEnabledChannelsForGroupModel` — 防死锁检查 |
| `web/classic/src/components/channel/PressureCoolingEditor.jsx` | 渠道级覆盖编辑器 |

### 17.4 FRT 采集链路

```
relay 成功 → controller/relay.go 写入 c.Set("relay_frt_ms", ms)
→ middleware/distributor.go afterRelay 读取 → gopool.Go(service.CheckPressureCooling)
```

FRT 只对成功响应（`relayInfo.HasSendResponse()`）采集，异步执行不阻塞请求。

### 17.5 防死锁机制

`canCoolChannel` 遍历该渠道所有 `(group, model)` 组合，确保冷却后每个组合仍有 ≥ `MinActiveChannelsPerGroup` 个可用渠道。一票否决制。

### 17.6 防频繁冷却

- **指数退避**: `effective_cooldown = base × (multiplier ^ consecutive_count)`，上限 `MaxCooldownSeconds`
- **宽限期**: 恢复后 `GracePeriodSeconds` 内不统计违规
- **最大连续冷却**: 达到 `MaxConsecutiveCooldowns` 后转为 SUSPENDED

### 17.7 自动测试交互

不阻止自动测试恢复冷却中的渠道。recovery loop 检测到"状态=cooling 但渠道已启用"时，保留 `consecutive_count` 并设置宽限期，确保退避逻辑不被绕过。

### 17.8 渠道级覆盖

`dto.ChannelSettings.PressureCooling` 存储在渠道 `setting` JSON 中，字段为指针类型：
- `nil` = 使用全局默认
- 非 `nil` = 使用渠道自定义值

前端 `PressureCoolingEditor` 组件开关控制是否启用覆盖，关闭时整个对象序列化为 `null`。

### 17.9 全局配置参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `Enabled` | `false` | 全局开关 |
| `ObservationWindowSeconds` | `60` | 观察窗口时长 |
| `FRTThresholdMs` | `8000` | FRT 阈值 |
| `TriggerCount` | `3` | 触发冷却所需违规次数 |
| `CooldownSeconds` | `300` | 基础冷却时长 |
| `MaxConsecutiveCooldowns` | `5` | 最大连续冷却次数 |
| `CooldownBackoffMultiplier` | `1.5` | 退避倍率 |
| `MaxCooldownSeconds` | `3600` | 冷却时长上限 |
| `GracePeriodSeconds` | `30` | 恢复后宽限期 |
| `MinActiveChannelsPerGroup` | `1` | 防死锁最小活跃渠道数 |
| `RecoveryCheckIntervalSeconds` | `30` | recovery loop 检查间隔 |

### 17.10 Recovery Loop

`StartPressureCoolingRecovery` 在 `main.go` 中启动（仅 master 节点）。每 `RecoveryCheckIntervalSeconds` 扫描所有 cooling 状态，到期则恢复渠道并进入新观察窗口。

---

## 18. Anthropic Session & Upstream Request ID 追踪

> **⚠️ 本功能已从 `dev/risk-control-group-scoping-20260502` 分支排除。**
> 原因：该功能涉及 `logs` 表新增 `session_id`、`upstream_request_id` 两列及索引（GORM AutoMigrate），生产环境 `logs` 表数据量大时迁移耗时过长。
> 相关 commit：`75ab50c9`、`d7fba814`（保留在旧分支 `dev/risk-control-group-scoping-20260428`）。
> 后续计划：待生产环境安排维护窗口后，单独合入此功能。

（以下为设计文档归档，供后续合入时参考）

<details>
<summary>展开查看完整设计</summary>

### 18.1 动机

通过 API 网关转发 Anthropic 请求时，管理员需要：
- 按 Claude Code 会话分组查看请求日志（session_id）
- 用 Anthropic 侧请求 ID 向 Anthropic 提工单排查问题（upstream_request_id）

### 18.2 数据来源

| 字段 | 来源 | 可用性 |
|------|------|--------|
| `session_id` | 优先 `x-claude-code-session-id` 请求头；回退解析 `metadata.user_id`（支持 legacy `_session_{UUID}` 和 JSON `{"session_id":"UUID"}` 两种格式） | 仅 Claude Code 客户端 |
| `upstream_request_id` | Anthropic 响应头 `request-id` | 每个 Anthropic API 请求 |

### 18.3 捕获链路

```
请求进入
  ├── ClaudeHelper (native Claude format)
  │     └── ExtractClaudeSessionID(c, metadata) → c.Set("claude_session_id")
  ├── TextHelper (OpenAI format → Claude channel)
  │     └── c.Request.Header.Get("x-claude-code-session-id") → c.Set("claude_session_id")
  │
  ├── adaptor.DoRequest → Anthropic API
  │
  └── ClaudeHandler / ClaudeStreamHandler
        └── CaptureUpstreamRequestID(c, resp) → c.Set("upstream_request_id")
                                                  从 resp.Header.Get("request-id")

日志写入
  ├── RecordConsumeLog → Log.SessionId, Log.UpstreamRequestId
  └── RecordErrorLog   → 同上
```

### 18.4 数据模型

`model/log.go` `Log` 结构新增两列：

```go
SessionId         string `gorm:"type:varchar(64);index:idx_logs_session_id;default:''"`
UpstreamRequestId string `gorm:"type:varchar(64);index:idx_logs_upstream_req_id;default:''"`
```

迁移由 `LOG_DB.AutoMigrate(&Log{})` 自动处理（GORM AddColumn），三库兼容。

### 18.5 API

`GET /api/log/` 新增查询参数 `session_id`（仅管理员）。

### 18.6 前端

- 管理员搜索栏新增 Session ID 过滤输入框
- 展开详情新增 Session ID 和 Upstream Request ID 行（仅管理员可见）
- 普通用户日志中 `session_id` 和 `upstream_request_id` 字段被清空（`formatUserLogs`）

### 18.7 关键文件

| 文件 | 变更 |
|------|------|
| `constant/context_key.go` | 新增 `ContextKeyUpstreamRequestId`、`ContextKeyClaudeSessionId` |
| `service/session_extract.go` | `ExtractClaudeSessionID`、`CaptureUpstreamRequestID`、metadata 解析 |
| `service/session_extract_test.go` | 覆盖 legacy/JSON/header 优先级/nil 安全 |
| `model/log.go` | `Log` 新增两列；`RecordConsumeLog`/`RecordErrorLog` 从 context 读取；`GetAllLogs` 支持 session_id 过滤；`formatUserLogs` 清除敏感字段 |
| `controller/log.go` | `GetAllLogs` 传递 `session_id` 参数 |
| `relay/claude_handler.go` | `ClaudeHelper` 入口提取 session_id |
| `relay/compatible_handler.go` | `TextHelper` 入口提取 header session_id |
| `relay/channel/claude/relay-claude.go` | `ClaudeHandler`/`ClaudeStreamHandler` 捕获 upstream request-id |
| `web/classic/src/hooks/usage-logs/useUsageLogsData.jsx` | 搜索参数、展开详情 |
| `web/classic/src/components/table/usage-logs/UsageLogsFilters.jsx` | Session ID 输入框 |

</details>

## 18. 渠道限流实时统计（Rate Limit Stats）

### 18.1 设计目标

在渠道列表的名称列下方，为启用了限流的渠道实时显示当前 RPM 和并发占用，帮助管理员快速识别负载压力。

### 18.2 数据来源

**零额外写入**——复用 `channel_limiter` 已有的 Redis key：
- `chnlrl:rpm:{channelId}`（ZSET）→ `ZREMRANGEBYSCORE` 清理后 `ZCARD` = 当前分钟请求数
- `chnlrl:conc:{channelId}`（String）→ `GET` = 当前在飞请求数

### 18.3 关键文件

| 文件 | 职责 |
|---|---|
| `service/channel_limiter/lua/stats.lua` | 批量读取 Lua 脚本：单次 EVALSHA 获取 N 个渠道的 RPM+并发 |
| `service/channel_limiter/redis.go` | `Stats()` 方法：调用 stats.lua |
| `service/channel_limiter/memory.go` | `Stats()` 内存实现：直接读 rpmHits/concurrency |
| `service/channel_limiter/limiter.go` | `GetStats()` 导出函数 + `ChannelLimitStats` 结构体 |
| `controller/channel.go` | `GetChannelRateLimitStats` handler |
| `router/api-router.go` | `GET /api/channel/rate-limit-stats` |
| `web/.../ChannelsColumnDefs.jsx` | name 列 render 追加容量指示 |
| `web/.../useChannelsData.jsx` | `rateLimitStats` state + 5s 轮询 |

### 18.4 API

`GET /api/channel/rate-limit-stats`（AdminAuth）

响应：
```json
{
  "success": true,
  "data": {
    "42": { "rpm": 120, "rpm_limit": 600, "conc": 5, "conc_limit": 25 }
  }
}
```

仅返回启用限流且 `RPM > 0` 或 `Concurrency > 0` 的渠道。

### 18.5 前端展示

名称列下方紧凑显示：
- `并发 5/25  RPM 120/600`
- 颜色：≥95% 红色（danger），≥80% 橙色（warning），其余灰色（text-2）
- 仅配并发 → 只显示并发行；仅配 RPM → 只显示 RPM 行
- 未启用限流 → 不显示

### 18.6 轮询策略

- 首次加载随渠道列表同步获取
- 之后每 5 秒轮询 `rate-limit-stats` 接口
- 组件卸载时清除 interval

## 19. 分组监控图表性能优化（VChart 反复刷新修复）

### 19.1 问题

分组监控页面趋势图在数据加载完成后每秒重新渲染。

### 19.2 根因

`GroupMonitoringDashboard` 内 1 秒倒计时 `setCountdown` → 整棵组件树每秒 re-render → `AvailabilityCacheChart` 的 `spec` 对象重建（含内联箭头函数）→ VChart `isEqual` 判定函数引用变化 → `updateSpec` 触发图表重绘。

### 19.3 修复策略

| 文件 | 措施 |
|---|---|
| `GroupMonitoringDashboard.jsx` | 倒计时隔离到 `RefreshButton`（`memo`）子组件，`setCountdown` 仅重渲染按钮；`handleCardClick`/`handleCloseDetail` 用 `useCallback` |
| `AvailabilityCacheChart.jsx` | `spec` 用 `useMemo`；`formatPercent`/`tooltipValueFn` 提升为模块级常量；`option` 提升为模块常量 `VCHART_OPTION`；加 `skipFunctionDiff` prop；`memo` 包裹 |
| `GroupStatusCard.jsx` | `memo` 包裹 |
| `GroupDetailPanel.jsx` | `memo` 包裹 |

### 19.4 VChart 最佳实践

- `spec` 对象必须用 `useMemo` 缓存，避免每次 render 创建新引用
- 不依赖组件状态的回调函数（如 `formatMethod`、`tooltipValueFn`）提升到模块级
- 对包含函数引用的 spec 使用 `skipFunctionDiff` prop 跳过函数比较
- `option={{ mode: 'desktop-browser' }}` 必须提升为模块常量，否则每次 render 生成新对象

## 20. 分组监控数据准确性修复

### 20.1 可用率永远 100%

**根因**：`controller/relay.go:processChannelError` 中，监控 hook 调用位于 `if constant.ErrorLogEnabled` 闸门内部（通过 `model.RecordErrorLog` 间接调用）。`ErrorLogEnabled` 默认 `false`（`common/init.go:156`），导致请求失败时监控 hook 从未被触发，只有成功请求入统计。

**修复**：在 `processChannelError` 中将 `common.GroupMonitoringHook(... false ...)` 提升到 `ErrorLogEnabled` 闸门之前无条件调用；同时从 `model.RecordErrorLog` 中移除重复的 hook 调用以防 `ErrorLogEnabled=true` 时双重计数。

### 20.2 缓存命中率 >100%

**根因**：部分 provider 的 `cacheTokens` 可独立于 `promptTokens` 报告（不是子集关系），聚合公式 `cacheTokens / promptTokens * 100` 可能超 100。

**修复**：在 `service/group_monitoring.go` 的 Redis 和 DB fallback 聚合路径中，对单渠道和分组级 `CacheHitRate` 均加 `if rate > 100 { rate = 100 }` clamp。

### 20.3 状态时间线 FRT 黄色指示

**变更**：
- `model/group_monitoring.go`：`MonitoringHistory` 新增 `AvgFRT int` 字段（GORM AutoMigrate 自动加列）
- `service/group_monitoring.go`：`InsertMonitoringHistory` 时写入 `AvgFRT`
- `web/.../StatusTimeline.jsx`：`segmentColor(rate, avgFrt)` — 当可用率 ≥99% 但 `avg_frt > 8000ms` 时显示黄色（warning）；tooltip 展示 FRT 值

---

## 21. 上游同步策略（长期决策 2026-05-02）

### 21.1 决策：方案 B — merge-based

**结论**：采用 `git merge upstream/main` 合入上游更新，二开功能同样 merge 回 `origin/main`。

**否决 rebase 方案的理由**：
1. 10+ 个二开模块在 6 个枢纽文件（`main.go`、`router/api-router.go`、`App.jsx`、`SiderBar.jsx`、`context_key.go`、`model/log.go`）交叉修改，rebase 冲突成本 = merge 的 N 倍
2. 上游 1-3 天/次更新，rebase 10+ 分支不可持续
3. force push 导致部署 hash 不稳定，生产回滚定位困难
4. 功能间依赖紧密（监控依赖 common hook，审核依赖风控），分支拓扑顺序固定，一个 rebase 失败级联所有

### 21.2 工作流规范

```
upstream/main ──── git merge ───► origin/main (上游 + 所有二开)
                                      │
                                      ├── dev/feature-x (新功能开发)
                                      │       └── merge 回 origin/main
                                      │
                                      └── 直接部署
```

**操作步骤**（每次同步上游）：
1. `git fetch upstream main`
2. `git tag pre-merge-upstream-YYYYMMDD` — 回退锚点
3. `git merge upstream/main` — 解决冲突
4. 运行 `scripts/merge-check.sh` 验证
5. 推送

**纪律要求**：
- commit message 前缀分类：`feat(risk):` / `upstream-merge:` 等，便于 `git log --grep` 区分
- 启用 `git rerere`：记住冲突解决方案
- merge 前打 tag：冲突失败可快速 `git reset --hard pre-merge-upstream-*`
- 同步频率 ≤ 3 天——避免堆积
- 新功能在 `dev/` 分支开发，dev 分支可用 rebase 保持整洁，合入 main 用 merge

### 21.3 坑点预案

| 场景 | 处理 |
|------|------|
| 上游前端大重构（如 `web/` → `web/classic/`） | 先分析 `git diff --stat`；前端目录重命名时优先保留本地路径，`--ours` 处理纯前端冲突 |
| 上游修改了本地深度定制的文件（如 `model/log.go`） | 逐行手动合并，运行风控/监控相关测试确认不回归 |
| merge 后编译失败 | 先 `go vet` 定位，再 `go build`；常见原因：上游新增了同名函数、改了接口签名 |
| merge 后前端构建失败 | 检查 import 路径是否因重命名失效；`bun run build` 验证 |
| rerere 记录的冲突解决方案过期 | 上游改了冲突区域的上下文 → `git rerere forget <file>` 后手动解决 |
| 上游 PR merge commit 引入的代码与本地冲突 | `git log --oneline --no-merges upstream/main` 只看非 merge commit 的实质变更 |

### 21.4 前端目录决策（2026-05-02）

上游在 `a42b3976` 引入双前端架构：`web/classic/`（原前端）+ `web/default/`（next-gen TypeScript）。

**本 fork 决策**：接受上游目录重命名，将二开前端代码迁移至 `web/classic/`。理由：
- 保持与上游目录结构一致，后续 merge 冲突更少
- `web/default/` 作为只读接收，不做定制
- Dockerfile / makefile 由上游维护双前端构建逻辑，无需额外适配

### 21.5 首次 merge 执行记录（2026-05-02）

上游 `bee339d2..dac55f0f`（29 commit）merge 到 main，关键经验：

| 事项 | 结果 |
|------|------|
| 后端冲突（`main.go` / `constants.go` 等 6 文件） | git 自动解决，无需手动干预 |
| 前端目录重命名（`web/` → `web/classic/`） | git 正确检测并 rename 已有文件；但本地**新建**的 35 个文件（Risk/Ticket/Monitoring 页面等）未被自动移动，需手动 `mv` 到 `web/classic/src/` |
| i18n JSON 冲突（7 个 locale 文件） | 用 python 脚本合并：ours 优先，上游新增 6-31 个 key 追加；rerere 已记录 |
| dev 分支 rebase 到新 main | 11 commit，每个涉及 i18n 的都冲突（共 5 轮），同样用脚本自动解决 |
| `go build` 验证 | 非 main 包全部编译通过；main 包因 `embed web/classic/dist` 无构建产物而报错（预期行为） |
| Risk 测试 | 通过 |

**i18n 合并策略**：对于 flat JSON 翻译文件，手动解决无意义（上千个 key 按 Unicode 排序）。标准做法：
1. `git show HEAD:path` 和 `git show MERGE_HEAD:path`（或 rebase 中的 commit）分别提取两端
2. python `{**theirs, **ours}` 合并（ours 优先保留已有翻译）
3. `dict(sorted(...))` 重新排序
4. 写回文件，`git add`

### 21.6 merge-check.sh 用法

```bash
scripts/merge-check.sh pre    # merge 前：检查分支、remote、gap、冲突区
scripts/merge-check.sh post   # merge 后：冲突标记、编译、文件完整、测试、i18n
scripts/merge-check.sh full   # 两者都跑
```

---

## 22. 前端同步策略：Classic → Default 二开功能移植

> 决策日期：2026-05-02
> 背景：上游在 `a42b3976` 引入 `web/default/`（React 19 + TypeScript + shadcn/ui + TanStack Router/Query/Table），本 fork 的二开 UI（~14,565 行、~40 文件）仅存在于 `web/classic/`，需同步到 `web/default/`。

### 22.1 核心约束

**模块化隔离**：最大化新建文件，最小化对上游文件的改动（~36 行改动 / 6 个上游文件），降低后续 merge upstream 冲突。

### 22.2 架构决策汇总

| 编号 | 决策 | 选型 | 理由 |
|-----|------|------|------|
| D1 | 目录隔离 | `src/features/<name>/` 标准目录 | TanStack Router 只扫描 `src/routes/`，`src-custom/` 需额外配置 |
| D2 | 侧边栏 | 独立 `use-custom-sidebar-items.ts` + 单行 spread | 上游文件仅增 ~3 行 |
| D3 | i18n | `addResourceBundle()` 注入独立翻译包 | 零修改上游 locale JSON |
| D4 | 渠道表单 | `ChannelCustomSections` 包装组件 + 单点插入 | drawer 3,358 行只增 1 import + 1 JSX |
| D5 | 系统设置 | 独立 section-registry + 追加导入 | 纯追加，不修改现有条目 |
| D6 | 工单角色 | `TICKET_STAFF: 5` 追加到 `roles.ts` | 后端已有 `RoleCustomerServiceUser = 5` |
| D7 | 路由 | `src/routes/_authenticated/` 下新建文件 | TanStack Router 自动发现，零冲突 |

### 22.3 上游文件改动清单

以下 8 个上游文件被改动，合计 ~45 行：

| 上游文件 | 改动 | 行数 | 风险 |
|---------|------|------|------|
| `src/hooks/use-sidebar-data.ts` | 1 import + spread + userRole | ~5 | 低 |
| `src/main.tsx` | 1 import | ~1 | 极低 |
| `src/lib/roles.ts` | 追加 `TICKET_STAFF: 5` | ~1 | 极低 |
| `src/components/layout/config/system-settings.config.ts` | 1 icon + 1 import + 5 行 sidebar 条目 | ~7 | 低 |
| `src/features/channels/lib/channel-form.ts` | schema 追加 4 字段 + transform/build 序列化 | ~20 | 中 |
| `src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 1 import + 2 行 JSX | ~3 | 中 |

### 22.4 技术转换速查表

| Classic (Semi Design) | Default (shadcn/ui + Tailwind) | 备注 |
|---|---|---|
| `<Table>` | TanStack Table + `DataTable*` 组件族 | `ColumnDef[]` + `useReactTable` |
| `<Modal>` | `<Dialog>` / `<AlertDialog>` | Dialog=表单, AlertDialog=确认 |
| `<SideSheet>` | `<Sheet>` | `side` prop 控制方向 |
| `<Form>` + `<Form.Input>` | react-hook-form + `<FormField>` | zod schema 验证 |
| `<Select>` | `<Select>` 或 `<Combobox>` | 需搜索用 Combobox |
| `<Tag>` | `<Badge>` / `<StatusBadge>` | |
| `<Banner>` | `<Alert>` | |
| `<Tabs>` + `<TabPane>` | `<Tabs>` + `<TabsContent>` | |
| `<Descriptions>` | `<dl>/<dt>/<dd>` + Tailwind | 无直接等效 |
| `<Spin>` / `<Empty>` | `<Skeleton>` / `<EmptyState>` | |
| `<Upload>` | 自建或 react-dropzone | shadcn 无内置 |
| `<ImagePreview>` | Dialog + `<img>` | 需自建 |
| `<Space>` | `flex gap-*` | Tailwind class |
| `<Row>` / `<Col>` | `grid grid-cols-*` | Tailwind grid |
| `<Popconfirm>` | `<ConfirmDialog>` | 项目已有 |
| `<DatePicker>` | `<DatePicker>` | 项目已有 |
| Toast (`showError`/`showSuccess`) | `toast.error()`/`toast.success()` (sonner) | |
| `isAdmin` (localStorage) | `useIsAdmin()` hook | |
| `StatusContext` | Zustand store / TanStack Query | 需确认 default 等效方式 |
| VChart | recharts（推荐）或沿用 | 需评估 |

### 22.5 坑点表

| 编号 | 坑点 | 说明 | 处理 |
|-----|------|------|------|
| P1 | i18n Key 风格 | classic 用中文 key `t('风控中心')`，default 用英文 key `t('Risk Control')` | 移植时**全部转英文 key**，zh.ts 提供中文翻译 |
| P2 | setting/settings JSON | 渠道编辑器数据存储在两个 JSON string 字段，有独立 build/parse 函数 | 新字段必须正确挂载且不破坏上游已有序列化 |
| P3 | routeTree.gen.ts | 自动生成文件 | **绝不手动编辑**，`bun run dev` 自动重生成 |
| P4 | Upload 组件 | shadcn 无内置 Upload | 用 react-dropzone + 自定义 UI |
| P5 | 工单角色粒度 | backend `RoleCustomerServiceUser=5`，default 只有 0/1/10/100 | roles.ts 追加 `TICKET_STAFF: 5` |
| P6 | StatusContext | classic 用 React Context 传服务器状态 | 找 default 等效方式（Zustand/Query） |
| P7 | 图表库 | classic 用 VChart（重量级） | default 无图表库时引入 recharts |
| P8 | 附件下载 | 用 SessionAuth（cookie-only） | `<img src>` / `<a href>` 自动带 cookie |
| P9 | drawer 冲突热区 | `channel-mutate-drawer.tsx` 3,358 行，上游高频变更 | 插入点加 `{/* fork: custom channel extensions */}` 注释作锚点 |
| P10 | mobile 兼容 | default 用 `MobileCardList` | 列定义需加 `meta.mobileHidden/mobileBadge/mobileTitle` |
| P11 | URL 路径 | classic `/console/risk`，default 不用 `/console/` | 统一为 `/risk`、`/tickets`、`/monitoring` |
| P12 | 额度工具函数 | 退款面板需 `renderQuota` / `quotaToDisplayAmount` | 确认 default 有等效工具再复用 |
| P13 | **Radix Select 禁止空字符串 value** | `@radix-ui/react-select` 的 `SelectItem` 在渲染时硬校验 `value !== ""`，违反即 throw（被 TanStack Router errorComponent 捕获后显示 500 页面）。Semi Design Select 无此限制，因此从 classic 移植 `<Select>` 筛选器时极易遗漏 | 用哨兵值 `"__all__"` 替代空字符串，初始 state、`<SelectItem value>`、API params 三处同步修改。**新增任何 Radix Select 组件时务必检查** |
| P14 | **API 响应结构不一致** | 后端 `GET /api/risk/groups` 返回 `{ schema_version, global_mode, items: [...] }` 包裹对象，而非直接数组。classic 正确处理了 `riskGroups.items`，但 default 移植时误用 `res.data?.data ?? []` 当数组 → `for...of` 报 `not iterable` → 500 | 移植 API 函数时**必须对照后端 controller 的 `common.ApiSuccess(c, ...)` 实际传入的数据结构**，特别注意 `gin.H{...items}` 包裹、`PageInfo` 分页封装等 |
| P15 | **TypeScript 接口字段必须对照后端 JSON tag** | 初始移植时 `api.ts` 的 54+ 个字段名凭猜测编写，与后端 Go struct JSON tag 不符（如 `sampling_rate` vs `sampling_rate_percent`、`logic` vs `match_mode`、`threshold` vs `value`、`user_id` vs `id`）。运行时表现为数据显示 undefined 或 React error #31（对象渲染为子节点） | 每新增/修改 `api.ts` 接口时，**必须 `grep` 对应后端 model/controller 的 JSON tag**，不可凭记忆。特别注意：(1) `ModerationConfig` 被 `ModerationConfigPayload { config, key_count }` 包裹；(2) `ModerationDebugResult` 被 `ModerationDebugPayload { pending, result }` 包裹；(3) `worker_state` 是对象数组不可直接渲染为 React children |
| P16 | **`tsc --noEmit` 需指定 `-p tsconfig.app.json`** | 项目根 `tsconfig.json` 设置 `"files": []` + references，直接 `tsc --noEmit` 不检查任何源文件，返回 0 exit code 造成误判 | 必须使用 `npx tsc --noEmit -p tsconfig.app.json` 才能真正检查前端 TypeScript |

### 22.6 分阶段实施记录（全部完成 2026-05-03）

| 阶段 | 内容 | 新建文件 | 改上游 | 状态 |
|-----|------|---------|-------|------|
| P0 | 基础设施（i18n + 角色 + 侧边栏） | 5 | 3 | ✅ |
| P1 | 分组监控 | 11 | 0 | ✅ |
| P2 | 邀请码管理 | 12 | 0 | ✅ |
| P3 | 充值历史 | 6 | 0 | ✅ |
| P4 | 工单系统 | 18 | 0 | ✅ |
| P5 | 风控中心 | 15 | 0 | ✅ |
| P6 | 渠道编辑器扩展 | 5 | 2 | ✅ |
| P7 | 系统设置扩展 | 8 | 1 | ✅ |
| P8 | i18n 补全（6 语言）+ 构建验证 | 4 | 0 | ✅ |

### 22.7 实际新建文件清单

**P0 基础设施**：
- `src/i18n/custom-translations/{en,zh,fr,ja,ru,vi,index}.ts` — 独立翻译注入
- `src/hooks/use-custom-sidebar-items.ts` — 二开侧边栏项（含 role 参数动态显示 Ticket Admin）

**P1 分组监控** (`src/features/monitoring/`)：
- `api.ts`, `constants.ts`, `components/monitoring-dashboard.tsx`, `group-status-card.tsx`, `group-detail-panel.tsx`, `status-timeline.tsx`, `availability-chart.tsx` 等 11 文件
- 路由 `src/routes/_authenticated/monitoring/index.tsx`

**P2 邀请码** (`src/features/invitation-codes/`)：
- `api.ts`, `constants.ts`, `components/{table,columns,provider,dialogs}` 等 12 文件
- 路由 `src/routes/_authenticated/invitation-codes/index.tsx`

**P3 充值历史** (`src/features/topup-history/`)：
- `api.ts`, `constants.ts`, `components/{table,columns}` 等 6 文件
- 路由 `src/routes/_authenticated/topup-history/index.tsx`

**P4 工单系统** (`src/features/tickets/`)：
- `api.ts`, `constants.ts`, `lib/`, `hooks/`, `components/{ticket-list,ticket-detail,ticket-admin-list,ticket-admin-detail,conversation,message-item,reply-box,...}` 等 18 文件
- 路由 4 个：`tickets/{index,$ticketId}`, `ticket-admin/{index,$ticketId}`
- 角色守卫：`TICKET_STAFF(5)` 用于 ticket-admin 路由

**P5 风控中心** (`src/features/risk/`)：
- `api.ts`（~300 行，30+ API 函数，完整类型定义）
- `constants.ts`（~180 行，9 metric 定义 + 全部 option/map）
- `components/risk-tabs.tsx` — 3 Tab 容器（distribution/moderation/enforcement）
- `components/distribution-tab.tsx` — 概览卡片 + 配置 + 分组矩阵 + 子 Tab
- `components/moderation-tab.tsx` — 队列统计 + 配置 + 规则 + 调试 + incidents
- `components/enforcement-tab.tsx` — 概览 + 配置 + 计数器 + incidents
- `components/{overview/,config/,groups/,subjects/,incidents/,rules/,moderation/}` 子组件
- 路由 `src/routes/_authenticated/risk/index.tsx`（ADMIN 守卫）

**P6 渠道编辑器** (`src/features/channels/components/custom/`)：
- `channel-custom-sections.tsx` — 包装组件（单一入口点）
- `pressure-cooling-editor.tsx` — 压力冷却（switch + 4 字段）
- `channel-rate-limit-editor.tsx` — 限流（switch + RPM/并发/策略/队列）
- `error-filter-rules-editor.tsx` — 错误过滤（动态规则数组）
- `risk-control-headers-editor.tsx` — 风控头部（8 种 source + custom 占位符）

**P7 系统设置** (`src/features/system-settings/custom/`)：
- `section-registry.tsx` — 3 section 注册
- `index.tsx` — 页面组件
- `email-template-settings-section.tsx` — 模板选择/编辑/预览/重置
- `ticket-settings-section.tsx` — 分配规则/通知/附件（3 Tab）
- `group-monitoring-settings-section.tsx` — 分组多选/参数/排除列表
- 路由 2 个：`custom/{index,$section}`

**P8 i18n**：
- `{fr,ja,ru,vi}.ts` — 4 语言翻译文件（高可见 key 翻译，其余 fallback 英文）

### 22.8 default 前端关键模式备忘

**Feature 目录结构**：
```
src/features/<name>/
  index.tsx, api.ts, types.ts, constants.ts
  components/ (table, columns, provider, dialogs, drawers)
  lib/ (actions, form, utils)
  hooks/
```

**新建页面清单**：
1. 路由文件 `src/routes/_authenticated/<name>/index.tsx`（`createFileRoute` + `beforeLoad` 权限检查 + `validateSearch`）
2. Feature 组件（`SectionPageLayout` compound component）
3. API 层（axios 函数 + TanStack Query keys factory）
4. 表格（`ColumnDef[]` + `useReactTable` + `useTableUrlState`）
5. 表单（zod schema + `useForm` + `zodResolver`）

**Dialog 状态管理**：Context provider 持有 `open: 'create' | 'edit' | null` + `currentRow`，子组件调用 `setOpen('create')` 打开。

**API 通信**：`api.ts` 导出函数 → TanStack Query `useQuery`/`useMutation` → `queryClient.invalidateQueries` 刷新。

**表格状态同步 URL**：`useTableUrlState({ search, navigate, pagination, globalFilter, columnFilters })`。

### 22.9 变更 §21.4 决策

> ~~`web/default/` 作为只读接收，不做定制~~ → 改为：`web/default/` 中添加二开功能，与 `web/classic/` 保持功能入口统一。二开代码通过模块化隔离（82 新建文件 vs ~45 行上游改动 / 6 个上游文件）最小化 merge 冲突。全部 8 阶段已于 2026-05-03 完成。
