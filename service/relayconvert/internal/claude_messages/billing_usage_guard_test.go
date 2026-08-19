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

func TestFormatClaudeResponseInfoPrefersUpstreamBillingUsage(t *testing.T) {
	tests := []struct {
		name         string
		responseType string
		withUpstream bool
	}{
		{name: "message_start preserves upstream", responseType: "message_start", withUpstream: true},
		{name: "message_start reconstructs fallback", responseType: "message_start"},
		{name: "message_delta preserves upstream", responseType: "message_delta", withUpstream: true},
		{name: "message_delta reconstructs fallback", responseType: "message_delta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstreamBillingUsage := &dto.BillingUsage{
				Source:   dto.BillingUsageSourceOAIChat,
				Semantic: dto.BillingUsageSemanticOpenAI,
				OpenAIUsage: &dto.Usage{
					PromptTokens:     777,
					CompletionTokens: 33,
				},
			}
			usage := &dto.ClaudeUsage{
				InputTokens:  100,
				OutputTokens: 20,
			}
			if tt.withUpstream {
				usage.BillingUsage = upstreamBillingUsage
			}
			response := &dto.ClaudeResponse{Type: tt.responseType}
			if tt.responseType == "message_start" {
				response.Message = &dto.ClaudeMediaMessage{Usage: usage}
			} else {
				response.Usage = usage
			}
			claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}

			require.True(t, FormatClaudeResponseInfo(response, nil, claudeInfo))
			require.NotNil(t, claudeInfo.Usage.BillingUsage)
			if tt.withUpstream {
				assert.Equal(t, upstreamBillingUsage, claudeInfo.Usage.BillingUsage)
				return
			}
			assert.Equal(t, dto.BillingUsageSourceClaudeMessages, claudeInfo.Usage.BillingUsage.Source)
			require.NotNil(t, claudeInfo.Usage.BillingUsage.ClaudeUsage)
			assert.Equal(t, 100, claudeInfo.Usage.BillingUsage.ClaudeUsage.InputTokens)
			assert.Equal(t, 20, claudeInfo.Usage.BillingUsage.ClaudeUsage.OutputTokens)
		})
	}
}

func TestFormatClaudeResponseInfoKeepsEarlierUpstreamBillingUsage(t *testing.T) {
	upstreamBillingUsage := &dto.BillingUsage{
		Source:   dto.BillingUsageSourceOAIChat,
		Semantic: dto.BillingUsageSemanticOpenAI,
		OpenAIUsage: &dto.Usage{
			PromptTokens:     777,
			CompletionTokens: 33,
		},
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}

	require.True(t, FormatClaudeResponseInfo(&dto.ClaudeResponse{
		Type: "message_start",
		Message: &dto.ClaudeMediaMessage{Usage: &dto.ClaudeUsage{
			InputTokens:  100,
			OutputTokens: 1,
			BillingUsage: upstreamBillingUsage,
		}},
	}, nil, claudeInfo))
	require.True(t, FormatClaudeResponseInfo(&dto.ClaudeResponse{
		Type:  "message_delta",
		Usage: &dto.ClaudeUsage{OutputTokens: 20},
	}, nil, claudeInfo))

	assert.Equal(t, upstreamBillingUsage, claudeInfo.Usage.BillingUsage)
}

func TestFormatClaudeResponseInfoRefreshesLocalBillingUsageFallback(t *testing.T) {
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}

	require.True(t, FormatClaudeResponseInfo(&dto.ClaudeResponse{
		Type: "message_start",
		Message: &dto.ClaudeMediaMessage{Usage: &dto.ClaudeUsage{
			InputTokens:  100,
			OutputTokens: 1,
		}},
	}, nil, claudeInfo))
	require.True(t, FormatClaudeResponseInfo(&dto.ClaudeResponse{
		Type:  "message_delta",
		Usage: &dto.ClaudeUsage{OutputTokens: 20},
	}, nil, claudeInfo))

	require.NotNil(t, claudeInfo.Usage.BillingUsage)
	require.NotNil(t, claudeInfo.Usage.BillingUsage.ClaudeUsage)
	assert.Equal(t, 20, claudeInfo.Usage.BillingUsage.ClaudeUsage.OutputTokens)
}
