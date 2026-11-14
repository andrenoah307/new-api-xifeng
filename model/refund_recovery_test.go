package model

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedLegacyRecoveryRefund(t *testing.T, version int) (refundFinanceFixture, Ticket, RefundInvoiceCorrection, []InvoiceCorrectionDocument) {
	t.Helper()
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 10000, 10000, 0, 1)
	require.NoError(t, DB.Model(&f.user).Update("quota", 9000).Error)
	ticket := Ticket{UserId: f.user.Id, Type: TicketTypeRefund, Status: TicketStatusProcessing}
	require.NoError(t, DB.Create(&ticket).Error)
	invoice := Ticket{UserId: f.user.Id, Type: TicketTypeInvoice, Status: TicketStatusResolved}
	require.NoError(t, DB.Create(&invoice).Error)
	invoiceData := TicketInvoice{TicketId: invoice.Id, UserId: f.user.Id, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 100}
	require.NoError(t, invoiceData.SetTopUpOrderIDs([]int{order.Id}))
	require.NoError(t, DB.Create(&invoiceData).Error)
	refund := TicketRefund{TicketId: ticket.Id, UserId: f.user.Id, RefundStatus: RefundStatusPending, RefundQuota: 1000, FrozenQuota: 1000, RefundCashCents: 1000, FinanceVersion: 1, SettlementVerified: true}
	require.NoError(t, DB.Create(&refund).Error)
	require.NoError(t, DB.Create(&RefundOrderAllocation{RefundTicketID: ticket.Id, UserID: f.user.Id, TopUpID: order.Id, TradeNo: order.TradeNo, OriginalQuota: 10000, RefundQuota: 1000, OriginalPaidCents: 10000, RefundCents: 1000, InvoiceTicketID: invoice.Id, Status: "reserved"}).Error)
	correction := RefundInvoiceCorrection{UserId: f.user.Id, RefundTicketId: ticket.Id, InvoiceTicketId: invoice.Id, Version: version, Phase: "documents_required", Status: "pending", OriginalCents: 10000, RefundCents: 1000, ReissueCents: 9000}
	require.NoError(t, DB.Create(&correction).Error)
	docs := []InvoiceCorrectionDocument{{Issuer: "Issuer", OriginalReference: "BLUE-1", OriginalCents: 10000, ReductionCents: 1000}}
	return f, ticket, correction, docs
}

func TestRefundRecoveryUnissuedCorrection(t *testing.T) {
	for _, version := range []int{0, 1} {
		for _, reject := range []bool{false, true} {
			t.Run(fmt.Sprintf("version_%d_reject_%t", version, reject), func(t *testing.T) {
				f, ticket, correction, docs := seedLegacyRecoveryRefund(t, version)
				if version == 1 {
					_, err := TransitionInvoiceCorrection(correction.Id, f.finance.Id, "prepare", docs, "Verified original invoice")
					require.NoError(t, err)
				}
				_, err := StartRefundInvoiceCorrection(correction.Id, f.finance.Id)
				require.NoError(t, err)
				closed, resolved := TicketStatusClosed, TicketStatusResolved
				_, _, err = UpdateTicketStatus(ticket.Id, f.admin.Id, &resolved, nil)
				require.Error(t, err)
				_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected, RejectionReason: "Wrong payment details"})
				require.Error(t, err)
				var messages int64
				require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ? AND content LIKE ?", ticket.Id, "驳回理由:%").Count(&messages).Error)
				assert.Zero(t, messages)
				_, err = TransitionInvoiceCorrection(correction.Id, f.user.Id, "abort", nil, "Checked issuer; no red invoice exists", true)
				require.ErrorIs(t, err, ErrTicketForbidden)
				_, err = TransitionInvoiceCorrection(correction.Id, f.admin.Id, "abort", nil, "Checked issuer; no red invoice exists")
				require.Error(t, err)
				for i := 0; i < 2; i++ {
					row, err := TransitionInvoiceCorrection(correction.Id, f.admin.Id, "abort", nil, "", true)
					require.NoError(t, err)
					assert.Equal(t, "pending", row.Status)
				}
				if reject {
					_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected, RejectionReason: "Wrong payment details"})
					require.NoError(t, err)
					require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ? AND content LIKE ?", ticket.Id, "驳回理由:%").Count(&messages).Error)
					assert.Equal(t, int64(1), messages)
				}
				for i := 0; i < 2; i++ {
					_, _, err = UpdateTicketStatus(ticket.Id, f.admin.Id, &closed, nil)
					require.NoError(t, err)
				}
				var owner User
				require.NoError(t, DB.First(&owner, f.user.Id).Error)
				assert.Equal(t, 10000, owner.Quota)
				var saved TicketRefund
				require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&saved).Error)
				assert.Zero(t, saved.FrozenQuota)
				require.NoError(t, DB.First(&correction, correction.Id).Error)
				assert.Equal(t, "cancelled", correction.Phase)
			})
		}
	}
}

func TestAdminCloseUnissuedRefundAllowsNewApplication(t *testing.T) {
	for _, version := range []int{0, 1} {
		t.Run(fmt.Sprintf("version_%d", version), func(t *testing.T) {
			f, ticket, correction, docs := seedLegacyRecoveryRefund(t, version)
			if version == 1 {
				_, err := TransitionInvoiceCorrection(correction.Id, f.finance.Id, "prepare", docs, "")
				require.NoError(t, err)
			}
			_, err := StartRefundInvoiceCorrection(correction.Id, f.finance.Id)
			require.NoError(t, err)
			// A user's ordinary cancellation must not cancel finance work in progress.
			err = DB.Transaction(func(tx *gorm.DB) error { return CancelPendingRefund(tx, ticket.Id, f.user.Id) })
			require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
			closed := TicketStatusClosed
			_, _, err = UpdateTicketStatus(ticket.Id, f.finance.Id, &closed, nil)
			require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
			for i := 0; i < 2; i++ {
				_, _, err = UpdateTicketStatus(ticket.Id, f.admin.Id, &closed, nil)
				require.NoError(t, err)
			}
			var owner User
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, 10000, owner.Quota)
			var refund TicketRefund
			require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&refund).Error)
			assert.Equal(t, RefundStatusCancelled, refund.RefundStatus)
			assert.Zero(t, refund.FrozenQuota)
			require.NoError(t, DB.First(&correction, correction.Id).Error)
			assert.Equal(t, "cancelled", correction.Status)
			assert.Equal(t, "cancelled", correction.Phase)
			var reserved int64
			require.NoError(t, DB.Model(&RefundOrderAllocation{}).Where("refund_ticket_id = ? AND status = ?", ticket.Id, "reserved").Count(&reserved).Error)
			assert.Zero(t, reserved)
			_, next, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 1000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()})
			require.NoError(t, err)
			assert.Equal(t, RefundStatusPending, next.RefundStatus)
			assert.Equal(t, 1000, next.FrozenQuota)
		})
	}
}

func TestRefundRecoveryIssuedCorrectionCompletesOnce(t *testing.T) {
	for _, version := range []int{0, 1} {
		t.Run(fmt.Sprintf("version_%d", version), func(t *testing.T) {
			f, ticket, correction, docs := seedLegacyRecoveryRefund(t, version)
			if version == 0 {
				_, err := StartRefundInvoiceCorrection(correction.Id, f.finance.Id)
				require.NoError(t, err)
			}
			row, err := TransitionInvoiceCorrection(correction.Id, f.finance.Id, "prepare", docs, "")
			require.NoError(t, err)
			assert.Equal(t, 1, row.Version)
			_, err = TransitionInvoiceCorrection(correction.Id, f.finance.Id, "start", nil, "")
			require.NoError(t, err)
			_, err = TransitionInvoiceCorrection(correction.Id, f.finance.Id, "red", docs, "")
			require.ErrorIs(t, err, ErrFinanceEvidence)
			docs[0].RedReference = "RED-1"
			for i := 0; i < 2; i++ {
				_, err = TransitionInvoiceCorrection(correction.Id, f.finance.Id, "red", docs, "")
				require.NoError(t, err)
			}
			_, err = TransitionInvoiceCorrection(correction.Id, f.admin.Id, "abort", nil, "Cannot discard actual red invoice", true)
			require.Error(t, err)
			closedAfterRed := TicketStatusClosed
			_, _, err = UpdateTicketStatus(ticket.Id, f.admin.Id, &closedAfterRed, nil)
			require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
			_, err = StartRefundPayout(ticket.Id, f.admin.Id, uuid.NewString())
			require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
			docs[0].ReissueReference = "BLUE-2"
			_, err = TransitionInvoiceCorrection(correction.Id, f.finance.Id, "reissue", docs, "")
			require.NoError(t, err)
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
			require.ErrorIs(t, err, ErrRefundLegacyWorkflow)
			attempt, err := StartRefundPayout(ticket.Id, f.admin.Id, uuid.NewString())
			require.NoError(t, err)
			_, err = RecordRefundPayoutReview(attempt.Id, f.admin.Id, "outcome_unknown", "Awaiting payment provider confirmation")
			require.NoError(t, err)
			closed := TicketStatusClosed
			_, _, err = UpdateTicketStatus(ticket.Id, f.admin.Id, &closed, nil)
			require.ErrorIs(t, err, ErrRefundPayout)
			_, err = RecordRefundPayoutReview(attempt.Id, f.admin.Id, "failed", "Payment provider confirmed no transfer occurred")
			require.NoError(t, err)
			_, _, err = UpdateTicketStatus(ticket.Id, f.admin.Id, &closed, nil)
			require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
			attempt, err = StartRefundPayout(ticket.Id, f.admin.Id, uuid.NewString())
			require.NoError(t, err)
			_, err = RecordRefundPayoutReview(attempt.Id, f.admin.Id, "outcome_unknown", "Awaiting confirmation of the retry")
			require.NoError(t, err)
			for i := 0; i < 2; i++ {
				refund, err := CompleteRefundPayout(attempt.Id, f.admin.Id, "BANK-1", "Payment provider confirmed the transfer", false)
				require.NoError(t, err)
				assert.Equal(t, RefundStatusRefunded, refund.RefundStatus)
				assert.Zero(t, refund.FrozenQuota)
			}
			_, _, err = UpdateTicketStatus(ticket.Id, f.admin.Id, &closed, nil)
			require.NoError(t, err)
			var owner User
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, 9000, owner.Quota)
			var publicAudits int64
			require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id IN ? AND (internal IS NULL OR internal = ?)", []int{ticket.Id, correction.InvoiceTicketId}, false).Count(&publicAudits).Error)
			assert.Zero(t, publicAudits)
		})
	}
}

func TestRefundAfterIndependentFullInvoiceReversal(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error) })
	order := seedRefundFinanceOrder(t, f.user.Id, 10000, 9000, 0, 1)
	require.NoError(t, DB.Model(&f.user).Update("quota", 10000).Error)
	invoice := TicketInvoice{TicketId: 800, UserId: f.user.Id, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 90}
	require.NoError(t, invoice.SetTopUpOrderIDs([]int{order.Id}))
	require.NoError(t, DB.Create(&invoice).Error)
	require.NoError(t, DB.Create(&RefundInvoiceCorrection{InvoiceTicketId: 800, UserId: f.user.Id, Version: 1, Status: "completed", Phase: "completed", OriginalCents: 9000, ReissueCents: 0}).Error)
	ticket, _, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 10000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()})
	require.NoError(t, err)
	refund, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	assert.Equal(t, int64(9000), refund.RefundCashCents)
	plan, err := GetRefundInvoiceExport(ticket.Id)
	require.NoError(t, err)
	assert.Empty(t, plan.Invoices)
	assert.Len(t, plan.Payments, 1)
}
