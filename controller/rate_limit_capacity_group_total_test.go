package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitCapacityHTTPIncludesGroupTotalsSection(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousUsable := setting.UserUsableGroups2JSONString()
	previousCard := setting.IsRateLimitCapacityCardEnabled()
	previousService := rateLimitCapacityService
	t.Cleanup(func() {
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
		_ = ratio_setting.UpdateGroupRatioByJSONString(previousRatios)
		_ = setting.UpdateUserUsableGroupsByJSONString(previousUsable)
		setting.SetRateLimitCapacityCardEnabled(previousCard)
		rateLimitCapacityService = previousService
	})
	setting.SetRateLimitCapacityCardEnabled(true)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"alpha":1,"zeta":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"alpha":"Alpha","zeta":"Zeta"}`))
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{},"groups":{"zeta":{"total_rpm":7},"alpha":{"total_rpm":3}}}`))
	rateLimitCapacityService = service.NewRateLimitCapacityService(&controllerCapacityInspector{counts: []int{5, 1}})

	payload := callCapacityHandler(t, "all")
	site, ok := payload["site"].(map[string]any)
	require.True(t, ok)
	groupTotals, ok := site["group_totals"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(2), groupTotals["total"])
	items, ok := groupTotals["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)
	assert.Equal(t, "alpha", items[0].(map[string]any)["group"])
	assert.Equal(t, "", items[0].(map[string]any)["model"])
	assert.Equal(t, float64(5), items[0].(map[string]any)["current"])
	assert.Equal(t, float64(1), items[1].(map[string]any)["current"])
	assert.Equal(t, float64(2), payload["total"])
	_, err := common.Marshal(payload)
	require.NoError(t, err)
}
