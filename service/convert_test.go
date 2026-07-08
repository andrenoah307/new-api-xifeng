package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestBuildClaudeUsageFromOpenAIUsage_SubtractsCacheFromInput(t *testing.T) {
	oaiUsage := &dto.Usage{PromptTokens: 236745, CompletionTokens: 63}
	oaiUsage.PromptTokensDetails.CachedTokens = 236416

	usage := buildClaudeUsageFromOpenAIUsage(oaiUsage)

	require.NotNil(t, usage)
	require.Equal(t, 329, usage.InputTokens)
	require.Equal(t, 236416, usage.CacheReadInputTokens)
	require.Equal(t, 63, usage.OutputTokens)
}

func TestBuildClaudeUsageFromOpenAIUsage_SubtractsCacheCreation(t *testing.T) {
	oaiUsage := &dto.Usage{PromptTokens: 1000}
	oaiUsage.PromptTokensDetails.CachedTokens = 200
	oaiUsage.PromptTokensDetails.CachedCreationTokens = 300

	usage := buildClaudeUsageFromOpenAIUsage(oaiUsage)

	require.NotNil(t, usage)
	require.Equal(t, 500, usage.InputTokens)
	require.Equal(t, 200, usage.CacheReadInputTokens)
	require.Equal(t, 300, usage.CacheCreationInputTokens)
}

func TestBuildClaudeUsageFromOpenAIUsage_CacheCreationSplitTakesMax(t *testing.T) {
	oaiUsage := &dto.Usage{PromptTokens: 1000}
	oaiUsage.PromptTokensDetails.CachedTokens = 100
	oaiUsage.PromptTokensDetails.CachedCreationTokens = 0
	oaiUsage.ClaudeCacheCreation5mTokens = 120
	oaiUsage.ClaudeCacheCreation1hTokens = 80

	usage := buildClaudeUsageFromOpenAIUsage(oaiUsage)

	require.NotNil(t, usage)
	require.Equal(t, 700, usage.InputTokens)
}

func TestBuildClaudeUsageFromOpenAIUsage_FloorsAtZero(t *testing.T) {
	oaiUsage := &dto.Usage{PromptTokens: 100}
	oaiUsage.PromptTokensDetails.CachedTokens = 500

	usage := buildClaudeUsageFromOpenAIUsage(oaiUsage)

	require.NotNil(t, usage)
	require.Equal(t, 0, usage.InputTokens)
}

func TestBuildClaudeUsageFromOpenAIUsage_NoCacheUnchanged(t *testing.T) {
	oaiUsage := &dto.Usage{PromptTokens: 1000}

	usage := buildClaudeUsageFromOpenAIUsage(oaiUsage)

	require.NotNil(t, usage)
	require.Equal(t, 1000, usage.InputTokens)
}

func TestBuildClaudeUsageFromOpenAIUsage_NilReturnsNil(t *testing.T) {
	usage := buildClaudeUsageFromOpenAIUsage(nil)

	require.Nil(t, usage)
}
