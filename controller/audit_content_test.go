package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
)

func TestAuditContentEN(t *testing.T) {
	generalSetting := operation_setting.GetGeneralSetting()
	oldDisplayType := generalSetting.QuotaDisplayType
	generalSetting.QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		generalSetting.QuotaDisplayType = oldDisplayType
	})

	quota := 500000
	legacyQuota := logger.LogQuota(quota)
	tests := []struct {
		name   string
		action string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "int quota uses FormatQuota",
			action: "user.quota_add",
			params: map[string]interface{}{"quota": quota},
			want:   "Increased user quota by " + logger.FormatQuota(quota),
		},
		{
			name:   "int32 and int64 bounds use FormatQuota",
			action: "user.quota_override",
			params: map[string]interface{}{"from": int32(quota), "to": int64(quota * 2)},
			want:   "Overrode user quota from " + logger.FormatQuota(quota) + " to " + logger.FormatQuota(quota*2),
		},
		{
			name:   "integral float64 quota uses FormatQuota",
			action: "user.quota_subtract",
			params: map[string]interface{}{"quota": float64(quota)},
			want:   "Decreased user quota by " + logger.FormatQuota(quota),
		},
		{
			name:   "fractional float64 stays unchanged",
			action: "user.quota_add",
			params: map[string]interface{}{"quota": 1.5},
			want:   "Increased user quota by 1.5",
		},
		{
			name:   "redemption string quota stays byte-for-byte unchanged",
			action: "redemption.create",
			params: map[string]interface{}{"count": 2, "name": "promo", "quota": legacyQuota},
			want:   "Created 2 redemption codes named promo (" + legacyQuota + " each)",
		},
		{
			name:   "unknown action falls back to action",
			action: "unknown.audit.action",
			params: map[string]interface{}{"quota": quota},
			want:   "unknown.audit.action",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, auditContentEN(test.action, test.params))
		})
	}
}
