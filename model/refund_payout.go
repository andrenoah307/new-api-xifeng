package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrRefundPayout = errors.New("refund payout requires a claimed attempt and verified payment evidence")

func CompleteRefundPayout(id, actorID int, reference, evidence string, clawback ...bool) (*TicketRefund, error) {
	var actor User
	if err := DB.Where("id = ? AND status = ? AND role IN ?", actorID, common.UserStatusEnabled, []int{common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
		return nil, ErrTicketForbidden
	}
	var attempt RefundPayoutAttempt
	if err := DB.First(&attempt, id).Error; err != nil {
		return nil, err
	}
	if attempt.Status == "succeeded" {
		if attempt.PaymentReference != strings.TrimSpace(reference) || attempt.Evidence != strings.TrimSpace(evidence) {
			return nil, ErrRefundPayout
		}
		var refund TicketRefund
		err := DB.Where("ticket_id = ?", attempt.TicketId).First(&refund).Error
		return &refund, err
	}
	deduct := len(clawback) == 0 || clawback[0]
	refund, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{TicketId: attempt.TicketId, AdminId: actorID, RefundStatus: RefundStatusRefunded, PayoutAttemptID: id, PayoutReference: reference, PayoutEvidence: evidence, ClawBackCommission: deduct})
	if err == nil {
		RecordLogWithAdminInfo(attempt.UserId, LogTypeManage, fmt.Sprintf("Refund payout #%d succeeded", id), map[string]interface{}{"admin_id": actorID, "ticket_id": attempt.TicketId, "reference": reference, "evidence": evidence})
	}
	if errors.Is(err, ErrTicketRefundNotPending) {
		var completed RefundPayoutAttempt
		if readErr := DB.First(&completed, id).Error; readErr == nil && completed.Status == "succeeded" && completed.PaymentReference == strings.TrimSpace(reference) && completed.Evidence == strings.TrimSpace(evidence) {
			var saved TicketRefund
			readErr = DB.Where("ticket_id = ?", attempt.TicketId).First(&saved).Error
			return &saved, readErr
		}
	}
	return refund, err
}

type RefundPayoutAttempt struct {
	Id               int     `json:"id"`
	TicketId         int     `json:"ticket_id" gorm:"index;not null"`
	UserId           int     `json:"user_id" gorm:"index;not null"`
	RequestKey       string  `json:"request_key" gorm:"size:64;uniqueIndex;not null"`
	Status           string  `json:"status" gorm:"size:24;not null"`
	CashCents        int64   `json:"cash_cents" gorm:"type:bigint;not null"`
	PaymentReference string  `json:"payment_reference" gorm:"size:128"`
	ReceiptKey       *string `json:"-" gorm:"size:64;uniqueIndex"`
	Evidence         string  `json:"evidence" gorm:"type:text"`
	ClaimedBy        int     `json:"claimed_by"`
	HandledBy        int     `json:"handled_by"`
	CreatedTime      int64   `json:"created_time"`
	HandledTime      int64   `json:"handled_time"`
}

func StartRefundPayout(ticketID, actorID int, key string) (*RefundPayoutAttempt, error) {
	if len(key) < 16 || len(key) > 64 {
		return nil, ErrRefundPayout
	}
	var known TicketRefund
	if err := DB.Where("ticket_id = ?", ticketID).First(&known).Error; err != nil {
		return nil, err
	}
	var result RefundPayoutAttempt
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, known.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := tx.Where("id = ? AND status = ? AND role IN ?", actorID, common.UserStatusEnabled, []int{common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
			return ErrTicketForbidden
		}
		if err := tx.Where("request_key = ?", key).First(&result).Error; err == nil {
			if result.TicketId != ticketID || result.ClaimedBy != actorID {
				return ErrRefundPayout
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var refund TicketRefund
		if err := lockForUpdate(tx).Where("ticket_id = ?", ticketID).First(&refund).Error; err != nil {
			return err
		}
		if refund.RefundStatus != RefundStatusPending || !refund.hasVerifiedRefundHold() || refund.FinanceVersion != 1 {
			return ErrRefundSettlementUnconfirmed
		}
		if err := verifyRefundLiabilitiesTx(tx, known.UserId); err != nil {
			return err
		}
		var pending int64
		if err := tx.Model(&RefundInvoiceCorrection{}).Where("refund_ticket_id = ? AND status != ?", ticketID, "completed").Count(&pending).Error; err != nil {
			return err
		}
		if pending != 0 {
			return ErrRefundFinanceCorrectionPending
		}
		if err := tx.Model(&RefundPayoutAttempt{}).Where("ticket_id = ? AND status != ?", ticketID, "failed").Count(&pending).Error; err != nil {
			return err
		}
		if pending != 0 {
			return ErrRefundPayout
		}
		result = RefundPayoutAttempt{TicketId: ticketID, UserId: known.UserId, RequestKey: key, Status: "processing", CashCents: refund.RefundCashCents, ClaimedBy: actorID, CreatedTime: common.GetTimestamp()}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		return recordTicketOperationTx(tx, ticketID, actorID, fmt.Sprintf("退款付款登记 #%d：已认领，实退%d分。", result.Id, result.CashCents))
	})
	if err == nil {
		RecordLogWithAdminInfo(known.UserId, LogTypeManage, fmt.Sprintf("Refund payout #%d claimed", result.Id), map[string]interface{}{"admin_id": actorID, "ticket_id": ticketID, "cash_cents": result.CashCents})
	}
	return &result, err
}

func RecordRefundPayoutReview(id, actorID int, status, evidence string) (*RefundPayoutAttempt, error) {
	evidence = strings.TrimSpace(evidence)
	if (status != "outcome_unknown" && status != "failed") || utf8.RuneCountInString(evidence) < 10 || utf8.RuneCountInString(evidence) > 4000 {
		return nil, ErrRefundPayout
	}
	var known RefundPayoutAttempt
	if err := DB.First(&known, id).Error; err != nil {
		return nil, err
	}
	var result RefundPayoutAttempt
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, known.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := tx.Where("id = ? AND status = ? AND role IN ?", actorID, common.UserStatusEnabled, []int{common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
			return ErrTicketForbidden
		}
		if err := lockForUpdate(tx).First(&result, id).Error; err != nil {
			return err
		}
		if result.Status == status && result.Evidence == evidence {
			return nil
		}
		if result.Status != "processing" && result.Status != "outcome_unknown" {
			return ErrRefundPayout
		}
		result.Status, result.Evidence, result.HandledBy, result.HandledTime = status, evidence, actorID, common.GetTimestamp()
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		return recordTicketOperationTx(tx, result.TicketId, actorID, fmt.Sprintf("退款付款登记 #%d：%s；依据：%s", id, status, evidence))
	})
	if err == nil {
		RecordLogWithAdminInfo(known.UserId, LogTypeManage, fmt.Sprintf("Refund payout #%d %s", id, status), map[string]interface{}{"admin_id": actorID, "ticket_id": result.TicketId, "evidence": evidence})
	}
	return &result, err
}

func completeRefundPayoutTx(tx *gorm.DB, refund *TicketRefund, params UpdateRefundStatusParams) error {
	reference, evidence := strings.TrimSpace(params.PayoutReference), strings.TrimSpace(params.PayoutEvidence)
	if params.PayoutAttemptID <= 0 || reference == "" || len(reference) > 128 || utf8.RuneCountInString(evidence) < 10 || utf8.RuneCountInString(evidence) > 4000 {
		return ErrRefundPayout
	}
	var attempt RefundPayoutAttempt
	if err := lockForUpdate(tx).Where("id = ? AND ticket_id = ? AND status IN ?", params.PayoutAttemptID, refund.TicketId, []string{"processing", "outcome_unknown"}).First(&attempt).Error; err != nil {
		return ErrRefundPayout
	}
	if attempt.CashCents != refund.RefundCashCents {
		return ErrRefundPayout
	}
	attempt.Status, attempt.PaymentReference, attempt.Evidence, attempt.HandledBy, attempt.HandledTime = "succeeded", reference, evidence, params.AdminId, common.GetTimestamp()
	// One receipt cannot be used to close multiple refund tickets.
	receiptKey := fmt.Sprintf("%x", sha256.Sum256([]byte(reference)))
	attempt.ReceiptKey = &receiptKey
	if err := tx.Save(&attempt).Error; err != nil {
		return err
	}
	return nil
}
