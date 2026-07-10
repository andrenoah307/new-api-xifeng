package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestBuildTieredTokenParamsBridgesOpenAICacheWriteTokens(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     2000,
		CompletionTokens: 100,
		UsageSemantic:    "openai",
		PromptTokensDetails: dto.InputTokenDetails{
			CacheWriteTokens: 1500,
		},
	}

	params := BuildTieredTokenParams(usage, false, map[string]bool{"cc": true})

	require.Equal(t, 1500.0, params.CC)
}

func TestBuildTieredTokenParamsDoesNotBridgeAnthropicCacheWriteTokens(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:                2000,
		CompletionTokens:            100,
		UsageSemantic:               "anthropic",
		ClaudeCacheCreation5mTokens: 250,
		ClaudeCacheCreation1hTokens: 50,
		PromptTokensDetails:         dto.InputTokenDetails{CacheWriteTokens: 1500},
	}

	params := BuildTieredTokenParams(usage, true, map[string]bool{"cc": true})

	require.Equal(t, 250.0, params.CC)
	require.Equal(t, 50.0, params.CC1h)
}
