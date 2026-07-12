package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGPT56LongContextInfo(model string) *relaycommon.RelayInfo {
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

func newGPT56LongContextUsage(inputTokens int) *dto.Usage {
	return &dto.Usage{
		PromptTokens:        inputTokens,
		CompletionTokens:    1000,
		PromptTokensDetails: dto.InputTokenDetails{},
	}
}

func calculateGPT56LongContextSummary(t *testing.T, model string, inputTokens int) textQuotaSummary {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	return calculateTextQuotaSummary(ctx, newGPT56LongContextInfo(model), newGPT56LongContextUsage(inputTokens))
}

func marshalBillingSettingJSON(t *testing.T, value any) string {
	t.Helper()

	jsonBytes, err := common.Marshal(value)
	require.NoError(t, err)
	return string(jsonBytes)
}

func setBillingModesForTextQuotaTest(t *testing.T, modes map[string]string) {
	t.Helper()

	oldModes := billing_setting.GetBillingModeCopy()
	oldExprs := billing_setting.GetBillingExprCopy()
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting." + billing_setting.BillingModeField: marshalBillingSettingJSON(t, modes),
		"billing_setting." + billing_setting.BillingExprField: marshalBillingSettingJSON(t, oldExprs),
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"billing_setting." + billing_setting.BillingModeField: marshalBillingSettingJSON(t, oldModes),
			"billing_setting." + billing_setting.BillingExprField: marshalBillingSettingJSON(t, oldExprs),
		}))
	})
}

// 长上下文(>272K)分档仅作用于 ratio 路径的 ModelRatio/CompletionRatio 与可观测标记；
// 精确 quota 依赖缓存计费口径（rc21 原生 48068ce92），此处只锁定分档倍率与标记契约（坑点 #146/#148）。
func TestGPT56LongContextRatioBilling(t *testing.T) {
	summary := calculateGPT56LongContextSummary(t, "gpt-5.6-terra", 300000)

	require.Equal(t, 2.0, summary.ModelRatio)
	require.Equal(t, 1.5, summary.CompletionRatio)
}

func TestGPT54LongContextRatioBilling(t *testing.T) {
	summary := calculateGPT56LongContextSummary(t, "gpt-5.4-nano", 300000)

	require.Equal(t, 2.0, summary.ModelRatio)
}

func TestGPT56LongContextThresholdBoundary(t *testing.T) {
	atThreshold := calculateGPT56LongContextSummary(t, "gpt-5.6-terra", 272000)
	overThreshold := calculateGPT56LongContextSummary(t, "gpt-5.6-terra", 272001)

	require.Equal(t, 1.0, atThreshold.ModelRatio)
	require.Equal(t, 2.0, overThreshold.ModelRatio)
}

func TestGPT56LongContextShortPromptDoesNotApply(t *testing.T) {
	summary := calculateGPT56LongContextSummary(t, "gpt-5.6-terra", 200000)

	require.Equal(t, 1.0, summary.ModelRatio)
}

func TestGPT56LongContextNonTargetModelsDoNotApply(t *testing.T) {
	tests := []string{"gpt-4o", "gpt-5.7-x"}

	for _, model := range tests {
		t.Run(model, func(t *testing.T) {
			summary := calculateGPT56LongContextSummary(t, model, 300000)

			require.Equal(t, 1.0, summary.ModelRatio)
		})
	}
}

func TestGPT55TieredExprSkipsLongContextRatioBilling(t *testing.T) {
	setBillingModesForTextQuotaTest(t, map[string]string{"gpt-5.5": billing_setting.BillingModeTieredExpr})

	summary := calculateGPT56LongContextSummary(t, "gpt-5.5", 300000)

	require.Equal(t, 1.0, summary.ModelRatio)
}

// TestGPT56LongContextTierMarker 锁定「长上下文计费触发」的日志友好标记：
// 仅 ratio 路径（gpt-5.4/5.6）输入 >272K 实际触发分档时置 LongContextTierApplied=true，
// 供 Content 兜底文案与双前端 other["long_context_tier"] 结构化提示消费；边界/短上下文/
// 非目标模型/tiered_expr 均不置位（与 ModelRatio 是否翻倍严格对齐）。
func TestGPT56LongContextTierMarker(t *testing.T) {
	require.True(t, calculateGPT56LongContextSummary(t, "gpt-5.6-terra", 300000).LongContextTierApplied)
	require.True(t, calculateGPT56LongContextSummary(t, "gpt-5.4-nano", 300000).LongContextTierApplied)
	require.False(t, calculateGPT56LongContextSummary(t, "gpt-5.6-terra", 272000).LongContextTierApplied)
	require.False(t, calculateGPT56LongContextSummary(t, "gpt-5.6-terra", 200000).LongContextTierApplied)
	require.False(t, calculateGPT56LongContextSummary(t, "gpt-4o", 300000).LongContextTierApplied)
}

func TestGPT55TieredExprLongContextTierMarkerNotApplied(t *testing.T) {
	setBillingModesForTextQuotaTest(t, map[string]string{"gpt-5.5": billing_setting.BillingModeTieredExpr})

	require.False(t, calculateGPT56LongContextSummary(t, "gpt-5.5", 300000).LongContextTierApplied)
}
