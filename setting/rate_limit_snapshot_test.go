package setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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
	require.NotNil(t, got.Models["gpt-4o"].GlobalRPM)
	*got.Models["gpt-4o"].GlobalRPM = 99
	got.Models["gpt-4o"] = ModelNameRPMRule{GlobalRPM: t1GlobalRPM(1)}
	latest := ListModelNameRPMRules()
	require.NotNil(t, latest.Models["gpt-4o"].GlobalRPM)
	assert.Equal(t, 10, *latest.Models["gpt-4o"].GlobalRPM)
	assert.Equal(t, 2, latest.Models["gpt-4o"].UserRPM)
	assert.Equal(t, 3, latest.Models["gpt-4o"].GroupRPM["free"])
	_, version := ListModelNameRPMRulesWithVersion()
	assert.Equal(t, version, ModelNameRPMConfigVersion())
	assert.Greater(t, ModelNameRPMConfigVersion(), before)
}

func TestRateLimitCapacityEnabledTruthTable(t *testing.T) {
	previousRPM := ModelNameRPMRateLimit2JSONString()
	previousCardEnabled := IsRateLimitCapacityCardEnabled()
	defer func() {
		require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(previousRPM))
		SetRateLimitCapacityCardEnabled(previousCardEnabled)
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models := map[string]ModelNameRPMRule{}
			if tt.hasModels {
				models["gpt-4o"] = ModelNameRPMRule{GlobalRPM: t1GlobalRPM(1)}
			}
			SetRateLimitCapacityCardEnabled(tt.cardEnabled)
			config, err := common.Marshal(ModelNameRPMConfig{Enabled: tt.a2Enabled, Models: models})
			require.NoError(t, err)
			require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(string(config)))
			assert.Equal(t, tt.want, IsRateLimitCapacityEnabled())
		})
	}
}

func TestRateLimitCapacityCardEnabledDefaultsFalseAndRoundTrips(t *testing.T) {
	previous := IsRateLimitCapacityCardEnabled()
	t.Cleanup(func() { SetRateLimitCapacityCardEnabled(previous) })

	assert.False(t, IsRateLimitCapacityCardEnabled())

	SetRateLimitCapacityCardEnabled(true)
	assert.True(t, IsRateLimitCapacityCardEnabled())

	SetRateLimitCapacityCardEnabled(false)
	assert.False(t, IsRateLimitCapacityCardEnabled())
}
