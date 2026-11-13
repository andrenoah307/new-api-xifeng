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
	ReferralGraceAmount     string `json:"referral_grace_amount,omitempty"`
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
	var referralGrace int64
	if refund.CalculationBasis == RefundBasisApplicationBalance {
		referralGrace = originalRefundReferralQuota(owner)
	}
	if (limitCents <= 0 && referralGrace == 0) || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || common.QuotaPerUnit <= 0 {
		return nil, nil, refundShortfallError("grace_unavailable", orders, lots, refund, owner, available, limitCents)
	}
	// Compare the configured allowance without narrowing it to a quota column.
	limit := decimal.NewFromInt(max(int64(0), limitCents)).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Div(decimal.NewFromInt(100)).Add(decimal.NewFromInt(referralGrace))
	walletDebt := decimal.NewFromInt(int64(owner.Quota)).Neg()
	// Only a fully frozen existing request can use this allowance. Application
	// balance callers pass the projected cash wallet, including noncash exclusions.
	if refund.RefundStatus != RefundStatusPending || refund.FrozenQuota != refund.RefundQuota || refund.RefundGiftQuota != 0 {
		return nil, nil, refundShortfallError("hold_not_eligible", orders, lots, refund, owner, available, limitCents)
	}
	if walletDebt.GreaterThan(limit) || decimal.NewFromInt(needed).GreaterThan(limit) {
		return nil, nil, refundShortfallError("grace_exceeded", orders, lots, refund, owner, available, limitCents)
	}
	if owner.Quota >= 0 || decimal.NewFromInt(needed).GreaterThan(walletDebt) {
		return nil, nil, refundShortfallError("unbacked_credit", orders, lots, refund, owner, available, limitCents)
	}
	for _, lot := range lots {
		if lot.GiftRemaining > 0 && refund.CalculationBasis != RefundBasisApplicationBalance {
			return nil, nil, refundShortfallError("gift_balance", orders, lots, refund, owner, available, limitCents)
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
	// Apply the same allowance to original-payment value without rounding up
	// fractional cents or narrowing a large system setting to a quota column.
	valueLimit := new(big.Rat).SetInt64(max(int64(0), limitCents))
	referralValue := new(big.Rat).Mul(new(big.Rat).SetInt64(referralGrace), big.NewRat(100, 1))
	referralValue.Quo(referralValue, decimal.NewFromFloat(common.QuotaPerUnit).Rat())
	valueLimit.Add(valueLimit, referralValue)
	if remaining != 0 || value.Cmp(valueLimit) > 0 {
		return nil, nil, refundShortfallError("funding_limit", orders, lots, refund, owner, available, limitCents)
	}
	for id, quota := range deferred {
		lot := lots[id]
		lot.UsedPaid -= quota
		lot.PaidRemaining += quota
		lots[id] = lot
	}
	return &RefundConsumptionOverdraft{ReferralGraceAmount: decimal.NewFromInt(referralGrace).Div(decimal.NewFromFloat(common.QuotaPerUnit)).String(), Quota: needed, LimitCents: limitCents, WalletQuota: int64(owner.Quota), DeferredValueCentsRatio: value.RatString()}, deferred, nil
}
