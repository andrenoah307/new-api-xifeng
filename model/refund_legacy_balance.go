package model

import (
	"math"
	"math/big"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// Historical occupancy is a policy allocation, never evidence of a bank payout.
// Persisted rows survive setting changes and are shared by all subsequent refunds.
type RefundLegacyBalance struct {
	TicketID  int    `json:"ticket_id"`
	Cutoff    int64  `json:"cutoff"`
	Basis     string `json:"basis"`
	CashCents int64  `json:"cash_cents"`
}

func allocateLegacyRefundBalance(old TicketRefund, cutoff int64, orders []TopUp, prior []RefundOrderAllocation) ([]RefundOrderAllocation, error) {
	fail := refundCalculationError(ErrRefundExport, "missing_history", old.TicketId, 0)
	if cutoff <= 0 || old.ProcessedTime <= 0 || old.ProcessedTime >= cutoff || old.RefundQuota <= 0 || old.RefundCashCents < 0 {
		return nil, fail
	}
	cash, basis := old.RefundCashCents, "recorded_cash"
	if cash == 0 {
		if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
			return nil, fail
		}
		value := decimal.NewFromInt(int64(old.RefundQuota)).Mul(decimal.NewFromInt(100)).DivRound(decimal.NewFromFloat(common.QuotaPerUnit), 0)
		if !value.IsPositive() || value.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
			return nil, fail
		}
		cash, basis = value.IntPart(), "recorded_quota"
	}
	lots, err := projectRefundCreditLots(0, orders, prior)
	if err != nil {
		return nil, err
	}
	rows := []RefundOrderAllocation{}
	for _, order := range orders {
		if cash == 0 {
			break
		}
		if order.CompleteTime <= 0 || order.CompleteTime >= cutoff {
			continue
		}
		lot := lots[order.Id]
		take := min(cash, order.PaymentTotalCents-lot.RefundedCash)
		if take <= 0 {
			continue
		}
		// Reserve quota by cumulative cash at the original order ratio, rounding
		// upward so historical occupancy cannot manufacture refundable credit.
		n := new(big.Int).Mul(big.NewInt(lot.RefundedCash+take), big.NewInt(order.QuotaGranted))
		d := big.NewInt(order.PaymentTotalCents)
		n.Add(n, new(big.Int).Sub(d, big.NewInt(1))).Div(n, d)
		if !n.IsInt64() {
			return nil, fail
		}
		quota := max(int64(0), n.Int64()-lot.RefundedPaid)
		if quota > lot.PaidRemaining {
			return nil, fail
		}
		rows = append(rows, RefundOrderAllocation{RefundTicketID: old.TicketId, UserID: old.UserId, TopUpID: order.Id, TradeNo: order.TradeNo, OriginalQuota: order.QuotaGranted, OriginalPaidCents: order.PaymentTotalCents, OriginalTaxCents: order.InvoiceTaxCents, RefundQuota: quota, RefundCents: take, Status: "committed", LegacyCutoff: cutoff, LegacyBasis: basis, QuotaPerUnit: decimal.NewFromFloat(common.QuotaPerUnit).String()})
		cash -= take
	}
	if cash != 0 {
		return nil, fail
	}
	return rows, nil
}
