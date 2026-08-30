package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
			"zero_charge_guard": map[string]interface{}{
				"reason":            "empty_output",
				"prompt_tokens":     1601,
				"completion_tokens": 553250,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}

func TestFormatUserLogsProtectsOnlyAdminOperatorIP(t *testing.T) {
	tests := []struct {
		name        string
		log         *Log
		wantIP      string
		wantPresent []string
		wantAbsent  []string
		wantAction  string
	}{
		{
			name: "managed account event hides administrator IP",
			log: &Log{
				Type: LogTypeManage,
				Ip:   "198.51.100.10",
				Other: common.MapToJsonStr(map[string]interface{}{
					"admin_info": map[string]interface{}{"admin_id": 7},
					"audit_info": map[string]interface{}{"route": "/api/user/manage"},
					"op":         map[string]interface{}{"action": "user.quota_add"},
				}),
			},
			wantIP:      "",
			wantPresent: []string{"op"},
			wantAbsent:  []string{"admin_info", "audit_info"},
			wantAction:  "user.quota_add",
		},
		{
			name: "self-service security event retains user IP",
			log: &Log{
				Type: LogTypeManage,
				Ip:   "203.0.113.20",
				Other: common.MapToJsonStr(map[string]interface{}{
					"op": map[string]interface{}{"action": "user.passkey_register"},
				}),
			},
			wantIP:      "203.0.113.20",
			wantPresent: []string{"op"},
			wantAbsent:  []string{"admin_info", "audit_info"},
			wantAction:  "user.passkey_register",
		},
		{
			name: "ordinary consumption log remains unchanged",
			log: &Log{
				Type: LogTypeConsume,
				Ip:   "192.0.2.30",
				Other: common.MapToJsonStr(map[string]interface{}{
					"model_price": 0.004,
				}),
			},
			wantIP:      "192.0.2.30",
			wantPresent: []string{"model_price"},
			wantAbsent:  []string{"admin_info", "audit_info"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs := []*Log{test.log}
			formatUserLogs(logs, 0)

			assert.Equal(t, test.wantIP, logs[0].Ip)
			parsed, err := common.StrToMap(logs[0].Other)
			require.NoError(t, err)
			for _, key := range test.wantPresent {
				assert.Contains(t, parsed, key)
			}
			for _, key := range test.wantAbsent {
				assert.NotContains(t, parsed, key)
			}
			if test.wantAction != "" {
				op, ok := parsed["op"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, test.wantAction, op["action"])
			}
		})
	}
}

func TestFormatUserLogsRemovesChannelAndRiskControlSentinelsRecursively(t *testing.T) {
	logs := []*Log{{
		ChannelName: "stored-channel-name",
		ChannelId:   812,
		Other: common.MapToJsonStr(map[string]interface{}{
			"channel_name": "channel-name-sentinel",
			"channel_type": 987654321,
			"risk_control": map[string]interface{}{
				"token_decision": map[string]interface{}{
					"group": "risk-group-sentinel",
				},
			},
			"model_price": 0.004,
		}),
	}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	var containsSentinel func(any) bool
	containsSentinel = func(value any) bool {
		switch current := value.(type) {
		case map[string]interface{}:
			for key, nested := range current {
				if key == "channel_name" || key == "channel_type" || key == "risk_control" {
					return true
				}
				if containsSentinel(nested) {
					return true
				}
			}
		case []interface{}:
			for _, nested := range current {
				if containsSentinel(nested) {
					return true
				}
			}
		case string:
			return current == "channel-name-sentinel" || current == "risk-group-sentinel"
		case float64:
			return current == 987654321
		}
		return false
	}

	assert.False(t, containsSentinel(parsed))
	assert.Equal(t, float64(0.004), parsed["model_price"])
	assert.Empty(t, logs[0].ChannelName)
	assert.Equal(t, 812, logs[0].ChannelId)
}
