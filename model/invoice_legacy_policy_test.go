package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyInvoicePolicyDefaultsClosedAndCannotBypass(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	})
	user := insertInvoiceTicketUser(t, 980501)
	orderID := seedInvoiceTicketTopUp(t, user.Id, "legacy-policy-disabled", 100)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", orderID).Update("payment_provider", PaymentProviderEpay).Error)
	for _, taxEnabled := range []bool{false, true} {
		config, err := GetInvoiceTaxConfig(DB)
		require.NoError(t, err)
		assert.Zero(t, config.LegacyInvoiceBefore)
		config.Enabled = taxEnabled
		config.ModuleEnabled = true
		config.LegacyTopUpsEnabled = true
		require.NoError(t, SaveInvoiceTaxConfig(config))
		orders, err := GetEligibleInvoiceOrders(user.Id, "")
		require.NoError(t, err)
		assert.Empty(t, orders)
		_, err = QuoteInvoiceTax(DB, user.Id, []int{orderID}, "ordinary")
		assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
		params := createInvoiceTicketParams(user, []int{orderID})
		params.InvoiceType = "ordinary"
		_, _, _, _, err = CreateInvoiceTicket(params)
		assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	}
	var count int64
	require.NoError(t, DB.Model(&TicketInvoice{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestLegacyInvoicePolicyClosurePreservesSettlementRefundAndIssuedStatus(t *testing.T) {
	truncateTables(t)
	config := InvoiceTaxConfig{ModuleEnabled: true, Mode: "vat", Enabled: true, LegacyTopUpsEnabled: true, LegacyInvoiceBefore: 4102416000, OrdinaryRateBps: 600}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	t.Cleanup(func() {
		require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
		require.NoError(t, DB.Where("user_id = ?", 980502).Delete(&InvoiceTaxCharge{}).Error)
	})
	user := insertInvoiceTicketUser(t, 980502)
	orderID := seedInvoiceTicketTopUp(t, user.Id, "legacy-policy-pending", 100)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", orderID).Update("payment_provider", PaymentProviderEpay).Error)
	orders, err := GetEligibleInvoiceOrders(user.Id, "")
	require.NoError(t, err)
	require.Len(t, orders, 1)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{orderID}, "ordinary")
	require.NoError(t, err)
	params := createInvoiceTicketParams(user, []int{orderID})
	params.InvoiceType, params.TaxPaymentMethod, params.TaxQuote = "ordinary", "online", quote.Token
	ticket, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	require.NotNil(t, invoice.TaxCharge)
	require.NoError(t, DB.Model(invoice.TaxCharge).Update("payment_method", "alipay").Error)
	issued := &TicketInvoice{TicketId: ticket.Id + 1000, UserId: user.Id, CompanyName: "Issued", TaxNumber: "91110000ABCDEFGH1X", Email: "issued@example.com", TopUpOrderIds: "[999999]", InvoiceStatus: InvoiceStatusIssued}
	require.NoError(t, DB.Create(issued).Error)
	config.LegacyTopUpsEnabled = false
	config.LegacyInvoiceBefore = 0
	require.NoError(t, SaveInvoiceTaxConfig(config))
	for range 2 {
		require.NoError(t, SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents))
	}
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusRejected)
	require.NoError(t, err)
	require.NoError(t, SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents))
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, quote.Quota, current.Quota)
	require.NoError(t, DB.First(issued, issued.Id).Error)
	assert.Equal(t, InvoiceStatusIssued, issued.InvoiceStatus)
}

func TestLegacyInvoiceDateBoundariesAndSubmissionRevalidation(t *testing.T) {
	truncateTables(t)
	const cutoff int64 = 1793462400 // 2026-11-01 00:00 Asia/Shanghai.
	config := InvoiceTaxConfig{ModuleEnabled: true, Mode: "vat", LegacyInvoiceBefore: cutoff}
	require.True(t, config.Valid())
	require.NoError(t, SaveInvoiceTaxConfig(config))
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	user := insertInvoiceTicketUser(t, 980503)
	ids := make([]int, 4)
	for i, completed := range []int64{cutoff - 1, cutoff, cutoff + 1, 0} {
		ids[i] = seedInvoiceTicketTopUp(t, user.Id, []string{"before-cutoff", "at-cutoff", "after-cutoff", "unknown-completion"}[i], 100)
		require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", ids[i]).Updates(map[string]any{"complete_time": completed, "create_time": cutoff - 100}).Error)
	}
	orders, err := GetEligibleInvoiceOrders(user.Id, "ordinary")
	require.NoError(t, err)
	require.Len(t, orders, 1)
	assert.Equal(t, ids[0], orders[0].Id)
	for _, id := range ids[1:] {
		_, err := QuoteInvoiceTax(DB, user.Id, []int{id}, "ordinary")
		require.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
		params := createInvoiceTicketParams(user, []int{id})
		params.InvoiceType = "ordinary"
		_, _, _, _, err = CreateInvoiceTicket(params)
		require.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	}
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{ids[0]}, "ordinary")
	require.NoError(t, err)
	config.LegacyInvoiceBefore = cutoff - 86400
	require.NoError(t, SaveInvoiceTaxConfig(config))
	params := createInvoiceTicketParams(user, []int{ids[0]})
	params.InvoiceType, params.TaxQuote = "ordinary", quote.Token
	_, _, _, _, err = CreateInvoiceTicket(params)
	require.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid, "submission must recheck a narrowed cutoff")
	// An explicit administrator grant is independent of the legacy date gate.
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", ids[2]).Update("invoice_type", "ordinary").Error)
	orders, err = GetEligibleInvoiceOrders(user.Id, "ordinary")
	require.NoError(t, err)
	require.Len(t, orders, 1)
	assert.Equal(t, ids[2], orders[0].Id)
	_, err = QuoteInvoiceTax(DB, user.Id, []int{ids[2]}, "ordinary")
	require.NoError(t, err)
	var invoiceCount int64
	require.NoError(t, DB.Model(&TicketInvoice{}).Where("user_id = ?", user.Id).Count(&invoiceCount).Error)
	assert.Zero(t, invoiceCount)
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Zero(t, current.Quota)
}

func TestLegacyInvoiceDateRequiresBeijingMidnight(t *testing.T) {
	for _, cutoff := range []int64{-1, 1793462401, 253402272000} {
		config := InvoiceTaxConfig{Mode: "vat", LegacyInvoiceBefore: cutoff}
		assert.False(t, config.Valid())
		assert.ErrorIs(t, SaveInvoiceTaxConfig(config), ErrInvoiceTaxInvalid)
	}
	assert.True(t, (InvoiceTaxConfig{Mode: "vat", LegacyInvoiceBefore: 0}).Valid())
}
