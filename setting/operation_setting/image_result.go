package operation_setting

// 生图结果落地保存（应对 CDN 非流式超时：客户端拿不到响应时可去后台取图）。
// 开启后所有成功的 OpenAI 格式生图响应（/v1/images/generations、/v1/images/edits，
// 非流式）都会把图片内容保存到本地存储，base64 直接解码落盘、url 结果下载落盘，
// 数据库只存文件引用，由系统任务按保留天数自动清理。
var (
	ImageResultEnabled       = false // 总开关，默认关闭
	ImageResultRetentionDays = 7     // 保留天数，到期由清理任务删除文件与记录
	ImageResultMaxFileSizeMB = 30    // 单张图片上限（MB），超限跳过该张并记录日志
)
