# 低余额用户工单/开票周限流

> 状态：**已实现**（dev-20260512，2026-06-26）
> 关联坑点：[`99-pitfalls.md#129`](99-pitfalls.md)

---

## 1. 背景

低余额（余额接近耗尽）用户存在滥发工单 / 开票申请的风险。需求：**当用户余额 < 5（货币单位）时，当前 UTC+8 自然周内只能新建 1 次工单/开票申请**，超出则提示「提交过于频繁，如需帮助请先联系客服咨询」。

## 2. 判定规则（统一 helper）

`model.GetUserTicketWeeklyLimitStatus(userId, role)`：

1. `role >= RoleAdminUser(10)` → **豁免**（`Exempt=true`，不限）。客服(5) 不豁免。
2. 读 `quota = GetUserQuota(userId, false)`。`quota >= 5*QuotaPerUnit`（=2,500,000）→ 不限（`Remaining=-1`）。**严格小于**才算低余额。
3. 否则 `used = COUNT(本人本周UTC+8 所有类型工单)`：`used >= 1` → `Limited=true`；否则放行（`Remaining=1`）。

- 覆盖类型：**general / invoice / refund 三类合并计 1 次/周**（同存 `tickets` 表，单条 `COUNT(user_id, created_time>=weekStart)` 即可）。
- 自然周：**周一 00:00 (UTC+8)** 起，`reset_at` = 下周一 00:00 (UTC+8)。用 `time.FixedZone("CST", 8*3600)` 固定时区，免 tzdata 依赖。
- 计数含本周早先（即便当时余额较高）建的工单——即「低余额用户每周 1 次」。
- 用户不能自删工单（仅删附件），故无 soft-delete 绕过，计数取非删即可。

## 3. 文件链路

### 后端
| 文件 | 内容 |
|------|------|
| `common/week.go` | `WeekStartUnixUTC8` / `WeekEndUnixUTC8`（UTC+8 周一对齐） |
| `model/ticket_limit.go` | `LowBalanceTicketThreshold` / `CountUserTicketsCreatedSince` / `GetUserTicketWeeklyLimitStatus` / `TicketWeeklyLimitStatus` / `ErrTicketWeeklyLimit` |
| `controller/ticket.go` | `enforceWeeklyTicketLimit`（注入 `CreateTicket`/`CreateInvoiceTicket`/`CreateRefundTicket`）+ `GetTicketLimitStatus`（pre-check） |
| `router/api-router.go` | `GET /api/ticket/limit-status` |
| `i18n/keys.go` + `i18n/locales/{en,zh-CN,zh-TW}.yaml` | `ticket.weekly_limit_exceeded` |

### 前端（双门：pre-check 禁用 + 后端硬门兜底，符合坑点 #76）
| 主题 | 文件 |
|------|------|
| Default | `features/tickets/api.ts`(`getTicketLimitStatus`) + `components/dialogs/create-ticket-dialog.tsx`(general+refund) + `create-invoice-ticket-dialog.tsx`(invoice) + `i18n/locales/*.json` |
| Classic | `components/table/tickets/modals/CreateTicketModal.jsx`(general+refund) + `CreateInvoiceTicketModal.jsx`(invoice) + `i18n/locales/*.json` |

前端打开创建表单时调 `GET /api/ticket/limit-status`，`limited` 为真则禁用提交 + 顶部提示；提交被后端拒时维持现有 message toast。

## 4. API 契约

`GET /api/ticket/limit-status` →
```json
{ "success": true, "data": {
  "limited": false, "exempt": false,
  "threshold_quota": 2500000, "balance_quota": 1200000,
  "used": 0, "remaining": 1, "reset_at": 1719676800
}}
```
- `limited` = `!exempt && balance_quota < threshold_quota && used >= 1`。
- `remaining`：`-1` 表示无限（豁免或余额≥阈值）；否则 `1-used`（0 或 1）。

创建接口被限：`{ "success": false, "message": "<本地化 ticket.weekly_limit_exceeded>" }`（HTTP 200，沿用 `ApiErrorI18n`）。

## 5. 测试

- `common/week_test.go`：UTC+8 周一/周日边界、跨周、`WeekEnd==WeekStart+604800` 不变式（锚点 2024-01-01 00:00 UTC+8 = 周一 = unix 1704038400）。覆盖率 100%。
- `model/ticket_limit_test.go`：管理员豁免、余额阈值（等于阈值不限）、低余额首次放行/二次拦截、三类合并计数、跨周重置。`ticket_limit.go` 覆盖率 90–100%。

## 6. 取舍与残留

- **门控位置 = controller 层**（与既有 invoice/refund 冲突校验同层，符合坑点 #76「提交时二次校验」）。残留多 tab 并发窗口与现有冲突校验等价；如需强一致可加 per-user 顾问锁，**本批未做**。
- 高余额用户**无频控**（需求未覆盖，仅记录）。
- 工单回复 `CreateUserTicketMessage` 不在「新建」范围，不限流。

## 7. 参考
- 坑点：`docs/dev/99-pitfalls.md` #129、#76（前后端双校验）
- 关键代码：`controller/ticket.go`、`model/ticket_limit.go`、`common/week.go`
