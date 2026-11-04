package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoicePolicyPaymentAndRefundExamples(t *testing.T) {
	config := InvoiceTaxConfig{ModuleEnabled: true, PolicyEffectiveAt: InvoicePolicyEffectiveAt}
	for _, tc := range []struct {
		kind     string
		net, fee int64
	}{{"ordinary", 9400, 600}, {"special", 9100, 900}, {"none", 10000, 0}} {
		t.Run(tc.kind, func(t *testing.T) {
			before, err := CalculateEpayCheckout(10000, tc.kind, config.CheckoutAt(InvoicePolicyEffectiveAt-1), decimal.NewFromInt(1), 100)
			require.NoError(t, err)
			assert.Equal(t, int64(10000), before.NetCents)
			assert.Zero(t, before.TaxCents)
			for _, at := range []int64{InvoicePolicyEffectiveAt, InvoicePolicyEffectiveAt + 1} {
				quote, err := CalculateEpayCheckout(10000, tc.kind, config.CheckoutAt(at), decimal.NewFromInt(1), 100)
				require.NoError(t, err)
				assert.Equal(t, tc.net, quote.NetCents)
				assert.Equal(t, tc.fee, quote.TaxCents)
				assert.Equal(t, 600, quote.VATRateBps)
				assert.Equal(t, int64(10000), refundProportionalCents(10000, int64(quote.BaseQuota), int64(quote.BaseQuota)))
				assert.Equal(t, int64(5000), refundProportionalCents(10000, int64(quote.BaseQuota)/2, int64(quote.BaseQuota)))
				assert.Equal(t, tc.fee/2, refundProportionalCents(tc.fee, 5000, 10000))
			}
		})
	}
	quote, err := CalculateEpayCheckout(25, "ordinary", config.CheckoutAt(InvoicePolicyEffectiveAt), decimal.NewFromInt(1), 100)
	require.NoError(t, err)
	assert.Equal(t, int64(2), quote.TaxCents)
	assert.Equal(t, int64(23), quote.NetCents)
}

func TestInvoicePolicyUpgradeAndHistoricalFreeOrdinaryInvoice(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	// An existing option without the new field adopts the published policy.
	require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	require.NoError(t, DB.Create(&Option{Key: InvoiceTaxConfigKey, Value: `{"module_enabled":true,"mode":"vat","enabled":true,"ordinary_rate_bps":600,"special_rate_bps":1300}`}).Error)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	assert.Equal(t, InvoicePolicyEffectiveAt, config.LegacyInvoiceBefore)
	assert.Equal(t, "gross_up", config.Mode)
	assert.Equal(t, 900, config.SpecialRateBps)
	user := insertInvoiceTicketUser(t, 980701)
	for _, kind := range []string{"", "none", "ordinary", "special"} {
		id := seedInvoiceTicketTopUp(t, user.Id, "policy-legacy-"+kind, 100)
		require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", id).Updates(map[string]any{"invoice_type": kind, "complete_time": InvoicePolicyEffectiveAt - 1, "payment_provider": PaymentProviderEpay}).Error)
		quote, err := QuoteInvoiceTax(DB, user.Id, []int{id}, "ordinary")
		require.NoError(t, err)
		assert.Zero(t, quote.FeeCents)
		assert.Zero(t, quote.Quota)
		assert.Equal(t, int64(10000), quote.TotalCents)
		assert.True(t, quote.PrepaidOnly)
		_, err = QuoteInvoiceTax(DB, user.Id, []int{id}, "special")
		assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
		params := createInvoiceTicketParams(user, []int{id})
		params.InvoiceType = "ordinary"
		_, invoice, _, _, err := CreateInvoiceTicket(params)
		require.NoError(t, err)
		assert.Equal(t, float64(100), invoice.TotalMoney)
		assert.Equal(t, 600, invoice.VATRateBps)
		assert.Nil(t, invoice.TaxCharge)
	}
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Zero(t, current.Quota)
	items, total, err := ListInvoicesForExport(InvoiceExportFilter{}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	for _, item := range items {
		assert.Equal(t, 600, item.TaxRateBps)
		assert.Zero(t, item.TaxFeeCents)
	}
	for _, completed := range []int64{0, InvoicePolicyEffectiveAt, InvoicePolicyEffectiveAt + 1} {
		order := TopUp{UserId: user.Id, TradeNo: common.GetRandomString(32), Money: 100, InvoiceType: "none", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: completed}
		require.NoError(t, DB.Create(&order).Error)
		_, err := QuoteInvoiceTax(DB, user.Id, []int{order.Id}, "ordinary")
		assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	}
}

func TestInvoicePolicyPaymentCompletionBoundaryAndReplay(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 980702)
	config := InvoiceTaxConfig{PolicyEffectiveAt: InvoicePolicyEffectiveAt}
	for _, tc := range []struct {
		kind     string
		paidAt   int64
		net, fee int64
	}{{"ordinary", InvoicePolicyEffectiveAt - 1, 10000, 0}, {"ordinary", InvoicePolicyEffectiveAt, 9400, 600}, {"special", InvoicePolicyEffectiveAt + 1, 9100, 900}} {
		quote, err := calculateEpayCheckout(10000, tc.kind, config.CheckoutAt(InvoicePolicyEffectiveAt-10), decimal.NewFromInt(1), 100, decimal.NewFromInt(100))
		require.NoError(t, err)
		order := TopUp{UserId: user.Id, TradeNo: common.GetRandomString(32), Status: common.TopUpStatusPending, PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", CheckoutVersion: 1, InvoiceType: tc.kind, Money: 100, PaymentTotalCents: 10000, InvoiceNetCents: quote.NetCents, CheckoutBaseQuota: quote.BaseQuota, CheckoutQuota: quote.Quota, CheckoutPolicyEffectiveAt: quote.PolicyEffectiveAt, CheckoutUnitPrice: quote.UnitPrice, CheckoutQuotaPerUnit: quote.QuotaPerUnit, DiscountRate: 100, CreateTime: InvoicePolicyEffectiveAt - 10}
		require.NoError(t, DB.Create(&order).Error)
		completed, credited, err := completeEpayCheckoutAt(order.TradeNo, "alipay", 10000, tc.paidAt)
		require.NoError(t, err)
		assert.Equal(t, tc.net, completed.InvoiceNetCents)
		assert.Equal(t, tc.fee, completed.InvoiceTaxCents)
		assert.Equal(t, int(tc.net), credited)
		assert.Equal(t, tc.paidAt, completed.CompleteTime)
		duplicate, credited, err := completeEpayCheckoutAt(order.TradeNo, "alipay", 10000, InvoicePolicyEffectiveAt+100)
		require.NoError(t, err)
		assert.Zero(t, credited)
		assert.Equal(t, tc.net, duplicate.InvoiceNetCents)
		assert.Equal(t, tc.paidAt, duplicate.CompleteTime)
	}
}

func TestInvoicePolicyUpgradesPendingCheckoutWithoutPriceSnapshot(t *testing.T) {
	truncateTables(t)
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, PolicyEffectiveAt: InvoicePolicyEffectiveAt}))
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	user := insertInvoiceTicketUser(t, 980703)
	order := TopUp{UserId: user.Id, TradeNo: "policy-pending-before-upgrade", Status: common.TopUpStatusExpired, PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", CheckoutVersion: 1, InvoiceType: "ordinary", Money: 100, PaymentTotalCents: 10000, InvoiceNetCents: 9434, InvoiceTaxCents: 566, CheckoutBaseQuota: 943400, CheckoutQuota: 943400, CreateTime: InvoicePolicyEffectiveAt - 86400}
	require.NoError(t, DB.Create(&order).Error)
	completed, credited, err := completeEpayCheckoutAt(order.TradeNo, "alipay", 10000, InvoicePolicyEffectiveAt)
	require.NoError(t, err)
	assert.Equal(t, int64(9400), completed.InvoiceNetCents)
	assert.Equal(t, int64(600), completed.InvoiceTaxCents)
	assert.Equal(t, 940000, credited)
	_, err = UpdateTopUpInvoiceType(order.Id, "ordinary", "special")
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
}
