package captcha_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationRejectsIncompleteProtection(t *testing.T) {
	c := Config{Enabled: true, Scenes: map[string]string{"login": "tencent"}}
	require.Error(t, c.Validate())
	c.TencentAppID = "123456789"
	c.TencentAppSecret = "app-secret"
	c.TencentSecretID = "secret-id"
	c.TencentSecretKey = "secret-key"
	require.NoError(t, c.Validate())
	assert.Equal(t, "tencent", c.Provider("login"))
	assert.Equal(t, "off", c.Provider("register"))
	c.Scenes["payment_callback"] = "tencent"
	require.Error(t, c.Validate())
}

func TestLegacyCaptchaMigrationAndExplicitConfiguration(t *testing.T) {
	old := common.OptionMap
	t.Cleanup(func() { common.OptionMap = old })
	common.OptionMap = map[string]string{"TurnstileCheckEnabled": "true", "TurnstileSiteKey": "site", "TurnstileSecretKey": "secret"}
	c, err := Load()
	require.NoError(t, err)
	for _, scene := range []string{"login", "register", "email_verification", "password_reset_email", "checkin"} {
		assert.Equal(t, "turnstile", c.Provider(scene))
	}
	assert.Equal(t, "off", c.Provider("password_reset"))
	assert.Equal(t, "off", c.Provider("discount_code"))
	common.OptionMap[OptionKey] = `{"enabled":false,"scenes":{}}`
	c, err = Load()
	require.NoError(t, err)
	assert.Equal(t, "off", c.Provider("login"))
	assert.Empty(t, c.TurnstileSecret, "stored configuration must not inherit legacy secrets")
	common.OptionMap[OptionKey] = `null`
	_, err = Load()
	require.Error(t, err)
}

func TestDisabledConfigurationNeverChallenges(t *testing.T) {
	c := Config{Scenes: map[string]string{"login": "tencent"}}
	require.NoError(t, c.Validate())
	assert.Equal(t, "off", c.Provider("login"))
}
