package router

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinanceRoutesEnforceScopeAndExportOneRowPerInvoice(t *testing.T) {
	db := setupMonitoringRouterTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.TopUp{}, &model.Ticket{}, &model.TicketInvoice{}, &model.AuthzRole{}, &model.CasbinRule{}, &model.Log{}))
	require.NoError(t, model.SaveInvoiceTaxConfig(model.InvoiceTaxConfig{ModuleEnabled: true, Mode: "vat"}))
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = wasMaster })
	require.NoError(t, authz.Init(db))
	users := map[int]*model.User{}
	for _, role := range []int{common.RoleCommonUser, common.RoleFinanceUser, common.RoleCustomerServiceUser, common.RoleAdminUser} {
		token := fmt.Sprintf("finance-token-%d", role)
		user := &model.User{Username: fmt.Sprintf("finance-user-%d", role), Password: "password", Role: role, Status: common.UserStatusEnabled, Group: "default", AccessToken: &token}
		user.AffCode = fmt.Sprintf("finance-aff-%d", role)
		require.NoError(t, db.Create(user).Error)
		users[role] = user
	}
	owner := users[common.RoleCommonUser]
	orders := []model.TopUp{{UserId: owner.Id, TradeNo: "order-a", Money: 10, Status: "success"}, {UserId: owner.Id, TradeNo: "order-b", Money: 20, Status: "success"}}
	require.NoError(t, db.Create(&orders).Error)
	otherOrder := model.TopUp{UserId: users[common.RoleFinanceUser].Id, TradeNo: "private-finance-order", Money: 40, Status: "success"}
	require.NoError(t, db.Create(&otherOrder).Error)
	ticket := model.Ticket{UserId: owner.Id, Username: owner.Username, Type: model.TicketTypeInvoice}
	require.NoError(t, db.Create(&ticket).Error)
	invoice := model.TicketInvoice{TicketId: ticket.Id, UserId: owner.Id, CompanyName: "=SUM(1,2)", TotalMoney: 30, InvoiceStatus: model.InvoiceStatusPending}
	require.NoError(t, invoice.SetTopUpOrderIDs([]int{orders[0].Id, orders[1].Id}))
	require.NoError(t, db.Create(&invoice).Error)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("finance-test"))))
	SetApiRouter(engine)
	request := func(role int, method, path, body string) *httptest.ResponseRecorder {
		user := users[role]
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+*user.AccessToken)
		req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	require.NoError(t, model.SaveInvoiceTaxConfig(model.InvoiceTaxConfig{Mode: "vat"}))
	for _, role := range []int{common.RoleCommonUser, common.RoleFinanceUser, common.RoleAdminUser} {
		for _, path := range []string{"/api/finance/records", "/api/finance/export", "/api/finance/policy", "/api/user/epay/checkout/methods"} {
			assert.Contains(t, request(role, "GET", path, "").Body.String(), `"success":false`, path)
		}
	}
	require.NoError(t, model.SaveInvoiceTaxConfig(model.InvoiceTaxConfig{ModuleEnabled: true, Mode: "vat"}))
	for _, role := range []int{common.RoleCommonUser, common.RoleCustomerServiceUser} {
		for _, path := range []string{"/api/finance/records", "/api/finance/export", "/api/finance/policy"} {
			assert.Equal(t, http.StatusForbidden, request(role, "GET", path, "").Code, path)
		}
	}
	// Client-supplied owner/role flags cannot widen the authenticated self scope.
	w := request(common.RoleCommonUser, "GET", fmt.Sprintf("/api/user/topup/self?user_id=%d&admin=true&view=finance", otherOrder.UserId), "")
	require.Contains(t, w.Body.String(), `"success":true`)
	assert.Contains(t, w.Body.String(), "order-a")
	assert.NotContains(t, w.Body.String(), "private-finance-order")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	w = request(common.RoleFinanceUser, "GET", fmt.Sprintf("/api/user/topup/self?user_id=%d", owner.Id), "")
	require.Contains(t, w.Body.String(), `"success":true`)
	assert.Contains(t, w.Body.String(), "private-finance-order")
	assert.NotContains(t, w.Body.String(), "order-a")
	assert.NotContains(t, request(common.RoleCommonUser, "GET", "/api/user/topup?admin=true", "").Body.String(), `"success":true`)
	for _, path := range []string{"/api/user/topup", "/api/ticket/admin/", "/api/channel/"} {
		w := request(common.RoleFinanceUser, "GET", path, "")
		assert.NotContains(t, w.Body.String(), `"success":true`, path)
	}
	for _, path := range []string{"/api/finance/policy", "/api/user/topup/complete"} {
		method := "PUT"
		if strings.HasSuffix(path, "complete") {
			method = "POST"
		}
		assert.NotContains(t, request(common.RoleFinanceUser, method, path, `{}`).Body.String(), `"success":true`)
	}
	w = request(common.RoleFinanceUser, "GET", "/api/finance/export?view=invoices", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/csv")
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "'=SUM(1,2)", rows[1][5])
	assert.Equal(t, "30.00", rows[1][6])
	assert.Equal(t, "order-a | order-b", rows[1][11])
	w = request(common.RoleFinanceUser, "GET", "/api/finance/records?view=topups", "")
	assert.Contains(t, w.Body.String(), `"items":[]`)
	w = request(common.RoleAdminUser, "GET", "/api/finance/policy/choices", "")
	assert.Contains(t, w.Body.String(), `"success":true`)
	assert.Contains(t, w.Body.String(), `"payment_method":""`)
	assert.Contains(t, request(common.RoleFinanceUser, "GET", "/api/finance/policy/choices", "").Body.String(), `"success":false`)
	w = request(common.RoleAdminUser, "PUT", "/api/finance/policy", `{"invoice_statuses":[],"topup_statuses":["success"],"payment_methods":[],"sources":[]}`)
	assert.Contains(t, w.Body.String(), `"success":true`)
	w = request(common.RoleFinanceUser, "GET", "/api/finance/records?view=invoices", "")
	assert.Contains(t, w.Body.String(), `"items":[]`)
	w = request(common.RoleFinanceUser, "GET", "/api/finance/records?view=topups", "")
	assert.Contains(t, w.Body.String(), `"total":3`)
	require.NoError(t, authz.SetUserPermissions(users[common.RoleFinanceUser].Id, authz.PermissionsMap{authz.ResourceFinance: {"read": true, "export": false}}))
	assert.Contains(t, request(common.RoleFinanceUser, "GET", "/api/finance/records", "").Body.String(), `"success":true`)
	assert.Equal(t, http.StatusForbidden, request(common.RoleFinanceUser, "GET", "/api/finance/export", "").Code)
	require.NoError(t, authz.SetUserPermissions(users[common.RoleFinanceUser].Id, authz.PermissionsMap{authz.ResourceFinance: {"read": false, "export": true}}))
	assert.Equal(t, http.StatusForbidden, request(common.RoleFinanceUser, "GET", "/api/finance/records", "").Code)
	assert.Equal(t, http.StatusForbidden, request(common.RoleFinanceUser, "GET", "/api/finance/export", "").Code)
	require.NoError(t, authz.SetUserPermissions(users[common.RoleAdminUser].Id, authz.PermissionsMap{authz.ResourceFinance: {"export": false}}))
	w = request(common.RoleAdminUser, "GET", "/api/finance/export", "")
	assert.Equal(t, http.StatusForbidden, w.Code)
}
