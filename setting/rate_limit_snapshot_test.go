package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListGroupRateLimitsReturnsDeepCopyAndVersion(t *testing.T) {
	previous := ModelRequestRateLimitGroup2JSONString()
	defer func() { require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(previous)) }()

	before := ModelRequestRateLimitConfigVersion()
	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(`{"free":[3,2]}`))
	got := ListGroupRateLimits()
	got["free"] = [2]int{99, 99}
	assert.Equal(t, [2]int{3, 2}, ListGroupRateLimits()["free"])
	limits, version := ListGroupRateLimitsWithVersion()
	assert.Equal(t, [2]int{3, 2}, limits["free"])
	assert.Equal(t, version, ModelRequestRateLimitConfigVersion())
	assert.Greater(t, ModelRequestRateLimitConfigVersion(), before)
}

func TestListModelNameRPMRulesReturnsDeepCopyAndVersion(t *testing.T) {
	previous := ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(previous)) }()

	before := ModelNameRPMConfigVersion()
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":2,"group_rpm":{"free":3}}}}`))
	got := ListModelNameRPMRules()
	got.Models["gpt-4o"].GroupRPM["free"] = 99
	got.Models["gpt-4o"] = ModelNameRPMRule{GlobalRPM: 1}
	latest := ListModelNameRPMRules()
	assert.Equal(t, 10, latest.Models["gpt-4o"].GlobalRPM)
	assert.Equal(t, 2, latest.Models["gpt-4o"].UserRPM)
	assert.Equal(t, 3, latest.Models["gpt-4o"].GroupRPM["free"])
	_, version := ListModelNameRPMRulesWithVersion()
	assert.Equal(t, version, ModelNameRPMConfigVersion())
	assert.Greater(t, ModelNameRPMConfigVersion(), before)
}

func TestRateLimitCapacityEnabledTruthTable(t *testing.T) {
	previousGroup := ModelRequestRateLimitGroup2JSONString()
	previousRPM := ModelNameRPMRateLimit2JSONString()
	previousEnabled := ModelRequestRateLimitEnabled
	previousCount := ModelRequestRateLimitCount
	previousSuccessCount := ModelRequestRateLimitSuccessCount
	defer func() {
		_ = UpdateModelRequestRateLimitGroupByJSONString(previousGroup)
		_ = UpdateModelNameRPMRateLimitByJSONString(previousRPM)
		ModelRequestRateLimitEnabled = previousEnabled
		ModelRequestRateLimitMutex.Lock()
		ModelRequestRateLimitCount = previousCount
		ModelRequestRateLimitSuccessCount = previousSuccessCount
		ModelRequestRateLimitMutex.Unlock()
	}()

	tests := []struct {
		name       string
		a1Enabled  bool
		groupsJSON string
		rpmJSON    string
		count      int
		success    int
		want       bool
	}{
		{"nothing", false, `{}`, `{"enabled":false,"models":{}}`, 0, 0, false},
		{"a1-is-ignored", true, `{"free":[1,1]}`, `{"enabled":false,"models":{}}`, 0, 0, false},
		{"a1-global-default-is-ignored", true, `{}`, `{"enabled":false,"models":{}}`, 0, 1000, false},
		{"a2", false, `{}`, `{"enabled":true,"models":{"gpt-4o":{"global_rpm":1}}}`, 0, 0, true},
		{"disabled-rpm", false, `{}`, `{"enabled":false,"models":{"gpt-4o":{"global_rpm":1}}}`, 0, 0, false},
		{"enabled-with-zero-models", false, `{}`, `{"enabled":true,"models":{}}`, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ModelRequestRateLimitEnabled = tt.a1Enabled
			require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(tt.groupsJSON))
			ModelRequestRateLimitMutex.Lock()
			ModelRequestRateLimitCount = tt.count
			ModelRequestRateLimitSuccessCount = tt.success
			ModelRequestRateLimitMutex.Unlock()
			require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(tt.rpmJSON))
			assert.Equal(t, tt.want, IsRateLimitCapacityEnabled())
		})
	}
}
