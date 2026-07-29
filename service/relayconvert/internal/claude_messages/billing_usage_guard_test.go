package claudemessages

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S4 防御性守卫（坑点 #169）：含缓存的 OpenAI 口径 usage 不得被原样固化成
// anthropic 净口径 BillingUsage —— 否则结算按 anthropic 跳过减缓存，
// 同一批 token 会被输入价与缓存价双收。
func TestClaudeBillingUsageFromSemanticUsageNetsNonAnthropicInput(t *testing.T) {
	tests := []struct {
		name            string
		semantic        string
		wantInputTokens int
	}{
		{name: "openai 口径需净化", semantic: dto.BillingUsageSemanticOpenAI, wantInputTokens: 700},
		{name: "未标记口径需净化", semantic: "", wantInputTokens: 700},
		{name: "anthropic 口径原样", semantic: dto.BillingUsageSemanticAnthropic, wantInputTokens: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &dto.Usage{
				PromptTokens:     1000,
				CompletionTokens: 8,
				UsageSemantic:    tt.semantic,
			}
			usage.PromptTokensDetails.CachedTokens = 200
			usage.PromptTokensDetails.CachedCreationTokens = 100

			billingUsage := claudeBillingUsageFromSemanticUsage(usage)

			require.NotNil(t, billingUsage)
			require.NotNil(t, billingUsage.ClaudeUsage)
			assert.Equal(t, tt.wantInputTokens, billingUsage.ClaudeUsage.InputTokens)
			assert.Equal(t, 200, billingUsage.ClaudeUsage.CacheReadInputTokens)
			assert.Equal(t, 100, billingUsage.ClaudeUsage.CacheCreationInputTokens)
			assert.Equal(t, 8, billingUsage.ClaudeUsage.OutputTokens)
		})
	}
}

func TestClaudeBillingUsageFromSemanticUsageClampsNegativeInput(t *testing.T) {
	usage := &dto.Usage{PromptTokens: 100, UsageSemantic: dto.BillingUsageSemanticOpenAI}
	usage.PromptTokensDetails.CachedTokens = 500

	billingUsage := claudeBillingUsageFromSemanticUsage(usage)

	require.NotNil(t, billingUsage)
	require.NotNil(t, billingUsage.ClaudeUsage)
	assert.Equal(t, 0, billingUsage.ClaudeUsage.InputTokens)
}
