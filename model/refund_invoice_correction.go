package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrRefundInvoiceCorrection = errors.New("refund invoice correction is invalid or state changed")

type RefundInvoiceCorrection struct {
	Id                int                         `json:"id"`
	UserId            int                         `json:"user_id" gorm:"index;not null"`
	RefundTicketId    int                         `json:"refund_ticket_id" gorm:"index;not null"`
	InvoiceTicketId   int                         `json:"invoice_ticket_id" gorm:"index;not null"`
	RequestKey        *string                     `json:"-" gorm:"size:64;uniqueIndex"`
	Reason            string                      `json:"reason" gorm:"type:text"`
	Phase             string                      `json:"phase" gorm:"size:32"`
	Version           int                         `json:"version"`
	DocumentsJSON     string                      `json:"-" gorm:"type:text"`
	Documents         []InvoiceCorrectionDocument `json:"documents" gorm:"-"`
	OriginalCents     int64                       `json:"original_cents" gorm:"type:bigint;not null"`
	RefundCents       int64                       `json:"refund_cents" gorm:"type:bigint;not null"`
	ReissueCents      int64                       `json:"reissue_cents" gorm:"type:bigint;not null"`
	OriginalReference string                      `json:"original_reference" gorm:"size:128"`
	RedReference      string                      `json:"red_reference" gorm:"size:128"`
	ReissueReference  string                      `json:"reissue_reference" gorm:"size:128"`
	Evidence          string                      `json:"evidence" gorm:"type:text"`
	Status            string                      `json:"status" gorm:"size:16;index"`
	HandledBy         int                         `json:"handled_by"`
	HandledTime       int64                       `json:"handled_time" gorm:"type:bigint"`
	CreatedTime       int64                       `json:"created_time" gorm:"type:bigint"`
}

// The caller holds the user's wallet lock throughout allocation and creation.
// Red invoices reverse the entire active blue invoice, including untouched orders.
func createRefundInvoiceCorrectionsTx(tx *gorm.DB, refund *TicketRefund, allocations []RefundOrderAllocation) error {
	amounts := map[int]int64{}
	orders := map[int]map[int]bool{}
	for _, allocation := range allocations {
		if allocation.InvoiceTicketID == 0 {
			continue
		}
		if allocation.RefundCents < 0 || allocation.TopUpID <= 0 || allocation.InvoiceTicketID < 0 || amounts[allocation.InvoiceTicketID] > (1<<63-1)-allocation.RefundCents {
			return ErrRefundInvoiceCorrection
		}
		amounts[allocation.InvoiceTicketID] += allocation.RefundCents
		if orders[allocation.InvoiceTicketID] == nil {
			orders[allocation.InvoiceTicketID] = map[int]bool{}
		}
		if orders[allocation.InvoiceTicketID][allocation.TopUpID] {
			return ErrRefundInvoiceCorrection
		}
		orders[allocation.InvoiceTicketID][allocation.TopUpID] = true
	}
	ids := make([]int, 0, len(amounts))
	for id := range amounts {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, invoiceID := range ids {
		amount := amounts[invoiceID]
		if amount == 0 {
			continue
		}
		var invoice TicketInvoice
		if err := lockForUpdate(tx).Where("ticket_id = ? AND user_id = ? AND invoice_status = ?", invoiceID, refund.UserId, InvoiceStatusIssued).First(&invoice).Error; err != nil {
			return ErrRefundInvoiceCorrection
		}
		linked, err := invoice.GetTopUpOrderIDs()
		if err != nil {
			return err
		}
		for _, id := range linked {
			delete(orders[invoiceID], id)
		}
		if len(orders[invoiceID]) != 0 {
			return ErrRefundInvoiceCorrection
		}
		var pending int64
		if err := tx.Model(&RefundInvoiceCorrection{}).Where("invoice_ticket_id = ? AND status IN ?", invoiceID, []string{"pending", "processing"}).Count(&pending).Error; err != nil {
			return err
		}
		if pending != 0 {
			return ErrRefundInvoiceCorrection
		}
		baseline, err := invoiceMoneyCents(invoice.TotalMoney)
		if err != nil {
			return err
		}
		reference := fmt.Sprintf("#%d", invoiceID)
		var previous RefundInvoiceCorrection
		err = tx.Where("invoice_ticket_id = ? AND status = ?", invoiceID, "completed").Order("id DESC").First(&previous).Error
		if err == nil {
			baseline, reference = previous.ReissueCents, previous.ReissueReference
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if baseline == 0 {
			continue
		}
		reissue := baseline - amount
		if baseline < 0 || reissue < 0 || (baseline > 0 && reference == "") {
			return ErrRefundInvoiceCorrection
		}
		if baseline == 0 && reissue == 0 {
			continue
		}
		requestKey := fmt.Sprintf("refund:%d:invoice:%d", refund.TicketId, invoiceID)
		correction := RefundInvoiceCorrection{RequestKey: &requestKey, Version: 1, Phase: "documents_required", UserId: refund.UserId, RefundTicketId: refund.TicketId, InvoiceTicketId: invoiceID, OriginalCents: baseline, RefundCents: amount, ReissueCents: reissue, OriginalReference: reference, Status: "pending", CreatedTime: common.GetTimestamp()}
		if err := tx.Create(&correction).Error; err != nil {
			return err
		}
	}
	return nil
}

// New external invoice work must wait until wallet liabilities can be proven
// settled. Existing processing corrections can still record their evidence.
func StartRefundInvoiceCorrection(correctionID, actorID int) (*RefundInvoiceCorrection, error) {
	if correctionID <= 0 || actorID <= 0 {
		return nil, ErrRefundInvoiceCorrection
	}
	var known RefundInvoiceCorrection
	if err := DB.First(&known, correctionID).Error; err != nil {
		return nil, err
	}
	if known.Version == 1 {
		return TransitionInvoiceCorrection(correctionID, actorID, "start", nil, "")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, known.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := tx.Where("id = ? AND status = ? AND role IN ?", actorID, common.UserStatusEnabled, []int{common.RoleFinanceUser, common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
			return ErrTicketForbidden
		}
		var refund TicketRefund
		if err := lockForUpdate(tx).Where("ticket_id = ? AND user_id = ? AND refund_status = ?", known.RefundTicketId, known.UserId, RefundStatusPending).First(&refund).Error; err != nil {
			return ErrRefundInvoiceCorrection
		}
		if !refund.hasVerifiedRefundHold() {
			return ErrRefundSettlementUnconfirmed
		}
		if err := verifyRefundLiabilitiesTx(tx, known.UserId); err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("id = ? AND status IN ?", correctionID, []string{"pending", "processing"}).First(&known).Error; err != nil {
			return ErrRefundInvoiceCorrection
		}
		if known.Status == "processing" {
			return nil
		}
		if err := tx.Model(&known).Update("status", "processing").Error; err != nil {
			return err
		}
		known.Status = "processing"
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &known, nil
}

func CompleteRefundInvoiceCorrection(correctionID, actorID int, redReference, reissueReference, evidence string) (*RefundInvoiceCorrection, error) {
	redReference, reissueReference, evidence = strings.TrimSpace(redReference), strings.TrimSpace(reissueReference), strings.TrimSpace(evidence)
	if correctionID <= 0 || actorID <= 0 || utf8.RuneCountInString(redReference) > 128 || utf8.RuneCountInString(reissueReference) > 128 || utf8.RuneCountInString(evidence) > 2000 {
		return nil, ErrRefundInvoiceCorrection
	}
	var known RefundInvoiceCorrection
	if err := DB.First(&known, correctionID).Error; err != nil {
		return nil, err
	}
	if known.Version == 1 {
		return nil, ErrRefundInvoiceCorrection
	}
	var result RefundInvoiceCorrection
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, known.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := tx.Where("id = ? AND status = ? AND role IN ?", actorID, common.UserStatusEnabled, []int{common.RoleFinanceUser, common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
			return ErrTicketForbidden
		}
		var refund TicketRefund
		if err := lockForUpdate(tx).Where("ticket_id = ? AND user_id = ? AND refund_status = ?", known.RefundTicketId, known.UserId, RefundStatusPending).First(&refund).Error; err != nil {
			return ErrRefundInvoiceCorrection
		}
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ? AND refund_ticket_id = ? AND status = ?", correctionID, known.UserId, known.RefundTicketId, "processing").First(&result).Error; err != nil {
			return ErrRefundInvoiceCorrection
		}
		if result.OriginalCents < 0 || result.ReissueCents < 0 || (result.OriginalCents > 0) != (redReference != "") || (result.ReissueCents > 0) != (reissueReference != "") || (redReference != "" && (redReference == reissueReference || redReference == result.OriginalReference)) || (reissueReference != "" && reissueReference == result.OriginalReference) {
			return ErrRefundInvoiceCorrection
		}
		now := common.GetTimestamp()
		update := tx.Model(&RefundInvoiceCorrection{}).Where("id = ? AND status = ?", correctionID, "processing").Updates(map[string]any{"status": "completed", "red_reference": redReference, "reissue_reference": reissueReference, "evidence": evidence, "handled_by": actorID, "handled_time": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrRefundInvoiceCorrection
		}
		result.Status, result.RedReference, result.ReissueReference, result.Evidence, result.HandledBy, result.HandledTime = "completed", redReference, reissueReference, evidence, actorID, now
		return tx.Create(&TicketMessage{TicketId: refund.TicketId, UserId: actorID, Username: actor.Username, Role: actor.Role, Content: fmt.Sprintf("Invoice correction recorded: invoice ticket #%d; original %s; red %s (%d cents); reissue %s (%d cents); correction #%d", result.InvoiceTicketId, result.OriginalReference, redReference, result.OriginalCents, reissueReference, result.ReissueCents, result.Id), CreatedTime: now}).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
