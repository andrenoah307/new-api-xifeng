package model

import (
	"fmt"
	"sync"
	"testing"

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
	if closeErr == nil {
		require.ErrorIs(t, calculateErr, ErrTicketRefundNotPending)
		assert.Equal(t, RefundStatusCancelled, refund.RefundStatus)
		assert.Zero(t, refund.FrozenQuota)
		assert.Equal(t, 10000, owner.Quota)
	} else {
		require.ErrorIs(t, closeErr, ErrTicketForbidden)
		require.NoError(t, calculateErr)
		assert.Equal(t, RefundStatusPending, refund.RefundStatus)
		assert.Equal(t, TicketStatusProcessing, ticket.Status)
		assert.Equal(t, 5000, refund.FrozenQuota)
		assert.Equal(t, 5000, owner.Quota)
	}
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
				if processing {
					require.ErrorIs(t, err, ErrTicketForbidden)
				} else {
					require.NoError(t, err)
				}
				require.NoError(t, DB.First(&refund, refund.Id).Error)
				require.NoError(t, DB.First(&ticket, ticket.Id).Error)
				var owner User
				require.NoError(t, DB.First(&owner, f.user.Id).Error)
				if processing {
					assert.Equal(t, RefundStatusPending, refund.RefundStatus)
					assert.Equal(t, 1000, refund.FrozenQuota)
					assert.Equal(t, TicketStatusProcessing, ticket.Status)
					assert.Zero(t, owner.Quota)
				} else {
					assert.Equal(t, RefundStatusCancelled, refund.RefundStatus)
					assert.Equal(t, 1000, owner.Quota)
				}
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
			require.ErrorIs(t, err, ErrRefundPayout)
			require.NoError(t, DB.First(&refund, refund.Id).Error)
			assert.Equal(t, 1000, refund.FrozenQuota)
			assert.Equal(t, RefundStatusPending, refund.RefundStatus)
		})
	}
}
