package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	cfg "github.com/QuantumNous/new-api/setting/captcha_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptchaSettingsSecretsAndFreshRootAuthorization(t *testing.T) {
	db := setupOptionControllerTest(t)
	user := model.User{Id: 1, Username: "captcha-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	call := func(method, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("id", 1)
		c.Set("role", common.RoleRootUser)
		c.Request = httptest.NewRequest(method, "/api/captcha/settings", strings.NewReader(body))
		CaptchaSettings(c)
		return w
	}
	config, err := cfg.Load()
	require.NoError(t, err)
	config.TencentAppSecret = "distinct-app-secret"
	config.TencentSecretID = "distinct-cam-id"
	config.TencentSecretKey = "distinct-cam-secret"
	marshalUpdate := func(c cfg.Config, revision string, clear []string) string {
		raw, err := common.Marshal(gin.H{"config": c, "revision": revision, "clear_secrets": clear})
		require.NoError(t, err)
		return string(raw)
	}
	original, _ := cfg.Load()
	result := call("PUT", marshalUpdate(config, original.Fingerprint(), nil))
	require.Equal(t, true, decodeBody(t, result)["success"])
	for _, secret := range []string{config.TencentAppSecret, config.TencentSecretID, config.TencentSecretKey} {
		assert.NotContains(t, result.Body.String(), secret)
	}
	get := call("GET", "")
	assert.NotContains(t, get.Body.String(), "distinct-")
	options := httptest.NewRecorder()
	optionContext, _ := gin.CreateTestContext(options)
	GetOptions(optionContext)
	assert.NotContains(t, options.Body.String(), cfg.OptionKey)
	assert.NotContains(t, options.Body.String(), "distinct-")
	config.TencentAppSecret = ""
	config.TencentSecretID = ""
	config.TencentSecretKey = ""
	saved, err := cfg.Load()
	require.NoError(t, err)
	result = call("PUT", marshalUpdate(config, saved.Fingerprint(), []string{"tencent_secret_key"}))
	require.Equal(t, true, decodeBody(t, result)["success"])
	saved, err = cfg.Load()
	require.NoError(t, err)
	assert.Equal(t, "distinct-app-secret", saved.TencentAppSecret)
	assert.Equal(t, "distinct-cam-id", saved.TencentSecretID)
	assert.Empty(t, saved.TencentSecretKey)
	assert.Equal(t, false, decodeBody(t, call("PUT", marshalUpdate(config, original.Fingerprint(), nil)))["success"], "stale saves must fail")
	assert.Equal(t, false, decodeBody(t, putSingleOption(t, `{"key":"CaptchaConfigSecret","value":"{}"}`))["success"])
	assert.Equal(t, false, decodeBody(t, putSingleOption(t, `{"key":"captchaconfigsecret ","value":"{}"}`))["success"])
	assert.Equal(t, false, decodeBody(t, postOptionBatch(t, `{"options":[{"key":"CaptchaConfigSecret","value":"{}"}]}`))["success"])
	// A stale session role must not authorize a demoted or disabled account.
	require.NoError(t, db.Model(&user).Update("role", common.RoleAdminUser).Error)
	assert.Equal(t, false, decodeBody(t, call("GET", ""))["success"])
}

func TestCaptchaConfigRejectsConcurrentDatabaseUpdate(t *testing.T) {
	db := setupOptionControllerTest(t)
	require.NoError(t, model.UpdateCaptchaConfig("", "first"))
	require.Error(t, model.UpdateOption(cfg.OptionKey, "bypass"))
	require.Error(t, model.UpdateOptionsBulk(map[string]string{cfg.OptionKey: "bypass"}))
	require.Error(t, model.UpdateCaptchaConfig("", "second"))
	require.NoError(t, db.Model(&model.Option{}).Where(&model.Option{Key: cfg.OptionKey}).Update("value", "rotated").Error)
	require.Error(t, model.UpdateCaptchaConfig("first", "lost rotation"))
	var row model.Option
	require.NoError(t, db.Where(&model.Option{Key: cfg.OptionKey}).First(&row).Error)
	assert.Equal(t, "rotated", row.Value)
}
