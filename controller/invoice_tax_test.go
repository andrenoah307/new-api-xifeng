package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceTaxCallbackVerifiesAmountMerchantAndDurableSettlement(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.Ticket{}, &model.TicketInvoice{}, &model.TicketMessage{}, &model.InvoiceTaxCharge{}, &model.Log{}))
	address, id, key := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	operation_setting.PayAddress = "https://payment.example.test"
	operation_setting.EpayId = "tax-test-merchant"
	operation_setting.EpayKey = "tax-test-secret"
	t.Cleanup(func() {
		operation_setting.PayAddress = address
		operation_setting.EpayId = id
		operation_setting.EpayKey = key
	})
	user := model.User{Username: "tax-webhook", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "tax-webhook"}
	require.NoError(t, db.Create(&user).Error)
	ticket := model.Ticket{UserId: user.Id, Type: model.TicketTypeInvoice, Status: model.TicketStatusOpen}
	require.NoError(t, db.Create(&ticket).Error)
	invoice := model.TicketInvoice{TicketId: ticket.Id, UserId: user.Id, TopUpOrderIds: "[]", InvoiceStatus: model.InvoiceStatusPending}
	require.NoError(t, db.Create(&invoice).Error)
	charge := model.InvoiceTaxCharge{TicketId: ticket.Id, UserId: user.Id, TradeNo: "ITAX-test", Status: "pending", PaymentMethod: "alipay", FeeCents: 60000, Quota: 300000000}
	require.NoError(t, db.Create(&charge).Error)
	for _, tc := range []struct{ name, money, merchant, sign, method, want string }{
		{"wrong amount", "599.99", "tax-test-merchant", "", "GET", "fail"},
		{"fractional cent", "600.001", "tax-test-merchant", "", "GET", "fail"},
		{"invalid signature", "600.00", "tax-test-merchant", "invalid", "GET", "fail"},
		{"wrong merchant", "600.00", "other", "", "GET", "fail"},
		{"valid POST", "600.00", "tax-test-merchant", "", "POST", "success"},
		{"duplicate GET", "600.00", "tax-test-merchant", "", "GET", "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := epay.GenerateParams(map[string]string{"pid": tc.merchant, "type": "alipay", "out_trade_no": charge.TradeNo, "trade_status": "TRADE_SUCCESS", "money": tc.money}, operation_setting.EpayKey)
			if tc.sign != "" {
				params["sign"] = tc.sign
			}
			values := url.Values{}
			for k, v := range params {
				values.Set(k, v)
			}
			request := httptest.NewRequest(tc.method, "/api/invoice-tax/epay/notify?"+values.Encode(), nil)
			if tc.method == "POST" {
				request = httptest.NewRequest(http.MethodPost, "/api/invoice-tax/epay/notify", strings.NewReader(values.Encode()))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = request
			InvoiceTaxEpayNotify(c)
			assert.Equal(t, tc.want, w.Body.String())
		})
	}
	var current model.User
	require.NoError(t, db.First(&current, user.Id).Error)
	assert.Zero(t, current.Quota)
	require.NoError(t, db.First(&charge, charge.Id).Error)
	assert.Equal(t, "paid", charge.Status)
	var messages int64
	require.NoError(t, db.Model(&model.TicketMessage{}).Where("ticket_id = ?", ticket.Id).Count(&messages).Error)
	assert.Equal(t, int64(1), messages)
}
