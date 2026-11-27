package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedInvoiceMail(t *testing.T) (*Ticket, *TicketInvoice, *User) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&InvoiceMailBatch{}, &InvoiceMailDelivery{}))
	t.Cleanup(func() {
		require.NoError(t, DB.Where("id = ?", "invoice-mail-test").Delete(&InvoiceMailBatch{}).Error)
		require.NoError(t, DB.Where("1 = 1").Delete(&InvoiceMailDelivery{}).Error)
		require.NoError(t, DB.Where("user_id = ?", 930001).Delete(&InvoiceTaxCharge{}).Error)
	})
	owner := insertInvoiceTicketUser(t, 930001)
	require.NoError(t, DB.Model(owner).Update("email", "account@example.test").Error)
	actor := insertInvoiceTicketUser(t, 930002)
	require.NoError(t, DB.Model(actor).Update("role", common.RoleFinanceUser).Error)
	require.NoError(t, DB.Create(&InvoiceMailBatch{ID: "invoice-mail-test", OwnerID: actor.Id, ExpiresAt: common.GetTimestamp() + 86400}).Error)
	ticket := &Ticket{UserId: owner.Id, Subject: "Invoice", Type: TicketTypeInvoice, Status: TicketStatusProcessing}
	require.NoError(t, DB.Create(ticket).Error)
	invoice := &TicketInvoice{TicketId: ticket.Id, UserId: owner.Id, Email: "invoice@example.test", TopUpOrderIds: "[]", InvoiceStatus: InvoiceStatusPending}
	orderID := seedInvoiceTicketTopUp(t, owner.Id, "invoice-mail-order", 10)
	require.NoError(t, invoice.SetTopUpOrderIDs([]int{orderID}))
	require.NoError(t, DB.Create(invoice).Error)
	return ticket, invoice, actor
}

func TestInvoiceMailRecipientPriorityAndRoleBoundary(t *testing.T) {
	ticket, invoice, actor := seedInvoiceMail(t)
	_, recipient, err := InvoiceMailRecipient(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, "invoice@example.test", recipient)
	require.NoError(t, DB.Model(invoice).Update("email", "").Error)
	_, recipient, err = InvoiceMailRecipient(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, "account@example.test", recipient)
	require.NoError(t, DB.Model(invoice).Update("email", "invalid").Error)
	_, _, err = InvoiceMailRecipient(ticket.Id)
	require.ErrorIs(t, err, ErrTicketInvoiceEmailEmpty)
	for _, role := range []int{common.RoleCommonUser, common.RoleCustomerServiceUser, common.RoleAdminUser, common.RoleFinanceUser, common.RoleRootUser} {
		require.NoError(t, DB.Model(actor).Update("role", role).Error)
		_, err := AuthorizeInvoiceMail(actor.Id)
		if role == common.RoleFinanceUser || role == common.RoleRootUser {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, ErrTicketForbidden)
		}
	}
	require.NoError(t, DB.Model(actor).Update("status", common.UserStatusDisabled).Error)
	_, err = AuthorizeInvoiceMail(actor.Id)
	require.ErrorIs(t, err, ErrTicketForbidden)
}

func TestInvoiceMailRetentionValidation(t *testing.T) {
	for _, value := range []string{`{"reply":"Sent","status":3}`, `{"reply":"Sent","status":3,"retention_hours":1}`, `{"reply":"Sent","status":3,"retention_hours":168}`} {
		normalized, err := NormalizeInvoiceMailConfig(value)
		require.NoError(t, err)
		var config InvoiceMailConfig
		require.NoError(t, common.UnmarshalJsonStr(normalized, &config))
		assert.GreaterOrEqual(t, config.RetentionHours, 1)
		assert.LessOrEqual(t, config.RetentionHours, 168)
		if !strings.Contains(value, "retention_hours") {
			assert.Equal(t, 24, config.RetentionHours)
		}
	}
	for _, hours := range []string{"0", "-1", "169", "1.5", "9999999999999999999999999"} {
		_, err := NormalizeInvoiceMailConfig(`{"reply":"Sent","status":3,"retention_hours":` + hours + `}`)
		require.Error(t, err)
	}
}

func TestInvoiceMailCleanedBatchAllowsReceiptReviewWithoutSending(t *testing.T) {
	ticket, invoice, actor := seedInvoiceMail(t)
	require.NoError(t, DB.Model(&InvoiceMailBatch{}).Where("id = ?", "invoice-mail-test").Updates(map[string]any{"cancelled_at": common.GetTimestamp(), "cleaned_at": common.GetTimestamp(), "expires_at": common.GetTimestamp() - 1}).Error)
	key := strings.Repeat("f", 64)
	require.NoError(t, DB.Create(&InvoiceMailDelivery{TicketID: ticket.Id, DeliveryKey: key, Recipient: "invoice@example.test", State: "unknown"}).Error)
	calls := 0
	err := DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, "invoice@example.test", InvoiceMailConfig{Reply: "Sent", Status: TicketStatusResolved}, "delivered", func() (bool, error) { calls++; return false, nil })
	require.NoError(t, err)
	assert.Zero(t, calls)
	require.NoError(t, DB.First(invoice, invoice.Id).Error)
	assert.Equal(t, InvoiceStatusIssued, invoice.InvoiceStatus)
	var receipt InvoiceMailDelivery
	require.NoError(t, DB.Where("delivery_key = ?", key).First(&receipt).Error)
	assert.Equal(t, "sent", receipt.State)
}

func TestInvoiceMailRevokedBatchCannotSendFromStalePreview(t *testing.T) {
	for _, state := range []string{"cancelled", "expired", "cleaned", "other-owner"} {
		t.Run(state, func(t *testing.T) {
			ticket, invoice, actor := seedInvoiceMail(t)
			batch := InvoiceMailBatch{ID: "batch-" + state, OwnerID: actor.Id, ExpiresAt: common.GetTimestamp() + 3600}
			switch state {
			case "cancelled":
				batch.CancelledAt = common.GetTimestamp()
			case "expired":
				batch.ExpiresAt = common.GetTimestamp() - 1
			case "cleaned":
				batch.CleanedAt = common.GetTimestamp()
			case "other-owner":
				batch.OwnerID++
			}
			require.NoError(t, DB.Create(&batch).Error)
			t.Cleanup(func() { require.NoError(t, DB.Delete(&batch).Error) })
			calls := 0
			err := DeliverInvoiceMail(batch.ID, ticket.Id, actor.Id, strings.Repeat("d", 64), "invoice@example.test", InvoiceMailConfig{Reply: "Sent", Status: TicketStatusResolved}, "", func() (bool, error) { calls++; return false, nil })
			require.Error(t, err)
			assert.Zero(t, calls)
			require.NoError(t, DB.First(invoice, invoice.Id).Error)
			assert.Equal(t, InvoiceStatusPending, invoice.InvoiceStatus)
			var count int64
			require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.Id).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestInvoiceMailDeliveryAtomicityAndDeduplication(t *testing.T) {
	for _, outcome := range []string{"success", "failed", "unknown"} {
		t.Run(outcome, func(t *testing.T) {
			ticket, invoice, actor := seedInvoiceMail(t)
			calls := 0
			send := func() (bool, error) {
				calls++
				if outcome == "success" {
					return false, nil
				}
				return outcome == "unknown", errors.New("SMTP failed")
			}
			config := InvoiceMailConfig{Reply: DefaultInvoiceMailReply, Status: TicketStatusResolved}
			key := strings.Repeat("a", 64)
			err := DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, "invoice@example.test", config, "", send)
			require.Equal(t, 1, calls)
			require.NoError(t, DB.First(ticket, ticket.Id).Error)
			require.NoError(t, DB.First(invoice, invoice.Id).Error)
			var messages []TicketMessage
			require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).Find(&messages).Error)
			var delivery InvoiceMailDelivery
			require.NoError(t, DB.Where("delivery_key = ?", key).First(&delivery).Error)
			if outcome == "success" {
				require.NoError(t, err)
				assert.Equal(t, InvoiceStatusIssued, invoice.InvoiceStatus)
				assert.Equal(t, TicketStatusResolved, ticket.Status)
				require.Len(t, messages, 1)
				assert.Equal(t, DefaultInvoiceMailReply, messages[0].Content)
				assert.Positive(t, messages[0].CreatedTime)
				assert.Equal(t, "sent", delivery.State)
				require.NoError(t, DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, "invoice@example.test", config, "", send))
				assert.Equal(t, 1, calls)
			} else {
				require.Error(t, err)
				assert.Equal(t, InvoiceStatusPending, invoice.InvoiceStatus)
				assert.Equal(t, TicketStatusProcessing, ticket.Status)
				assert.Empty(t, messages)
				assert.Equal(t, outcome, delivery.State)
				if outcome == "unknown" {
					require.Error(t, DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, "invoice@example.test", config, "", send))
					assert.Equal(t, 1, calls)
					require.NoError(t, DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, "invoice@example.test", config, "delivered", send))
					assert.Equal(t, 1, calls)
					require.NoError(t, DB.First(invoice, invoice.Id).Error)
					assert.Equal(t, InvoiceStatusIssued, invoice.InvoiceStatus)
				} else {
					require.NoError(t, DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, "invoice@example.test", config, "", func() (bool, error) { calls++; return false, nil }))
					assert.Equal(t, 2, calls)
				}
			}
		})
	}
}

func TestInvoiceMailRevalidatesBeforeSMTP(t *testing.T) {
	for _, change := range []string{"recipient", "closed", "tax", "role", "duplicate", "issued-refund"} {
		t.Run(change, func(t *testing.T) {
			ticket, invoice, actor := seedInvoiceMail(t)
			switch change {
			case "recipient":
				require.NoError(t, DB.Model(invoice).Update("email", "changed@example.test").Error)
			case "closed":
				require.NoError(t, DB.Model(ticket).Update("status", TicketStatusClosed).Error)
			case "tax":
				require.NoError(t, DB.Create(&InvoiceTaxCharge{TicketId: ticket.Id, UserId: ticket.UserId, TradeNo: "unpaid-invoice-mail", Status: "pending"}).Error)
			case "role":
				require.NoError(t, DB.Model(actor).Update("role", common.RoleAdminUser).Error)
			case "issued-refund":
				require.NoError(t, DB.Model(invoice).Update("invoice_status", InvoiceStatusIssued).Error)
				correction := RefundInvoiceCorrection{InvoiceTicketId: ticket.Id, Status: "pending"}
				require.NoError(t, DB.Create(&correction).Error)
				t.Cleanup(func() { require.NoError(t, DB.Delete(&correction).Error) })
			case "duplicate":
				other := Ticket{UserId: ticket.UserId, Type: TicketTypeInvoice, Status: TicketStatusResolved}
				require.NoError(t, DB.Create(&other).Error)
				require.NoError(t, DB.Create(&TicketInvoice{TicketId: other.Id, UserId: ticket.UserId, TopUpOrderIds: invoice.TopUpOrderIds, InvoiceStatus: InvoiceStatusIssued}).Error)
			}
			called := false
			err := DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, strings.Repeat("b", 64), "invoice@example.test", InvoiceMailConfig{Reply: "Sent", Status: TicketStatusResolved}, "", func() (bool, error) { called = true; return false, nil })
			require.Error(t, err)
			assert.False(t, called)
		})
	}
}

func TestInvoiceMailAcceptedButPersistenceFailsRequiresReview(t *testing.T) {
	ticket, invoice, actor := seedInvoiceMail(t)
	key := strings.Repeat("c", 64)
	config := InvoiceMailConfig{Reply: "Delivered", Status: TicketStatusResolved}
	injected := errors.New("delivery persistence failed")
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("test:invoice_mail_receipt", func(tx *gorm.DB) {
		if tx.Statement.Table != "invoice_mail_deliveries" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if ok && updates["state"] == "sent" {
			tx.AddError(injected)
		}
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove("test:invoice_mail_receipt")) })
	calls := 0
	send := func() (bool, error) { calls++; return false, nil }
	err := DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, "invoice@example.test", config, "", send)
	require.ErrorIs(t, err, injected)
	assert.Equal(t, 1, calls)
	var delivery InvoiceMailDelivery
	require.NoError(t, DB.Where("delivery_key = ?", key).First(&delivery).Error)
	assert.Equal(t, "unknown", delivery.State)
	require.NoError(t, DB.First(invoice, invoice.Id).Error)
	assert.Equal(t, InvoiceStatusPending, invoice.InvoiceStatus)
	require.Error(t, DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, "invoice@example.test", config, "", send))
	assert.Equal(t, 1, calls)
}

func TestInvoiceMailSettingsValidateReplyAndStatus(t *testing.T) {
	for _, raw := range []string{`{"reply":"Sent","status":3}`, `{"reply":"Sent","status":2}`} {
		_, err := NormalizeInvoiceMailConfig(raw)
		require.NoError(t, err)
	}
	for _, raw := range []string{`{}`, `{"reply":" ","status":3}`, `{"reply":"Sent","status":4}`, `{"reply":"Sent","status":1}`, `{"reply":"Sent","status":-1}`} {
		_, err := NormalizeInvoiceMailConfig(raw)
		require.Error(t, err)
	}
}

func TestInvoiceMailStaleAttemptRequiresExplicitReview(t *testing.T) {
	ticket, invoice, actor := seedInvoiceMail(t)
	require.NoError(t, DB.Model(invoice).Updates(map[string]any{"invoice_status": InvoiceStatusIssued, "issued_time": int64(100)}).Error)
	key := strings.Repeat("d", 64)
	receipt := InvoiceMailDelivery{DeliveryKey: key, TicketID: ticket.Id, ActorID: actor.Id, Recipient: "invoice@example.test", State: "sending", AttemptID: "old-attempt", UpdatedAt: common.GetTimestamp() - 121}
	require.NoError(t, DB.Create(&receipt).Error)
	config := InvoiceMailConfig{Reply: "Sent", Status: TicketStatusProcessing}
	calls := 0
	send := func() (bool, error) { calls++; return false, nil }
	require.Error(t, DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, receipt.Recipient, config, "", send))
	assert.Zero(t, calls)
	require.NoError(t, DeliverInvoiceMail("invoice-mail-test", ticket.Id, actor.Id, key, receipt.Recipient, config, "not_delivered", send))
	assert.Equal(t, 1, calls)
	require.NoError(t, DB.First(invoice, invoice.Id).Error)
	assert.Equal(t, int64(100), invoice.IssuedTime)
	require.NoError(t, DB.First(ticket, ticket.Id).Error)
	assert.Equal(t, TicketStatusProcessing, ticket.Status)
}
