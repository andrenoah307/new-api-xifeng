package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundPaymentCalculationSnapshot(t *testing.T) {
	for _, test := range []struct {
		name                            string
		order                           TopUp
		lot                             RefundCreditLot
		take, used, cash, before, after int64
	}{
		{"service fee proportional refund", TopUp{Id: 29, QuotaGranted: 364000000, PaymentTotalCents: 80000, InvoiceTaxCents: 7200}, RefundCreditLot{UsedPaid: 181785582, PaidRemaining: 182214418}, 16500000, 39953, 3626, 40047, 36421},
		{"cumulative rounding", TopUp{Id: 1, QuotaGranted: 3, PaymentTotalCents: 100}, RefundCreditLot{PaidRemaining: 2, RefundedPaid: 1, RefundedCash: 33}, 1, 0, 34, 67, 33},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := calculateOriginalRefundPayments([]TopUp{test.order}, map[int]RefundCreditLot{test.order.Id: test.lot}, test.take, "prior_expiry")
			require.NoError(t, err)
			require.Len(t, result.Payments, 1)
			p := result.Payments[0]
			require.NotNil(t, p.Calculation)
			assert.Equal(t, test.used, p.UsedCents)
			assert.Equal(t, test.cash, p.RefundCents)
			assert.Equal(t, test.lot.RefundedCash, p.Calculation.PreviouslyRefundedCents)
			assert.Equal(t, test.before, p.Calculation.RemainingBeforeCents)
			assert.Equal(t, test.after, p.Calculation.RemainingAfterCents)
			assert.Equal(t, p.PaidCents, p.UsedCents+p.Calculation.PreviouslyRefundedCents+p.RefundCents+p.Calculation.RemainingAfterCents)
		})
	}
}

func TestRefundPaymentBreakdownAcrossDifferentServiceFees(t *testing.T) {
	orders := []TopUp{
		{Id: 1, QuotaGranted: 10000, PaymentTotalCents: 10000},
		{Id: 2, QuotaGranted: 9400, PaymentTotalCents: 10000, InvoiceTaxCents: 600},
		{Id: 3, QuotaGranted: 9100, PaymentTotalCents: 10000, InvoiceTaxCents: 900},
	}
	lots := map[int]RefundCreditLot{
		1: {UsedPaid: 5000, RefundedPaid: 1000, RefundedCash: 1000, PaidRemaining: 4000},
		2: {PaidRemaining: 9400},
		3: {PaidRemaining: 9100},
	}
	plan, err := calculateOriginalRefundPayments(orders, lots, 14400, "prior_expiry")
	require.NoError(t, err)
	require.Len(t, plan.Payments, 3)
	var principal, fees, cash int64
	for i, payment := range plan.Payments {
		assert.Equal(t, i+1, payment.OrderID)
		assert.Equal(t, []int64{4000, 10000, 1099}[i], payment.RefundCents)
		assert.Equal(t, []int64{0, 600, 99}[i], payment.RefundTaxCents)
		principal += payment.RefundCents - payment.RefundTaxCents
		fees += payment.RefundTaxCents
		cash += payment.RefundCents
	}
	assert.Equal(t, int64(14400), principal)
	assert.Equal(t, int64(699), fees)
	assert.Equal(t, int64(15099), cash)
	assert.Equal(t, plan.CashCents, principal+fees)
	assert.Equal(t, int64(1000), plan.Payments[0].Calculation.PreviouslyRefundedCents)
	assert.Equal(t, int64(8901), plan.Payments[2].Calculation.RemainingAfterCents)
}
