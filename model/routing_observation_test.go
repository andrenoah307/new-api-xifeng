package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutingObservationIsAdminOnly(t *testing.T) {
	raw, err := common.Marshal(map[string]interface{}{
		"cache_tokens": 123,
		"admin_info": map[string]interface{}{
			"routing_observation": map[string]interface{}{
				"signals":           map[string]string{"body:prompt_cache_key": "private-fingerprint"},
				"preferred_channel": 17,
			},
		},
	})
	require.NoError(t, err)
	logs := []*Log{{Other: string(raw)}}
	formatUserLogs(logs, 0)
	assert.NotContains(t, logs[0].Other, "routing_observation")
	assert.NotContains(t, logs[0].Other, "private-fingerprint")
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	assert.EqualValues(t, 123, other["cache_tokens"])
}
