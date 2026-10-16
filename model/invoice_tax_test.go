package model

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceTaxPaymentLifecycle(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 980101)
	const startingQuota = 100000000
	require.NoError(t, DB.Model(user).Update("quota", startingQuota).Error)
	config := InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600, SpecialRateBps: 1300}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	t.Cleanup(func() {
		DB.Where("user_id = ?", user.Id).Delete(&InvoiceTaxCharge{})
		DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{})
	})
	orderID := seedInvoiceTicketTopUp(t, user.Id, "tax-balance-order", 1000)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", orderID).Update("payment_provider", PaymentProviderEpay).Error)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{orderID}, "ordinary")
	require.NoError(t, err)
	require.True(t, quote.CanBalance)
	params := createInvoiceTicketParams(user, []int{orderID})
	params.InvoiceType = "ordinary"
	params.TaxPaymentMethod = "balance"
	params.TaxQuote = quote.Token
	ticket, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	assert.Equal(t, float64(1060), invoice.TotalMoney)
	require.NotNil(t, invoice.TaxCharge)
	assert.Equal(t, "paid", invoice.TaxCharge.Status)
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, startingQuota-quote.Quota, current.Quota)
	_, _, _, _, err = CreateInvoiceTicket(params)
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderDuplicate)
	for range 2 {
		_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusRejected)
		require.NoError(t, err)
	}
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, startingQuota, current.Quota)
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusIssued)
	assert.ErrorIs(t, err, ErrInvoiceTaxUnpaid)
}

func TestInvoiceTaxOnlineCannotCreditRechargeAndRejectsWrongAmount(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 980102)
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "gross_up", OrdinaryRateBps: 600}))
	t.Cleanup(func() {
		DB.Where("user_id = ?", user.Id).Delete(&InvoiceTaxCharge{})
		DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{})
	})
	orderID := seedInvoiceTicketTopUp(t, user.Id, "tax-online-order", 1000)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", orderID).Update("payment_provider", PaymentProviderEpay).Error)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{orderID}, "ordinary")
	require.NoError(t, err)
	assert.False(t, quote.CanBalance)
	params := createInvoiceTicketParams(user, []int{orderID})
	params.InvoiceType = "ordinary"
	params.TaxPaymentMethod = "balance"
	params.TaxQuote = quote.Token
	ticket, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	assert.Equal(t, "pending", invoice.TaxCharge.Status)
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusIssued)
	assert.ErrorIs(t, err, ErrInvoiceTaxUnpaid)
	require.NoError(t, DB.Model(invoice.TaxCharge).Update("payment_method", "alipay").Error)
	assert.ErrorIs(t, SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents+1), ErrInvoiceTaxInvalid)
	assert.ErrorIs(t, SettleInvoiceTax(invoice.TaxCharge.TradeNo, "wxpay", quote.FeeCents), ErrInvoiceTaxInvalid)
	for range 2 {
		require.NoError(t, SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents))
	}
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Zero(t, current.Quota)
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusRejected)
	require.NoError(t, err)
	require.NoError(t, SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents))
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, quote.Quota, current.Quota)
}

func TestInvoiceTaxQuoteCannotBypassCurrencyOrChangedConfig(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 980103)
	config := InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	t.Cleanup(func() {
		DB.Where("user_id = ?", user.Id).Delete(&InvoiceTaxCharge{})
		DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{})
	})
	orderID := seedInvoiceTicketTopUp(t, user.Id, "tax-currency", 1000)
	_, err := QuoteInvoiceTax(DB, user.Id, []int{orderID}, "ordinary")
	assert.ErrorIs(t, err, ErrInvoiceTaxCurrency)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", orderID).Update("payment_provider", PaymentProviderEpay).Error)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{orderID}, "ordinary")
	require.NoError(t, err)
	config.OrdinaryRateBps = 900
	require.NoError(t, SaveInvoiceTaxConfig(config))
	params := createInvoiceTicketParams(user, []int{orderID})
	params.InvoiceType = "ordinary"
	params.TaxPaymentMethod = "online"
	params.TaxQuote = quote.Token
	_, _, _, _, err = CreateInvoiceTicket(params)
	assert.ErrorIs(t, err, ErrInvoiceTaxQuoteChanged)
	var count int64
	require.NoError(t, DB.Model(&Ticket{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
	batch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = batch })
	require.NoError(t, DB.Model(user).Update("quota", common.MaxQuota).Error)
	quote, err = QuoteInvoiceTax(DB, user.Id, []int{orderID}, "ordinary")
	require.NoError(t, err)
	assert.False(t, quote.CanBalance)
}

func TestInvoiceTaxLatePaymentAfterRejectionRefundsOnce(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 980104)
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600}))
	t.Cleanup(func() {
		DB.Where("user_id = ?", user.Id).Delete(&InvoiceTaxCharge{})
		DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{})
	})
	orderID := seedInvoiceTicketTopUp(t, user.Id, "tax-late-order", 1000)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", orderID).Update("payment_provider", PaymentProviderEpay).Error)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{orderID}, "ordinary")
	require.NoError(t, err)
	params := createInvoiceTicketParams(user, []int{orderID})
	params.InvoiceType = "ordinary"
	params.TaxPaymentMethod = "online"
	params.TaxQuote = quote.Token
	ticket, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	require.NoError(t, DB.Model(invoice.TaxCharge).Update("payment_method", "alipay").Error)
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusRejected)
	require.NoError(t, err)
	for range 2 {
		require.NoError(t, SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents))
	}
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, quote.Quota, current.Quota)
	charge, err := GetInvoiceTaxCharge(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, "refunded", charge.Status)
	items, _, err := ListInvoicesForExport(InvoiceExportFilter{}, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	for _, item := range items {
		assert.NotEqual(t, ticket.Id, item.TicketId)
	}
}

func TestInvoiceTaxModesAndAccounting(t *testing.T) {
	for _, tc := range []struct {
		mode       string
		fee, total int64
	}{
		{"vat", 60000, 1060000}, {"compound", 123600, 1123600}, {"gross_up", 63830, 1063830},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			config := InvoiceTaxConfig{Enabled: true, Mode: tc.mode, OrdinaryRateBps: 600, SpecialRateBps: 1300}
			quote, err := CalculateInvoiceTax(1000000, "ordinary", config)
			require.NoError(t, err)
			assert.Equal(t, tc.fee, quote.FeeCents)
			assert.Equal(t, tc.total, quote.TotalCents)
			assert.Equal(t, quote.BaseCents+quote.FeeCents, quote.TotalCents)
			special, err := CalculateInvoiceTax(1000000, "special", config)
			require.NoError(t, err)
			assert.Equal(t, 1300, special.RateBps)
			assert.Greater(t, special.FeeCents, quote.FeeCents)
			config.Enabled = false
			legacy, err := CalculateInvoiceTax(1000000, "ordinary", config)
			require.NoError(t, err)
			assert.Equal(t, int64(0), legacy.FeeCents)
		})
	}
}

func TestInvoiceTaxRejectsInvalidRatesAndAmounts(t *testing.T) {
	config := InvoiceTaxConfig{Enabled: true, Mode: "gross_up", OrdinaryRateBps: 600}
	for _, amount := range []int64{-1, 0, 100000000001, math.MaxInt64} {
		_, err := CalculateInvoiceTax(amount, "ordinary", config)
		assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	}
	for _, rate := range []int{-1, 10000, 10001} {
		config.OrdinaryRateBps = rate
		_, err := CalculateInvoiceTax(1000000, "ordinary", config)
		assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	}
	for _, money := range []float64{math.NaN(), math.Inf(1), -1, 1e20} {
		_, err := invoiceMoneyCents(money)
		assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	}
	config.OrdinaryRateBps = 600
	quote, err := CalculateInvoiceTax(1, "ordinary", config)
	require.NoError(t, err)
	assert.Equal(t, int64(1), quote.TotalCents)
	assert.Equal(t, int64(0), quote.FeeCents)
}
