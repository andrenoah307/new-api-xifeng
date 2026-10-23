package router

import (
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceEpayConfigurationIsRootOnlyAndNeverReturnsSecret(t *testing.T) {
	db := setupMonitoringRouterTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Log{}))
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("invoice-config-test"))))
	SetApiRouter(engine)
	for _, role := range []int{common.RoleCommonUser, common.RoleFinanceUser, common.RoleAdminUser, common.RoleRootUser} {
		token := fmt.Sprintf("invoice-config-%d", role)
		user := model.User{Username: token, Password: "password", Role: role, Status: common.UserStatusEnabled, AffCode: token, AccessToken: &token}
		require.NoError(t, db.Create(&user).Error)
		for _, method := range []string{"PUT", "GET"} {
			req := httptest.NewRequest(method, "/api/option/invoice-epay", strings.NewReader(`{"strategy":"round_robin","private_below_cents":50000,"private_gateway_name":"Gateway P","gateways":[{"id":"corporate-a","name":"Gateway A","account_name":"Issuer A","address":"https://pay.example.test","partner_id":"invoice-merchant","key":"hidden-invoice-key","payment_methods":["wxpay"]}]}`))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			assert.NotContains(t, w.Body.String(), "hidden-invoice-key")
			if role == common.RoleRootUser {
				assert.Contains(t, w.Body.String(), `"success":true`)
				assert.Contains(t, w.Body.String(), `"key_configured":true`)
			} else {
				assert.NotContains(t, w.Body.String(), `"success":true`)
				assert.NotContains(t, w.Body.String(), "Issuer A")
			}
		}
	}
	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	for _, log := range logs {
		assert.NotContains(t, log.Content+log.Other, "hidden-invoice-key")
	}
}
