package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceInitialMessageDoesNotCountAsUnread(t *testing.T) {
	for _, kind := range []string{TicketTypeInvoice, TicketTypeGeneral, TicketTypeRefund} {
		t.Run(kind, func(t *testing.T) {
			truncateTables(t)
			ticket := Ticket{UserId: 1, Type: kind, Subject: "Application", Status: TicketStatusOpen}
			require.NoError(t, DB.Create(&ticket).Error)
			if kind == TicketTypeInvoice {
				require.NoError(t, DB.Create(&TicketInvoice{TicketId: ticket.Id, UserId: 1, InvoiceStatus: InvoiceStatusPending}).Error)
			}
			require.NoError(t, DB.Create(&TicketMessage{TicketId: ticket.Id, Role: common.RoleAdminUser, Content: "System information", CreatedTime: 100}).Error)
			stored, err := GetTicketById(ticket.Id)
			require.NoError(t, err)
			assert.False(t, stored.HasUnreadMessages)
			require.NoError(t, DB.Create(&TicketMessage{TicketId: ticket.Id, Role: common.RoleCommonUser, Content: "Initial", CreatedTime: 100}).Error)
			stored, err = GetTicketById(ticket.Id)
			require.NoError(t, err)
			assert.Equal(t, kind != TicketTypeInvoice, stored.HasUnreadMessages)
			unread := true
			var count int64
			require.NoError(t, applyTicketFilters(DB.Model(&Ticket{}), TicketQueryOptions{Unread: &unread}).Count(&count).Error)
			if kind == TicketTypeInvoice {
				assert.Zero(t, count)
			} else {
				assert.EqualValues(t, 1, count)
			}
			require.NoError(t, DB.Create(&TicketMessage{TicketId: ticket.Id, Role: common.RoleAdminUser, Content: "Staff", CreatedTime: 100}).Error)
			stored, err = GetTicketById(ticket.Id)
			require.NoError(t, err)
			assert.Equal(t, kind != TicketTypeInvoice, stored.HasUnreadMessages)
			reply := TicketMessage{TicketId: ticket.Id, Role: common.RoleCommonUser, Content: "Follow up", CreatedTime: 100}
			require.NoError(t, DB.Create(&reply).Error)
			stored, err = GetTicketById(ticket.Id)
			require.NoError(t, err)
			assert.True(t, stored.HasUnreadMessages)
			require.NoError(t, applyTicketFilters(DB.Model(&Ticket{}), TicketQueryOptions{Unread: &unread}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
			require.NoError(t, MarkTicketMessagesRead(ticket.Id, reply.Id))
			stored, err = GetTicketById(ticket.Id)
			require.NoError(t, err)
			assert.False(t, stored.HasUnreadMessages)
			assert.Equal(t, TicketStatusOpen, stored.Status)
			if kind == TicketTypeInvoice {
				var invoice TicketInvoice
				require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&invoice).Error)
				assert.Equal(t, InvoiceStatusPending, invoice.InvoiceStatus)
			}
		})
	}
}

func TestTicketSubjectCharacterLimit(t *testing.T) {
	truncateTables(t)
	for _, char := range []string{"a", "题", "😀"} {
		subject := strings.Repeat(char, 50)
		ticket, _, err := CreateTicketWithMessage(CreateTicketParams{UserId: 1, Subject: " " + subject + " ", Content: "Body", Type: TicketTypeGeneral, Role: common.RoleCommonUser})
		require.NoError(t, err)
		assert.Equal(t, subject, ticket.Subject)
		_, _, err = CreateTicketWithMessage(CreateTicketParams{Subject: subject + char, Content: "Body"})
		require.ErrorIs(t, err, ErrTicketSubjectTooLong)
		_, _, _, _, err = CreateInvoiceTicket(CreateInvoiceTicketParams{Subject: subject + char})
		require.ErrorIs(t, err, ErrTicketSubjectTooLong)
		_, _, _, err = CreateRefundTicket(CreateRefundTicketParams{Subject: subject + char})
		require.ErrorIs(t, err, ErrTicketSubjectTooLong)
	}
	var count int64
	require.NoError(t, DB.Model(&Ticket{}).Count(&count).Error)
	assert.EqualValues(t, 3, count)
}
