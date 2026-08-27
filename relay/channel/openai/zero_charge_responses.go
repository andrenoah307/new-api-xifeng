package openai

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// Some adapters intentionally expose a no-usage Responses contract. They are
// kept on their existing accounting path until their provider-specific rules
// are implemented; the shared Responses guard must not reinterpret missing
// usage for them.
func responsesZeroChargeGuardEnabled(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return true
	}
	if info.ChannelMeta == nil {
		return true
	}
	switch info.ChannelMeta.ChannelType {
	case constant.ChannelTypeXai, constant.ChannelCloudflare:
		return false
	default:
		return true
	}
}

// finalizeResponsesUsage applies the Responses-family zero-charge contract
// after conversion has finished composing usage. A missing or empty upstream
// usage must not be replaced with a local estimate, even when output exists.
func finalizeResponsesUsage(info *relaycommon.RelayInfo, usage *dto.Usage, hasOutput, usagePresent bool) *dto.Usage {
	if usage == nil {
		usage = &dto.Usage{}
	}
	if !responsesZeroChargeGuardEnabled(info) {
		return usage
	}
	if !usagePresent || (usage.CompletionTokens == 0 && !relaycommon.UsageHasOutputTokens(usage) && !hasOutput) {
		reason := relaycommon.ZeroChargeReasonEmptyOutput
		if !usagePresent {
			reason = relaycommon.ZeroChargeReasonUsageMissing
		}
		relaycommon.CloseoutZeroCharge(info, usage, reason)
	}
	return usage
}
