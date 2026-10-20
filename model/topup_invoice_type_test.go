package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminGrantedInvoiceEligibilityDoesNotChangeFunds(t *testing.T) {
	oldMinimum := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 0
	t.Cleanup(func() {
		operation_setting.MinInvoiceAmount = oldMinimum
		require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	})
	for _, test := range []struct {
		name          string
		version       int
		originalType  string
		tax           int64
		legacyEnabled bool
	}{
		{"historical", 0, "", 0, false},
		{"legacy-noninvoiceable", 0, "none", 0, true},
		{"checkout-noninvoiceable", 1, "none", 0, false},
		{"checkout-ordinary", 1, "ordinary", 566, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			user := insertInvoiceTicketUser(t, 980602)
			require.NoError(t, DB.Model(user).Update("quota", 1234567).Error)
			require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600, SpecialRateBps: 1300, LegacyTopUpsEnabled: test.legacyEnabled}))
			order := TopUp{UserId: user.Id, TradeNo: test.name, Status: common.TopUpStatusSuccess, Money: 100, PaymentProvider: PaymentProviderEpay, InvoiceType: test.originalType, CheckoutVersion: test.version, InvoiceTaxCents: test.tax}
			if test.version == 1 {
				order.PaymentTotalCents, order.InvoiceNetCents = 10000, 10000-test.tax
			}
			require.NoError(t, DB.Create(&order).Error)
			if test.originalType == "" {
				require.NoError(t, DB.Model(&order).Update("invoice_type", "").Error)
			}
			var before TopUp
			require.NoError(t, DB.First(&before, order.Id).Error)
			_, err := UpdateTopUpInvoiceType(order.Id, test.originalType, "special")
			require.NoError(t, err)
			var after TopUp
			require.NoError(t, DB.First(&after, order.Id).Error)
			before.InvoiceType = "special"
			assert.Equal(t, before, after)
			eligible, err := GetEligibleInvoiceOrders(user.Id, "special")
			require.NoError(t, err)
			require.Len(t, eligible, 1)
			assert.Equal(t, order.Id, eligible[0].Id)
			eligible, err = GetEligibleInvoiceOrders(user.Id, "ordinary")
			require.NoError(t, err)
			assert.Empty(t, eligible)
			quote, err := QuoteInvoiceTax(DB, user.Id, []int{order.Id}, "special")
			require.NoError(t, err)
			assert.True(t, quote.PrepaidOnly)
			assert.Zero(t, quote.FeeCents)
			assert.Equal(t, int64(10000), quote.TotalCents)
			assert.Equal(t, test.tax, quote.PrepaidTaxCents)
			params := createInvoiceTicketParams(user, []int{order.Id})
			params.InvoiceType = "special"
			ticket, invoice, _, _, err := CreateInvoiceTicket(params)
			require.NoError(t, err)
			assert.Equal(t, "special", invoice.InvoiceType)
			assert.Equal(t, float64(100), invoice.TotalMoney)
			assert.Equal(t, test.tax, invoice.InvoiceTaxCents)
			assert.Nil(t, invoice.TaxCharge)
			_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusRejected)
			require.NoError(t, err)
			require.NoError(t, DB.First(user, user.Id).Error)
			assert.Equal(t, 1234567, user.Quota)
			_, err = UpdateTopUpInvoiceType(order.Id, "special", "none")
			require.NoError(t, err)
			// Even enabling historical supplements must not override an explicit opt-out.
			require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600, LegacyTopUpsEnabled: true}))
			eligible, err = GetEligibleInvoiceOrders(user.Id, "ordinary")
			require.NoError(t, err)
			assert.Empty(t, eligible)
			_, err = QuoteInvoiceTax(DB, user.Id, []int{order.Id}, "ordinary")
			assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
		})
	}
}

func TestUpdateTopUpInvoiceTypePreservesPaymentAndChecksOccupancy(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 980601)
	order := TopUp{UserId: user.Id, TradeNo: "invoice-type-edit", Status: common.TopUpStatusSuccess, PaymentProvider: PaymentProviderEpay,
		Money: 100, InvoiceType: "ordinary", CheckoutVersion: 1, PaymentTotalCents: 10000, InvoiceNetCents: 9434, InvoiceTaxCents: 566, CheckoutQuota: 943400}
	require.NoError(t, DB.Create(&order).Error)
	updated, err := UpdateTopUpInvoiceType(order.Id, "ordinary", "special")
	require.NoError(t, err)
	assert.Equal(t, "special", updated.InvoiceType)
	_, err = UpdateTopUpInvoiceType(order.Id, "ordinary", "none")
	assert.ErrorIs(t, err, ErrInvoiceTaxQuoteChanged)
	items, total, err := GetAllTopUpsWithUsername(TopUpFilter{InvoiceType: "special"}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	assert.Equal(t, order.Id, items[0].Id)
	_, err = UpdateTopUpInvoiceType(order.Id, "special", "none")
	require.NoError(t, err)
	eligible, err := GetEligibleInvoiceOrders(user.Id, "special")
	require.NoError(t, err)
	assert.Empty(t, eligible)
	_, err = QuoteInvoiceTax(DB, user.Id, []int{order.Id}, "special")
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	var current TopUp
	require.NoError(t, DB.First(&current, order.Id).Error)
	assert.Equal(t, order.Money, current.Money)
	assert.Equal(t, order.InvoiceTaxCents, current.InvoiceTaxCents)
	assert.Equal(t, order.InvoiceNetCents, current.InvoiceNetCents)
	assert.Equal(t, order.CheckoutQuota, current.CheckoutQuota)
	for _, status := range []int{InvoiceStatusPending, InvoiceStatusIssued} {
		invoice := TicketInvoice{TicketId: 980600 + status, UserId: user.Id, InvoiceStatus: status}
		require.NoError(t, invoice.SetTopUpOrderIDs([]int{order.Id}))
		require.NoError(t, DB.Create(&invoice).Error)
		_, err = UpdateTopUpInvoiceType(order.Id, "none", "ordinary")
		assert.ErrorIs(t, err, ErrTicketInvoiceOrderDuplicate)
		require.NoError(t, DB.Delete(&invoice).Error)
	}
	ticket := Ticket{UserId: user.Id, Type: TicketTypeInvoice, Status: TicketStatusClosed}
	require.NoError(t, DB.Create(&ticket).Error)
	invoice := TicketInvoice{TicketId: ticket.Id, UserId: user.Id, InvoiceStatus: InvoiceStatusRejected, InvoiceType: "ordinary"}
	require.NoError(t, invoice.SetTopUpOrderIDs([]int{order.Id}))
	require.NoError(t, DB.Create(&invoice).Error)
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusIssued)
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	require.NoError(t, DB.Model(&order).Update("status", common.TopUpStatusPending).Error)
	_, err = UpdateTopUpInvoiceType(order.Id, "none", "ordinary")
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
}

func TestTopUpInvoiceTypeEditAndApplicationCannotBothSucceed(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 980603)
	oldMinimum := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 0
	t.Cleanup(func() { operation_setting.MinInvoiceAmount = oldMinimum })
	order := TopUp{UserId: user.Id, TradeNo: "invoice-type-race", Status: common.TopUpStatusSuccess, Money: 100,
		PaymentProvider: PaymentProviderEpay, InvoiceType: "ordinary", CheckoutVersion: 1, InvoiceNetCents: 9434, InvoiceTaxCents: 566, PaymentTotalCents: 10000}
	require.NoError(t, DB.Create(&order).Error)
	params := createInvoiceTicketParams(user, []int{order.Id})
	params.InvoiceType = "ordinary"
	type result struct {
		edit bool
		err  error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	go func() {
		<-start
		_, err := UpdateTopUpInvoiceType(order.Id, "ordinary", "none")
		results <- result{edit: true, err: err}
	}()
	go func() {
		<-start
		_, _, _, _, err := CreateInvoiceTicket(params)
		results <- result{err: err}
	}()
	close(start)
	var editErr, applicationErr error
	for range 2 {
		outcome := <-results
		if outcome.edit {
			editErr = outcome.err
		} else {
			applicationErr = outcome.err
		}
	}
	var current TopUp
	require.NoError(t, DB.First(&current, order.Id).Error)
	var count int64
	require.NoError(t, DB.Model(&TicketInvoice{}).Where("user_id = ?", user.Id).Count(&count).Error)
	if editErr == nil {
		assert.ErrorIs(t, applicationErr, ErrTicketInvoiceOrderInvalid)
		assert.Equal(t, "none", current.InvoiceType)
		assert.Zero(t, count)
	} else {
		assert.ErrorIs(t, editErr, ErrTicketInvoiceOrderDuplicate)
		require.NoError(t, applicationErr)
		assert.Equal(t, "ordinary", current.InvoiceType)
		assert.Equal(t, int64(1), count)
	}
}
