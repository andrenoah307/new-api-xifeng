package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchTicketStatusAuthorizationAndRollback(t *testing.T) {
	truncateTables(t)
	insertInvoiceTicketUser(t, 1)
	first := Ticket{UserId: 1, AssigneeId: 2, Subject: "One", Type: TicketTypeGeneral, Status: TicketStatusOpen}
	second := Ticket{UserId: 1, AssigneeId: 3, Subject: "Two", Type: TicketTypeInvoice, Status: TicketStatusOpen}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)
	require.NoError(t, DB.Create(&TicketInvoice{TicketId: second.Id, UserId: 1, InvoiceStatus: InvoiceStatusPending}).Error)
	_, err := BatchUpdateTicketStatus([]int{first.Id, second.Id}, 2, common.RoleCustomerServiceUser, TicketStatusClosed, nil)
	require.ErrorIs(t, err, ErrTicketForbidden)
	stored, err := GetTicketById(first.Id)
	require.NoError(t, err)
	assert.Equal(t, TicketStatusOpen, stored.Status)
	changes, err := BatchUpdateTicketStatus([]int{first.Id, second.Id, first.Id}, 4, common.RoleAdminUser, TicketStatusClosed, nil)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	assert.Positive(t, changes[0].Ticket.ClosedTime)
	changes, err = BatchUpdateTicketStatus([]int{first.Id, second.Id}, 4, common.RoleAdminUser, TicketStatusClosed, nil)
	require.NoError(t, err)
	assert.Empty(t, changes)
	changes, err = BatchUpdateTicketStatus([]int{first.Id}, 4, common.RoleAdminUser, TicketStatusProcessing, nil)
	require.NoError(t, err)
	assert.Zero(t, changes[0].Ticket.ClosedTime)
	for _, ids := range [][]int{nil, make([]int, MaxTicketStatusBatchSize+1), {0}, {first.Id, second.Id + 1000}} {
		_, err = BatchUpdateTicketStatus(ids, 4, common.RoleAdminUser, TicketStatusClosed, nil)
		require.Error(t, err)
	}
}

func TestBatchTicketRefundCancellationIsAtomic(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 925001)
	admin := insertInvoiceTicketUser(t, 4)
	require.NoError(t, DB.Model(admin).Update("role", common.RoleAdminUser).Error)
	ticket := Ticket{UserId: user.Id, Subject: "Refund", Type: TicketTypeRefund, Status: TicketStatusOpen}
	require.NoError(t, DB.Create(&ticket).Error)
	refund := TicketRefund{TicketId: ticket.Id, UserId: user.Id, RefundStatus: RefundStatusPending, FrozenQuota: 100, RefundQuota: 100}
	require.NoError(t, DB.Create(&refund).Error)
	_, err := BatchUpdateTicketStatus([]int{ticket.Id, ticket.Id + 1000}, 4, common.RoleAdminUser, TicketStatusClosed, nil)
	require.ErrorIs(t, err, ErrTicketNotFound)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Zero(t, user.Quota)
	require.NoError(t, DB.First(&refund, refund.Id).Error)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
	assert.Equal(t, 100, refund.FrozenQuota)
	var auditCount int64
	require.NoError(t, DB.Model(&Log{}).Where("user_id = ?", user.Id).Count(&auditCount).Error)
	assert.Zero(t, auditCount)
	_, err = BatchUpdateTicketStatus([]int{ticket.Id}, 4, common.RoleAdminUser, TicketStatusClosed, nil)
	require.NoError(t, err)
	_, err = BatchUpdateTicketStatus([]int{ticket.Id}, 4, common.RoleAdminUser, TicketStatusClosed, nil)
	require.NoError(t, err)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	require.NoError(t, DB.First(&refund, refund.Id).Error)
	assert.Equal(t, RefundStatusCancelled, refund.RefundStatus)
	assert.Zero(t, refund.FrozenQuota)
	require.NoError(t, DB.Model(&Log{}).Where("user_id = ?", user.Id).Count(&auditCount).Error)
	assert.Equal(t, int64(2), auditCount)
}

func TestSingleTicketRefundCancellation(t *testing.T) {
	for _, actor := range []string{"owner", "admin"} {
		t.Run(actor, func(t *testing.T) {
			truncateTables(t)
			user := insertInvoiceTicketUser(t, 925002)
			admin := insertInvoiceTicketUser(t, 4)
			require.NoError(t, DB.Model(admin).Update("role", common.RoleAdminUser).Error)
			ticket := Ticket{UserId: user.Id, Subject: "Refund", Type: TicketTypeRefund, Status: TicketStatusOpen}
			require.NoError(t, DB.Create(&ticket).Error)
			require.NoError(t, DB.Create(&TicketRefund{TicketId: ticket.Id, UserId: user.Id, RefundStatus: RefundStatusPending, RefundQuota: 100, FrozenQuota: 100}).Error)
			var err error
			if actor == "owner" {
				_, err = CloseUserTicket(ticket.Id, user.Id)
				require.ErrorIs(t, err, ErrTicketForbidden)
				require.NoError(t, DB.First(user, user.Id).Error)
				assert.Zero(t, user.Quota)
				return
			} else {
				status := TicketStatusClosed
				_, _, err = UpdateTicketStatus(ticket.Id, 4, &status, nil)
			}
			require.NoError(t, err)
			require.NoError(t, DB.First(user, user.Id).Error)
			assert.Equal(t, 100, user.Quota)
		})
	}
}

func TestTicketReadWatermarkIsMonotonic(t *testing.T) {
	truncateTables(t)
	ticket := Ticket{UserId: 1, Subject: "Read", Type: TicketTypeGeneral, UpdatedTime: 100}
	require.NoError(t, DB.Create(&ticket).Error)
	first := TicketMessage{TicketId: ticket.Id, Role: common.RoleCommonUser, Content: "First", CreatedTime: 100}
	require.NoError(t, DB.Create(&first).Error)
	second := TicketMessage{TicketId: ticket.Id, Role: common.RoleCommonUser, Content: "Second", CreatedTime: 100}
	require.NoError(t, DB.Create(&second).Error)
	require.NoError(t, MarkTicketMessagesRead(ticket.Id, first.Id))
	stored, err := GetTicketById(ticket.Id)
	require.NoError(t, err)
	assert.True(t, stored.HasUnreadMessages)
	require.NoError(t, MarkTicketMessagesRead(ticket.Id, second.Id))
	require.NoError(t, MarkTicketMessagesRead(ticket.Id, first.Id))
	stored, err = GetTicketById(ticket.Id)
	require.NoError(t, err)
	assert.False(t, stored.HasUnreadMessages)
	assert.Equal(t, int64(100), stored.UpdatedTime)
	require.NoError(t, DB.Create(&TicketMessage{TicketId: ticket.Id, Role: common.RoleCustomerServiceUser, Content: "Staff"}).Error)
	stored, err = GetTicketById(ticket.Id)
	require.NoError(t, err)
	assert.False(t, stored.HasUnreadMessages)
}

func TestBatchInvoiceClosureRollsBackTaxAndAudit(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 925003)
	ticket := Ticket{UserId: user.Id, Subject: "Invoice", Type: TicketTypeInvoice, Status: TicketStatusOpen}
	require.NoError(t, DB.Create(&ticket).Error)
	invoice := TicketInvoice{TicketId: ticket.Id, UserId: user.Id, InvoiceStatus: InvoiceStatusPending}
	require.NoError(t, DB.Create(&invoice).Error)
	charge := InvoiceTaxCharge{TicketId: ticket.Id, UserId: user.Id, Status: "paid", Quota: 100}
	require.NoError(t, DB.Create(&charge).Error)
	_, err := BatchUpdateTicketStatus([]int{ticket.Id, ticket.Id + 1000}, 4, common.RoleAdminUser, TicketStatusClosed, nil)
	require.ErrorIs(t, err, ErrTicketNotFound)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Zero(t, user.Quota)
	require.NoError(t, DB.First(&invoice, invoice.Id).Error)
	assert.Equal(t, InvoiceStatusPending, invoice.InvoiceStatus)
	require.NoError(t, DB.First(&charge, charge.Id).Error)
	assert.Equal(t, "paid", charge.Status)
	var count int64
	require.NoError(t, DB.Model(&Log{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.Id).Count(&count).Error)
	assert.Zero(t, count)
	for range 2 {
		_, err = BatchUpdateTicketStatus([]int{ticket.Id}, 4, common.RoleAdminUser, TicketStatusClosed, nil)
		require.NoError(t, err)
	}
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	require.NoError(t, DB.First(&charge, charge.Id).Error)
	assert.Equal(t, "refunded", charge.Status)
	require.NoError(t, DB.Model(&Log{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
