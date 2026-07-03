package common

func GetTrustQuota() int {
	return int(10 * QuotaPerUnit)
}

// ClampPreConsumeCompletionTokens 将客户端传入的 max_tokens 钳制到预扣估算上限，
// 仅用于预扣费估算（真实计费以上游 usage 为准）。MaxPreConsumeCompletionTokens<=0 表示不钳制。
func ClampPreConsumeCompletionTokens(maxTokens int) int {
	if MaxPreConsumeCompletionTokens > 0 && maxTokens > MaxPreConsumeCompletionTokens {
		return MaxPreConsumeCompletionTokens
	}
	return maxTokens
}
