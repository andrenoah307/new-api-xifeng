package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/captcha"
	cfg "github.com/QuantumNous/new-api/setting/captcha_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptchaBlocksBusinessAndBindsReplay(t *testing.T) {
	oldMap, oldRedis := common.OptionMap, common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.OptionMap = oldMap; common.RedisEnabled = oldRedis })
	config := cfg.Config{Enabled: true, Scenes: map[string]string{"login": "turnstile"}, TurnstileSiteKey: "site", TurnstileSecret: "secret"}
	raw, err := common.Marshal(config)
	require.NoError(t, err)
	common.OptionMap = map[string]string{cfg.OptionKey: string(raw)}
	router := gin.New()
	calls := 0
	uid := 1
	router.POST("/api/user/login", func(c *gin.Context) { c.Set("id", uid) }, TurnstileCheck(), func(c *gin.Context) { calls++; c.JSON(200, gin.H{"success": true}) })
	request := func(body, cookie, proof string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/user/login", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Captcha-Proof", proof)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: CaptchaCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	initial := request(`{"password":"first"}`, "", "")
	require.Equal(t, 200, initial.Code)
	assert.Equal(t, 0, calls)
	var response struct {
		Code string                  `json:"code"`
		Data captcha.PublicChallenge `json:"data"`
	}
	require.NoError(t, common.Unmarshal(initial.Body.Bytes(), &response))
	require.Equal(t, "captcha_required", response.Code)
	cookies := initial.Result().Cookies()
	require.Len(t, cookies, 1)
	browser := cookies[0].Value
	claim, err := captcha.ConsumeChallenge(context.Background(), captcha.ActiveStore(), response.Data.ID, captcha.Digest(browser), config.Fingerprint())
	require.NoError(t, err)
	_, err = captcha.ConsumeChallenge(context.Background(), captcha.ActiveStore(), response.Data.ID, captcha.Digest(browser), config.Fingerprint())
	require.ErrorIs(t, err, captcha.ErrInvalid)
	proof, err := captcha.Issue(context.Background(), captcha.ActiveStore(), "proof:", claim)
	require.NoError(t, err)
	assert.Contains(t, request(`{"password":"changed"}`, browser, proof).Body.String(), "captcha_failed")
	assert.Contains(t, request(`{"password":"first"}`, strings.Repeat("f", 64), proof).Body.String(), "captcha_failed")
	uid = 2
	assert.Contains(t, request(`{"password":"first"}`, browser, proof).Body.String(), "captcha_failed")
	uid = 1
	assert.Equal(t, 0, calls)
	config.TurnstileSiteKey = "rotated-site"
	changed, err := common.Marshal(config)
	require.NoError(t, err)
	common.OptionMap[cfg.OptionKey] = string(changed)
	assert.Contains(t, request(`{"password":"first"}`, browser, proof).Body.String(), "captcha_failed")
	common.OptionMap[cfg.OptionKey] = string(raw)
	assert.Contains(t, request(`{"password":"first"}`, browser, proof).Body.String(), `"success":true`)
	assert.Equal(t, 1, calls)
	assert.Contains(t, request(`{"password":"first"}`, browser, proof).Body.String(), "captcha_failed")
	assert.Equal(t, 1, calls)
}
