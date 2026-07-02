# 使用日志默认结束时间：now+1h → 当天 23:59:59

## 背景
使用日志页面的默认时间窗结束时间历史上取 `当前时刻 + 1 小时`（前端为兜住“当前小时”数据的经验值）。该值语义不直观，且与后端过滤语义未严格对齐。

## 变更
默认结束时间改为「当天 23:59:59」（浏览器本地时区），抽公共 helper 复用。

| 端 | 文件 | 改动 |
|---|---|---|
| Classic | `web/classic/src/helpers/utils.jsx` | 新增 `getTodayEndTimestamp()`（今日 0 点 + 86399s） |
| Classic | `web/classic/src/hooks/usage-logs/useUsageLogsData.jsx` | 表单初值与 getFormValues 两处默认结束时间改用 helper；移除不再使用的 `now` |
| Default | `web/default/src/features/usage-logs/lib/utils.ts` | 新增 `getEndOfToday()`（`setHours(23,59,59,999)`），`getDefaultTimeRange()` 调用之 |

## 为什么是 23:59:59 而不是次日 00:00:00
后端 `model/log.go` 过滤为 `created_at <= endTimestamp`（含右端）：
- 「当天 23:59:59」= 今日 0 点 + 86399s → 精确含当天全部、排除次日 0 点。✅
- 「次日 00:00:00」= + 86400s → `<=` 会多纳入恰好次日零点 1 秒的记录，轻微溢出到明天；除非后端改用 `<`，否则不采用。

Default 侧 `end` 为 `Date`（23:59:59.999），经 `timestampToSeconds` 向下取整为当天 0 点 + 86399s，与 Classic 完全一致。

## 后端
无需改动：结束时间是纯前端默认展示值；controller 仅在 `startTimestamp==0` 时兜底默认起点，从不注入默认结束时间。

## 未纳入范围（同类可选后续）
dashboard / 绘图(mj) / 任务日志仍保留 `+3600` 模式：`web/classic/src/hooks/dashboard/useDashboardData.js`、`web/classic/src/hooks/mj-logs/useMjLogsData.js`、`web/classic/src/hooks/task-logs/useTaskLogsData.js`、`web/default/src/lib/time.ts`。如需统一，可复用同一 helper 思路。

## 相关坑点
见 `docs/dev/99-pitfalls.md` #132。
