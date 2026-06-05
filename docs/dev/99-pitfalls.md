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

---

### #92 下载 403：window.open / 邮件链接无法携带自定义请求头

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| middleware/auth.go, router/api-router.go, export-tasks-sheet.tsx | 用户点击下载按钮或邮件中下载链接时 | P1 |

**问题**：`authHelper` 两阶段认证——Stage 1 验 cookie session，Stage 2 验 `New-Api-User` 请求头（CSRF 防护）。`window.open()` 和邮件客户端点击链接只发送 cookie，不发自定义头，Stage 2 返回 403/401。

**解法**：
- **站内下载**：改用 `fetch()` + blob 下载，通过 `getCommonHeaders()` 携带 `New-Api-User` 头。三处都改：Default 用户端、Classic 用户端、Default 管理端。
- **邮件链接**：下载路由从 `UserAuth()` 改为 `SessionAuth()`（仅验 cookie，跳过 CSRF 头检查）。`SessionAuth` 仍设置 `c.Set("id", ...)` 和 `c.Set("role", ...)`，rate limiter 和 ownership 校验正常工作。仅用于幂等只读 GET 资源，与工单附件下载同模式。

**相关代码**：`middleware/auth.go:SessionAuth`、`router/api-router.go:458`、`export-tasks-sheet.tsx:handleDownload`、`ExportTasksModal.jsx:onClick`、`admin-export-tab.tsx:onClick`

---

### #93 前端构建缓存：Go embed 导致代码修改不生效

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| main.go, web/default/dist | 修改前端代码后直接 go build 部署 | P1 |

**问题**：`main.go` 的 `//go:embed web/default/dist` 在编译时将前端静态文件嵌入 Go 二进制。如果只修改了前端源码但没有重新 `bun run build`，`dist/` 目录仍是旧版本，Go 二进制打包旧文件。表现为"代码已提交但线上行为不变"。

**解法**：前端代码修改后部署流程必须：`cd web/default && bun run build` → `go build`。可通过检查 `dist/` 文件时间戳 vs 源码最后修改时间排查。CI 中确保 build 步骤在 go build 之前。

---

### #94 Classic 自动分组字符串指标：独立于数值指标的完整链路

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| web/classic/src/pages/AutoGroup/index.jsx | 添加 current_group 等字符串类型条件指标时 | P2 |

**问题**：Classic 前端自动分组条件编辑器仅支持数值类型指标——操作符固定 6 个（>=, >, <=, <, ==, !=），值输入固定 InputNumber。`current_group` 是字符串类型指标，需要不同的操作符（仅 ==, !=）和值输入（分组选择器 Select）。

**解法**：新增 `STRING_METRICS` Set 和 `STRING_OPS` 数组。条件行渲染根据 `STRING_METRICS.has(metric)` 切换：操作符用 `STRING_OPS` / `OP_OPTIONS`；值输入用分组 Select / InputNumber。指标切换时通过 `handleMetricChange` 重置 op/value/value_str，防止数值条件的残留值污染字符串条件。`formatConditionsSummary` 对字符串指标展示 `value_str` 而非 `value`。

**相关代码**：`web/classic/src/pages/AutoGroup/index.jsx:STRING_METRICS`、`handleMetricChange`、条件行渲染 (line 822+)
