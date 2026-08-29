package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeInputTokenDetailsResponsesSourceOverwritesAllFields(t *testing.T) {
	sourceDetails := &InputTokenDetails{
		CachedTokens:         11,
		CachedCreationTokens: 12,
		CacheWriteTokens:     13,
		TextTokens:           14,
		AudioTokens:          15,
		ImageTokens:          16,
	}
	existingInputDetails := &InputTokenDetails{CachedTokens: 99}
	usage := &Usage{
		PromptCacheHitTokens: 77,
		PromptTokensDetails: InputTokenDetails{
			CachedTokens:         1,
			CachedCreationTokens: 2,
			CacheWriteTokens:     3,
			TextTokens:           4,
			AudioTokens:          5,
			ImageTokens:          6,
		},
		InputTokensDetails: existingInputDetails,
	}

	usage.MergeInputTokenDetails(BillingUsageSourceOAIResponses, sourceDetails)

	assert.Equal(t, InputTokenDetails{
		CachedTokens:         11,
		CachedCreationTokens: 12,
		CacheWriteTokens:     13,
		TextTokens:           14,
		AudioTokens:          15,
		ImageTokens:          16,
	}, usage.PromptTokensDetails)
	assert.Equal(t, 77, usage.PromptCacheHitTokens)
	assert.Same(t, existingInputDetails, usage.InputTokensDetails)
}

func TestMergeInputTokenDetailsResponsesTreatsNonPositiveSourceAsZero(t *testing.T) {
	usage := &Usage{
		PromptTokensDetails: InputTokenDetails{
			CachedTokens:         10,
			CachedCreationTokens: 10,
			CacheWriteTokens:     10,
			TextTokens:           10,
			AudioTokens:          10,
			ImageTokens:          10,
		},
		InputTokensDetails: &InputTokenDetails{
			CachedTokens:         0,
			CachedCreationTokens: -1,
			CacheWriteTokens:     0,
			TextTokens:           -2,
			AudioTokens:          0,
			ImageTokens:          -3,
		},
	}

	usage.MergeInputTokenDetails(BillingUsageSourceOAIResponses, usage.InputTokensDetails)

	assert.Equal(t, InputTokenDetails{}, usage.PromptTokensDetails)
}

func TestMergeInputTokenDetailsChatSourceIsNoOp(t *testing.T) {
	usage := &Usage{
		PromptCacheHitTokens: 77,
		PromptTokensDetails: InputTokenDetails{
			CachedTokens: 1,
			TextTokens:   2,
		},
		InputTokensDetails: &InputTokenDetails{
			CachedTokens: 11,
			TextTokens:   12,
		},
	}
	before := usage.PromptTokensDetails

	usage.MergeInputTokenDetails(BillingUsageSourceOAIChat, usage.InputTokensDetails)

	assert.Equal(t, before, usage.PromptTokensDetails)
	assert.Equal(t, 77, usage.PromptCacheHitTokens)
}

func TestMergeInputTokenDetailsUnknownSourceFillsOnlyMissingFields(t *testing.T) {
	usage := &Usage{
		PromptTokensDetails: InputTokenDetails{
			CachedTokens: 1,
			TextTokens:   2,
			AudioTokens:  -3,
			ImageTokens:  0,
		},
		InputTokensDetails: &InputTokenDetails{
			CachedTokens:         11,
			CachedCreationTokens: 12,
			CacheWriteTokens:     -13,
			TextTokens:           14,
			AudioTokens:          15,
			ImageTokens:          16,
		},
	}

	usage.MergeInputTokenDetails("", usage.InputTokensDetails)

	assert.Equal(t, 1, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 12, usage.PromptTokensDetails.CachedCreationTokens)
	assert.Zero(t, usage.PromptTokensDetails.CacheWriteTokens)
	assert.Equal(t, 2, usage.PromptTokensDetails.TextTokens)
	assert.Equal(t, -3, usage.PromptTokensDetails.AudioTokens)
	assert.Equal(t, 16, usage.PromptTokensDetails.ImageTokens)

	// A second pass must not add or replace any already-canonical values.
	before := usage.PromptTokensDetails
	usage.MergeInputTokenDetails("unknown", usage.InputTokensDetails)
	assert.Equal(t, before, usage.PromptTokensDetails)
}

func TestNormalizePromptTokenDetailsDelegatesAndHandlesNilDetails(t *testing.T) {
	usage := &Usage{
		PromptTokensDetails: InputTokenDetails{CachedTokens: 9},
		InputTokensDetails:  &InputTokenDetails{CachedTokens: 11, TextTokens: 12},
	}

	usage.NormalizePromptTokenDetails(BillingUsageSourceOAIResponses)
	usage.NormalizePromptTokenDetails(BillingUsageSourceOAIResponses)

	assert.Equal(t, InputTokenDetails{CachedTokens: 11, TextTokens: 12}, usage.PromptTokensDetails)
	assert.NotPanics(t, func() {
		usage.NormalizePromptTokenDetails(BillingUsageSourceOAIResponses)
	})

	var nilUsage *Usage
	require.NotPanics(t, func() {
		nilUsage.NormalizePromptTokenDetails(BillingUsageSourceOAIResponses)
	})
	require.NotPanics(t, func() {
		nilUsage.MergeInputTokenDetails(BillingUsageSourceOAIResponses, nil)
	})
	require.NotPanics(t, func() {
		usage.MergeInputTokenDetails(BillingUsageSourceOAIResponses, nil)
	})
}
