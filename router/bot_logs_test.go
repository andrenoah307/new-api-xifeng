package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBotLogRouteSystemTokenOnlyAndRevocation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMonitoringRouterTestDB(t)
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	require.NoError(t, i18n.Init())
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Token{}, &model.CasbinRule{}, &model.AuthzRole{}))
	oldMaster, oldGlobal, oldLogLimit := common.IsMasterNode, common.GlobalApiRateLimitEnable, common.LogQueryRateLimitEnable
	common.IsMasterNode, common.GlobalApiRateLimitEnable, common.LogQueryRateLimitEnable = true, false, false
	t.Cleanup(func() {
		common.IsMasterNode, common.GlobalApiRateLimitEnable, common.LogQueryRateLimitEnable = oldMaster, oldGlobal, oldLogLimit
	})
	require.NoError(t, authz.Init(db))
	for _, role := range []int{1, 2, 5, 10, 100} {
		key := fmt.Sprintf("system-key-%d", role)
		require.NoError(t, db.Create(&model.User{Id: role, Username: key, AffCode: key, AccessToken: &key, Role: role, Status: common.UserStatusEnabled}).Error)
	}
	require.NoError(t, db.Create(&model.Token{UserId: 2, Key: "relaykey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 1, Username: "ordinary", Type: model.LogTypeConsume, CreatedAt: 100, Group: "billed", Other: `{"admin_info":{"private":"do-not-return"}}`}).Error)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("bot-test-session-secret"))))
	engine.GET("/test-session", func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("id", 2)
		s.Set("role", common.RoleBotUser)
		s.Set("username", "system-key-2")
		s.Set("status", common.UserStatusEnabled)
		require.NoError(t, s.Save())
		c.Status(http.StatusOK)
	})
	SetApiRouter(engine)
	sessionResponse := httptest.NewRecorder()
	engine.ServeHTTP(sessionResponse, httptest.NewRequest(http.MethodGet, "/test-session", nil))
	cookies := sessionResponse.Result().Cookies()
	require.NotEmpty(t, cookies)
	for _, tc := range []struct {
		name, authorization, id string
		want                    int
	}{
		{"cookie only", "", "2", http.StatusUnauthorized},
		{"relay key", "Bearer sk-relaykey", "2", http.StatusUnauthorized},
		{"ordinary", "Bearer system-key-1", "1", http.StatusForbidden},
		{"staff", "Bearer system-key-5", "5", http.StatusForbidden},
		{"admin", "Bearer system-key-10", "10", http.StatusForbidden},
		{"root", "Bearer system-key-100", "100", http.StatusForbidden},
		{"wrong id", "Bearer system-key-2", "1", http.StatusForbidden},
		{"missing id", "Bearer system-key-2", "", http.StatusForbidden},
		{"bot", "Bearer system-key-2", "2", http.StatusOK},
		{"lowercase bearer", "bearer system-key-2", "2", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/log/bot?start_timestamp=1&end_timestamp=200&group=billed", nil)
			req.Header.Set("Authorization", tc.authorization)
			req.Header.Set("New-Api-User", tc.id)
			req.AddCookie(cookies[0])
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			assert.Equal(t, tc.want, rec.Code)
			assert.NotContains(t, rec.Body.String(), "admin_info")
			assert.NotContains(t, rec.Body.String(), "do-not-return")
			if tc.want == http.StatusOK {
				assert.Contains(t, rec.Body.String(), `"user_id":1`)
				assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			}
		})
	}
	for _, change := range []struct {
		field string
		value any
		want  int
	}{
		{"role", common.RoleCommonUser, http.StatusForbidden},
		{"status", common.UserStatusDisabled, http.StatusForbidden},
		{"access_token", "replacement-key", http.StatusUnauthorized},
	} {
		t.Run("revoke "+change.field, func(t *testing.T) {
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).Update(change.field, change.value).Error)
			req := httptest.NewRequest(http.MethodGet, "/api/log/bot", nil)
			req.Header.Set("Authorization", "Bearer system-key-2")
			req.Header.Set("New-Api-User", strconv.Itoa(2))
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			assert.Equal(t, change.want, rec.Code)
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).Updates(map[string]any{"role": common.RoleBotUser, "status": common.UserStatusEnabled, "access_token": "system-key-2"}).Error)
		})
	}
}
