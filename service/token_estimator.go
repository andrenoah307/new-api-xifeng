package service

import "unicode/utf8"

// EstimateTokenByModel 粗略估算文本的 token 数量（字符数 × 1.5）。
//
// 这是一个有意保持简单的估算：仅用于预扣费预估，以及上游未返回 usage 时的
// 兜底计费；真实计费以上游返回的 usage 为准，所以估算精度无需很高。
//
// 按字符数（rune）而非字节数（len）计算，避免中文等多字节字符被严重高估。
// model 参数保留以兼容调用方签名，当前不参与计算。
func EstimateTokenByModel(model, text string) int {
	return EstimateTokenByRuneCount(utf8.RuneCountInString(text))
}

// EstimateTokenByRuneCount applies the same integer estimate as
// EstimateTokenByModel without requiring the complete text to be retained.
func EstimateTokenByRuneCount(runeCount int) int {
	if runeCount <= 0 {
		return 0
	}
	return runeCount * 3 / 2
}
