package model

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
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
