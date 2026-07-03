package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func mustPriceTestJSONString(t *testing.T, value any) string {
	t.Helper()

	jsonBytes, err := common.Marshal(value)
	require.NoError(t, err)
	return string(jsonBytes)
}

func withBillingSetting(t *testing.T, modes map[string]string, exprs map[string]string) {
	t.Helper()

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": mustPriceTestJSONString(t, modes),
		"billing_setting.billing_expr": mustPriceTestJSONString(t, exprs),
	}))
}

func withRatioSettings(t *testing.T, modelPrices map[string]float64, modelRatios map[string]float64, groupRatios map[string]float64) {
	t.Helper()

	oldModelPrice := ratio_setting.ModelPrice2JSONString()
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	oldSelfUseMode := operation_setting.SelfUseModeEnabled
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrice))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatio))
		operation_setting.SelfUseModeEnabled = oldSelfUseMode
	})

	operation_setting.SelfUseModeEnabled = false
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(mustPriceTestJSONString(t, modelPrices)))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(mustPriceTestJSONString(t, modelRatios)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(mustPriceTestJSONString(t, groupRatios)))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
}

func newPriceTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return ctx
}

func TestModelPriceHelperUnsetRatioUsesConservativePreConsume(t *testing.T) {
	withBillingSetting(t, map[string]string{}, map[string]string{})
	withRatioSettings(t, map[string]float64{}, map[string]float64{}, map[string]float64{
		"default":  1,
		"f2-group": 1.2,
	})

	ctx := newPriceTestContext()
	info := &relaycommon.RelayInfo{
		OriginModelName: "f2-unset-ratio-model-137",
		UserGroup:       "default",
		UsingGroup:      "f2-group",
	}
	info.UserSetting.AcceptUnsetRatioModel = true

	priceData, err := ModelPriceHelper(ctx, info, 3000, &types.TokenCountMeta{MaxTokens: 999999})
	require.NoError(t, err)

	expectedConservativeQuota := int(float64(common.PreConsumedQuota) * 1.2)
	oldSentinelQuota := int(float64(common.Max(3000, common.PreConsumedQuota)+common.ClampPreConsumeCompletionTokens(999999)) * 37.5 * 1.2)
	require.Equal(t, expectedConservativeQuota, priceData.QuotaToPreConsume)
	require.Equal(t, expectedConservativeQuota, priceData.QuotaToPreConsumeMin)
	require.NotEqual(t, oldSentinelQuota, priceData.QuotaToPreConsume)
	require.Equal(t, 37.5, priceData.ModelRatio)
	require.False(t, priceData.UsePrice)
}

func TestModelPriceHelperPriceModeMinEqualsFullPreConsume(t *testing.T) {
	withBillingSetting(t, map[string]string{}, map[string]string{})
	withRatioSettings(t, map[string]float64{"priced-min-model": 0.01}, map[string]float64{}, map[string]float64{"default": 1})

	ctx := newPriceTestContext()
	info := &relaycommon.RelayInfo{
		OriginModelName: "priced-min-model",
		UserGroup:       "default",
		UsingGroup:      "default",
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{MaxTokens: 1000})
	require.NoError(t, err)
	require.Equal(t, priceData.QuotaToPreConsume, priceData.QuotaToPreConsumeMin)
	require.Equal(t, int(0.01*common.QuotaPerUnit), priceData.QuotaToPreConsumeMin)
}

func TestModelPriceHelperTieredSetsPromptOnlyMinPreConsume(t *testing.T) {
	withBillingSetting(t,
		map[string]string{"tiered-min-model": billing_setting.BillingModeTieredExpr},
		map[string]string{"tiered-min-model": `tier("base", p * 2 + c * 10)`},
	)

	ctx := newPriceTestContext()
	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-min-model",
		UserGroup:       "default",
		UsingGroup:      "default",
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{MaxTokens: 1000})
	require.NoError(t, err)
	require.Equal(t, 6000, priceData.QuotaToPreConsume)
	require.Equal(t, 1000, priceData.QuotaToPreConsumeMin)
	require.Less(t, priceData.QuotaToPreConsumeMin, priceData.QuotaToPreConsume)
}

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}
