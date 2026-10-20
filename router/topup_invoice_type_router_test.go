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

func TestTopUpInvoiceTypeWriteIsAdminOnly(t *testing.T) {
	db := setupMonitoringRouterTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.TicketInvoice{}, &model.Log{}))
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("topup-invoice-type-test"))))
	SetApiRouter(engine)
	for _, role := range []int{common.RoleCommonUser, common.RoleFinanceUser, common.RoleCustomerServiceUser, common.RoleAdminUser, common.RoleRootUser} {
		token := fmt.Sprintf("invoice-type-%d", role)
		user := model.User{Username: token, Password: "password", Role: role, Status: common.UserStatusEnabled, AffCode: token, AccessToken: &token}
		require.NoError(t, db.Create(&user).Error)
		order := model.TopUp{UserId: user.Id, TradeNo: token, Status: common.TopUpStatusSuccess, InvoiceType: "ordinary", PaymentProvider: model.PaymentProviderEpay, Money: 100}
		require.NoError(t, db.Create(&order).Error)
		body := fmt.Sprintf(`{"id":%d,"invoice_type":"none","expected_invoice_type":"ordinary"}`, order.Id)
		req := httptest.NewRequest("PUT", "/api/user/topup/invoice-type", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		require.NoError(t, db.First(&order, order.Id).Error)
		if role >= common.RoleAdminUser {
			assert.Contains(t, w.Body.String(), `"success":true`)
			assert.Equal(t, "none", order.InvoiceType)
		} else {
			assert.NotContains(t, w.Body.String(), `"success":true`)
			assert.Equal(t, "ordinary", order.InvoiceType)
		}
	}
}
