package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectiveBillingUsageRebuildsResponsesDetailsFromNestedSnapshot(t *testing.T) {
	nested := &dto.Usage{
		InputTokens:  100,
		OutputTokens: 7,
		TotalTokens:  107,
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens:         40,
			CachedCreationTokens: 10,
			CacheWriteTokens:     11,
			TextTokens:           30,
			AudioTokens:          4,
			ImageTokens:          5,
		},
	}
	original := &dto.Usage{
		BillingUsage: &dto.BillingUsage{
			Source:      dto.BillingUsageSourceOAIResponses,
			Semantic:    dto.BillingUsageSemanticOpenAI,
			OpenAIUsage: nested,
		},
	}

	got := effectiveBillingUsage(original)

	require.NotNil(t, got)
	assert.Equal(t, 100, got.PromptTokens)
	assert.Equal(t, 7, got.CompletionTokens)
	assert.Equal(t, 107, got.TotalTokens)
	assert.Equal(t, 40, got.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 10, got.PromptTokensDetails.CachedCreationTokens)
	assert.Equal(t, 11, got.PromptTokensDetails.CacheWriteTokens)
	assert.Equal(t, 30, got.PromptTokensDetails.TextTokens)
	assert.Equal(t, 4, got.PromptTokensDetails.AudioTokens)
	assert.Equal(t, 5, got.PromptTokensDetails.ImageTokens)
	assert.Equal(t, dto.BillingUsageSemanticOpenAI, got.UsageSemantic)
	assert.Equal(t, dto.BillingUsageSourceOAIResponses, got.UsageSource)
}

func TestEffectiveBillingUsageHonorsChatAndResponsesDetailPrecedence(t *testing.T) {
	for _, tt := range []struct {
		name   string
		source string
		want   int
	}{
		{name: "chat", source: dto.BillingUsageSourceOAIChat, want: 2},
		{name: "responses", source: dto.BillingUsageSourceOAIResponses, want: 9},
	} {
		t.Run(tt.name, func(t *testing.T) {
			usage := &dto.Usage{
				BillingUsage: &dto.BillingUsage{
					Source:   tt.source,
					Semantic: dto.BillingUsageSemanticOpenAI,
					OpenAIUsage: &dto.Usage{
						PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 2},
						InputTokensDetails:  &dto.InputTokenDetails{CachedTokens: 9},
					},
				},
			}

			got := effectiveBillingUsage(usage)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.PromptTokensDetails.CachedTokens)
		})
	}
}
