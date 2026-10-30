package router

import (
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFundingOverviewRootOnly(t *testing.T) {
	db := setupMonitoringRouterTestDB(t)
	oldLog, oldExport := model.LOG_DB, common.DataExportEnabled
	model.LOG_DB, common.DataExportEnabled = db, true
	t.Cleanup(func() { model.LOG_DB, common.DataExportEnabled = oldLog, oldExport })
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.TopUp{}, &model.QuotaData{}, &model.Log{}))
	require.NoError(t, model.SaveInvoiceTaxConfig(model.InvoiceTaxConfig{ModuleEnabled: true, Mode: "vat"}))
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("funding-test"))))
	var sessionUser model.User
	engine.GET("/test-root-session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", sessionUser.Id)
		session.Set("username", sessionUser.Username)
		session.Set("role", common.RoleRootUser)
		session.Set("status", common.UserStatusEnabled)
		require.NoError(t, session.Save())
		c.Status(200)
	})
	SetApiRouter(engine)
	for _, role := range []int{common.RoleCommonUser, common.RoleFinanceUser, common.RoleCustomerServiceUser, common.RoleAdminUser, common.RoleRootUser} {
		token := fmt.Sprintf("funding-role-%d", role)
		actor := model.User{Username: token, AffCode: token, Role: role, Status: common.UserStatusEnabled, AccessToken: &token}
		require.NoError(t, db.Create(&actor).Error)
		if role == common.RoleRootUser {
			sessionUser = actor
		}
		for _, query := range []string{"start_timestamp=3600&end_timestamp=7200", "start_timestamp=0&end_timestamp=7200", "start_timestamp=3601&end_timestamp=7200"} {
			req := httptest.NewRequest("GET", "/api/data/funding?"+query, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("New-Api-User", strconv.Itoa(actor.Id))
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			if role == common.RoleRootUser && query == "start_timestamp=3600&end_timestamp=7200" {
				assert.Contains(t, response.Body.String(), `"success":true`)
				assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			} else {
				assert.NotContains(t, response.Body.String(), `"consumption"`)
				assert.Contains(t, response.Body.String(), `"success":false`)
			}
		}
	}
	login := httptest.NewRecorder()
	engine.ServeHTTP(login, httptest.NewRequest("GET", "/test-root-session", nil))
	require.NotEmpty(t, login.Result().Cookies())
	for _, change := range []map[string]interface{}{
		{"role": common.RoleAdminUser},
		{"role": common.RoleRootUser, "status": common.UserStatusDisabled},
	} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", sessionUser.Id).Updates(change).Error)
		req := httptest.NewRequest("GET", "/api/data/funding?start_timestamp=3600&end_timestamp=7200", nil)
		req.Header.Set("New-Api-User", strconv.Itoa(sessionUser.Id))
		for _, cookie := range login.Result().Cookies() {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		assert.Contains(t, response.Body.String(), `"success":false`)
		assert.NotContains(t, response.Body.String(), `"consumption"`)
	}
}
