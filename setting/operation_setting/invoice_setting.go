package operation_setting

// 开票票种配置（发票工单）。费率为百分比（如 6 表示 6%）。
// 用户申请时会把费率快照到 TicketInvoice.FeeRate；管理员标记「已开票」时按该快照扣除
// 手续费（计费基数 = 关联充值订单的实际到账额度之和），修改费率不影响已提交的申请。
// 说明文本由管理员自由撰写，会原样展示给用户（如起开金额、税点、开票主体等）。
var (
	InvoiceRegularFeeRate     float64 = 0  // 普票（增值税普通发票）手续费率（%）
	InvoiceRegularDescription         = "" // 普票票种说明
	InvoiceSpecialEnabled             = false
	InvoiceSpecialFeeRate     float64 = 0  // 增票（增值税专用发票）手续费率（%）
	InvoiceSpecialDescription         = "" // 增票票种说明
	InvoiceServiceName                = "" // 应税服务名称/发票内容（空 = 前端回退默认文案"*生产生活服务*技术服务费"）
)
