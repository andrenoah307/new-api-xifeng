package model

import (
	"errors"
	"net/mail"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const DefaultInvoiceMailReply = "发票已发送至您的邮箱，若未在收件箱找到，请留意查看垃圾箱。有任何问题请联系提提处理QQ：1528201845"

type InvoiceMailConfig struct {
	Reply          string `json:"reply"`
	Status         int    `json:"status"`
	RetentionHours int    `json:"retention_hours"`
}

func GetInvoiceMailConfig() InvoiceMailConfig {
	config := InvoiceMailConfig{Reply: DefaultInvoiceMailReply, Status: TicketStatusResolved, RetentionHours: 24}
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["InvoiceMailSettings"]
	common.OptionMapRWMutex.RUnlock()
	if raw != "" {
		_ = common.UnmarshalJsonStr(raw, &config)
	}
	if config.RetentionHours < 1 || config.RetentionHours > 168 {
		config.RetentionHours = 24
	}
	return config
}

func NormalizeInvoiceMailConfig(raw string) (string, error) {
	config := InvoiceMailConfig{RetentionHours: 24}
	if common.UnmarshalJsonStr(raw, &config) != nil || len(strings.TrimSpace(config.Reply)) == 0 || len(config.Reply) > 5000 || (config.Status != TicketStatusResolved && config.Status != TicketStatusProcessing) {
		return "", ErrInvoiceMail
	}
	if config.RetentionHours < 1 || config.RetentionHours > 168 {
		return "", ErrInvoiceMail
	}
	config.Reply = strings.TrimSpace(config.Reply)
	encoded, err := common.Marshal(config)
	return string(encoded), err
}

type InvoiceMailBatch struct {
	ID          string `json:"id" gorm:"primaryKey;type:varchar(36)"`
	OwnerID     int    `json:"owner_id" gorm:"index"`
	CreatedAt   int64  `json:"created_at" gorm:"index"`
	ExpiresAt   int64  `json:"expires_at" gorm:"index"`
	CleanedAt   int64  `json:"cleaned_at"`
	CancelledAt int64  `json:"cancelled_at"`
	// The bounded ZIP manifest can exceed MySQL TEXT's 64 KiB capacity.
	Manifest string `json:"-" gorm:"size:4194304"`
}

// Delivery keys identify the ticket plus sorted attachment content hashes. They
// survive draft deletion and repeat uploads, so a sent invoice is not re-sent.
type InvoiceMailDelivery struct {
	ID          int    `json:"id"`
	DeliveryKey string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	TicketID    int    `json:"ticket_id" gorm:"index"`
	ActorID     int    `json:"actor_id"`
	Recipient   string `json:"-" gorm:"type:varchar(254)"`
	State       string `json:"state" gorm:"type:varchar(16);index"`
	AttemptID   string `json:"-" gorm:"type:varchar(36)"`
	UpdatedAt   int64  `json:"updated_at"`
	MessageID   int    `json:"message_id"`
}

var ErrInvoiceMail = errors.New("invoice mail operation unavailable")

// A write lock also serializes SQLite cleanup against sending. Call only inside
// a transaction; keep the lock until SMTP or attachment deletion has finished.
func LockInvoiceMailBatch(tx *gorm.DB, id string) (*InvoiceMailBatch, error) {
	if err := tx.Model(&InvoiceMailBatch{}).Where("id = ?", id).UpdateColumn("cleaned_at", gorm.Expr("cleaned_at")).Error; err != nil {
		return nil, err
	}
	var batch InvoiceMailBatch
	if err := tx.First(&batch, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &batch, nil
}

func AuthorizeInvoiceMail(actorID int) (*User, error) {
	var actor User
	if err := DB.Select("id, username, role, status").First(&actor, actorID).Error; err != nil {
		return nil, ErrTicketForbidden
	}
	if actor.Status != common.UserStatusEnabled || (actor.Role != common.RoleFinanceUser && actor.Role != common.RoleRootUser) {
		return nil, ErrTicketForbidden
	}
	return &actor, nil
}

// Invoice delivery explicitly prefers the address supplied on the invoice.
// Only an empty invoice address falls back to the account's bound address.
func invoiceMailAddress(invoice *TicketInvoice, user *User) (string, error) {
	email := NormalizeEmail(invoice.Email)
	if email == "" {
		email = NormalizeEmail(user.Email)
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n") {
		return "", ErrTicketInvoiceEmailEmpty
	}
	return email, nil
}

func invoiceMailRefundLocked(tx *gorm.DB, invoice TicketInvoice) (bool, error) {
	// Issued invoices can already be in a refund/red-invoice workflow. Mail
	// delivery must still check the underlying order reservations in that case.
	invoice.InvoiceStatus = InvoiceStatusPending
	locked, err := invoiceRefundLocked(tx, &invoice)
	if err != nil || locked {
		return locked, err
	}
	var corrections int64
	err = tx.Model(&RefundInvoiceCorrection{}).Where("invoice_ticket_id = ? AND status IN ?", invoice.TicketId, []string{"pending", "processing", "completed"}).Count(&corrections).Error
	return corrections > 0, err
}

func InvoiceMailRecipient(ticketID int) (*Ticket, string, error) {
	var ticket Ticket
	if err := DB.First(&ticket, ticketID).Error; err != nil || ticket.Type != TicketTypeInvoice {
		return nil, "", ErrTicketInvoiceNotFound
	}
	var user User
	if err := DB.First(&user, ticket.UserId).Error; err != nil || user.Status != common.UserStatusEnabled {
		return &ticket, "", ErrInvoiceMail
	}
	var invoice TicketInvoice
	if err := DB.Where("ticket_id = ?", ticketID).First(&invoice).Error; err != nil || invoice.InvoiceStatus == InvoiceStatusRejected || ticket.Status == TicketStatusClosed {
		return &ticket, "", ErrTicketInvoiceStatusInvalid
	}
	email, err := invoiceMailAddress(&invoice, &user)
	if err != nil {
		return &ticket, "", err
	}
	locked, err := invoiceMailRefundLocked(DB, invoice)
	if err != nil || locked {
		return &ticket, email, ErrRefundFinancePending
	}
	var charge InvoiceTaxCharge
	err = DB.Where("ticket_id = ?", ticketID).Take(&charge).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return &ticket, email, err
	}
	if err == nil && charge.Status != "paid" {
		return &ticket, email, ErrInvoiceTaxUnpaid
	}
	return &ticket, email, nil
}

// DeliverInvoiceMail persists a claim before SMTP. Account/invoice locks remain
// held during the bounded SMTP call, preventing concurrent refund or tax changes.
// Any ambiguous SMTP/commit outcome requires explicit operator review.
func DeliverInvoiceMail(batchID string, ticketID, actorID int, key, recipient string, config InvoiceMailConfig, review string, send func() (bool, error)) error {
	if batchID == "" {
		return ErrInvoiceMail
	}
	if strings.TrimSpace(config.Reply) == "" || len(config.Reply) > 5000 || (config.Status != TicketStatusResolved && config.Status != TicketStatusProcessing) || len(key) != 64 {
		return ErrInvoiceMail
	}
	actor, err := AuthorizeInvoiceMail(actorID)
	if err != nil {
		return err
	}
	var ticket Ticket
	if err := DB.First(&ticket, ticketID).Error; err != nil {
		return ErrTicketNotFound
	}
	now := common.GetTimestamp()
	delivery := InvoiceMailDelivery{TicketID: ticketID, ActorID: actorID, DeliveryKey: key, Recipient: recipient, State: "ready", UpdatedAt: now}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&delivery).Error; err != nil {
		return err
	}
	delivery.ID = 0
	if err := DB.Where("delivery_key = ?", key).First(&delivery).Error; err != nil {
		return err
	}
	if delivery.State == "sent" {
		return nil
	}
	if delivery.TicketID != ticketID || (delivery.Recipient != recipient && delivery.State != "failed") {
		return ErrInvoiceMail
	}
	claim := DB.Model(&InvoiceMailDelivery{}).Where("id = ?", delivery.ID)
	if review == "delivered" || review == "not_delivered" {
		claim = claim.Where("state = ? OR (state = ? AND updated_at < ?)", "unknown", "sending", now-120)
	} else if review == "" {
		claim = claim.Where("state IN ?", []string{"ready", "failed"})
	} else {
		return ErrInvoiceMail
	}
	attemptID := uuid.NewString()
	result := claim.Updates(map[string]any{"state": "sending", "updated_at": now, "actor_id": actorID, "recipient": recipient, "attempt_id": attemptID})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvoiceMail
	}
	uncertain, accepted := false, false
	err = DB.Transaction(func(tx *gorm.DB) error {
		batch, err := LockInvoiceMailBatch(tx, batchID)
		if err != nil {
			return err
		}
		if (actor.Role != common.RoleRootUser && batch.OwnerID != actorID) || (review != "delivered" && (batch.CancelledAt != 0 || batch.CleanedAt != 0 || batch.ExpiresAt <= common.GetTimestamp())) {
			return ErrInvoiceMail
		}
		var owner User
		if err := lockForUpdate(tx).First(&owner, ticket.UserId).Error; err != nil {
			return err
		}
		var invoice TicketInvoice
		if err := lockForUpdate(tx).Where("ticket_id = ?", ticketID).First(&invoice).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&ticket, ticketID).Error; err != nil {
			return err
		}
		// An operator may reclaim a stale attempt while it is waiting for owner
		// locks. Fence that old attempt and hold the receipt lock through SMTP.
		var current InvoiceMailDelivery
		if err := lockForUpdate(tx).First(&current, delivery.ID).Error; err != nil {
			return err
		}
		if current.State != "sending" || current.AttemptID != attemptID {
			return ErrInvoiceMail
		}
		currentRecipient, addressErr := invoiceMailAddress(&invoice, &owner)
		if ticket.Type != TicketTypeInvoice || ticket.Status == TicketStatusClosed || invoice.InvoiceStatus == InvoiceStatusRejected || owner.Status != common.UserStatusEnabled || addressErr != nil || currentRecipient != recipient {
			return ErrInvoiceMail
		}
		locked, err := invoiceMailRefundLocked(tx, invoice)
		if err != nil {
			return err
		}
		if locked {
			return ErrRefundFinancePending
		}
		var other int64
		if err := tx.Model(&InvoiceMailDelivery{}).Where("ticket_id = ? AND id <> ? AND state IN ?", ticketID, delivery.ID, []string{"sending", "unknown"}).Count(&other).Error; err != nil {
			return err
		}
		if other != 0 {
			return ErrInvoiceMail
		}
		// Reuse all invoice tax, duplicate-order and refund checks on the same
		// transaction connection. No refunded quota side effects occur on issue.
		if _, _, _, err := updateInvoiceStatus(tx, ticketID, actorID, InvoiceStatusIssued); err != nil {
			return err
		}
		message := TicketMessage{TicketId: ticketID, UserId: actorID, Username: actor.Username, Role: common.RoleAdminUser, Content: config.Reply, CreatedTime: now}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Model(&Ticket{}).Where("id = ?", ticketID).Updates(map[string]any{"status": config.Status, "updated_time": now, "admin_id": actorID}).Error; err != nil {
			return err
		}
		if review != "" {
			if err := recordTicketOperationTx(tx, ticketID, actorID, "Invoice mail delivery reviewed: "+review); err != nil {
				return err
			}
		}
		if review != "delivered" {
			var sendErr error
			uncertain, sendErr = send()
			if sendErr != nil {
				return sendErr
			}
		}
		accepted = true
		result := tx.Model(&InvoiceMailDelivery{}).Where("id = ? AND state = ? AND attempt_id = ?", delivery.ID, "sending", attemptID).Updates(map[string]any{"state": "sent", "message_id": message.Id, "updated_at": common.GetTimestamp()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceMail
		}
		return nil
	})
	if err != nil {
		state := "failed"
		if uncertain || accepted || review == "delivered" {
			state = "unknown"
		}
		if saveErr := DB.Model(&InvoiceMailDelivery{}).Where("id = ? AND state = ? AND attempt_id = ?", delivery.ID, "sending", attemptID).Updates(map[string]any{"state": state, "updated_at": common.GetTimestamp()}).Error; saveErr != nil {
			common.SysError("invoice mail outcome persistence failed: " + saveErr.Error())
		}
		return err
	}
	RecordLog(ticket.UserId, LogTypeSystem, "Invoice email delivered; ticket "+common.Interface2String(ticketID)+"; operator "+common.Interface2String(actorID))
	return nil
}
