package model

import (
	"math"
	"math/big"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// The wallet and UsedQuota remain unchanged. Later paid credits settle this
// consumption before becoming refundable through projectRefundCreditLots.
type RefundConsumptionOverdraft struct {
	Quota                   int64  `json:"quota"`
	LimitCents              int64  `json:"limit_cents"`
	WalletQuota             int64  `json:"wallet_quota"`
	DeferredValueCentsRatio string `json:"deferred_value_cents_ratio"`
}

func applyRefundConsumptionOverdraft(orders []TopUp, lots map[int]RefundCreditLot, refund *TicketRefund, owner *User, limitCents int64) (*RefundConsumptionOverdraft, map[int]int64, error) {
	if refund.RefundQuota <= 0 {
		return nil, nil, ErrRefundExport
	}
	var available int64
	for _, order := range orders {
		lot := lots[order.Id]
		if lot.PaidRemaining < 0 || lot.PaidRemaining > math.MaxInt64-available {
			return nil, nil, ErrRefundExport
		}
		available += lot.PaidRemaining
	}
	needed := int64(refund.RefundQuota) - available
	if needed <= 0 {
		return nil, nil, nil
	}
	if limitCents <= 0 || limitCents > 1000 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || common.QuotaPerUnit <= 0 {
		return nil, nil, ErrRefundExport
	}
	limit, clamp := common.QuotaFromDecimalChecked(decimal.NewFromInt(limitCents).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Div(decimal.NewFromInt(100)))
	// Only a fully frozen existing request can use this allowance. It must be
	// real wallet debt, not a positive gift balance lacking cash backing.
	if clamp != nil || limit <= 0 || refund.RefundStatus != RefundStatusPending || refund.FrozenQuota != refund.RefundQuota || refund.RefundGiftQuota != 0 || owner.Quota >= 0 || int64(owner.Quota) < -int64(limit) || needed > -int64(owner.Quota) {
		return nil, nil, ErrRefundExport
	}
	for _, lot := range lots {
		if lot.GiftRemaining > 0 {
			return nil, nil, ErrRefundExport
		}
	}
	remaining := needed
	deferred := make(map[int]int64)
	var value big.Rat
	// Defer the latest funded consumption. Each source remains bounded by
	// original paid quota and cash after all earlier refunds.
	for i := len(orders) - 1; i >= 0 && remaining > 0; i-- {
		order := orders[i]
		lot := lots[order.Id]
		if order.QuotaGranted <= 0 || order.PaymentTotalCents <= 0 || lot.UsedPaid < 0 || lot.RefundedPaid < 0 || lot.RefundedPaid > order.QuotaGranted || lot.UsedPaid > order.QuotaGranted-lot.RefundedPaid || lot.PaidRemaining > order.QuotaGranted-lot.RefundedPaid-lot.UsedPaid {
			return nil, nil, ErrRefundExport
		}
		take := min(remaining, lot.UsedPaid)
		if take == 0 {
			continue
		}
		deferred[order.Id] = take
		remaining -= take
		value.Add(&value, new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(order.PaymentTotalCents), big.NewInt(take)), big.NewInt(order.QuotaGranted)))
	}
	if remaining != 0 || value.Cmp(new(big.Rat).SetInt64(limitCents)) > 0 {
		return nil, nil, ErrRefundExport
	}
	for id, quota := range deferred {
		lot := lots[id]
		lot.UsedPaid -= quota
		lot.PaidRemaining += quota
		lots[id] = lot
	}
	return &RefundConsumptionOverdraft{Quota: needed, LimitCents: limitCents, WalletQuota: int64(owner.Quota), DeferredValueCentsRatio: value.RatString()}, deferred, nil
}
