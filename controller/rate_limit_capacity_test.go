package controller

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type controllerCapacityInspector struct {
	counts        []int
	err           error
	personalCount []int
	personalErr   error
}

func (s controllerCapacityInspector) Inspect(_ context.Context, keys []string) ([]int, error) {
	if len(keys) > 0 && strings.Contains(keys[0], ":user:") {
		return s.personalCount, s.personalErr
	}
	return s.counts, s.err
}

func callCapacityHandler(t *testing.T, scope string) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/api/rate_limit/capacity?scope="+scope, nil)
	c.Set("id", 7)
	c.Set("role", 1)
	c.Set("user_group", "default")
	GetRateLimitCapacity(c)
	require.Equal(t, 200, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	return data
}

func TestRateLimitCapacityHTTPThreeStatesAlwaysReturnOK(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousA1 := setting.ModelRequestRateLimitEnabled
	previousService := rateLimitCapacityService
	defer func() {
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
		setting.ModelRequestRateLimitEnabled = previousA1
		rateLimitCapacityService = previousService
	}()
	setting.ModelRequestRateLimitEnabled = false

	// No configuration: the site section is omitted.
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":false,"models":{}}`))
	rateLimitCapacityService = service.NewRateLimitCapacityService(controllerCapacityInspector{})
	payload := callCapacityHandler(t, "top")
	assert.NotContains(t, payload, "site")
	assert.Equal(t, false, payload["degraded"])

	// Configured with a genuine zero count: zero is retained, not treated as
	// absent.
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"group_rpm":{"default":5}}}}`))
	rateLimitCapacityService = service.NewRateLimitCapacityService(controllerCapacityInspector{counts: []int{0, 0}})
	payload = callCapacityHandler(t, "all")
	site, ok := payload["site"].(map[string]any)
	require.True(t, ok)
	global := site["global"].(map[string]any)
	items := global["items"].([]any)
	assert.Equal(t, float64(0), items[0].(map[string]any)["current"])
	groups := site["groups"].(map[string]any)
	groupSections := groups["groups"].([]any)
	require.Len(t, groupSections, 1)
	group := groupSections[0].(map[string]any)
	assert.Equal(t, "default", group["group"])
	groupItems := group["items"].([]any)
	require.Len(t, groupItems, 1)
	assert.Equal(t, "gpt-4o", groupItems[0].(map[string]any)["model"])
	assert.Equal(t, float64(1), group["total"])
	assert.Equal(t, float64(1), groups["total"])
	assert.Equal(t, float64(2), payload["total"])
	assert.Equal(t, false, payload["degraded"])

	// Backend failure: still 200, with null current and degraded=true.
	rateLimitCapacityService = service.NewRateLimitCapacityService(controllerCapacityInspector{err: errors.New("redis down")})
	payload = callCapacityHandler(t, "top")
	assert.Equal(t, true, payload["degraded"])
	site = payload["site"].(map[string]any)
	global = site["global"].(map[string]any)
	items = global["items"].([]any)
	assert.Nil(t, items[0].(map[string]any)["current"])
	groups = site["groups"].(map[string]any)
	groupSections = groups["groups"].([]any)
	require.Len(t, groupSections, 1)
	groupItems = groupSections[0].(map[string]any)["items"].([]any)
	assert.Nil(t, groupItems[0].(map[string]any)["current"])
}

func TestRateLimitCapacityHTTPPersonalItemsMatchSiteItemShape(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousService := rateLimitCapacityService
	defer func() {
		require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM))
		rateLimitCapacityService = previousService
	}()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":2}}}`))
	rateLimitCapacityService = service.NewRateLimitCapacityService(controllerCapacityInspector{
		counts:        []int{0},
		personalCount: []int{0},
	})

	payload := callCapacityHandler(t, "top")
	personal, ok := payload["personal"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ok", personal["status"])
	items := personal["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, "gpt-4o", item["model"])
	assert.Equal(t, "", item["group"])
	assert.Equal(t, float64(0), item["current"])
	assert.Equal(t, float64(2), item["limit"])
	assert.Equal(t, float64(0), item["utilization"])
	assert.Equal(t, true, item["available"])
	assert.Equal(t, false, item["unlimited"])
	assert.Equal(t, false, item["over_limit"])
}

func TestGetStatusRateLimitCapacityBooleanUsesMemorySnapshots(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousGroup := setting.ModelRequestRateLimitGroup2JSONString()
	previousA1 := setting.ModelRequestRateLimitEnabled
	defer func() {
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
		_ = setting.UpdateModelRequestRateLimitGroupByJSONString(previousGroup)
		setting.ModelRequestRateLimitEnabled = previousA1
	}()

	setting.ModelRequestRateLimitEnabled = false
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`))
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":false,"models":{}}`))
	assert.Equal(t, false, statusRateLimitCapacityEnabled(t))

	setting.ModelRequestRateLimitEnabled = true
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{"default":[1,1]}`))
	assert.Equal(t, false, statusRateLimitCapacityEnabled(t))

	setting.ModelRequestRateLimitEnabled = false
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`))
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":1}}}`))
	assert.Equal(t, true, statusRateLimitCapacityEnabled(t))

	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":false,"models":{}}`))
	assert.Equal(t, false, statusRateLimitCapacityEnabled(t))
}

func statusRateLimitCapacityEnabled(t *testing.T) bool {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/api/status", nil)
	GetStatus(c)
	require.Equal(t, 200, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	return data["rate_limit_capacity_enabled"].(bool)
}
