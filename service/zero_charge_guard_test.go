package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZeroChargeSettlementBypassesNestedBillingUsageAndTiered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		FinalRequestRelayFormat: types.RelayFormatClaude,
		OriginModelName:         "claude-test",
		StartTime:               time.Now(),
		PriceData: types.PriceData{
			ModelRatio:         5,
			CompletionRatio:    5,
			CacheCreationRatio: 1.25,
			GroupRatioInfo:     types.GroupRatioInfo{GroupRatio: 1.6},
		},
		ZeroChargeGuardTriggered: true,
		ZeroChargeGuardSnapshot: &relaycommon.ZeroChargeGuardSnapshot{
			Reason: relaycommon.ZeroChargeReasonEmptyOutput,
		},
		TieredBillingSnapshot: &structTieredSnapshotForTest,
	}
	usage := &dto.Usage{
		PromptTokens:     1601,
		CompletionTokens: 0,
		BillingUsage: dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{
			InputTokens:              1601,
			CacheCreationInputTokens: 553250,
			OutputTokens:             0,
		}),
	}

	// The unguarded path reproduces the production shape and proves the
	// regression would have charged a non-zero amount.
	unguarded := *info
	unguarded.ZeroChargeGuardTriggered = false
	unguarded.ZeroChargeGuardSnapshot = nil
	unguardedSummary := calculateTextQuotaSummary(ctx, &unguarded, effectiveBillingUsage(usage))
	assert.Equal(t, 5545308, unguardedSummary.Quota)

	// The same expression would produce a non-zero tiered result when invoked
	// directly; the guarded settlement predicate must prevent that branch from
	// running at all.
	tieredInfo := makeRelayInfo(flatExpr, 1, 100, 0)
	ok, tieredQuota, _ := TryTieredSettle(tieredInfo, billingexpr.TokenParams{P: 100})
	require.True(t, ok)
	assert.NotZero(t, tieredQuota)

	settlementUsage := usageForTextSettlement(info, usage)
	require.NotNil(t, settlementUsage)
	assert.Equal(t, dto.Usage{}, *settlementUsage)
	assert.False(t, shouldApplyTieredSettlement(info, usage))
	guardedSummary := calculateTextQuotaSummary(ctx, info, settlementUsage)
	assert.Zero(t, guardedSummary.Quota)
}

func TestZeroChargeGuardDoesNotUseContextLocalCountAsBillingUsageVeto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("local_count_tokens", true)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-test",
		StartTime:       time.Now(),
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
		},
	}
	usage := &dto.Usage{
		PromptTokens:     10,
		CompletionTokens: 4,
		BillingUsage:     dto.NewEstimatedGeminiChatBillingUsage(&dto.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14}),
	}

	settlementUsage := usageForTextSettlement(info, usage)
	require.NotNil(t, settlementUsage)
	assert.Equal(t, usage.PromptTokens, settlementUsage.PromptTokens)
	assert.Equal(t, usage.CompletionTokens, settlementUsage.CompletionTokens)
	summary := calculateTextQuotaSummary(ctx, info, effectiveBillingUsage(settlementUsage))
	assert.NotZero(t, summary.Quota)
}

func TestAttachZeroChargeGuardAdminMarker(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ZeroChargeGuardTriggered: true,
		ZeroChargeGuardSnapshot: &relaycommon.ZeroChargeGuardSnapshot{
			Reason:              relaycommon.ZeroChargeReasonEmptyOutput,
			PromptTokens:        1601,
			CompletionTokens:    553250,
			CacheReadTokens:     7,
			CacheCreationTokens: 11,
			PreConsumedQuota:    13,
		},
	}
	other := map[string]interface{}{
		"admin_info": map[string]interface{}{"existing": true},
	}

	attachZeroChargeGuard(nil, info, other)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, adminInfo["existing"])
	marker, ok := adminInfo["zero_charge_guard"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "empty_output", marker["reason"])
	assert.Equal(t, 1601, marker["prompt_tokens"])
	assert.Equal(t, 553250, marker["completion_tokens"])
	assert.Equal(t, 7, marker["cache_read_tokens"])
	assert.Equal(t, 11, marker["cache_creation_tokens"])
	assert.Equal(t, 13, marker["pre_consumed_quota"])
}

// A valid tiered snapshot is supplied by the production billing expression
// package in integration tests; this zero-value fixture keeps this unit test
// independent of settings while still exercising the guard predicate.
var structTieredSnapshotForTest = billingSnapshotForTest()

func billingSnapshotForTest() billingexpr.BillingSnapshot {
	return billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: "P + 1"}
}
