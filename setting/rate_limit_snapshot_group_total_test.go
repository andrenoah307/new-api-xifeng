package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsRateLimitCapacityEnabledForGroupsOnly(t *testing.T) {
	previousRPM := ModelNameRPMRateLimit2JSONString()
	previousCard := IsRateLimitCapacityCardEnabled()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(previousRPM))
		SetRateLimitCapacityCardEnabled(previousCard)
	})
	SetRateLimitCapacityCardEnabled(true)
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":10}}}`))
	assert.True(t, IsRateLimitCapacityEnabled())
}
