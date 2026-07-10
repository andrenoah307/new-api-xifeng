package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGpt56CacheWriteCompatInfo(model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
		OriginModelName: model,
		PriceData: types.PriceData{
			ModelRatio:         1,
			CompletionRatio:    2,
			CacheRatio:         0.1,
			CacheCreationRatio: 1.25,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
		StartTime: time.Now(),
	}
}

func newGpt56CacheWriteCompatContext(t *testing.T) *gin.Context {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	return ctx
}

func calculateGpt56CacheWriteCompatSummary(t *testing.T, model string, usage *dto.Usage) textQuotaSummary {
	t.Helper()

	return calculateTextQuotaSummary(newGpt56CacheWriteCompatContext(t), newGpt56CacheWriteCompatInfo(model), usage)
}

func TestIsGpt56OrHigherModel(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "gpt-5.6", want: true},
		{name: "gpt-5.6-sol", want: true},
		{name: "gpt-5.6[1m]", want: true},
		{name: "gpt-5.7", want: true},
		{name: "gpt-5.10", want: true},
		{name: "gpt-6", want: true},
		{name: "gpt-6.1", want: true},
		{name: "gpt-5.5", want: false},
		{name: "gpt-5.5-pro", want: false},
		{name: "gpt-5.4", want: false},
		{name: "gpt-5", want: false},
		{name: "gpt-4o", want: false},
		{name: "claude-x", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isGpt56OrHigherModel(tt.name))
		})
	}
}

func TestGpt56CacheWriteCompat_Reconstructs(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     1000,
		CompletionTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 300,
		},
	}

	summary := calculateGpt56CacheWriteCompatSummary(t, "gpt-5.6-terra", usage)
	withoutReconstruction := calculateGpt56CacheWriteCompatSummary(t, "gpt-5.5", usage)

	require.Equal(t, 700, summary.CacheCreationTokens)
	require.Equal(t, 1105, summary.Quota)
	require.Equal(t, 930, withoutReconstruction.Quota)
	require.Equal(t, 175, summary.Quota-withoutReconstruction.Quota)
}

func TestGpt56CacheWriteCompat_CachedZeroFullInput(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens: 1000,
	}

	summary := calculateGpt56CacheWriteCompatSummary(t, "gpt-5.6-terra", usage)

	require.Equal(t, 1000, summary.CacheCreationTokens)
}

func TestGpt56CacheWriteCompat_RealValueWins(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     1000,
		CompletionTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:     300,
			CacheWriteTokens: 200,
		},
	}

	summary := calculateGpt56CacheWriteCompatSummary(t, "gpt-5.6-terra", usage)

	require.Equal(t, 200, summary.CacheCreationTokens)
}

func TestGpt56CacheWriteCompat_SkipsAnthropic(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     1000,
		CompletionTokens: 100,
		UsageSemantic:    "anthropic",
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 300,
		},
	}

	summary := calculateGpt56CacheWriteCompatSummary(t, "gpt-5.6-terra", usage)

	require.True(t, summary.IsClaudeUsageSemantic)
	require.Equal(t, 0, summary.CacheCreationTokens)
}

func TestGpt56CacheWriteCompat_SkipsLowerModels(t *testing.T) {
	models := []string{"gpt-5.5", "gpt-5.4", "gpt-4o"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			usage := &dto.Usage{
				PromptTokens: 1000,
				PromptTokensDetails: dto.InputTokenDetails{
					CachedTokens: 300,
				},
			}

			summary := calculateGpt56CacheWriteCompatSummary(t, model, usage)

			require.Equal(t, 0, summary.CacheCreationTokens)
		})
	}
}

func TestGpt56CacheWriteCompat_SkipsNonOpenAIPath(t *testing.T) {
	relayInfo := newGpt56CacheWriteCompatInfo("gpt-5.6-terra")
	relayInfo.RelayFormat = types.RelayFormatGemini
	usage := &dto.Usage{
		PromptTokens: 1000,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 300,
		},
	}

	summary := calculateTextQuotaSummary(newGpt56CacheWriteCompatContext(t), relayInfo, usage)

	require.Equal(t, 0, summary.CacheCreationTokens)
}

func TestGpt56CacheWriteCompat_SkipsUsePrice(t *testing.T) {
	relayInfo := newGpt56CacheWriteCompatInfo("gpt-5.6-terra")
	relayInfo.PriceData.UsePrice = true
	usage := &dto.Usage{
		PromptTokens: 1000,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 300,
		},
	}

	summary := calculateTextQuotaSummary(newGpt56CacheWriteCompatContext(t), relayInfo, usage)

	require.Equal(t, 0, summary.CacheCreationTokens)
}

func TestGpt56CacheWriteCompat_LongContext(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens: 300000,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 100000,
		},
	}

	summary := calculateGpt56CacheWriteCompatSummary(t, "gpt-5.6-terra", usage)

	require.Equal(t, 200000, summary.CacheCreationTokens)
	require.Equal(t, 2.0, summary.ModelRatio)
	require.Equal(t, 520000, summary.Quota)
}
