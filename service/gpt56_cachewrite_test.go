package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/require"
)

func newReconstructGpt56CacheWriteInfo(model string) *relaycommon.RelayInfo {
	return newGpt56CacheWriteCompatInfo(model)
}

func newReconstructGpt56CacheWriteUsage() *dto.Usage {
	return &dto.Usage{
		PromptTokens: 1000,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 300,
			ImageTokens:  40,
			AudioTokens:  10,
		},
	}
}

func TestReconstructGpt56CacheWrite_TriggerModels(t *testing.T) {
	models := []string{"gpt-5.6", "gpt-5.6-sol", "gpt-5.7", "gpt-5.10", "gpt-6", "gpt-6.1"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			v, ok, reclaim := ReconstructGpt56CacheWrite(newReconstructGpt56CacheWriteInfo(model), newReconstructGpt56CacheWriteUsage())

			require.True(t, ok)
			require.Equal(t, 650, v)
			require.False(t, reclaim)
		})
	}
}

func TestReconstructGpt56CacheWrite_SkipModels(t *testing.T) {
	models := []string{"gpt-5.5", "gpt-5.4", "gpt-4o", "claude-x"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			v, ok, reclaim := ReconstructGpt56CacheWrite(newReconstructGpt56CacheWriteInfo(model), newReconstructGpt56CacheWriteUsage())

			require.False(t, ok)
			require.Equal(t, 0, v)
			require.False(t, reclaim)
		})
	}
}

func TestReconstructGpt56CacheWrite_Guards(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*relaycommon.RelayInfo, *dto.Usage)
		wantOK      bool
		wantVal     int
		wantReclaim bool
	}{
		{
			name: "use price skips",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				info.PriceData.UsePrice = true
			},
		},
		{
			name: "anthropic usage semantic with claude final format reconstructs",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				info.RelayFormat = types.RelayFormatClaude
				usage.UsageSemantic = "anthropic"
			},
			wantOK:      true,
			wantVal:     950,
			wantReclaim: true,
		},
		{
			name: "claude final format defaults to anthropic semantic and reconstructs",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				info.RelayFormat = types.RelayFormatClaude
			},
			wantOK:      true,
			wantVal:     950,
			wantReclaim: true,
		},
		{
			name: "non openai final format skips",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				info.RelayFormat = types.RelayFormatGemini
			},
		},
		{
			name: "prompt cache write wins",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				usage.PromptTokensDetails.CacheWriteTokens = 200
			},
		},
		{
			name: "prompt cache creation wins",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				usage.PromptTokensDetails.CacheCreationTokens = 200
			},
		},
		{
			name: "input cache write wins",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				usage.InputTokensDetails = &dto.InputTokenDetails{CacheWriteTokens: 200}
			},
		},
		{
			name: "input cache creation wins",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				usage.InputTokensDetails = &dto.InputTokenDetails{CacheCreationTokens: 200}
			},
		},
		{
			name: "openai responses final format reconstructs",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				info.RelayFormat = types.RelayFormatOpenAIResponses
			},
			wantOK:  true,
			wantVal: 650,
		},
		{
			name: "openai responses compaction final format reconstructs",
			mutate: func(info *relaycommon.RelayInfo, usage *dto.Usage) {
				info.RelayFormat = types.RelayFormatOpenAIResponsesCompaction
			},
			wantOK:  true,
			wantVal: 650,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newReconstructGpt56CacheWriteInfo("gpt-5.6-terra")
			usage := newReconstructGpt56CacheWriteUsage()
			tt.mutate(info, usage)

			v, ok, reclaim := ReconstructGpt56CacheWrite(info, usage)

			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantVal, v)
			require.Equal(t, tt.wantReclaim, reclaim)
		})
	}
}

func TestReconstructGpt56CacheWrite_AnthropicSemanticUsesInputExcludedFormula(t *testing.T) {
	info := newReconstructGpt56CacheWriteInfo("gpt-5.6-sol")
	info.RelayFormat = types.RelayFormatClaude
	usage := &dto.Usage{
		PromptTokens:  1000,
		UsageSemantic: "anthropic",
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 300,
			ImageTokens:  40,
			AudioTokens:  10,
		},
	}

	v, ok, reclaim := ReconstructGpt56CacheWrite(info, usage)

	require.True(t, ok)
	require.Equal(t, 950, v)
	require.True(t, reclaim)
}

func TestReconstructGpt56CacheWrite_AnthropicRealCacheCreationWins(t *testing.T) {
	info := newReconstructGpt56CacheWriteInfo("gpt-5.6-sol")
	info.RelayFormat = types.RelayFormatClaude
	usage := &dto.Usage{
		PromptTokens:  1000,
		UsageSemantic: "anthropic",
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         300,
			CachedCreationTokens: 50,
		},
	}

	v, ok, reclaim := ReconstructGpt56CacheWrite(info, usage)

	require.False(t, ok)
	require.Equal(t, 0, v)
	require.False(t, reclaim)
}

func TestReconstructGpt56CacheWrite_InputTokensDetailsFold(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens: 100,
		InputTokens:  900,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 100,
			ImageTokens:  20,
			AudioTokens:  5,
		},
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens: 300,
			ImageTokens:  40,
			AudioTokens:  10,
		},
	}

	v, ok, reclaim := ReconstructGpt56CacheWrite(newReconstructGpt56CacheWriteInfo("gpt-5.6-terra"), usage)

	require.True(t, ok)
	require.Equal(t, 650, v)
	require.False(t, reclaim)
}

func TestReconstructGpt56CacheWrite_NormalizedDuplicateInputTokens(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens: 1000,
		InputTokens:  1000,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 300,
			ImageTokens:  40,
			AudioTokens:  10,
		},
	}

	v, ok, reclaim := ReconstructGpt56CacheWrite(newReconstructGpt56CacheWriteInfo("gpt-5.6-terra"), usage)

	require.True(t, ok)
	require.Equal(t, 650, v)
	require.False(t, reclaim)
}

func TestReconstructGpt56CacheWrite_UnnormalizedResponsesUsage(t *testing.T) {
	usage := &dto.Usage{
		InputTokens: 1000,
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens: 300,
			ImageTokens:  40,
			AudioTokens:  10,
		},
	}
	info := newReconstructGpt56CacheWriteInfo("gpt-5.6-terra")
	info.RelayFormat = types.RelayFormatOpenAIResponses

	v, ok, reclaim := ReconstructGpt56CacheWrite(info, usage)

	require.True(t, ok)
	require.Equal(t, 650, v)
	require.False(t, reclaim)
}

func TestReconstructGpt56CacheWrite_ReconstructedNonPositive(t *testing.T) {
	tests := []struct {
		name  string
		usage *dto.Usage
	}{
		{
			name: "cached equals prompt",
			usage: &dto.Usage{
				PromptTokens: 300,
				PromptTokensDetails: dto.InputTokenDetails{
					CachedTokens: 300,
				},
			},
		},
		{
			name: "image closes prompt",
			usage: &dto.Usage{
				PromptTokens: 350,
				PromptTokensDetails: dto.InputTokenDetails{
					CachedTokens: 300,
					ImageTokens:  50,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok, reclaim := ReconstructGpt56CacheWrite(newReconstructGpt56CacheWriteInfo("gpt-5.6-terra"), tt.usage)

			require.False(t, ok)
			require.Equal(t, 0, v)
			require.False(t, reclaim)
		})
	}
}

func TestReconstructGpt56CacheWrite_NilInputs(t *testing.T) {
	v, ok, reclaim := ReconstructGpt56CacheWrite(newReconstructGpt56CacheWriteInfo("gpt-5.6-terra"), nil)
	require.False(t, ok)
	require.Equal(t, 0, v)
	require.False(t, reclaim)

	v, ok, reclaim = ReconstructGpt56CacheWrite(nil, newReconstructGpt56CacheWriteUsage())
	require.False(t, ok)
	require.Equal(t, 0, v)
	require.False(t, reclaim)
}
