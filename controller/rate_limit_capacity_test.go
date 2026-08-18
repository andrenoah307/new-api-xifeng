package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

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
	calls         int
}

func (s *controllerCapacityInspector) Inspect(_ context.Context, keys []string) ([]int, error) {
	s.calls++
	if len(keys) > 0 && strings.Contains(keys[0], ":user:") {
		return s.personalCount, s.personalErr
	}
	return s.counts, s.err
}

type controllerCapacityServiceSpy struct {
	delegate *service.RateLimitCapacityService
	calls    int
}

func (s *controllerCapacityServiceSpy) Get(ctx context.Context, request service.CapacityRequest) service.RateLimitCapacityResponse {
	s.calls++
	return s.delegate.Get(ctx, request)
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
	require.Equal(t, true, payload["success"])
	require.Equal(t, "", payload["message"])
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	return data
}

func TestRateLimitCapacityHTTPThreeStatesAlwaysReturnOK(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousA1 := setting.ModelRequestRateLimitEnabled
	previousCardEnabled := setting.IsRateLimitCapacityCardEnabled()
	previousService := rateLimitCapacityService
	defer func() {
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
		setting.ModelRequestRateLimitEnabled = previousA1
		setting.SetRateLimitCapacityCardEnabled(previousCardEnabled)
		rateLimitCapacityService = previousService
	}()
	setting.SetRateLimitCapacityCardEnabled(true)
	setting.ModelRequestRateLimitEnabled = false

	// No configuration: the site section is omitted.
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":false,"models":{}}`))
	rateLimitCapacityService = service.NewRateLimitCapacityService(&controllerCapacityInspector{})
	payload := callCapacityHandler(t, "top")
	assert.NotContains(t, payload, "site")
	assert.Equal(t, false, payload["degraded"])

	// Configured with a genuine zero count: zero is retained, not treated as
	// absent.
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"group_rpm":{"default":5}}}}`))
	rateLimitCapacityService = service.NewRateLimitCapacityService(&controllerCapacityInspector{counts: []int{0, 0}})
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
	rateLimitCapacityService = service.NewRateLimitCapacityService(&controllerCapacityInspector{err: errors.New("redis down")})
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
	previousCardEnabled := setting.IsRateLimitCapacityCardEnabled()
	previousService := rateLimitCapacityService
	defer func() {
		require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM))
		setting.SetRateLimitCapacityCardEnabled(previousCardEnabled)
		rateLimitCapacityService = previousService
	}()
	setting.SetRateLimitCapacityCardEnabled(true)
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":2}}}`))
	rateLimitCapacityService = service.NewRateLimitCapacityService(&controllerCapacityInspector{
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
	previousCardEnabled := setting.IsRateLimitCapacityCardEnabled()
	defer func() {
		require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM))
		setting.SetRateLimitCapacityCardEnabled(previousCardEnabled)
	}()

	tests := []struct {
		name        string
		cardEnabled bool
		a2Enabled   bool
		hasModels   bool
		want        bool
	}{
		{name: "all false"},
		{name: "models only", hasModels: true},
		{name: "a2 only", a2Enabled: true},
		{name: "a2 and models", a2Enabled: true, hasModels: true},
		{name: "card only", cardEnabled: true},
		{name: "card and models", cardEnabled: true, hasModels: true},
		{name: "card and a2", cardEnabled: true, a2Enabled: true},
		{name: "all true", cardEnabled: true, a2Enabled: true, hasModels: true, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			models := map[string]setting.ModelNameRPMRule{}
			if test.hasModels {
				models["gpt-4o"] = setting.ModelNameRPMRule{GlobalRPM: 1}
			}
			setting.SetRateLimitCapacityCardEnabled(test.cardEnabled)
			config, err := common.Marshal(setting.ModelNameRPMConfig{Enabled: test.a2Enabled, Models: models})
			require.NoError(t, err)
			require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(string(config)))
			enabled, relatedFields := statusRateLimitCapacityState(t)
			assert.Equal(t, test.want, enabled)
			assert.Equal(t, []string{"rate_limit_capacity_enabled"}, relatedFields)
		})
	}
}

func statusRateLimitCapacityState(t *testing.T) (bool, []string) {
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
	relatedFields := make([]string, 0, 1)
	for key := range data {
		if strings.Contains(key, "rate_limit_capacity") {
			relatedFields = append(relatedFields, key)
		}
	}
	sort.Strings(relatedFields)
	enabled, ok := data["rate_limit_capacity_enabled"].(bool)
	require.True(t, ok)
	return enabled, relatedFields
}

func TestRateLimitCapacityDisabledReturnsCompleteEmptyResponseWithoutDependencies(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousCardEnabled := setting.IsRateLimitCapacityCardEnabled()
	previousService := rateLimitCapacityService
	previousGetUserGroup := getRateLimitCapacityUserGroup
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM))
		setting.SetRateLimitCapacityCardEnabled(previousCardEnabled)
		rateLimitCapacityService = previousService
		getRateLimitCapacityUserGroup = previousGetUserGroup
	})

	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10}}}`))
	inspector := &controllerCapacityInspector{counts: []int{0}}
	serviceSpy := &controllerCapacityServiceSpy{delegate: service.NewRateLimitCapacityService(inspector)}
	rateLimitCapacityService = serviceSpy
	userGroupCalls := 0
	getRateLimitCapacityUserGroup = func(int, bool) (string, error) {
		userGroupCalls++
		return "default", nil
	}

	setting.SetRateLimitCapacityCardEnabled(true)
	callCapacityHandlerWithoutGroup(t, "invalid")
	require.Equal(t, 1, serviceSpy.calls)
	require.Equal(t, 1, userGroupCalls)
	require.Equal(t, 1, inspector.calls)

	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":false,"models":{}}`))
	emptyInspector := &controllerCapacityInspector{}
	serviceSpy = &controllerCapacityServiceSpy{delegate: service.NewRateLimitCapacityService(emptyInspector)}
	rateLimitCapacityService = serviceSpy
	enabled := callCapacityHandlerWithoutGroup(t, "invalid")
	require.Equal(t, 1, serviceSpy.calls)
	require.Equal(t, 2, userGroupCalls)
	require.Equal(t, 0, emptyInspector.calls)

	serviceSpy.calls = 0
	userGroupCalls = 0
	setting.SetRateLimitCapacityCardEnabled(false)
	disabled := callCapacityHandlerWithoutGroup(t, "invalid")

	assert.Equal(t, 0, serviceSpy.calls)
	assert.Equal(t, 0, userGroupCalls)
	assert.Equal(t, 0, emptyInspector.calls)
	assert.Equal(t, sortedMapKeys(enabled), sortedMapKeys(disabled))
	assert.Equal(t, "top", disabled["scope"])
	observedAt, ok := disabled["observed_at"].(string)
	require.True(t, ok)
	_, err := time.Parse(time.RFC3339Nano, observedAt)
	require.NoError(t, err)
	assert.Equal(t, enabled["model_name_rpm_version"], disabled["model_name_rpm_version"])
	assert.Equal(t, enabled["group_rate_limit_version"], disabled["group_rate_limit_version"])
	assert.Equal(t, "global", disabled["backend_scope"])

	rateLimitCapacityService = nil
	allScope := callCapacityHandlerWithoutGroup(t, "all")
	assert.Nil(t, rateLimitCapacityService, "disabled handler must return before default service initialization")
	assert.Equal(t, "all", allScope["scope"])
}

func callCapacityHandlerWithoutGroup(t *testing.T, scope string) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/rate_limit/capacity?scope="+scope, nil)
	c.Set("id", 7)
	c.Set("role", common.RoleCommonUser)
	GetRateLimitCapacity(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, true, payload["success"])
	require.Equal(t, "", payload["message"])
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	return data
}

func sortedMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
