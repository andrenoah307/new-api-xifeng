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
		assert.False(t, config.LegacyTopUpsEnabled)
		config.Enabled = taxEnabled
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
	config := InvoiceTaxConfig{Mode: "vat", Enabled: true, LegacyTopUpsEnabled: true, OrdinaryRateBps: 600}
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
