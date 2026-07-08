package operation_setting

// 开票票种配置（发票工单）。费率为百分比（如 6 表示 6%），仅用于展示与留档，不参与扣费。
// 说明文本由管理员自由撰写，会原样展示给用户（如起开金额、税点、开票主体等）。
var (
	InvoiceRegularFeeRate     float64 = 0  // 普票（增值税普通发票）手续费率（%）
	InvoiceRegularDescription         = "" // 普票票种说明
	InvoiceSpecialEnabled             = false
	InvoiceSpecialFeeRate     float64 = 0  // 增票（增值税专用发票）手续费率（%）
	InvoiceSpecialDescription         = "" // 增票票种说明
)
