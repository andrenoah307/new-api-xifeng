package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func t3ConfigureModelNameRPMGroupsTest(t *testing.T, enabled bool, models map[string]t3ModelNameRPMRule, groups map[string]setting.GroupTotalRPMRule) {
	t.Helper()
	previous := setting.ModelNameRPMRateLimit2JSONString()
	t.Cleanup(func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) })
	payload, err := common.Marshal(struct {
		Enabled bool                                 `json:"enabled"`
		Models  map[string]t3ModelNameRPMRule        `json:"models"`
		Groups  map[string]setting.GroupTotalRPMRule `json:"groups"`
	}{Enabled: enabled, Models: models, Groups: groups})
	require.NoError(t, err)
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(string(payload)))
	require.NoError(t, i18n.Init())
}

func TestT3ModelNameRPMGroupTotalOnlyBuildsOneBucket(t *testing.T) {
	t3ConfigureModelNameRPMGroupsTest(t, true, map[string]t3ModelNameRPMRule{}, map[string]setting.GroupTotalRPMRule{
		"vip_2_cheap": {TotalRPM: 30},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		assert.Equal(t, []model_name_limiter.Bucket{{
			Key: model_name_limiter.GroupTotalKey("vip_2_cheap"), Limit: 30, Scope: "group_total",
		}}, buckets)
		return model_name_limiter.Result{Allowed: true}
	})

	require.True(t, enforceModelNameRPM(c, "unconfigured-model", "vip_2_cheap", "/v1/chat/completions"))
	assert.Equal(t, 1, *calls)
}

func TestT3ModelNameRPMAppendsGroupTotalAfterThreeModelBuckets(t *testing.T) {
	t3ConfigureModelNameRPMGroupsTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10, UserRPM: 2, GroupRPM: map[string]int{"vip_2_cheap": 3}},
	}, map[string]setting.GroupTotalRPMRule{"vip_2_cheap": {TotalRPM: 30}})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	common.SetContextKey(c, constant.ContextKeyUserId, 42)
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		assert.Equal(t, []model_name_limiter.Bucket{
			{Key: model_name_limiter.ModelKey("gpt-4o"), Limit: 10, Scope: "global"},
			{Key: model_name_limiter.GroupKey("gpt-4o", "vip_2_cheap"), Limit: 3, Scope: "group"},
			{Key: model_name_limiter.UserKey("gpt-4o", 42), Limit: 2, Scope: "user"},
			{Key: model_name_limiter.GroupTotalKey("vip_2_cheap"), Limit: 30, Scope: "group_total"},
		}, buckets)
		return model_name_limiter.Result{Allowed: true}
	})
	require.True(t, enforceModelNameRPM(c, "gpt-4o", "vip_2_cheap", "/v1/chat/completions"))
	assert.Equal(t, 1, *calls)
}

func TestT3ModelNameRPMEmptyModelSkipsGroupTotal(t *testing.T) {
	t3ConfigureModelNameRPMGroupsTest(t, true, map[string]t3ModelNameRPMRule{}, map[string]setting.GroupTotalRPMRule{
		"vip_2_cheap": {TotalRPM: 30},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
		t.Fatal("empty model must skip the RPM gate")
		return model_name_limiter.Result{}
	})
	require.True(t, enforceModelNameRPM(c, "", "vip_2_cheap", "/v1/chat/completions"))
	assert.Equal(t, 0, *calls)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyModelNameRPMChecked))
}

func TestT3ModelNameRPMGroupTotalRejectionIsNeutralAndRetryable(t *testing.T) {
	const group = "internal-vip-group"
	t3ConfigureModelNameRPMGroupsTest(t, true, map[string]t3ModelNameRPMRule{}, map[string]setting.GroupTotalRPMRule{
		group: {TotalRPM: 30},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	c.Set(string(constant.ContextKeyLanguage), i18n.LangEn)
	t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
		return model_name_limiter.Result{Allowed: false, Scope: "group_total", Limit: 30, Current: 30}
	})

	require.False(t, enforceModelNameRPM(c, "unconfigured-model", group, "/v1/chat/completions"))
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "60", recorder.Header().Get("Retry-After"))
	assert.Contains(t, recorder.Body.String(), string(types.ErrorCodeModelNameRateLimited))
	assert.Contains(t, recorder.Body.String(), "Too many requests for your current group. Please try again later.")
	assert.NotContains(t, recorder.Body.String(), group)
}
