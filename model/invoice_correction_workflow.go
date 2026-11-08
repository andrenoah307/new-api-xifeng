package model

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type InvoiceCorrectionDocument struct {
	Issuer            string `json:"issuer"`
	OriginalReference string `json:"original_reference"`
	OriginalCents     int64  `json:"original_cents"`
	ReductionCents    int64  `json:"reduction_cents"`
	RedReference      string `json:"red_reference"`
	ReissueReference  string `json:"reissue_reference"`
}

func migrateFinanceCorrectionIndex() error {
	if DB.Migrator().HasIndex(&RefundInvoiceCorrection{}, "idx_refund_invoice_correction") {
		return DB.Migrator().DropIndex(&RefundInvoiceCorrection{}, "idx_refund_invoice_correction")
	}
	return nil
}

func (correction *RefundInvoiceCorrection) AfterFind(tx *gorm.DB) error {
	correction.Documents = []InvoiceCorrectionDocument{}
	if correction.DocumentsJSON != "" {
		return common.UnmarshalJsonStr(correction.DocumentsJSON, &correction.Documents)
	}
	return nil
}

// Legacy pending/processing rows can adopt per-document evidence. Recorded red
// invoices are irreversible here; failed replacement work resumes in place.
func TransitionInvoiceCorrection(id, actorID int, action string, documents []InvoiceCorrectionDocument, evidence string, confirmedNoRed ...bool) (*RefundInvoiceCorrection, error) {
	evidence = strings.TrimSpace(evidence)
	if len(documents) > 100 || utf8.RuneCountInString(evidence) > 4000 {
		return nil, ErrFinanceEvidence
	}
	var known RefundInvoiceCorrection
	if err := DB.First(&known, id).Error; err != nil {
		return nil, err
	}
	var result RefundInvoiceCorrection
	var audit string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, known.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := tx.Where("id = ? AND status = ? AND role IN ?", actorID, common.UserStatusEnabled, []int{common.RoleFinanceUser, common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
			return ErrTicketForbidden
		}
		if err := lockForUpdate(tx).First(&result, id).Error; err != nil {
			return err
		}
		if result.Version != 1 && result.Version != 0 {
			return ErrRefundInvoiceCorrection
		}
		legacyPreparing := result.Version == 0 && action == "prepare" && (result.Status == "pending" || result.Status == "processing") && result.RedReference == "" && result.ReissueReference == ""
		if action == "abort" && len(confirmedNoRed) == 1 && confirmedNoRed[0] && evidence != "" && result.Status == "pending" && strings.HasSuffix(result.Evidence, "\nNo red invoice issued: "+evidence) {
			return nil
		}
		// A retried evidence submission must not advance the state a second time.
		if (action == "red" && (result.Phase == "reissue_pending" || result.Phase == "completed")) || (action == "reissue" && result.Phase == "completed") {
			marker := "\nRed invoice evidence: "
			if action == "reissue" {
				marker = "\nReplacement evidence: "
			}
			if evidence == "" || len(documents) != len(result.Documents) || !strings.Contains(result.Evidence, marker+evidence) {
				return ErrFinanceEvidence
			}
			for i, doc := range documents {
				if action == "red" && strings.TrimSpace(doc.RedReference) != result.Documents[i].RedReference {
					return ErrFinanceEvidence
				}
				if action == "reissue" && strings.TrimSpace(doc.ReissueReference) != result.Documents[i].ReissueReference {
					return ErrFinanceEvidence
				}
			}
			return nil
		}
		if result.RefundTicketId > 0 {
			var refund TicketRefund
			if err := tx.Where("ticket_id = ? AND refund_status = ?", result.RefundTicketId, RefundStatusPending).First(&refund).Error; err != nil {
				return ErrRefundInvoiceCorrection
			}
			if action != "abort" && !refund.hasVerifiedRefundHold() {
				return ErrRefundSettlementUnconfirmed
			}
			if action == "start" {
				if err := verifyRefundLiabilitiesTx(tx, result.UserId); err != nil {
					return err
				}
			}
		}
		previous := result.Phase
		switch action {
		case "abort":
			if len(confirmedNoRed) != 1 || !confirmedNoRed[0] || evidence == "" || result.Status != "processing" || (result.Version == 1 && result.Phase != "red_processing") || result.RedReference != "" || result.ReissueReference != "" {
				return ErrRefundInvoiceCorrection
			}
			for _, document := range result.Documents {
				if document.RedReference != "" || document.ReissueReference != "" {
					return ErrRefundInvoiceCorrection
				}
			}
			result.Status, result.Phase = "pending", "ready"
			if len(result.Documents) == 0 {
				result.Phase = "documents_required"
			}
			result.Evidence += "\nNo red invoice issued: " + evidence
		case "prepare":
			if (result.Status != "pending" && !legacyPreparing) || evidence == "" || len(documents) == 0 || result.RedReference != "" || result.ReissueReference != "" {
				return ErrRefundInvoiceCorrection
			}
			for _, document := range result.Documents {
				if document.RedReference != "" || document.ReissueReference != "" {
					return ErrRefundInvoiceCorrection
				}
			}
			var original, reduction int64
			seen := map[string]bool{}
			for i := range documents {
				doc := &documents[i]
				doc.Issuer = strings.TrimSpace(doc.Issuer)
				doc.OriginalReference = strings.TrimSpace(doc.OriginalReference)
				key := doc.Issuer + "\x00" + doc.OriginalReference
				if doc.Issuer == "" || len(doc.Issuer) > 255 || doc.OriginalReference == "" || len(doc.OriginalReference) > 128 || strings.HasPrefix(doc.OriginalReference, "#") || seen[key] || doc.OriginalCents <= 0 || doc.OriginalCents > result.OriginalCents-original || doc.ReductionCents < 0 || doc.ReductionCents > doc.OriginalCents || doc.ReductionCents > result.OriginalCents-result.ReissueCents-reduction || doc.RedReference != "" || doc.ReissueReference != "" {
					return ErrFinanceEvidence
				}
				seen[key] = true
				original += doc.OriginalCents
				reduction += doc.ReductionCents
			}
			if original != result.OriginalCents || reduction != result.OriginalCents-result.ReissueCents {
				return ErrFinanceEvidence
			}
			var previousCorrection RefundInvoiceCorrection
			previousErr := tx.Where("invoice_ticket_id = ? AND status = ? AND id < ?", result.InvoiceTicketId, "completed", result.Id).Order("id DESC").First(&previousCorrection).Error
			if previousErr != nil && !errors.Is(previousErr, gorm.ErrRecordNotFound) {
				return previousErr
			}
			if previousErr == nil && previousCorrection.Version == 1 {
				active := map[string]int64{}
				for _, doc := range previousCorrection.Documents {
					if doc.OriginalCents > doc.ReductionCents {
						active[doc.Issuer+"\x00"+doc.ReissueReference] = doc.OriginalCents - doc.ReductionCents
					}
				}
				for _, doc := range documents {
					key := doc.Issuer + "\x00" + doc.OriginalReference
					if active[key] != doc.OriginalCents {
						return ErrFinanceEvidence
					}
					delete(active, key)
				}
				if len(active) != 0 {
					return ErrFinanceEvidence
				}
			}
			result.Documents = documents
			result.Evidence = evidence
			result.Phase = "ready"
			if legacyPreparing && result.Status == "processing" {
				result.Phase = "red_processing"
			}
			result.Version = 1
		case "start":
			if result.Status == "processing" && result.Phase == "red_processing" {
				return nil
			}
			if result.Status != "pending" || result.Phase != "ready" {
				return ErrRefundInvoiceCorrection
			}
			result.Status, result.Phase = "processing", "red_processing"
		case "red":
			if result.Status != "processing" || result.Phase != "red_processing" || evidence == "" || len(documents) != len(result.Documents) {
				return ErrRefundInvoiceCorrection
			}
			seen := map[string]bool{}
			for i, doc := range documents {
				original := result.Documents[i]
				ref := strings.TrimSpace(doc.RedReference)
				key := original.Issuer + "\x00" + ref
				if ref == "" || len(ref) > 128 || ref == original.OriginalReference || seen[key] {
					return ErrFinanceEvidence
				}
				seen[key] = true
				result.Documents[i].RedReference = ref
			}
			result.Phase = "reissue_pending"
			if result.ReissueCents == 0 {
				result.Phase, result.Status = "completed", "completed"
			}
			result.Evidence += "\nRed invoice evidence: " + evidence
		case "reissue":
			if result.Status != "processing" || result.Phase != "reissue_pending" || evidence == "" || len(documents) != len(result.Documents) {
				return ErrRefundInvoiceCorrection
			}
			seen := map[string]bool{}
			for i, doc := range documents {
				original := result.Documents[i]
				ref := strings.TrimSpace(doc.ReissueReference)
				key := original.Issuer + "\x00" + ref
				if (original.OriginalCents > original.ReductionCents) != (ref != "") || len(ref) > 128 || (ref != "" && (ref == original.OriginalReference || ref == original.RedReference || seen[key])) {
					return ErrFinanceEvidence
				}
				seen[key] = true
				result.Documents[i].ReissueReference = ref
			}
			result.Phase, result.Status = "completed", "completed"
			result.Evidence += "\nReplacement evidence: " + evidence
		case "cancel":
			if result.RefundTicketId != 0 || result.Status != "pending" || evidence == "" {
				return ErrRefundInvoiceCorrection
			}
			result.Phase, result.Status = "cancelled", "cancelled"
			result.Evidence += "\nCancelled: " + evidence
		default:
			return ErrRefundInvoiceCorrection
		}
		encoded, err := common.Marshal(result.Documents)
		if err != nil {
			return err
		}
		result.DocumentsJSON = string(encoded)
		result.HandledBy, result.HandledTime = actorID, common.GetTimestamp()
		if result.Status == "completed" {
			var red, blue []string
			for _, doc := range result.Documents {
				red = append(red, doc.RedReference)
				if doc.ReissueReference != "" {
					blue = append(blue, doc.ReissueReference)
				}
			}
			result.RedReference, result.ReissueReference = strings.Join(red, ", "), strings.Join(blue, ", ")
			// Aggregate references are display-only; individual records remain authoritative.
			if len(result.RedReference) > 128 {
				result.RedReference = "multiple documents"
			}
			if len(result.ReissueReference) > 128 {
				result.ReissueReference = "multiple documents"
			}
		}
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		audit = fmt.Sprintf("Invoice correction #%d: %s -> %s", id, previous, result.Phase)
		return nil
	})
	if err == nil && audit != "" {
		RecordLogWithAdminInfo(result.UserId, LogTypeManage, audit, map[string]interface{}{"admin_id": actorID, "invoice_ticket_id": result.InvoiceTicketId, "refund_ticket_id": result.RefundTicketId, "action": action, "evidence": evidence})
	}
	return &result, err
}

func CreateIndependentInvoiceCorrection(ticketID, actorID int, key, reason string, reissue bool) (*RefundInvoiceCorrection, error) {
	reason = strings.TrimSpace(reason)
	if len(key) < 16 || len(key) > 64 || utf8.RuneCountInString(reason) < 5 || utf8.RuneCountInString(reason) > 2000 {
		return nil, ErrFinanceEvidence
	}
	var known TicketInvoice
	if err := DB.Where("ticket_id = ?", ticketID).First(&known).Error; err != nil {
		return nil, err
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
		if err := tx.Where("request_key = ?", key).First(&result).Error; err == nil {
			if result.InvoiceTicketId != ticketID || result.RefundTicketId != 0 || result.Reason != reason || (result.ReissueCents > 0) != reissue {
				return ErrFinanceRevision
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var invoice TicketInvoice
		if err := lockForUpdate(tx).Where("ticket_id = ? AND invoice_status = ?", ticketID, InvoiceStatusIssued).First(&invoice).Error; err != nil {
			return ErrTicketInvoiceStatusInvalid
		}
		var active int64
		if err := tx.Model(&RefundInvoiceCorrection{}).Where("invoice_ticket_id = ? AND status IN ?", ticketID, []string{"pending", "processing"}).Count(&active).Error; err != nil {
			return err
		}
		if active != 0 {
			return ErrRefundFinanceCorrectionPending
		}
		var refunds int64
		if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", known.UserId, RefundStatusPending).Count(&refunds).Error; err != nil {
			return err
		}
		if refunds != 0 {
			return ErrRefundFinancePending
		}
		amount, err := invoiceMoneyCents(invoice.TotalMoney)
		if err != nil {
			return err
		}
		var previous RefundInvoiceCorrection
		if err := tx.Where("invoice_ticket_id = ? AND status = ?", ticketID, "completed").Order("id DESC").First(&previous).Error; err == nil {
			amount = previous.ReissueCents
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if amount <= 0 {
			return ErrRefundInvoiceCorrection
		}
		replacement := int64(0)
		if reissue {
			replacement = amount
		}
		result = RefundInvoiceCorrection{RequestKey: &key, Reason: reason, Version: 1, Phase: "documents_required", UserId: known.UserId, InvoiceTicketId: ticketID, OriginalCents: amount, ReissueCents: replacement, Status: "pending", CreatedTime: common.GetTimestamp()}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		return tx.Create(&TicketMessage{TicketId: ticketID, UserId: actorID, Username: actor.Username, Role: actor.Role, Content: fmt.Sprintf("Independent invoice correction #%d requested: %s", result.Id, reason), CreatedTime: result.CreatedTime}).Error
	})
	return &result, err
}
