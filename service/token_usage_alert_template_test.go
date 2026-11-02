package service

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTokenUsageAlertCustomTemplateAndEscaping(t *testing.T) {
	config := setting.TokenUsageAlertConfig{Subject: "Notice {{key_id}}", Body: "{{username}} {{key_name}} {{scope}} {{usage_percent}} {{reset_at}}"}
	token := model.Token{Id: 123, Name: "<img src=x>", Key: "never-expose-this-key"}
	user := model.User{Username: "<script>alert(1)</script>"}
	for _, scope := range []string{"total", "period"} {
		usage := model.TokenUsageAlertUsage{Scope: scope, Used: 80, Limit: 100}
		if scope == "period" {
			usage.ResetAt = 1793635200
		}
		subject, body, err := RenderTokenUsageAlertEmail(config, token, user, usage, setting.TokenUsageAlertRule{Percent: 50, CooldownDays: 7})
		require.NoError(t, err)
		assert.Equal(t, "Notice 123", subject)
		assert.Contains(t, body, "80.00")
		assert.Contains(t, body, "&lt;img src=x&gt;")
		assert.NotContains(t, body, "<script>")
		assert.NotContains(t, body, token.Key)
		if scope == "period" {
			assert.Contains(t, body, "2026-11-03 00:00:00")
		} else {
			assert.Contains(t, body, "不适用")
		}
	}
	config.Subject = "{{username}}"
	user.Username = "name\r\nBcc: attack@example.test"
	_, _, err := RenderTokenUsageAlertEmail(config, token, user, model.TokenUsageAlertUsage{Limit: 1}, setting.TokenUsageAlertRule{})
	require.Error(t, err)
}

func TestTokenUsageAlertSenderUsesPersistedTemplate(t *testing.T) {
	_, _ = tokenUsageAlertFixture(t)
	config := setting.GetTokenUsageAlerts()
	config.Subject = "Custom {{key_id}}"
	config.Body = "Hello {{username}} / {{usage_percent}}"
	raw, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.Option{}).Where(&model.Option{Key: setting.TokenUsageAlertOption}).Update("value", string(raw)).Error)
	sent := 0
	sendTokenUsageAlertEmail = func(_ context.Context, subject, to, body string) (string, error) {
		sent++
		assert.Equal(t, "Custom 2", subject)
		assert.Contains(t, body, "Hello &lt;user&gt; / 80.00")
		return "accepted", nil
	}
	processTokenUsageAlert("1:2:half")
	assert.Equal(t, 1, sent)
	config.Subject = "{{username}}"
	raw, err = common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.Option{}).Where(&model.Option{Key: setting.TokenUsageAlertOption}).Update("value", string(raw)).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = 1").Update("username", "name\r\nBcc: injected@example.test").Error)
	processTokenUsageAlert("1:2:high")
	assert.Equal(t, 1, sent)
	var count int64
	require.NoError(t, model.DB.Model(&model.TokenUsageAlertDelivery{}).Where("rule_id = ?", "high").Count(&count).Error)
	assert.Zero(t, count, "invalid rendered header must not consume a rule cooldown")
}
