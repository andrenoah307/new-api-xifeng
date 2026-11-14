package model

import (
	"errors"
	"fmt"
	"math"

	"gorm.io/gorm"
)

// SynchronizeRefundHold repairs legacy holds when an authorized viewer opens a refund.
// Wallet and ticket changes are atomic; terminal refunds must never be frozen again.
func SynchronizeRefundHold(ticketID int) error {
	var identity TicketRefund
	if err := DB.Select("user_id").Where("ticket_id = ?", ticketID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTicketRefundNotFound
		}
		return err
	}
	previous, target := 0, 0
	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).Select("id, quota").First(&owner, identity.UserId).Error; err != nil {
			return err
		}
		var refund TicketRefund
		if err := lockForUpdate(tx).Where("ticket_id = ? AND user_id = ?", ticketID, owner.Id).First(&refund).Error; err != nil {
			return err
		}
		if refund.RefundStatus != RefundStatusPending {
			return nil
		}
		if refund.RefundQuota <= 0 || refund.FrozenQuota < 0 || refund.RefundGiftQuota < 0 || refund.RefundGiftQuota > math.MaxInt64-int64(refund.RefundQuota) || (!refund.SettlementVerified && refund.RefundGiftQuota != 0) {
			return ErrRefundFinanceReview
		}
		previous, target = refund.FrozenQuota, refund.RefundQuota+int(refund.RefundGiftQuota)
		if previous == target {
			return nil
		}
		var pending int64
		if err := tx.Model(&FinanceReconciliation{}).Where("user_id = ? AND status = ?", owner.Id, "submitted").Count(&pending).Error; err != nil {
			return err
		}
		var before *RefundReconciliationReport
		if pending > 0 {
			var err error
			before, err = refundReconciliationReportTx(tx, refund)
			if err != nil {
				return err
			}
		}
		if err := applyRefundWalletDeltaTx(tx, owner.Id, previous-target); err != nil {
			return err
		}
		if err := tx.Model(&refund).Update("frozen_quota", target).Error; err != nil {
			return err
		}
		// Moving money between available and held balances changes the source hash,
		// but not its accounting evidence. Preserve only proposals valid before this move.
		if before != nil {
			after, err := refundReconciliationReportTx(tx, refund)
			if err != nil {
				return err
			}
			if err := tx.Model(&FinanceReconciliation{}).Where("user_id = ? AND status = ? AND source_revision = ?", owner.Id, "submitted", before.Revision).Update("source_revision", after.Revision).Error; err != nil {
				return err
			}
		}
		changed = true
		return recordTicketOperationTx(tx, ticketID, 0, fmt.Sprintf("退款冻结同步：%d → %d quota。", previous, target))
	})
	if err == nil && changed {
		_ = InvalidateUserCache(identity.UserId)
		RecordLogWithAdminInfo(identity.UserId, LogTypeManage, fmt.Sprintf("Refund #%d hold synchronized", ticketID), map[string]interface{}{
			"ticket_id": ticketID, "previous_frozen_quota": previous, "frozen_quota": target, "reason": "view_refund_hold_sync",
		})
	}
	return err
}

func GetTicketRefundForView(ticketID int) (*TicketRefund, error) {
	if err := SynchronizeRefundHold(ticketID); err != nil {
		return nil, err
	}
	return GetTicketRefundByTicketId(ticketID)
}
