package controller

import (
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
