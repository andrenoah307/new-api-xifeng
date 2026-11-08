package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomInvoiceCheckoutKeepsSnapshotAfterSettingsChange(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	config := InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "gross_up", OrdinaryRateBps: 350, SpecialRateBps: 725}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	quote, err := calculateEpayCheckout(10000, "ordinary", config, decimal.NewFromInt(1), 100, decimal.NewFromInt(100))
	require.NoError(t, err)
	user := insertInvoiceTicketUser(t, 980705)
	order := TopUp{UserId: user.Id, TradeNo: "custom-policy-snapshot", Status: common.TopUpStatusPending, PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", CheckoutVersion: 1, InvoiceType: "ordinary", Money: 100, PaymentTotalCents: quote.TotalCents, InvoiceNetCents: quote.NetCents, InvoiceTaxCents: quote.TaxCents, InvoiceRateBps: quote.RateBps, InvoiceTaxMode: quote.Mode, CheckoutBaseQuota: quote.BaseQuota, CheckoutQuota: quote.Quota, DiscountRate: 100}
	require.NoError(t, DB.Create(&order).Error)
	config.OrdinaryRateBps = 1200
	require.NoError(t, SaveInvoiceTaxConfig(config))
	completed, credited, err := completeEpayCheckoutAt(order.TradeNo, "alipay", 10000, InvoicePolicyEffectiveAt+86400)
	require.NoError(t, err)
	assert.Equal(t, int64(9650), completed.InvoiceNetCents)
	assert.Equal(t, int64(350), completed.InvoiceTaxCents)
	assert.Equal(t, 9650, credited)
	_, credited, err = completeEpayCheckoutAt(order.TradeNo, "alipay", 10000, InvoicePolicyEffectiveAt+2*86400)
	require.NoError(t, err)
	assert.Zero(t, credited)
}

func TestCustomInvoicePolicyPreservesSettingsAndHistoricalSwitch(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	cutoff := InvoicePolicyEffectiveAt + 7*86400
	config := InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "gross_up", OrdinaryRateBps: 350, SpecialRateBps: 725, LegacyInvoiceBefore: cutoff, LegacyFreeOrdinaryEnabled: true}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	loaded, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.ReconciliationReviewCount = 1
	config.RefundGiftRecoveryPolicy = RefundGiftPriorExpiry
	assert.Equal(t, config, loaded)
	for _, tc := range []struct {
		kind string
		fee  int64
	}{{"ordinary", 350}, {"special", 725}} {
		quote, err := CalculateEpayCheckout(10000, tc.kind, loaded.CheckoutAt(InvoicePolicyEffectiveAt-1), decimal.NewFromInt(1), 100)
		require.NoError(t, err)
		assert.Equal(t, tc.fee, quote.TaxCents)
		assert.Equal(t, int64(0), quote.PolicyEffectiveAt)
		assert.Equal(t, 600, quote.VATRateBps)
	}
	user := insertInvoiceTicketUser(t, 980704)
	id := seedInvoiceTicketTopUp(t, user.Id, "custom-free-ordinary", 100)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", id).Updates(map[string]any{"complete_time": cutoff - 1, "payment_provider": PaymentProviderEpay}).Error)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{id}, "ordinary")
	require.NoError(t, err)
	assert.Zero(t, quote.FeeCents)
	eligible, err := GetEligibleInvoiceOrders(user.Id, "ordinary")
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	assert.Equal(t, id, eligible[0].Id)
	eligible, err = GetEligibleInvoiceOrders(user.Id, "special")
	require.NoError(t, err)
	assert.Empty(t, eligible)
	_, err = QuoteInvoiceTax(DB, user.Id, []int{id}, "special")
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	config.LegacyFreeOrdinaryEnabled = false
	require.NoError(t, SaveInvoiceTaxConfig(config))
	quote, err = QuoteInvoiceTax(DB, user.Id, []int{id}, "ordinary")
	require.NoError(t, err)
	assert.Positive(t, quote.FeeCents)
	config.LegacyFreeOrdinaryEnabled = true
	config.LegacyInvoiceBefore = 0
	require.NoError(t, SaveInvoiceTaxConfig(config))
	_, err = QuoteInvoiceTax(DB, user.Id, []int{id}, "ordinary")
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	assert.False(t, config.AllowsLegacyInvoice(&TopUp{CompleteTime: cutoff - 1}))
	config.LegacyInvoiceBefore = cutoff
	assert.False(t, config.AllowsLegacyInvoice(&TopUp{CompleteTime: cutoff}))
	assert.False(t, config.AllowsLegacyInvoice(&TopUp{CompleteTime: 0}))
	config.Enabled = false
	quote, err = CalculateInvoiceTaxFromTotal(10000, "ordinary", config)
	require.NoError(t, err)
	assert.Zero(t, quote.FeeCents)
}
