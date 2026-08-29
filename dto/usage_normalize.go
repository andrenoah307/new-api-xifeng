package dto

// MergeInputTokenDetails folds a Responses-shaped input_tokens_details object
// onto the Chat-shaped prompt_tokens_details consumed by billing code.
func (u *Usage) MergeInputTokenDetails(source string, details *InputTokenDetails) {
	if u == nil || details == nil {
		return
	}

	if source == BillingUsageSourceOAIChat {
		return
	}
	if source == BillingUsageSourceOAIResponses {
		u.PromptTokensDetails.CachedTokens = nonNegativeTokenDetail(details.CachedTokens)
		u.PromptTokensDetails.CachedCreationTokens = nonNegativeTokenDetail(details.CachedCreationTokens)
		u.PromptTokensDetails.CacheWriteTokens = nonNegativeTokenDetail(details.CacheWriteTokens)
		u.PromptTokensDetails.TextTokens = nonNegativeTokenDetail(details.TextTokens)
		u.PromptTokensDetails.AudioTokens = nonNegativeTokenDetail(details.AudioTokens)
		u.PromptTokensDetails.ImageTokens = nonNegativeTokenDetail(details.ImageTokens)
		return
	}

	fillPromptTokenDetail(&u.PromptTokensDetails.CachedTokens, details.CachedTokens)
	fillPromptTokenDetail(&u.PromptTokensDetails.CachedCreationTokens, details.CachedCreationTokens)
	fillPromptTokenDetail(&u.PromptTokensDetails.CacheWriteTokens, details.CacheWriteTokens)
	fillPromptTokenDetail(&u.PromptTokensDetails.TextTokens, details.TextTokens)
	fillPromptTokenDetail(&u.PromptTokensDetails.AudioTokens, details.AudioTokens)
	fillPromptTokenDetail(&u.PromptTokensDetails.ImageTokens, details.ImageTokens)
}

// NormalizePromptTokenDetails preserves the legacy source-on-Usage API.
func (u *Usage) NormalizePromptTokenDetails(source string) {
	if u == nil {
		return
	}
	u.MergeInputTokenDetails(source, u.InputTokensDetails)
}

func fillPromptTokenDetail(target *int, source int) {
	if *target == 0 && source > 0 {
		*target = source
	}
}

func nonNegativeTokenDetail(value int) int {
	if value <= 0 {
		return 0
	}
	return value
}
