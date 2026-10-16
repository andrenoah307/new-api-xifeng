package model

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestInvoiceTaxTicketClosure(t *testing.T) {
	for _, method := range []string{"balance", "online", "issued", "historical"} {
		t.Run(method, func(t *testing.T) {
			truncateTables(t)
			user := insertInvoiceTicketUser(t, 980110)
			const startingQuota = 100000000
			require.NoError(t, DB.Model(user).Update("quota", startingQuota).Error)
			require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600}))
			t.Cleanup(func() {
				DB.Where("user_id = ?", user.Id).Delete(&InvoiceTaxCharge{})
				DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{})
			})
			id := seedInvoiceTicketTopUp(t, user.Id, "tax-close-order", 1000)
			require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", id).Update("payment_provider", PaymentProviderEpay).Error)
			quote, err := QuoteInvoiceTax(DB, user.Id, []int{id}, "ordinary")
			require.NoError(t, err)
			params := createInvoiceTicketParams(user, []int{id})
			params.InvoiceType, params.TaxQuote, params.TaxPaymentMethod = "ordinary", quote.Token, "balance"
			if method == "online" {
				params.TaxPaymentMethod = "online"
			}
			ticket, invoice, _, _, err := CreateInvoiceTicket(params)
			require.NoError(t, err)
			if method == "issued" {
				_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusIssued)
				require.NoError(t, err)
			}
			if method == "historical" {
				require.NoError(t, DB.Model(ticket).Update("status", TicketStatusClosed).Error)
				eligible, err := GetEligibleInvoiceOrders(user.Id)
				require.NoError(t, err)
				require.Len(t, eligible, 1)
				replacement, nextInvoice, _, _, err := CreateInvoiceTicket(params)
				require.NoError(t, err)
				assert.NotEqual(t, ticket.Id, replacement.Id)
				assert.Equal(t, "paid", nextInvoice.TaxCharge.Status)
				previous, err := GetInvoiceTaxCharge(ticket.Id)
				require.NoError(t, err)
				assert.Equal(t, "refunded", previous.Status)
			}
			for range 2 {
				_, err = CloseUserTicket(ticket.Id, user.Id)
				require.NoError(t, err)
			}
			if method == "online" {
				require.NoError(t, DB.Model(invoice.TaxCharge).Update("payment_method", "alipay").Error)
				for range 2 {
					require.NoError(t, SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents))
				}
			}
			var current User
			require.NoError(t, DB.First(&current, user.Id).Error)
			orders, err := GetEligibleInvoiceOrders(user.Id)
			require.NoError(t, err)
			switch method {
			case "issued", "historical":
				assert.Equal(t, startingQuota-quote.Quota, current.Quota)
				assert.Empty(t, orders)
			case "online":
				assert.Equal(t, startingQuota+quote.Quota, current.Quota)
				require.Len(t, orders, 1)
			default:
				assert.Equal(t, startingQuota, current.Quota)
				require.Len(t, orders, 1)
			}
		})
	}
}

func TestInvoiceTaxWriteFailureRollsBackMoneyAndState(t *testing.T) {
	for _, action := range []string{"create", "close", "callback", "overflow", "reapply"} {
		t.Run(action, func(t *testing.T) {
			truncateTables(t)
			user := insertInvoiceTicketUser(t, 980111)
			const startingQuota = 100000000
			require.NoError(t, DB.Model(user).Update("quota", startingQuota).Error)
			require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600}))
			t.Cleanup(func() {
				DB.Where("user_id = ?", user.Id).Delete(&InvoiceTaxCharge{})
				DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{})
			})
			id := seedInvoiceTicketTopUp(t, user.Id, "tax-atomic-order", 1000)
			require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", id).Update("payment_provider", PaymentProviderEpay).Error)
			quote, err := QuoteInvoiceTax(DB, user.Id, []int{id}, "ordinary")
			require.NoError(t, err)
			params := createInvoiceTicketParams(user, []int{id})
			params.InvoiceType, params.TaxQuote, params.TaxPaymentMethod = "ordinary", quote.Token, "balance"
			if action == "callback" {
				params.TaxPaymentMethod = "online"
			}
			var ticket *Ticket
			var invoice *TicketInvoice
			if action != "create" {
				ticket, invoice, _, _, err = CreateInvoiceTicket(params)
				require.NoError(t, err)
			}
			if action == "reapply" {
				require.NoError(t, DB.Model(ticket).Update("status", TicketStatusClosed).Error)
			}
			writeFailure := errors.New("injected invoice message write failure")
			require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("test:invoice_message_failure", func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "TicketMessage" {
					if message, ok := tx.Statement.Dest.(*TicketMessage); action == "reapply" && ok && message.TicketId == ticket.Id {
						return
					}
					tx.AddError(writeFailure)
				}
			}))
			t.Cleanup(func() { require.NoError(t, DB.Callback().Create().Remove("test:invoice_message_failure")) })
			expectedQuota := startingQuota
			expectedError := writeFailure
			switch action {
			case "create":
				_, _, _, _, err = CreateInvoiceTicket(params)
			case "close":
				expectedQuota -= quote.Quota
				_, err = CloseUserTicket(ticket.Id, user.Id)
			case "callback":
				require.NoError(t, DB.Model(invoice.TaxCharge).Update("payment_method", "alipay").Error)
				err = SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents)
			case "overflow":
				expectedQuota, expectedError = common.MaxQuota, ErrInvoiceTaxInvalid
				require.NoError(t, DB.Model(user).Update("quota", expectedQuota).Error)
				_, err = CloseUserTicket(ticket.Id, user.Id)
			case "reapply":
				expectedQuota -= quote.Quota
				_, _, _, _, err = CreateInvoiceTicket(params)
			}
			require.ErrorIs(t, err, expectedError)
			var current User
			require.NoError(t, DB.First(&current, user.Id).Error)
			assert.Equal(t, expectedQuota, current.Quota)
			if action == "create" {
				for _, table := range []any{&Ticket{}, &TicketInvoice{}, &InvoiceTaxCharge{}} {
					var count int64
					require.NoError(t, DB.Model(table).Where("user_id = ?", user.Id).Count(&count).Error)
					assert.Zero(t, count)
				}
			} else {
				require.NoError(t, DB.First(ticket, ticket.Id).Error)
				if action == "reapply" {
					assert.Equal(t, TicketStatusClosed, ticket.Status)
					for _, table := range []any{&Ticket{}, &TicketInvoice{}, &InvoiceTaxCharge{}} {
						var count int64
						require.NoError(t, DB.Model(table).Where("user_id = ?", user.Id).Count(&count).Error)
						assert.Equal(t, int64(1), count)
					}
				} else {
					assert.Equal(t, TicketStatusOpen, ticket.Status)
				}
				require.NoError(t, DB.First(invoice, invoice.Id).Error)
				assert.Equal(t, InvoiceStatusPending, invoice.InvoiceStatus)
				charge, err := GetInvoiceTaxCharge(ticket.Id)
				require.NoError(t, err)
				assert.Equal(t, invoice.TaxCharge.Status, charge.Status)
			}
		})
	}
}

func TestInvoiceTaxConcurrentCallbackAndClosure(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 980112)
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600}))
	t.Cleanup(func() {
		DB.Where("user_id = ?", user.Id).Delete(&InvoiceTaxCharge{})
		DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{})
	})
	id := seedInvoiceTicketTopUp(t, user.Id, "tax-concurrent-order", 1000)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", id).Update("payment_provider", PaymentProviderEpay).Error)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{id}, "ordinary")
	require.NoError(t, err)
	params := createInvoiceTicketParams(user, []int{id})
	params.InvoiceType, params.TaxQuote, params.TaxPaymentMethod = "ordinary", quote.Token, "online"
	ticket, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	require.NoError(t, DB.Model(invoice.TaxCharge).Update("payment_method", "alipay").Error)
	start := make(chan struct{})
	results := make(chan error, 4)
	var wg sync.WaitGroup
	for _, operation := range []string{"callback", "close", "callback", "close"} {
		wg.Add(1)
		go func(operation string) {
			defer wg.Done()
			<-start
			if operation == "callback" {
				results <- SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents)
			} else {
				_, err := CloseUserTicket(ticket.Id, user.Id)
				results <- err
			}
		}(operation)
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, quote.Quota, current.Quota)
	charge, err := GetInvoiceTaxCharge(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, "refunded", charge.Status)
	require.NoError(t, DB.First(ticket, ticket.Id).Error)
	assert.Equal(t, TicketStatusClosed, ticket.Status)
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
