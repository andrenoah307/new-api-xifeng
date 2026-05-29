# 发票-退款交叉校验

## 背景

工单系统的退款（refund）和发票（invoice）类型原来完全独立。用户可在已开发票的情况下申请退款，管理员处理退款时也无法看到发票记录。

## 设计

采用**警告+确认**模式（非硬阻断），因为退款是额度级（quota-based），发票是订单级（order-based），无法做精确订单级匹配。

## 数据流

### 用户提交退款前
```
前端 GET /api/ticket/refund/invoice-check
  → model.GetUserInvoiceSummaries(userId)
  → 返回 { has_invoices, has_active_invoices, invoices[] }
  → 有 active 发票时：显示 Alert + Checkbox
  → 提交时附带 invoice_conflict_acknowledged=true
  → CreateRefundTicket controller 二次校验
```

### 用户提交发票前
```
前端 GET /api/ticket/invoice/refund-check
  → model.GetUserActiveRefundSummaries(userId)
  → 返回 { has_refunds, refunds[] }
  → 有 active 退款时：显示 Alert + Checkbox
  → 提交时附带 refund_conflict_acknowledged=true
  → CreateInvoiceTicket controller 二次校验
```

### 管理员查看退款详情
```
GET /api/ticket/admin/:id/refund
  → 返回 { refund, user_invoices[] }
  → RefundDetail 组件顶部渲染 InvoiceHistoryAlert
  → 首条展开，多条折叠
```

## 涉及文件

| 层 | 文件 |
|---|---|
| Model | model/ticket_invoice.go (GetUserInvoiceSummaries) |
| Model | model/ticket_refund.go (GetUserActiveRefundSummaries) |
| Controller | controller/ticket.go (Check* handlers + Create* 校验 + GetTicketRefund 增强) |
| Router | router/api-router.go (2 新路由) |
| i18n | i18n/keys.go + locales/ |
| Default 前端 | features/tickets/api.ts, create-ticket-dialog.tsx, create-invoice-ticket-dialog.tsx, refund-detail.tsx, ticket-admin-detail.tsx |
| Classic 前端 | CreateTicketModal.jsx, CreateInvoiceTicketModal.jsx, RefundDetail.jsx, TicketAdmin/index.jsx |

## 退款方式精简

退款收款方式仅保留**支付宝**（alipay）和**银行卡**（bank），移除微信和其他选项。

- 后端 `IsValidRefundPayeeType` 仅接受 `alipay`/`bank`，`refundPayeeTypeText` 保留历史类型显示（向后兼容）
- Default 前端 `PAYEE_TYPE_OPTIONS` 仅含两项（表单用），`PAYEE_TYPE_LABELS` 含所有类型（显示用）
- Classic 前端新增 `REFUND_PAYEE_TYPE_OPTIONS`（下拉框用），原 `REFUND_PAYEE_TYPES` 保留完整（显示用）

## 视觉优化

- **InvoiceHistoryAlert**：自定义 div + amber 配色 + `max-w-2xl` + `grid-cols-[auto_1fr_auto_auto]` + 列标题
- **Alert warning 变体**：`alert.tsx` 新增 amber warning 变体，对话框冲突提醒统一使用
- **Classic Grid**：列宽从 `1fr×4` 改为 `auto 1fr auto auto`，`maxWidth: 672`
