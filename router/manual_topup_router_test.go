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

func TestManualTopUpRoutePermissions(t *testing.T) {
	db := setupMonitoringRouterTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.Option{}, &model.Log{}))
	require.NoError(t, model.SaveInvoiceTaxConfig(model.InvoiceTaxConfig{ModuleEnabled: true, Mode: "vat"}))
	require.NoError(t, model.SaveInvoiceEpayConfig(model.InvoiceEpayConfig{Strategy: "round_robin", PrivateGatewayName: "Private", Gateways: []model.InvoiceEpayGateway{
		{ID: "corporate-a", Name: "Gateway A", AccountName: "Issuer A", Address: "https://a.example.test", PartnerID: "merchant", Key: "secret-key", PaymentMethods: []string{"alipay"}},
	}}))
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("manual-topup-test"))))
	SetApiRouter(engine)
	target := model.User{Username: "manual-target", AffCode: "manual-target", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&target).Error)
	for _, role := range []int{common.RoleCommonUser, common.RoleFinanceUser, common.RoleCustomerServiceUser, common.RoleAdminUser, common.RoleRootUser} {
		token := fmt.Sprintf("manual-role-%d", role)
		actor := model.User{Username: token, AffCode: token, Role: role, Status: common.UserStatusEnabled, AccessToken: &token}
		require.NoError(t, db.Create(&actor).Error)
		request := httptest.NewRequest("GET", "/api/user/topup/manual/accounts", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("New-Api-User", strconv.Itoa(actor.Id))
		accountsResponse := httptest.NewRecorder()
		engine.ServeHTTP(accountsResponse, request)
		if role >= common.RoleAdminUser {
			assert.Contains(t, accountsResponse.Body.String(), `"account_name":"Issuer A"`)
			assert.Equal(t, "no-store", accountsResponse.Header().Get("Cache-Control"))
		} else {
			assert.NotContains(t, accountsResponse.Body.String(), "Issuer A")
		}
		for _, field := range []string{"secret-key", "merchant", "address", "payment_methods"} {
			assert.NotContains(t, accountsResponse.Body.String(), field)
		}
		body := fmt.Sprintf(`{"user_id":%d,"total_cents":100,"invoice_type":"none","gateway_id":"corporate-a"}`, target.Id)
		for _, path := range []string{"/api/user/topup/manual/quote", "/api/user/topup/manual"} {
			if role >= common.RoleAdminUser && path == "/api/user/topup/manual" {
				quote, err := model.QuoteManualTopUp(db, actor.Id, model.ManualTopUpRequest{UserID: target.Id, TotalCents: 100, InvoiceType: "none", GatewayID: "corporate-a"})
				require.NoError(t, err)
				body = fmt.Sprintf(`{"user_id":%d,"total_cents":100,"invoice_type":"none","gateway_id":"corporate-a","request_id":"manual-route-request-0001","token":"%s"}`, target.Id, quote.Token)
			}
			req := httptest.NewRequest("POST", path, strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("New-Api-User", strconv.Itoa(actor.Id))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			if role >= common.RoleAdminUser {
				assert.Contains(t, response.Body.String(), `"success":true`)
			} else {
				assert.NotContains(t, response.Body.String(), `"success":true`)
			}
		}
	}
	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}
