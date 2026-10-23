package controller

import (
	"encoding/csv"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinanceExportIncludesRoutingOnlyForAuthorizedOrders(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.Ticket{}, &model.TicketInvoice{}, &model.Option{}))
	user := model.User{Username: "finance-export", Password: "hashed", AffCode: "finance-export", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	for _, input := range []struct{ trade, method, issuer string }{{"=route-a", "alipay", "Issuer A"}, {"route-b", "wxpay", "Excluded issuer"}} {
		credentials, err := common.Marshal(model.InvoiceEpayConfig{Address: "https://secret.example.test", Key: "hidden-key", GatewayID: "corporate", GatewayName: "Corporate Provider", AccountName: input.issuer})
		require.NoError(t, err)
		order := model.TopUp{UserId: user.Id, TradeNo: input.trade, PaymentMethod: input.method, Money: 500, Status: common.TopUpStatusSuccess, EpayCredentials: string(credentials)}
		require.NoError(t, db.Create(&order).Error)
	}
	policy := model.DefaultFinancePolicy()
	policy.TopUpStatuses = []string{common.TopUpStatusSuccess}
	policy.PaymentMethods = []string{"alipay"}
	require.NoError(t, model.SaveFinancePolicy(policy))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/finance/export?view=topups", nil)
	ExportFinanceRecords(c)
	assert.Equal(t, "1", w.Header().Get("X-Export-Count"))
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "Payment routing / invoice issuer", rows[0][len(rows[0])-1])
	assert.Equal(t, "'=route-a | alipay | Corporate Provider | Issuer A", rows[1][len(rows[1])-1])
	assert.NotContains(t, w.Body.String(), "Excluded issuer")
	assert.NotContains(t, w.Body.String(), "hidden-key")
	assert.NotContains(t, w.Body.String(), "secret.example.test")
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/finance/records?view=topups", nil)
	GetFinanceRecords(c)
	assert.NotContains(t, w.Body.String(), "Issuer A")
	assert.NotContains(t, w.Body.String(), "payment_routing_notes")
}
