package controller

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEpayProductNameSettingsAndSignedPurchase(t *testing.T) {
	db := setupOptionControllerTest(t)
	user := model.User{Username: "payer", DisplayName: "not-the-username"}
	require.NoError(t, db.Create(&user).Error)
	client, err := epay.NewClient(&epay.Config{PartnerID: "merchant", Key: "test-secret"}, "https://pay.example.test")
	require.NoError(t, err)
	callback, err := url.Parse("https://app.example.test/callback")
	require.NoError(t, err)
	for _, tc := range []struct{ template, kind, money, want string }{
		{"", "充值", "9.50", "米醋API(payer)，充值9.50元"},
		{"{username}:{type}:{amount}:{order_no}", "订阅", "20.00", "payer:订阅:20.00:ORDER123"},
		{"{username}:{type}:{amount}:{order_no}", "补税", "0.60", "payer:补税:0.60:ORDER123"},
	} {
		require.NoError(t, model.UpdateOption("EpayProductNameTemplate", tc.template))
		_, params, err := service.PurchaseEpay(client, user.Id, tc.kind, &epay.PurchaseArgs{Money: tc.money, ServiceTradeNo: "ORDER123", Name: "untrusted", Type: "alipay", NotifyUrl: callback, ReturnUrl: callback})
		require.NoError(t, err)
		assert.Equal(t, tc.want, params["name"])
		assert.Equal(t, tc.money, params["money"])
		verified, err := client.Verify(params)
		require.NoError(t, err)
		assert.True(t, verified.VerifyStatus)
	}
	before, exists := optionValueInDB(t, db, "EpayProductNameTemplate")
	require.True(t, exists)
	require.Error(t, model.UpdateOption("EpayProductNameTemplate", "{amount}"))
	require.Error(t, model.UpdateOptionsBulk(map[string]string{"EpayProductNameTemplate": "{username}{amount}{unknown}"}))
	after, exists := optionValueInDB(t, db, "EpayProductNameTemplate")
	require.True(t, exists)
	assert.Equal(t, before, after)
	_, params, err := service.PurchaseEpay(client, user.Id+1, "充值", &epay.PurchaseArgs{Money: "10.00", NotifyUrl: callback, ReturnUrl: callback})
	require.Error(t, err)
	assert.Nil(t, params)
}

func TestEpayProductNamesAcrossLegacySubscriptionAndTax(t *testing.T) {
	setupDiscountControllerTest(t)
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.Ticket{}, &model.TicketInvoice{}, &model.InvoiceTaxCharge{}, &model.RefundInvoiceCorrection{}))
	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
	})
	confirmPaymentComplianceForTest(t)
	address, merchant, key, methods := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods
	price, minTopUp := operation_setting.Price, operation_setting.MinTopUp
	discounts := operation_setting.GetPaymentSetting().AmountDiscount
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://pay.example.test", "merchant", "test-secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	operation_setting.Price, operation_setting.MinTopUp = 2, 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{10: 0.95}
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods = address, merchant, key, methods
		operation_setting.Price, operation_setting.MinTopUp = price, minTopUp
		operation_setting.GetPaymentSetting().AmountDiscount = discounts
	})
	user := model.User{Username: "actual-payer", AffCode: "actual-payer", Group: "epay-test", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	plan := model.SubscriptionPlan{Id: 88123, Title: "Plan", PriceAmount: 12.34, Enabled: true}
	require.NoError(t, db.Create(&plan).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)
	t.Cleanup(func() { model.InvalidateSubscriptionPlanCache(plan.Id) })
	ticket := model.Ticket{UserId: user.Id, Type: model.TicketTypeInvoice, Status: model.TicketStatusOpen}
	require.NoError(t, db.Create(&ticket).Error)
	require.NoError(t, db.Create(&model.TicketInvoice{TicketId: ticket.Id, UserId: user.Id, InvoiceStatus: model.InvoiceStatusPending}).Error)
	require.NoError(t, db.Create(&model.InvoiceTaxCharge{TicketId: ticket.Id, UserId: user.Id, TradeNo: "TAX-DEMO", Status: "pending", FeeCents: 61}).Error)
	for _, tc := range []struct {
		name        string
		handler     gin.HandlerFunc
		body        any
		money, kind string
	}{
		{"legacy", RequestEpay, EpayRequest{Amount: 10, PaymentMethod: "alipay"}, "19.00", "充值"},
		{"subscription", SubscriptionRequestEpay, SubscriptionEpayPayRequest{PlanId: plan.Id, PaymentMethod: "alipay"}, "12.34", "订阅"},
		{"tax", RequestInvoiceTaxPayment, map[string]string{"payment_method": "alipay"}, "0.61", "补税"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := common.Marshal(tc.body)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("id", user.Id)
			c.Set("username", "untrusted-context-name")
			c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(ticket.Id)}}
			c.Request = httptest.NewRequest("POST", "/payment", bytes.NewReader(raw))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("Accept-Language", "en")
			tc.handler(c)
			var response struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response), w.Body.String())
			params := response.Data
			if tc.name == "tax" {
				var ok bool
				params, ok = response.Data["params"].(map[string]any)
				require.True(t, ok, w.Body.String())
			}
			assert.Equal(t, tc.money, params["money"], w.Body.String())
			assert.Equal(t, "米醋API(actual-payer)，"+tc.kind+tc.money+"元", params["name"])
		})
	}
}
