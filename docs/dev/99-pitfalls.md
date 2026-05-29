# 踩坑预案汇总

---

### #76 工单交叉校验竞态：pre-check + create 双重校验

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| controller/ticket.go | 退款/发票创建时 | P2 |

**问题**：用户多 tab 操作时，tab A 执行 pre-check 返回无冲突，同时 tab B 提交了发票/退款工单。如果创建接口不做二次校验，tab A 的提交将绕过冲突检查。

**解法**：`CreateRefundTicket` 和 `CreateInvoiceTicket` controller 内部必须在提交时再查一次 `GetUserInvoiceSummaries` / `GetUserActiveRefundSummaries`。有冲突但请求无 `acknowledged` 标记时返回 400。前端的 pre-check 只是 UX 优化，后端才是安全屏障。

**相关代码**：`controller/ticket.go:CreateRefundTicket`、`controller/ticket.go:CreateInvoiceTicket`

---

### #77 GetAdminRefundDetail 返回结构变更

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| controller/ticket.go, api.ts, TicketAdmin/index.jsx | 管理员查看退款工单详情时 | P1 |

**问题**：`GetTicketRefund` API 原来返回 `{ refund: {...} }`，现在返回 `{ refund: {...}, user_invoices: [...] }`。Default 前端 `getAdminRefundDetail` 的消费方式从 `.data?.data?.refund` 改为 `.data?.data`（返回完整对象），Classic 前端 `loadRefundDetail` 需要额外提取 `data.user_invoices`。两套前端必须同步更新，否则 `refundData` 结构不匹配导致退款详情无法渲染。

**解法**：API 返回结构变更必须同时更新所有消费端。Default 前端的 `ticket-admin-detail.tsx` 改为 `refundData.refund` 访问退款记录，Classic 前端的 `TicketAdmin/index.jsx` 新增 `userInvoices` state 提取。

**相关代码**：`controller/ticket.go:GetTicketRefund`、`api.ts:getAdminRefundDetail`、`TicketAdmin/index.jsx:loadRefundDetail`
