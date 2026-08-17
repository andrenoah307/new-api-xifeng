package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 生产超收形状（坑点 #169）：上游把 OpenAI 含缓存的 prompt_tokens 当成 Claude 的
// input_tokens 发回，我们按 anthropic 净口径结算（不减缓存）→ 同一批 token 被输入价
// 与缓存价双收。模型倍率 10 / 缓存 0.1 / 补全 5 / 分组 0.5 对应生产日志 id=217399881。
func newInclusiveSemanticRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		FinalRequestRelayFormat: types.RelayFormatClaude,
		OriginModelName:         "kimi-k3",
		PriceData: types.PriceData{
			ModelRatio:         10,
			CompletionRatio:    5,
			CacheRatio:         0.1,
			CacheCreationRatio: 1.25,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 0.5,
			},
		},
		StartTime: time.Now(),
	}
}

func newInclusiveAnthropicUsage() *dto.Usage {
	usage := &dto.Usage{
		PromptTokens:     38503,
		CompletionTokens: 8,
		UsageSemantic:    dto.BillingUsageSemanticAnthropic,
		UsageSource:      dto.BillingUsageSourceClaudeMessages,
	}
	usage.PromptTokensDetails.CachedTokens = 38503
	return usage
}

func summarizeUsage(t *testing.T, relayInfo *relaycommon.RelayInfo, usage *dto.Usage) textQuotaSummary {
	t.Helper()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return calculateTextQuotaSummary(ctx, relayInfo, usage)
}

func TestNormalizeInclusivePromptForNetSemanticsFixesProductionOvercharge(t *testing.T) {
	relayInfo := newInclusiveSemanticRelayInfo()
	usage := newInclusiveAnthropicUsage()

	normalized, mismatch := normalizeInclusivePromptForNetSemantics(relayInfo, usage)

	require.NotNil(t, normalized)
	require.NotNil(t, mismatch)
	assert.Equal(t, 38503, mismatch["prompt_tokens"])
	assert.Equal(t, 38503, mismatch["cache_tokens"])
	assert.Equal(t, 0, mismatch["cache_creation_tokens"])
	assert.Equal(t, 0, mismatch["normalized_prompt_tokens"])
	// 原 usage 不得被就地改写（PostTextConsumeQuota 仍用 originUsage 做日志路径分类）
	assert.Equal(t, 38503, usage.PromptTokens)

	summary := summarizeUsage(t, relayInfo, normalized)

	// (0 + 38503*0.1 + 8*5) * (10*0.5) = 19451.5 → 半远零舍入 19452
	assert.Equal(t, 0, summary.PromptTokens)
	assert.Equal(t, 38503, summary.CacheTokens)
	assert.Equal(t, 19452, summary.Quota)
}

// 修复前的错误值：不归一化时 38503 个 token 同时按输入价与缓存价计费。
// 保留为回归对照，任何让该分支重新生效的改动都会打红这里。
func TestInclusiveAnthropicUsageWithoutNormalizationOvercharges(t *testing.T) {
	summary := summarizeUsage(t, newInclusiveSemanticRelayInfo(), newInclusiveAnthropicUsage())

	assert.Equal(t, 211967, summary.Quota)
}

// S1：缓存创建桶同型。prompt == cached + cache_creation 时，缓存创建 token 也会被双收。
func TestNormalizeInclusivePromptForNetSemanticsCoversCacheCreation(t *testing.T) {
	relayInfo := newInclusiveSemanticRelayInfo()
	usage := &dto.Usage{
		PromptTokens:     1500,
		CompletionTokens: 8,
		UsageSemantic:    dto.BillingUsageSemanticAnthropic,
		UsageSource:      dto.BillingUsageSourceClaudeMessages,
	}
	usage.PromptTokensDetails.CachedTokens = 1000
	usage.PromptTokensDetails.CachedCreationTokens = 500

	normalized, mismatch := normalizeInclusivePromptForNetSemantics(relayInfo, usage)

	require.NotNil(t, mismatch)
	assert.Equal(t, 500, mismatch["cache_creation_tokens"])

	summary := summarizeUsage(t, relayInfo, normalized)

	// (0 + 1000*0.1 + 500*1.25 + 8*5) * (10*0.5) = 765*5 = 3825
	assert.Equal(t, 0, summary.PromptTokens)
	assert.Equal(t, 3825, summary.Quota)
}

func TestNormalizeInclusivePromptForNetSemanticsSkipsSafeShapes(t *testing.T) {
	netAnthropicUsage := func() *dto.Usage {
		usage := &dto.Usage{
			PromptTokens:     1000,
			CompletionTokens: 8,
			UsageSemantic:    dto.BillingUsageSemanticAnthropic,
		}
		usage.PromptTokensDetails.CachedTokens = 38503
		return usage
	}
	inclusiveOpenAIUsage := func() *dto.Usage {
		usage := &dto.Usage{
			PromptTokens:     38503,
			CompletionTokens: 8,
			UsageSemantic:    dto.BillingUsageSemanticOpenAI,
		}
		usage.PromptTokensDetails.CachedTokens = 38503
		return usage
	}
	noCacheUsage := func() *dto.Usage {
		return &dto.Usage{PromptTokens: 0, CompletionTokens: 8, UsageSemantic: dto.BillingUsageSemanticAnthropic}
	}

	tests := []struct {
		name      string
		relayInfo func() *relaycommon.RelayInfo
		usage     func() *dto.Usage
	}{
		{
			name:      "真净口径 anthropic usage",
			relayInfo: newInclusiveSemanticRelayInfo,
			usage:     netAnthropicUsage,
		},
		{
			name:      "openai 口径本就会减缓存",
			relayInfo: newInclusiveSemanticRelayInfo,
			usage:     inclusiveOpenAIUsage,
		},
		{
			name:      "无缓存 token",
			relayInfo: newInclusiveSemanticRelayInfo,
			usage:     noCacheUsage,
		},
		{
			// OpenRouter Claude 在 calculateTextQuotaSummary 内已做同类扣减，
			// 这里再减一次会让 PromptTokens 变负。
			name: "OpenRouter Claude 不重复扣减",
			relayInfo: func() *relaycommon.RelayInfo {
				relayInfo := newInclusiveSemanticRelayInfo()
				relayInfo.ChannelMeta = &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter}
				return relayInfo
			},
			usage: newInclusiveAnthropicUsage,
		},
		{
			name:      "nil usage",
			relayInfo: newInclusiveSemanticRelayInfo,
			usage:     func() *dto.Usage { return nil },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := tt.usage()

			normalized, mismatch := normalizeInclusivePromptForNetSemantics(tt.relayInfo(), usage)

			assert.Nil(t, mismatch)
			assert.Same(t, usage, normalized)
		})
	}
}

// legacyClaudeDerived（无 UsageSource/UsageSemantic 但带 Claude 缓存创建拆分）同样按净口径
// 跳过减缓存，因此同型超收也必须被收口。
func TestNormalizeInclusivePromptForNetSemanticsCoversLegacyClaudeDerived(t *testing.T) {
	relayInfo := newInclusiveSemanticRelayInfo()
	relayInfo.FinalRequestRelayFormat = types.RelayFormatOpenAI
	usage := &dto.Usage{
		PromptTokens:                500,
		CompletionTokens:            8,
		ClaudeCacheCreation5mTokens: 300,
	}
	usage.PromptTokensDetails.CachedTokens = 200
	usage.PromptTokensDetails.CachedCreationTokens = 300

	normalized, mismatch := normalizeInclusivePromptForNetSemantics(relayInfo, usage)

	require.NotNil(t, mismatch)
	assert.Equal(t, 500, mismatch["prompt_tokens"])
	assert.Equal(t, 0, normalized.PromptTokens)
}

// S2：tiered_expr 的 p 与 Len 必须消费归一化后的 usage，否则表达式路径同样双计缓存。
func TestBuildTieredTokenParamsUsesNormalizedUsage(t *testing.T) {
	relayInfo := newInclusiveSemanticRelayInfo()
	normalized, mismatch := normalizeInclusivePromptForNetSemantics(relayInfo, newInclusiveAnthropicUsage())
	require.NotNil(t, mismatch)

	params := BuildTieredTokenParams(normalized, true, nil)

	assert.Equal(t, float64(0), params.P)
	assert.Equal(t, float64(38503), params.CR)
	assert.Equal(t, float64(38503), params.Len)
}

// 标记必须嵌在 admin_info 下（非管理员视图整块剥离），且不得写入任何用户可见字段。
func TestAttachUsageSemanticMismatchNestsUnderAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := newInclusiveSemanticRelayInfo()

	other := map[string]interface{}{"model_ratio": 10}
	_, mismatch := normalizeInclusivePromptForNetSemantics(relayInfo, newInclusiveAnthropicUsage())
	require.NotNil(t, mismatch)

	attachUsageSemanticMismatch(ctx, relayInfo, other, mismatch)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, mismatch, adminInfo["usage_semantic_mismatch"])
	assert.NotContains(t, other, "usage_semantic_mismatch")

	// 未触发归一化时不得写入任何标记
	clean := map[string]interface{}{}
	attachUsageSemanticMismatch(ctx, relayInfo, clean, nil)
	assert.Empty(t, clean)
}

func TestBuildUsageSemanticProbe(t *testing.T) {
	tests := []struct {
		name         string
		relayInfo    func() *relaycommon.RelayInfo
		usage        func() *dto.Usage
		want         map[string]interface{}
		wantMismatch bool
	}{
		{
			name: "anthropic net semantic sample",
			relayInfo: func() *relaycommon.RelayInfo {
				info := newInclusiveSemanticRelayInfo()
				info.SetEstimatePromptTokens(51000)
				return info
			},
			usage: func() *dto.Usage {
				usage := &dto.Usage{
					PromptTokens:  1000,
					UsageSemantic: dto.BillingUsageSemanticAnthropic,
				}
				usage.PromptTokensDetails.CachedTokens = 50000
				return usage
			},
			want: map[string]interface{}{
				"l": 51000, "p": 1000, "cr": 50000, "cc": 0, "sem": dto.BillingUsageSemanticAnthropic,
			},
		},
		{
			name: "inclusive prompt keeps original probe value",
			relayInfo: func() *relaycommon.RelayInfo {
				info := newInclusiveSemanticRelayInfo()
				info.SetEstimatePromptTokens(1500)
				return info
			},
			usage: func() *dto.Usage {
				usage := &dto.Usage{
					PromptTokens:  1500,
					UsageSemantic: dto.BillingUsageSemanticAnthropic,
				}
				usage.PromptTokensDetails.CachedTokens = 1000
				usage.PromptTokensDetails.CachedCreationTokens = 500
				return usage
			},
			want: map[string]interface{}{
				"l": 1500, "p": 1500, "cr": 1000, "cc": 500, "sem": dto.BillingUsageSemanticAnthropic,
			},
			wantMismatch: true,
		},
		{
			name: "zero cache sum",
			relayInfo: func() *relaycommon.RelayInfo {
				info := newInclusiveSemanticRelayInfo()
				info.SetEstimatePromptTokens(100)
				return info
			},
			usage: func() *dto.Usage {
				return &dto.Usage{PromptTokens: 100, UsageSemantic: dto.BillingUsageSemanticAnthropic}
			},
		},
		{
			name:      "estimate prompt tokens disabled",
			relayInfo: newInclusiveSemanticRelayInfo,
			usage: func() *dto.Usage {
				usage := &dto.Usage{PromptTokens: 1000, UsageSemantic: dto.BillingUsageSemanticAnthropic}
				usage.PromptTokensDetails.CachedTokens = 500
				return usage
			},
		},
		{
			name: "openrouter",
			relayInfo: func() *relaycommon.RelayInfo {
				info := newInclusiveSemanticRelayInfo()
				info.ChannelMeta = &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter}
				info.SetEstimatePromptTokens(1000)
				return info
			},
			usage: func() *dto.Usage {
				usage := &dto.Usage{PromptTokens: 1000, UsageSemantic: dto.BillingUsageSemanticAnthropic}
				usage.PromptTokensDetails.CachedTokens = 500
				return usage
			},
		},
		{
			name: "openai non legacy claude derived",
			relayInfo: func() *relaycommon.RelayInfo {
				info := newInclusiveSemanticRelayInfo()
				info.SetEstimatePromptTokens(1000)
				return info
			},
			usage: func() *dto.Usage {
				usage := &dto.Usage{
					PromptTokens:  1000,
					UsageSemantic: dto.BillingUsageSemanticOpenAI,
					UsageSource:   dto.BillingUsageSourceOAIChat,
				}
				usage.PromptTokensDetails.CachedTokens = 500
				return usage
			},
		},
		{
			name:      "nil usage",
			relayInfo: newInclusiveSemanticRelayInfo,
			usage:     func() *dto.Usage { return nil },
		},
		{
			name: "nil relay info",
			relayInfo: func() *relaycommon.RelayInfo {
				return nil
			},
			usage: func() *dto.Usage {
				usage := &dto.Usage{PromptTokens: 1000, UsageSemantic: dto.BillingUsageSemanticAnthropic}
				usage.PromptTokensDetails.CachedTokens = 500
				return usage
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relayInfo := tt.relayInfo()
			usage := tt.usage()
			probe := buildUsageSemanticProbe(relayInfo, usage)
			if tt.want == nil {
				assert.Nil(t, probe)
			} else {
				require.Equal(t, tt.want, probe)
			}

			_, mismatch := normalizeInclusivePromptForNetSemantics(relayInfo, usage)
			if tt.wantMismatch {
				require.NotNil(t, mismatch)
				assert.Equal(t, tt.want["p"], mismatch["prompt_tokens"])
				assert.Equal(t, tt.want["p"], usage.PromptTokens)
			} else {
				assert.Nil(t, mismatch)
			}
		})
	}
}

func TestAttachUsageSemanticProbePreservesAdminInfo(t *testing.T) {
	probe := map[string]interface{}{"l": 51000, "p": 1000, "cr": 50000, "cc": 0, "sem": "anthropic"}
	other := map[string]interface{}{
		"admin_info": map[string]interface{}{"admin_username": "root"},
	}

	attachUsageSemanticProbe(other, probe)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "root", adminInfo["admin_username"])
	assert.Equal(t, probe, adminInfo["usage_semantic_probe"])

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := newInclusiveSemanticRelayInfo()
	usage := &dto.Usage{PromptTokens: 1500, UsageSemantic: dto.BillingUsageSemanticAnthropic}
	usage.PromptTokensDetails.CachedTokens = 1000
	usage.PromptTokensDetails.CachedCreationTokens = 500
	_, mismatch := normalizeInclusivePromptForNetSemantics(relayInfo, usage)
	require.NotNil(t, mismatch)
	attachUsageSemanticMismatch(ctx, relayInfo, other, mismatch)
	assert.Equal(t, mismatch, adminInfo["usage_semantic_mismatch"])
	assert.Equal(t, probe, adminInfo["usage_semantic_probe"])

	created := map[string]interface{}{}
	attachUsageSemanticProbe(created, probe)
	createdAdminInfo, ok := created["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, probe, createdAdminInfo["usage_semantic_probe"])

	clean := map[string]interface{}{}
	attachUsageSemanticProbe(clean, nil)
	assert.NotContains(t, clean, "admin_info")

	emptyProbe := map[string]interface{}{}
	attachUsageSemanticProbe(clean, emptyProbe)
	assert.NotContains(t, clean, "admin_info")

	var nilOther map[string]interface{}
	attachUsageSemanticProbe(nilOther, probe)
}
