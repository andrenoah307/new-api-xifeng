package model

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinanceDisabledPreservesLegacyInvoice(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	user := insertInvoiceTicketUser(t, 980601)
	order := TopUp{UserId: user.Id, TradeNo: "legacy-compatible", Money: 100, Status: common.TopUpStatusSuccess, PaymentProvider: PaymentProviderStripe}
	require.NoError(t, DB.Create(&order).Error)
	assert.Empty(t, order.InvoiceType)
	orders, err := GetEligibleInvoiceOrders(user.Id, "")
	require.NoError(t, err)
	require.Len(t, orders, 1)
	_, invoice, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{order.Id}))
	require.NoError(t, err)
	assert.Equal(t, 100.0, invoice.TotalMoney)
	assert.Nil(t, invoice.TaxCharge)
	assert.Empty(t, invoice.InvoiceType)
}

func TestLegacyRechargeBigintCreditAndOverflowRollback(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		balance    int
		amount     int64
		wantCredit int
	}{
		{"large credit", 3000000000, 10000, 5000000000},
		{"balance overflow", math.MaxInt64, 1, 0},
		{"credit overflow", 0, math.MaxInt64, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			setupEpayRechargeTest(t)
			oldUnit := common.QuotaPerUnit
			common.QuotaPerUnit = 500000
			t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
			user, order := insertEpayRechargeFixture(t, "bigint-credit", PaymentProviderEpay, common.TopUpStatusPending, scenario.amount, 100, 0)
			require.NoError(t, DB.Model(user).Update("quota", scenario.balance).Error)
			_, credited, err := RechargeEpay(order.TradeNo, "alipay")
			if scenario.wantCredit == 0 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, scenario.wantCredit, credited)
				_, duplicateCredit, err := RechargeEpay(order.TradeNo, "alipay")
				require.NoError(t, err)
				assert.Zero(t, duplicateCredit)
			}
			require.NoError(t, DB.First(user, user.Id).Error)
			assert.Equal(t, scenario.balance+scenario.wantCredit, user.Quota)
			require.NoError(t, DB.First(order, order.Id).Error)
			assert.Equal(t, int64(scenario.wantCredit), order.QuotaGranted)
			if scenario.wantCredit == 0 {
				assert.Equal(t, common.TopUpStatusPending, order.Status)
			}
		})
	}
}

func TestFinanceDisabledRejectsNewCheckoutAndEligibilityChanges(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	user := insertInvoiceTicketUser(t, 980602)
	orderID := seedInvoiceTicketTopUp(t, user.Id, "legacy-no-edit", 100)
	_, err := UpdateTopUpInvoiceType(orderID, "", "ordinary")
	assert.ErrorIs(t, err, ErrTicketForbidden)
	_, err = QuoteEpayCheckout(DB, user.Id, EpayCheckoutRequest{TotalCents: 10000, InvoiceType: "none", PaymentMethod: "alipay"}, false)
	assert.Error(t, err)
}

func TestFinanceDisabledRetainsRecordedInvoiceEligibility(t *testing.T) {
	truncateTables(t)
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Mode: "vat", Enabled: true, OrdinaryRateBps: 600}))
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	user := insertInvoiceTicketUser(t, 980604)
	orders := []TopUp{
		{UserId: user.Id, TradeNo: "disabled-ordinary", Status: common.TopUpStatusSuccess, Money: 100, InvoiceType: "ordinary", CheckoutVersion: 1, InvoiceNetCents: 9434, InvoiceTaxCents: 566, PaymentTotalCents: 10000},
		{UserId: user.Id, TradeNo: "disabled-special", Status: common.TopUpStatusSuccess, Money: 100, InvoiceType: "special", CheckoutVersion: 1, InvoiceNetCents: 9434, InvoiceTaxCents: 566, PaymentTotalCents: 10000},
		{UserId: user.Id, TradeNo: "disabled-none", Status: common.TopUpStatusSuccess, Money: 100, InvoiceType: "none"},
	}
	require.NoError(t, DB.Create(&orders).Error)
	eligible, err := GetEligibleInvoiceOrders(user.Id, "special")
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	assert.Equal(t, orders[1].Id, eligible[0].Id)
	for _, ids := range [][]int{{orders[2].Id}, {orders[0].Id, orders[1].Id}} {
		params := createInvoiceTicketParams(user, ids)
		params.InvoiceType = "ordinary"
		_, _, _, _, err := CreateInvoiceTicket(params)
		assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	}
	params := createInvoiceTicketParams(user, []int{orders[1].Id})
	params.InvoiceType = "special"
	_, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	assert.Equal(t, "special", invoice.InvoiceType)
	assert.Equal(t, 100.0, invoice.TotalMoney)
	assert.Equal(t, int64(566), invoice.InvoiceTaxCents)
	assert.Nil(t, invoice.TaxCharge)
}
