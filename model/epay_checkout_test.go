package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEpayCheckoutFixedPayment(t *testing.T) {
	previous := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previous })
	config := InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600, SpecialRateBps: 1300}
	for _, test := range []struct {
		kind     string
		net, fee int64
		quota    int
	}{
		{"none", 10000, 0, 55555556},
		{"ordinary", 9434, 566, 52411111},
		{"special", 8850, 1150, 49166667},
	} {
		t.Run(test.kind, func(t *testing.T) {
			quote, err := CalculateEpayCheckout(10000, test.kind, config, decimal.NewFromInt(1), 90)
			require.NoError(t, err)
			assert.Equal(t, int64(10000), quote.TotalCents)
			assert.Equal(t, test.net, quote.NetCents)
			assert.Equal(t, test.fee, quote.TaxCents)
			assert.Equal(t, test.quota, quote.Quota)
		})
	}
	_, err := CalculateEpayCheckout(10000, "ordinary", config, decimal.Zero, 90)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	_, err = CalculateEpayCheckout(10000, "ordinary", config, decimal.NewFromInt(1), 0)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	_, err = CalculateEpayCheckout(100000000000, "ordinary", config, decimal.NewFromInt(1), 1)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
}

func TestEpayCheckoutMerchantMethodsAreEnforced(t *testing.T) {
	setupEpayRechargeTest(t)
	oldPayment := *operation_setting.GetPaymentSetting()
	oldPrice, oldMin, oldMethods := operation_setting.Price, operation_setting.MinTopUp, operation_setting.PayMethods
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	operation_setting.Price, operation_setting.MinTopUp = 1, 1
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	t.Cleanup(func() {
		*operation_setting.GetPaymentSetting() = oldPayment
		operation_setting.Price, operation_setting.MinTopUp, operation_setting.PayMethods = oldPrice, oldMin, oldMethods
		require.NoError(t, DB.Where("key IN ?", []string{InvoiceTaxConfigKey, InvoiceEpayConfigKey}).Delete(&Option{}).Error)
	})
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600}))
	config := InvoiceEpayConfig{Address: "https://payment.example.test", PartnerID: "invoice", Key: "secret", PaymentMethods: []string{"wxpay"}}
	require.NoError(t, SaveInvoiceEpayConfig(config))
	methods, err := GetEpayCheckoutMethods(DB, "ordinary")
	require.NoError(t, err)
	assert.Equal(t, []string{"wxpay"}, methods)
	user := insertInvoiceTicketUser(t, 980509)
	req := EpayCheckoutRequest{TotalCents: 10000, InvoiceType: "ordinary", PaymentMethod: "alipay", RequestID: "method-check-request-001"}
	_, err = QuoteEpayCheckout(DB, user.Id, req, false)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	req.PaymentMethod = "wxpay"
	quote, err := QuoteEpayCheckout(DB, user.Id, req, false)
	require.NoError(t, err)
	req.Token = quote.Token
	config.PaymentMethods = []string{}
	require.NoError(t, SaveInvoiceEpayConfig(config))
	_, err = CreateEpayCheckout(user.Id, req)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
	methods, err = GetEpayCheckoutMethods(DB, "ordinary")
	require.NoError(t, err)
	assert.Empty(t, methods)
	config.PaymentMethods = []string{"stripe"}
	assert.ErrorIs(t, SaveInvoiceEpayConfig(config), ErrInvoiceTaxInvalid)
	_, err = GetEpayCheckoutMethods(DB, "invalid")
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
}

func TestEpayCheckoutQuoteReservationPinsRatesAndMerchant(t *testing.T) {
	setupEpayRechargeTest(t)
	oldPayment := *operation_setting.GetPaymentSetting()
	oldCode := *operation_setting.GetDiscountCodeSetting()
	oldPrice, oldMin, oldMethods := operation_setting.Price, operation_setting.MinTopUp, operation_setting.PayMethods
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	operation_setting.GetDiscountCodeSetting().Enabled = true
	operation_setting.Price, operation_setting.MinTopUp = 1, 1
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	t.Cleanup(func() {
		*operation_setting.GetPaymentSetting() = oldPayment
		*operation_setting.GetDiscountCodeSetting() = oldCode
		operation_setting.Price, operation_setting.MinTopUp, operation_setting.PayMethods = oldPrice, oldMin, oldMethods
		require.NoError(t, DB.Where("key IN ?", []string{InvoiceTaxConfigKey, InvoiceEpayConfigKey}).Delete(&Option{}).Error)
	})
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600}))
	require.NoError(t, SaveInvoiceEpayConfig(InvoiceEpayConfig{Address: "https://payment.example.test", PartnerID: "invoice", Key: "old-key"}))
	user := insertInvoiceTicketUser(t, 980504)
	code := DiscountCode{Code: "checkout-90", DiscountRate: 90, Status: DiscountCodeStatusEnabled, MaxUsesPerUser: 1}
	require.NoError(t, DB.Create(&code).Error)
	req := EpayCheckoutRequest{TotalCents: 10000, InvoiceType: "ordinary", PaymentMethod: "alipay", DiscountCode: code.Code, RequestID: "checkout-request-0001"}
	quote, err := QuoteEpayCheckout(DB, user.Id, req, false)
	require.NoError(t, err)
	req.Token = quote.Token
	require.NoError(t, DB.Model(&code).Update("discount_rate", 80).Error)
	_, err = CreateEpayCheckout(user.Id, req)
	assert.ErrorIs(t, err, ErrInvoiceTaxQuoteChanged)
	require.NoError(t, DB.Model(&code).Update("discount_rate", 90).Error)
	order, err := CreateEpayCheckout(user.Id, req)
	require.NoError(t, err)
	duplicate, err := CreateEpayCheckout(user.Id, req)
	require.NoError(t, err)
	assert.Equal(t, order.Id, duplicate.Id)
	req.RequestID = "checkout-request-0002"
	_, err = CreateEpayCheckout(user.Id, req)
	require.Error(t, err)
	require.NoError(t, SaveInvoiceEpayConfig(InvoiceEpayConfig{Address: "https://payment.example.test", PartnerID: "invoice", Key: "rotated-key"}))
	require.NoError(t, DB.First(order, order.Id).Error)
	var credentials InvoiceEpayConfig
	require.NoError(t, common.UnmarshalJsonStr(order.EpayCredentials, &credentials))
	assert.Equal(t, "old-key", credentials.Key)
	_, credited, err := CompleteEpayCheckout(order.TradeNo, "alipay", 10000)
	require.NoError(t, err)
	assert.Equal(t, quote.Quota, credited)
	var bonus TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo+"_bonus").Take(&bonus).Error)
	assert.Equal(t, int64(quote.Quota-quote.BaseQuota), bonus.QuotaGranted)
}

func TestEpayCheckoutSettlementAndManualCompletionAreIdempotent(t *testing.T) {
	setupEpayRechargeTest(t)
	user, order := insertEpayRechargeFixture(t, "checkout-idempotent", PaymentProviderEpay, common.TopUpStatusPending, 100, 100, 0)
	order.CheckoutVersion, order.InvoiceType = 1, "ordinary"
	order.InvoiceNetCents, order.InvoiceTaxCents, order.PaymentTotalCents = 9434, 566, 10000
	order.CheckoutBaseQuota, order.CheckoutQuota = 943400, 943400
	require.NoError(t, DB.Save(order).Error)
	_, credit, err := CompleteEpayCheckout(order.TradeNo, "alipay", 9999)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	assert.Zero(t, credit)
	_, _, err = CompleteEpayCheckout(order.TradeNo, "wxpay", 10000)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	_, _, err = RechargeEpay(order.TradeNo, "alipay")
	assert.ErrorIs(t, err, ErrPaymentMethodMismatch)
	var concurrent sync.WaitGroup
	results := make(chan error, 2)
	concurrent.Add(2)
	go func() { defer concurrent.Done(); results <- ManualCompleteTopUp(order.TradeNo, "127.0.0.1") }()
	go func() {
		defer concurrent.Done()
		_, _, err := CompleteEpayCheckout(order.TradeNo, "alipay", 10000)
		results <- err
	}()
	concurrent.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	_, credit, err = CompleteEpayCheckout(order.TradeNo, "alipay", 10000)
	require.NoError(t, err)
	assert.Zero(t, credit)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 100+order.CheckoutQuota, user.Quota)
	require.NoError(t, DB.First(order, order.Id).Error)
	assert.Equal(t, int64(order.CheckoutBaseQuota), order.QuotaGranted)
}

func TestEpayCheckoutWalletFailureRollsBackOrderAndBonus(t *testing.T) {
	setupEpayRechargeTest(t)
	user, order := insertEpayRechargeFixture(t, "checkout-rollback", PaymentProviderEpay, common.TopUpStatusPending, 100, 100, 1)
	order.CheckoutVersion, order.InvoiceType = 1, "ordinary"
	order.InvoiceNetCents, order.InvoiceTaxCents, order.PaymentTotalCents = 9434, 566, 10000
	order.CheckoutBaseQuota, order.CheckoutQuota, order.DiscountRate = 900, 1000, 90
	require.NoError(t, DB.Save(order).Error)
	injected := errors.New("wallet write failed")
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("test:checkout_wallet_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(injected)
		}
	}))
	t.Cleanup(func() { DB.Callback().Update().Remove("test:checkout_wallet_failure") })
	_, credit, err := CompleteEpayCheckout(order.TradeNo, "alipay", 10000)
	assert.ErrorIs(t, err, injected)
	assert.Zero(t, credit)
	require.NoError(t, DB.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, order.QuotaGranted)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", order.TradeNo+"_bonus").Count(&count).Error)
	assert.Zero(t, count)
}

func TestPrepaidInvoiceTypeFilteringAndNoSecondTaxCharge(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	user := insertInvoiceTicketUser(t, 980503)
	ordinary := &TopUp{UserId: user.Id, TradeNo: "prepaid-ordinary", Status: common.TopUpStatusSuccess, Money: 100, CheckoutVersion: 1, InvoiceType: "ordinary", InvoiceNetCents: 9434, InvoiceTaxCents: 566, PaymentTotalCents: 10000, PaymentProvider: PaymentProviderEpay}
	special := &TopUp{UserId: user.Id, TradeNo: "prepaid-special", Status: common.TopUpStatusSuccess, Money: 100, CheckoutVersion: 1, InvoiceType: "special", InvoiceNetCents: 8850, InvoiceTaxCents: 1150, PaymentTotalCents: 10000, PaymentProvider: PaymentProviderEpay}
	none := &TopUp{UserId: user.Id, TradeNo: "prepaid-none", Status: common.TopUpStatusSuccess, Money: 100}
	for _, order := range []*TopUp{ordinary, special, none} {
		require.NoError(t, DB.Create(order).Error)
	}
	orders, err := GetEligibleInvoiceOrders(user.Id, "ordinary")
	require.NoError(t, err)
	require.Len(t, orders, 1)
	assert.Equal(t, ordinary.Id, orders[0].Id)
	params := createInvoiceTicketParams(user, []int{ordinary.Id, special.Id})
	params.InvoiceType = "ordinary"
	_, _, _, _, err = CreateInvoiceTicket(params)
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	params.TopUpOrderIds = []int{none.Id}
	_, _, _, _, err = CreateInvoiceTicket(params)
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	params.TopUpOrderIds = []int{ordinary.Id}
	ticket, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	assert.Equal(t, "ordinary", invoice.InvoiceType)
	assert.Equal(t, int64(566), invoice.InvoiceTaxCents)
	assert.Equal(t, float64(100), invoice.TotalMoney)
	assert.Nil(t, invoice.TaxCharge)
	_, err = CloseUserTicket(ticket.Id, user.Id)
	require.NoError(t, err)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Zero(t, user.Quota)
	orders, err = GetEligibleInvoiceOrders(user.Id, "ordinary")
	require.NoError(t, err)
	require.Len(t, orders, 1)
}
