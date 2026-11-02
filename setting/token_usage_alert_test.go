package setting

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTokenUsageAlertRulesValidation(t *testing.T) {
	valid := TokenUsageAlertConfig{Enabled: true, Rules: []TokenUsageAlertRule{{ID: "a", Percent: 50, CooldownDays: 7, Scope: "any"}, {ID: "b", Percent: 70, CooldownDays: 3, Scope: "period"}}}
	data, err := common.Marshal(valid)
	require.NoError(t, err)
	parsed, err := ParseTokenUsageAlerts(string(data))
	require.NoError(t, err)
	assert.Equal(t, valid, parsed)
	for _, test := range []struct {
		name string
		rule TokenUsageAlertRule
	}{
		{"duplicate ID", TokenUsageAlertRule{ID: "a", Percent: 70, CooldownDays: 3, Scope: "period"}},
		{"overlapping threshold", TokenUsageAlertRule{ID: "b", Percent: 50, CooldownDays: 3, Scope: "total"}},
		{"zero cooldown", TokenUsageAlertRule{ID: "b", Percent: 70, CooldownDays: 0, Scope: "any"}},
		{"invalid percentage", TokenUsageAlertRule{ID: "b", Percent: 101, CooldownDays: 3, Scope: "any"}},
		{"invalid scope", TokenUsageAlertRule{ID: "b", Percent: 70, CooldownDays: 3, Scope: "weekly"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			config.Rules = []TokenUsageAlertRule{valid.Rules[0], test.rule}
			data, err := common.Marshal(config)
			require.NoError(t, err)
			_, err = ParseTokenUsageAlerts(string(data))
			require.Error(t, err)
		})
	}
	_, err = ParseTokenUsageAlerts(`{"enabled":true,"rules":[]}`)
	require.Error(t, err)
	_, err = ParseTokenUsageAlerts(`{"enabled":false,"rules":[]}`)
	require.NoError(t, err)
}
