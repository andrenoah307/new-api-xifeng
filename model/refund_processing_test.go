package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundRecalculationAndWithdrawalSerialize(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 10000, 1000, 0, 1)
	require.NoError(t, DB.Model(&f.user).Update("quota", 10000).Error)
	ticket, _, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 5000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()})
	require.NoError(t, err)
	start := make(chan struct{})
	var workers sync.WaitGroup
	var closeErr, calculateErr error
	workers.Add(2)
	go func() { defer workers.Done(); <-start; _, closeErr = CloseUserTicket(ticket.Id, f.user.Id) }()
	go func() {
		defer workers.Done()
		<-start
		_, calculateErr = SaveRefundInvoiceExport(ticket.Id, f.finance.Id)
	}()
	close(start)
	workers.Wait()
	var refund TicketRefund
	var owner User
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&refund).Error)
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	require.NoError(t, DB.First(ticket, ticket.Id).Error)
	require.ErrorIs(t, closeErr, ErrTicketForbidden)
	require.NoError(t, calculateErr)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
	assert.Equal(t, TicketStatusProcessing, ticket.Status)
	assert.Equal(t, 5000, refund.FrozenQuota)
	assert.Equal(t, 5000, owner.Quota)
}

func TestProcessingRefundCannotBeWithdrawn(t *testing.T) {
	for _, version := range []int{0, 1, 2} {
		for _, processing := range []bool{false, true} {
			t.Run(fmt.Sprintf("version_%d_processing_%t", version, processing), func(t *testing.T) {
				f := newRefundFinanceFixture(t)
				status := TicketStatusOpen
				if processing {
					status = TicketStatusProcessing
				}
				ticket := Ticket{UserId: f.user.Id, Type: TicketTypeRefund, Status: status}
				require.NoError(t, DB.Create(&ticket).Error)
				refund := TicketRefund{TicketId: ticket.Id, UserId: f.user.Id, FinanceVersion: version, RefundStatus: RefundStatusPending, RefundQuota: 1000, FrozenQuota: 1000}
				require.NoError(t, DB.Create(&refund).Error)
				_, err := CloseUserTicket(ticket.Id, f.user.Id)
				require.ErrorIs(t, err, ErrTicketForbidden)
				require.NoError(t, DB.First(&refund, refund.Id).Error)
				require.NoError(t, DB.First(&ticket, ticket.Id).Error)
				var owner User
				require.NoError(t, DB.First(&owner, f.user.Id).Error)
				assert.Equal(t, RefundStatusPending, refund.RefundStatus)
				assert.Equal(t, 1000, refund.FrozenQuota)
				assert.Equal(t, status, ticket.Status)
				assert.Zero(t, owner.Quota)
			})
		}
	}
}

func TestPayoutPreventsWithdrawalAcrossRefundVersions(t *testing.T) {
	for _, version := range []int{0, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			ticket := Ticket{UserId: f.user.Id, Type: TicketTypeRefund, Status: TicketStatusOpen}
			require.NoError(t, DB.Create(&ticket).Error)
			refund := TicketRefund{TicketId: ticket.Id, UserId: f.user.Id, FinanceVersion: version, RefundStatus: RefundStatusPending, RefundQuota: 1000, FrozenQuota: 1000}
			require.NoError(t, DB.Create(&refund).Error)
			require.NoError(t, DB.Create(&RefundPayoutAttempt{TicketId: ticket.Id, Status: "processing", RequestKey: fmt.Sprintf("withdrawal-%d", ticket.Id)}).Error)
			_, err := CloseUserTicket(ticket.Id, f.user.Id)
			require.ErrorIs(t, err, ErrTicketForbidden)
			closed := TicketStatusClosed
			_, _, err = UpdateTicketStatus(ticket.Id, f.admin.Id, &closed, nil)
			require.ErrorIs(t, err, ErrRefundPayout)
			require.NoError(t, DB.First(&refund, refund.Id).Error)
			assert.Equal(t, 1000, refund.FrozenQuota)
			assert.Equal(t, RefundStatusPending, refund.RefundStatus)
		})
	}
}

func TestUserRefundClosureRequiresNoPendingRefundOrHold(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status, frozen int
		blocked        bool
	}{
		{"pending_without_hold", RefundStatusPending, 0, true},
		{"refunded_with_hold", RefundStatusRefunded, 1000, true},
		{"rejected_with_hold", RefundStatusRejected, 1000, true},
		{"cancelled_with_hold", RefundStatusCancelled, 1000, true},
		{"refunded", RefundStatusRefunded, 0, false},
		{"rejected", RefundStatusRejected, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			ticket := Ticket{UserId: f.user.Id, Type: TicketTypeRefund, Status: TicketStatusResolved}
			require.NoError(t, DB.Create(&ticket).Error)
			refund := TicketRefund{TicketId: ticket.Id, UserId: f.user.Id, RefundStatus: tc.status, RefundQuota: 1000, FrozenQuota: tc.frozen}
			require.NoError(t, DB.Create(&refund).Error)
			rows, _, err := ListTicketsWithSummary(TicketQueryOptions{UserId: f.user.Id}, &common.PageInfo{Page: 1, PageSize: 10})
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.NotNil(t, rows[0].FrozenQuota)
			assert.Equal(t, tc.frozen, *rows[0].FrozenQuota)
			_, err = CloseUserTicket(ticket.Id, f.user.Id)
			if tc.blocked {
				require.ErrorIs(t, err, ErrTicketForbidden)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, DB.First(&refund, refund.Id).Error)
			require.NoError(t, DB.First(&ticket, ticket.Id).Error)
			var owner User
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, tc.status, refund.RefundStatus)
			assert.Equal(t, tc.frozen, refund.FrozenQuota)
			assert.Zero(t, owner.Quota)
			if tc.blocked {
				assert.Equal(t, TicketStatusResolved, ticket.Status)
			} else {
				assert.Equal(t, TicketStatusClosed, ticket.Status)
			}
		})
	}
}
