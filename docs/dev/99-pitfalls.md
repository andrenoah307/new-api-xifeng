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

---

### #78 退款方式精简：表单选项与显示标签必须分离

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| model/ticket_refund.go, constants.ts, ticketUtils.js | 精简收款方式下拉选项时 | P2 |

**问题**：移除微信/其他收款方式后，如果同时删除后端的 `refundPayeeTypeText` 和前端的显示映射，历史退款记录中 `payee_type=wechat` 或 `payee_type=other` 的工单将无法正确显示收款方式名称。

**解法**：分离"表单选项"（用户可选的值）和"显示标签"（管理员/用户查看的值）。后端 `IsValidRefundPayeeType` 仅允许 `alipay`/`bank`；`refundPayeeTypeText` 保留所有历史类型的中文映射。前端用独立的 `PAYEE_TYPE_OPTIONS`（表单用）和 `PAYEE_TYPE_LABELS`（显示用），Classic 用 `REFUND_PAYEE_TYPE_OPTIONS`（下拉框）和完整 `REFUND_PAYEE_TYPES`（显示）。

**相关代码**：`model/ticket_refund.go:IsValidRefundPayeeType`、`model/ticket_refund.go:refundPayeeTypeText`、`constants.ts:PAYEE_TYPE_OPTIONS/PAYEE_TYPE_LABELS`、`ticketUtils.js:REFUND_PAYEE_TYPE_OPTIONS`

---

### #79 Alert 组件 warning 变体：对话框内提醒统一规范

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| alert.tsx, create-ticket-dialog.tsx, create-invoice-ticket-dialog.tsx | 对话框内展示冲突提醒时 | P3 |

**问题**：shadcn Alert 仅有 `default` 和 `destructive` 两种变体。冲突提醒不是错误（destructive 语义），也不是纯信息（default 无视觉突出），需要中间态的 warning 样式。使用 `destructive` 会让用户误以为出错而放弃操作。

**解法**：CVA 新增 `warning` 变体（amber 配色 + dark mode 适配），对话框内的冲突提醒统一改用 `variant="warning"`。InvoiceHistoryAlert 因需控制宽度（max-w-2xl），不使用 Alert 组件而用自定义 div + 相同 amber 配色。

**相关代码**：`alert.tsx:alertVariants`、`create-ticket-dialog.tsx`、`create-invoice-ticket-dialog.tsx`、`refund-detail.tsx:InvoiceHistoryAlert`
