package service

// EstimateTokenByModel 粗略估算文本的 token 数量（字符数 × 1.5）。
//
// 这是一个有意保持简单的估算：仅用于预扣费预估，以及上游未返回 usage 时的
// 兜底计费；真实计费以上游返回的 usage 为准，所以估算精度无需很高。
//
// 按字符数（rune）而非字节数（len）计算，避免中文等多字节字符被严重高估。
// model 参数保留以兼容调用方签名，当前不参与计算。
func EstimateTokenByModel(model, text string) int {
	if text == "" {
		return 0
	}
	return len([]rune(text)) * 3 / 2
}
