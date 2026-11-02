package setting

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTokenUsageAlertTemplateValidation(t *testing.T) {
	for _, test := range []struct {
		subject, body string
		valid         bool
	}{
		{"{{username}}", "{{ key_name }} {{usage_percent}}", true},
		{"", "", true},
		{"hello\r\nBcc: other@example.test", "body", false},
		{"hello", "{{key}}", false},
		{"hello", "{{username", false},
		{"hello", "}} {{username}}", false},
	} {
		raw, err := common.Marshal(TokenUsageAlertConfig{Subject: test.subject, Body: test.body})
		require.NoError(t, err)
		parsed, err := ParseTokenUsageAlerts(string(raw))
		if !test.valid {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		assert.NotEmpty(t, parsed.Subject)
		assert.NotEmpty(t, parsed.Body)
	}
}
