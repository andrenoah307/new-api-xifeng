package service

import (
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// ReconstructGpt56CacheWrite 返回 gpt-5.6+ OpenAI/Responses 路径上游漏报缓存写入时应重构的写入 token 量。
// 计费与下游响应 patch 同源调用，杜绝分叉（坑点 #142/#150）。
// 仅当以下全部成立时返回 (value>0, true, reclaimFromBase)，否则 (0, false, false)：
//   - !relayInfo.PriceData.UsePrice
//   - 上游 effective 缓存写入/Claude cache_creation == 0（真实值优先）
//   - 转换链末端为 OpenAI/Responses，或 usage 是 anthropic 语义
//   - 模型 ≥ gpt-5.6（isGpt56OrHigherModel(relayInfo.OriginModelName)）
//   - openai 语义重构量 = prompt − cached − image − audio > 0
//   - anthropic 语义重构量 = prompt − image − audio > 0（input 已排除缓存读）
func ReconstructGpt56CacheWrite(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) (int, bool, bool) {
	if usage == nil || relayInfo == nil {
		return 0, false, false
	}
	isAnthropic := usageSemanticFromUsage(relayInfo, usage) == "anthropic"

	prompt := usage.PromptTokens
	if usage.InputTokens > 0 {
		if prompt == 0 {
			prompt = usage.InputTokens
		} else if usage.InputTokens != prompt {
			prompt += usage.InputTokens
		}
	}

	cached := usage.PromptTokensDetails.CachedTokens
	image := usage.PromptTokensDetails.ImageTokens
	audio := usage.PromptTokensDetails.AudioTokens
	existingWrite := usage.PromptTokensDetails.EffectiveCacheWriteTokens()
	if usage.PromptTokensDetails.CachedCreationTokens > existingWrite {
		existingWrite = usage.PromptTokensDetails.CachedCreationTokens
	}
	if splitWrite := usage.ClaudeCacheCreation5mTokens + usage.ClaudeCacheCreation1hTokens; splitWrite > existingWrite {
		existingWrite = splitWrite
	}
	if usage.InputTokensDetails != nil {
		if usage.InputTokensDetails.CachedTokens > cached {
			cached = usage.InputTokensDetails.CachedTokens
		}
		if usage.InputTokensDetails.ImageTokens > image {
			image = usage.InputTokensDetails.ImageTokens
		}
		if usage.InputTokensDetails.AudioTokens > audio {
			audio = usage.InputTokensDetails.AudioTokens
		}
		if inputWrite := usage.InputTokensDetails.EffectiveCacheWriteTokens(); inputWrite > existingWrite {
			existingWrite = inputWrite
		}
		if usage.InputTokensDetails.CachedCreationTokens > existingWrite {
			existingWrite = usage.InputTokensDetails.CachedCreationTokens
		}
	}
	if existingWrite > 0 {
		return 0, false, false
	}

	if relayInfo.PriceData.UsePrice {
		return 0, false, false
	}
	if !isAnthropic && !isOpenAITextRelayFormat(relayInfo.GetFinalRequestRelayFormat()) {
		return 0, false, false
	}
	if !isGpt56OrHigherModel(relayInfo.OriginModelName) {
		return 0, false, false
	}

	reconstructed := prompt - image - audio
	if !isAnthropic {
		reconstructed -= cached
	}
	if reconstructed <= 0 {
		return 0, false, false
	}
	return reconstructed, true, isAnthropic
}
