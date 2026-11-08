package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

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
	var paid, bonus, refunded, frozen int64
	if err := tx.Model(&TopUp{}).Where("user_id = ? AND status = ? AND (source IS NULL OR source != ?)", userID, common.TopUpStatusSuccess, "discount_bonus").Select("COALESCE(SUM(quota_granted),0)").Scan(&paid).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&TopUp{}).Where("user_id = ? AND status = ? AND source = ?", userID, common.TopUpStatusSuccess, "discount_bonus").Select("COALESCE(SUM(quota_granted),0)").Scan(&bonus).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", userID, RefundStatusRefunded).Select("COALESCE(SUM(refund_quota),0)").Scan(&refunded).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", userID, RefundStatusPending).Select("COALESCE(SUM(frozen_quota),0)").Scan(&frozen).Error; err != nil {
		return nil, err
	}
	available := max(int64(0), int64(owner.Quota))
	available = max(int64(0), available-max(int64(0), int64(owner.AffHistoryQuota)-int64(owner.AffQuota)))
	available = max(int64(0), available-max(int64(0), bonus))
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
