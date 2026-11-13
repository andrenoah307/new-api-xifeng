package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"math"
)

const RefundBasisApplicationBalance = "application_balance"

// These credits are excluded at application time, not clawed back on refund.
func originalRefundNoncashQuotaTx(tx *gorm.DB, owner *User) (int64, error) {
	var bonus, recovered int64
	if err := tx.Model(&TopUp{}).Where("user_id = ? AND status = ? AND source = ?", owner.Id, common.TopUpStatusSuccess, "discount_bonus").Select("COALESCE(SUM(quota_granted),0)").Scan(&bonus).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", owner.Id, RefundStatusRefunded).Select("COALESCE(SUM(refund_gift_quota),0)").Scan(&recovered).Error; err != nil {
		return 0, err
	}
	bonus = max(0, bonus-max(0, recovered))
	referral := max(int64(0), int64(owner.AffHistoryQuota)-int64(owner.AffQuota))
	if bonus > math.MaxInt64-referral {
		return 0, ErrRefundExport
	}
	return bonus + referral, nil
}

// OriginalRefundEligibility retains the common ancestor's balance/payment cap.
// It does not require historical finance snapshots or a reconciliation proposal.
func OriginalRefundEligibility(userID int) (*RefundEligibility, error) {
	return originalRefundEligibilityTx(DB, userID)
}

func originalRefundEligibilityTx(tx *gorm.DB, userID int) (*RefundEligibility, error) {
	var owner User
	if err := tx.First(&owner, userID).Error; err != nil {
		return nil, err
	}
	var paid, refunded, frozen int64
	if err := tx.Model(&TopUp{}).Where("user_id = ? AND status = ? AND (source IS NULL OR source != ?)", userID, common.TopUpStatusSuccess, "discount_bonus").Select("COALESCE(SUM(quota_granted),0)").Scan(&paid).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", userID, RefundStatusRefunded).Select("COALESCE(SUM(refund_quota),0)").Scan(&refunded).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", userID, RefundStatusPending).Select("COALESCE(SUM(frozen_quota),0)").Scan(&frozen).Error; err != nil {
		return nil, err
	}
	noncash, err := originalRefundNoncashQuotaTx(tx, &owner)
	if err != nil {
		return nil, err
	}
	available := max(int64(0), int64(owner.Quota))
	available = max(int64(0), available-noncash)
	cap := max(int64(0), paid-max(int64(0), refunded))
	cap = max(int64(0), cap-max(int64(0), frozen))
	maximum := int(min(available, cap))
	return &RefundEligibility{WalletQuota: owner.Quota, MaxRefundableQuota: maximum, RequestableQuota: maximum, NonRefundableQuota: max(0, owner.Quota-maximum), FrozenQuota: int(frozen), Reasons: []RefundExclusion{}}, nil
}

// RefundOriginalView never changes balances when a ticket is viewed.
func RefundOriginalView(ticketID int) (*TicketRefund, error) {
	var refund TicketRefund
	if err := DB.Where("ticket_id = ?", ticketID).First(&refund).Error; err != nil {
		return nil, err
	}
	return &refund, nil
}
