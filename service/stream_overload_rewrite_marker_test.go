package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachStreamOverloadRewriteNoOp(t *testing.T) {
	tests := []struct {
		name      string
		relayInfo *relaycommon.RelayInfo
		other     map[string]interface{}
	}{
		{
			name:  "nil relay info",
			other: map[string]interface{}{"existing": true},
		},
		{
			name:      "nil marker",
			relayInfo: &relaycommon.RelayInfo{},
			other:     map[string]interface{}{"existing": true},
		},
		{
			name: "nil other",
			relayInfo: &relaycommon.RelayInfo{StreamOverloadRewrite: &relaycommon.StreamOverloadRewriteMarker{
				OriginalCode: "slow_down",
				Count:        1,
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var before map[string]interface{}
			if tt.other != nil {
				before = make(map[string]interface{}, len(tt.other))
				for key, value := range tt.other {
					before[key] = value
				}
			}

			assert.NotPanics(t, func() {
				attachStreamOverloadRewrite(nil, tt.relayInfo, tt.other)
			})
			assert.Equal(t, before, tt.other)
		})
	}
}

func TestAttachStreamOverloadRewriteAddsUserAndAdminMarkers(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{StreamOverloadRewrite: &relaycommon.StreamOverloadRewriteMarker{
		OriginalCode: "SERVER_IS_OVERLOADED",
		Count:        2,
	}}
	other := map[string]interface{}{"model_ratio": 1.0}

	attachStreamOverloadRewrite(nil, relayInfo, other)

	assert.Equal(t, true, other["stream_overload_rewrite"])
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	marker, ok := adminInfo["stream_overload_rewrite"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "SERVER_IS_OVERLOADED", marker["original_code"])
	assert.Equal(t, 2, marker["count"])
}

func TestAttachStreamOverloadRewritePreservesExistingAdminInfo(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{StreamOverloadRewrite: &relaycommon.StreamOverloadRewriteMarker{
		OriginalCode: "slow_down",
		Count:        1,
	}}
	other := map[string]interface{}{
		"admin_info": map[string]interface{}{
			"admin_username":   "root",
			"quota_saturation": map[string]interface{}{"kind": "overflow"},
		},
	}

	attachStreamOverloadRewrite(nil, relayInfo, other)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "root", adminInfo["admin_username"])
	assert.Equal(t, map[string]interface{}{"kind": "overflow"}, adminInfo["quota_saturation"])
	require.Contains(t, adminInfo, "stream_overload_rewrite")
}
