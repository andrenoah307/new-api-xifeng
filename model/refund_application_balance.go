package model

import (
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// The projection uses the same wallet/noncash boundary as application. Usage
// remains an independent audit fact; it is never rewritten to hide a difference.
type RefundApplicationBalance struct {
	ReferralGraceAmount string `json:"referral_grace_amount"`
	WalletAmount        string `json:"wallet_amount"`
	NoncashAmount       string `json:"noncash_amount"`
	CashAmount          string `json:"cash_amount"`
	ProjectedSpentQuota int64  `json:"projected_spent_quota,string"`
	RecordedSpentQuota  int64  `json:"recorded_spent_quota,string"`
}

func applicationRefundLotsTx(tx *gorm.DB, refund *TicketRefund, owner *User, orders []TopUp, allocations []RefundOrderAllocation, recordedSpent int64) (map[int]RefundCreditLot, *RefundApplicationBalance, *User, error) {
	if refund.FinanceVersion != 2 || refund.RefundStatus != RefundStatusPending || refund.RefundQuota <= 0 || refund.FrozenQuota != refund.RefundQuota || refund.RefundGiftQuota != 0 {
		return nil, nil, nil, ErrRefundAdjustment
	}
	var pending int64
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND ticket_id != ? AND refund_status = ?", owner.Id, refund.TicketId, RefundStatusPending).Count(&pending).Error; err != nil {
		return nil, nil, nil, err
	}
	if pending != 0 {
		return nil, nil, nil, ErrRefundFinancePending
	}
	noncash, err := originalRefundNoncashQuotaTx(tx, owner)
	if err != nil {
		return nil, nil, nil, err
	}
	if int64(owner.Quota) < math.MinInt64+noncash {
		return nil, nil, nil, ErrRefundExport
	}
	cash := int64(owner.Quota) - noncash
	if cash < math.MinInt32 || cash > math.MaxInt32 || cash > math.MaxInt64-int64(refund.FrozenQuota) || common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return nil, nil, nil, ErrRefundExport
	}
	// Validate original payment and prior refund capacities before projecting.
	lots, err := projectRefundCreditLots(0, orders, allocations)
	if err != nil {
		return nil, nil, nil, err
	}
	var funded int64
	for _, lot := range lots {
		if lot.PaidRemaining < 0 || lot.PaidRemaining > math.MaxInt64-funded {
			return nil, nil, nil, ErrRefundExport
		}
		funded += lot.PaidRemaining
	}
	available := cash + int64(refund.FrozenQuota)
	if available < 0 && funded > math.MaxInt64+available {
		return nil, nil, nil, ErrRefundExport
	}
	spent := max(int64(0), funded-available)
	lots, err = projectRefundCreditLots(spent, orders, allocations)
	if err != nil {
		return nil, nil, nil, err
	}
	unit := decimal.NewFromFloat(common.QuotaPerUnit)
	balance := &RefundApplicationBalance{
		ReferralGraceAmount: decimal.NewFromInt(originalRefundReferralQuota(owner)).Div(unit).String(),
		WalletAmount:        decimal.NewFromInt(int64(owner.Quota)).Div(unit).String(),
		NoncashAmount:       decimal.NewFromInt(noncash).Div(unit).String(),
		CashAmount:          decimal.NewFromInt(cash).Div(unit).String(),
		ProjectedSpentQuota: spent, RecordedSpentQuota: recordedSpent,
	}
	cashOwner := *owner
	cashOwner.Quota = int(cash)
	return lots, balance, &cashOwner, nil
}
