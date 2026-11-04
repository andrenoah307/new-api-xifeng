package router

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	cfg "github.com/QuantumNous/new-api/setting/captcha_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptchaRoutesRejectDirectBusinessCalls(t *testing.T) {
	db := setupMonitoringRouterTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, model.SaveInvoiceTaxConfig(model.InvoiceTaxConfig{ModuleEnabled: true, Mode: "vat"}))
	token := "captcha-route-token"
	user := model.User{Username: "captcha-route-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AccessToken: &token}
	require.NoError(t, db.Create(&user).Error)
	oldOptions := common.OptionMap
	oldGlobal, oldCritical := common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable
	common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable = false, false
	t.Cleanup(func() {
		common.OptionMap = oldOptions
		common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable = oldGlobal, oldCritical
	})
	config := cfg.Config{Enabled: true, Scenes: map[string]string{}, TurnstileSiteKey: "site", TurnstileSecret: "secret"}
	for _, scene := range cfg.Scenes {
		config.Scenes[scene] = "turnstile"
	}
	raw, err := common.Marshal(config)
	require.NoError(t, err)
	common.OptionMap = map[string]string{cfg.OptionKey: string(raw)}
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("captcha-route-fixture"))))
	SetApiRouter(engine)
	for i, route := range []struct{ method, path, body string }{
		{"GET", "/api/verification?email=test@example.org", ""},
		{"GET", "/api/reset_password?email=test@example.org", ""},
		{"POST", "/api/user/login", `{}`},
		{"POST", "/api/user/register", `{}`},
		{"POST", "/api/user/reset", `{}`},
		{"POST", "/api/user/topup", `{"key":"code"}`},
		{"POST", "/api/user/checkin", `{}`},
		{"POST", "/api/user/discount_code/validate", `{"code":"code"}`},
		{"POST", "/api/user/pay", `{"discount_code":"code"}`},
		{"POST", "/api/user/amount", `{"discount_code":"code"}`},
		{"POST", "/api/user/epay/checkout/quote", `{"discount_code":"code"}`},
		{"POST", "/api/user/epay/checkout/pay", `{"discount_code":"code"}`},
		{"POST", "/api/user/stripe/pay", `{"discount_code":"code"}`},
		{"POST", "/api/user/stripe/amount", `{"discount_code":"code"}`},
		{"POST", "/api/user/creem/pay", `{"discount_code":"code"}`},
		{"POST", "/api/user/waffo/pay", `{"discount_code":"code"}`},
		{"POST", "/api/user/waffo/amount", `{"discount_code":"code"}`},
		{"POST", "/api/user/waffo-pancake/pay", `{"discount_code":"code"}`},
		{"POST", "/api/user/waffo-pancake/amount", `{"discount_code":"code"}`},
		{"POST", "/api/ticket/", `{}`},
		{"POST", "/api/ticket/invoice/", `{}`},
		{"POST", "/api/ticket/refund/", `{}`},
		{"POST", "/api/ticket/self/1/message", `{}`},
		{"POST", "/api/ticket/attachment", `{}`},
	} {
		t.Run(route.path, func(t *testing.T) {
			r := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			r.RemoteAddr = "192.0.2." + strconv.Itoa(i+1) + ":1234"
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("New-Api-User", strconv.Itoa(user.Id))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)
			assert.Equal(t, 200, w.Code)
			assert.Contains(t, w.Body.String(), `"code":"captcha_required"`)
		})
	}
}
