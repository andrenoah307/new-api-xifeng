package controller

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEpayCheckoutMethodsExposeOnlyConfiguredMethods(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	confirmPaymentComplianceForTest(t)
	require.NoError(t, model.SaveInvoiceEpayConfig(model.InvoiceEpayConfig{
		Address: "https://payment.example.test", PartnerID: "invoice-merchant", Key: "invoice-secret", PaymentMethods: []string{"wxpay"},
	}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/user/epay/checkout/methods?invoice_type=ordinary", nil)
	GetEpayCheckoutMethods(c)
	var response struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, []string{"wxpay"}, response.Data)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.NotContains(t, w.Body.String(), "invoice-secret")
	assert.NotContains(t, w.Body.String(), "invoice-merchant")
	assert.NotContains(t, w.Body.String(), "payment.example.test")
}

func TestEpayCheckoutCallbackPinsMerchantAndAcknowledgesCommit(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.SaveInvoiceTaxConfig(model.InvoiceTaxConfig{Mode: "vat"}))
	user := model.User{Username: "checkout-callback", Password: "hashed", Status: common.UserStatusEnabled, AffCode: "checkout-callback"}
	require.NoError(t, db.Create(&user).Error)
	credentials, err := common.Marshal(model.InvoiceEpayConfig{Address: "https://payment.example.test", PartnerID: "invoice-merchant", Key: "invoice-secret"})
	require.NoError(t, err)
	order := model.TopUp{UserId: user.Id, TradeNo: "checkout-callback", Status: common.TopUpStatusPending, PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Money: 100, CheckoutVersion: 1, InvoiceType: "ordinary", InvoiceNetCents: 9434, InvoiceTaxCents: 566, PaymentTotalCents: 10000, CheckoutBaseQuota: 943400, CheckoutQuota: 943400, EpayCredentials: string(credentials)}
	require.NoError(t, db.Create(&order).Error)
	for _, tc := range []struct {
		name, money, merchant, key, method, want string
		failDB                                   bool
	}{
		{"wrong merchant", "100.00", "other", "invoice-secret", "alipay", "fail", false},
		{"wrong signature", "100.00", "invoice-merchant", "other", "alipay", "fail", false},
		{"wrong amount", "99.99", "invoice-merchant", "invoice-secret", "alipay", "fail", false},
		{"amount above checkout limit", "1000000000.01", "invoice-merchant", "invoice-secret", "alipay", "fail", false},
		{"wrong method", "100.00", "invoice-merchant", "invoice-secret", "wxpay", "fail", false},
		{"database failure", "100.00", "invoice-merchant", "invoice-secret", "alipay", "fail", true},
		{"paid", "100.00", "invoice-merchant", "invoice-secret", "alipay", "success", false},
		{"duplicate", "100.00", "invoice-merchant", "invoice-secret", "alipay", "success", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.failDB {
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:checkout_fail", func(tx *gorm.DB) {
					if tx.Statement.Table == "users" {
						tx.AddError(errors.New("write failed"))
					}
				}))
				t.Cleanup(func() { db.Callback().Update().Remove("test:checkout_fail") })
			}
			params := epay.GenerateParams(map[string]string{"pid": tc.merchant, "out_trade_no": order.TradeNo, "type": tc.method, "trade_status": "TRADE_SUCCESS", "money": tc.money}, tc.key)
			values := url.Values{}
			for key, value := range params {
				values.Set(key, value)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/user/epay/checkout/notify?"+values.Encode(), nil)
			EpayCheckoutNotify(c)
			assert.Equal(t, tc.want, w.Body.String())
			var current model.User
			require.NoError(t, db.First(&current, user.Id).Error)
			if tc.want == "fail" {
				assert.Zero(t, current.Quota)
			} else {
				assert.Equal(t, order.CheckoutQuota, current.Quota)
			}
		})
	}
	raw, err := common.Marshal(order)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "invoice-secret")
}

func TestEpayCheckoutLargePaymentHTTP(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.TopUp{}, &model.Log{}))
	confirmPaymentComplianceForTest(t)
	oldUnit, oldPrice, oldMin := common.QuotaPerUnit, operation_setting.Price, operation_setting.MinTopUp
	oldDiscounts := operation_setting.GetPaymentSetting().AmountDiscount
	common.QuotaPerUnit, operation_setting.Price, operation_setting.MinTopUp = 500000, 1, 50
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	t.Cleanup(func() {
		common.QuotaPerUnit, operation_setting.Price, operation_setting.MinTopUp = oldUnit, oldPrice, oldMin
		operation_setting.GetPaymentSetting().AmountDiscount = oldDiscounts
	})
	require.NoError(t, model.SaveInvoiceTaxConfig(model.InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "gross_up", OrdinaryRateBps: 600}))
	require.NoError(t, model.SaveInvoiceEpayConfig(model.InvoiceEpayConfig{
		Strategy: "round_robin", PrivateBelowCents: 500000, PrivateGatewayName: "Private", Gateways: []model.InvoiceEpayGateway{
			{ID: "corporate", Name: "Corporate", AccountName: "Issuer", Address: "https://payment.example.test", PartnerID: "corporate", Key: "test-key", PaymentMethods: []string{"alipay"}},
		},
	}))
	user := model.User{Username: "large-checkout-http", Password: "hashed", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	req := model.EpayCheckoutRequest{TotalCents: 10000001, InvoiceType: "ordinary", PaymentMethod: "alipay", RequestID: "large-checkout-http"}
	raw, err := common.Marshal(req)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", user.Id)
	c.Request = httptest.NewRequest("POST", "/api/user/epay/checkout/quote", bytes.NewReader(raw))
	QuoteEpayCheckout(c)
	var quoted struct {
		Success bool                    `json:"success"`
		Data    model.EpayCheckoutQuote `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &quoted))
	require.True(t, quoted.Success, w.Body.String())
	assert.Equal(t, 47000005000, quoted.Data.Quota)
	assert.Equal(t, "94000.01", quoted.Data.CreditAmount)
	req.Token = quoted.Data.Token
	raw, err = common.Marshal(req)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Set("id", user.Id)
	c.Request = httptest.NewRequest("POST", "/api/user/epay/checkout/pay", bytes.NewReader(raw))
	RequestEpayCheckout(c)
	var payment struct {
		Success bool `json:"success"`
		Data    struct {
			URL    string            `json:"url"`
			Params map[string]string `json:"params"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &payment))
	require.True(t, payment.Success, w.Body.String())
	assert.Equal(t, "https://payment.example.test/submit.php", payment.Data.URL)
	assert.Equal(t, "100000.01", payment.Data.Params["money"])
	assert.Equal(t, "corporate", payment.Data.Params["pid"])
	params := epay.GenerateParams(map[string]string{"pid": "corporate", "out_trade_no": payment.Data.Params["out_trade_no"], "type": "alipay", "trade_status": "TRADE_SUCCESS", "money": "100000.01"}, "test-key")
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	for _, attempt := range []string{"paid", "replay"} {
		t.Run(attempt, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/user/epay/checkout/notify?"+values.Encode(), nil)
			EpayCheckoutNotify(c)
			assert.Equal(t, "success", w.Body.String())
			require.NoError(t, db.First(&user, user.Id).Error)
			assert.Equal(t, 47000005000, user.Quota)
		})
	}
}

func TestLegacyEpayCallbackRejectsAmountMismatchAndWriteFailure(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.TopUp{}, &model.Log{}))
	confirmPaymentComplianceForTest(t)
	address, id, key, methods := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://payment.example.test", "legacy-merchant", "legacy-secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods = address, id, key, methods
	})
	user := model.User{Username: "legacy-callback", Password: "hashed", Status: common.UserStatusEnabled, AffCode: "legacy-callback"}
	require.NoError(t, db.Create(&user).Error)
	order := model.TopUp{UserId: user.Id, TradeNo: "legacy-callback", Status: common.TopUpStatusPending, PaymentProvider: model.PaymentProviderEpay, PaymentMethod: "alipay", Money: 100, Amount: 100}
	require.NoError(t, db.Create(&order).Error)
	for _, tc := range []struct {
		name, money, want string
		failDB            bool
	}{
		{"wrong amount", "99.99", "fail", false},
		{"database failure", "100.00", "fail", true},
		{"settled", "100.00", "success", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.failDB {
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:legacy_fail", func(tx *gorm.DB) {
					if tx.Statement.Table == "users" {
						tx.AddError(errors.New("write failed"))
					}
				}))
				t.Cleanup(func() { db.Callback().Update().Remove("test:legacy_fail") })
			}
			params := epay.GenerateParams(map[string]string{"pid": "legacy-merchant", "out_trade_no": order.TradeNo, "type": "alipay", "trade_status": "TRADE_SUCCESS", "money": tc.money}, "legacy-secret")
			values := url.Values{}
			for key, value := range params {
				values.Set(key, value)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/user/epay/notify?"+values.Encode(), nil)
			EpayNotify(c)
			assert.Equal(t, tc.want, w.Body.String())
			var current model.User
			require.NoError(t, db.First(&current, user.Id).Error)
			if tc.want == "fail" {
				assert.Zero(t, current.Quota)
			} else {
				assert.Equal(t, 100*int(common.QuotaPerUnit), current.Quota)
			}
		})
	}
}
