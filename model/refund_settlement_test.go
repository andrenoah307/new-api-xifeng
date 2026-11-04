package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestRefundSettlementPolicyFlow(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		quota, used, requested   int
		paid, fee, cash, reissue int64
	}{
		{"ordinary_unused", 9400, 0, 9400, 10000, 600, 10000, 0},
		{"ordinary_half", 9400, 4700, 4700, 10000, 600, 5000, 5000},
		{"special_half", 9100, 4550, 4550, 10000, 900, 5000, 5000},
		{"partial_remaining", 9400, 4700, 2350, 10000, 600, 2500, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			require.NoError(t, DB.Where("1 = 1").Delete(&RefundLiability{}).Error)
			order := seedRefundFinanceOrder(t, f.user.Id, int64(tc.quota), tc.paid, tc.fee, InvoicePolicyEffectiveAt)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": tc.quota - tc.used, "used_quota": tc.used}).Error)
			invoiceTicket := Ticket{UserId: f.user.Id, Type: TicketTypeInvoice, Status: TicketStatusResolved}
			require.NoError(t, DB.Create(&invoiceTicket).Error)
			ids, err := common.Marshal([]int{order.Id})
			require.NoError(t, err)
			require.NoError(t, DB.Create(&TicketInvoice{TicketId: invoiceTicket.Id, UserId: f.user.Id, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 100, TopUpOrderIds: string(ids)}).Error)
			liability, err := BeginRefundLiability(f.user.Id, "policy-request")
			require.NoError(t, err)
			ticket, refund, _, err := CreateRefundTicket(financeRefundParams(f.user, tc.requested))
			require.NoError(t, err)
			assert.Zero(t, refund.FrozenQuota)
			_, err = BeginRefundLiability(f.user.Id, "blocked-request")
			require.ErrorIs(t, err, ErrRefundConsumptionFrozen)
			quote, err := QuoteRefundFinance(f.user.Id, tc.requested)
			require.NoError(t, err)
			assert.Empty(t, quote.QuoteToken)
			_, err = ConfirmRefundFinance(f.user.Id, ticket.Id, tc.requested, "stale")
			require.ErrorIs(t, err, ErrRefundSettlementUnconfirmed)
			// A task completion alone cannot release its still-running request.
			HoldRefundLiability(liability)
			CompleteRefundLiability(liability.ID)
			quote, err = QuoteRefundFinance(f.user.Id, tc.requested)
			require.NoError(t, err)
			assert.Empty(t, quote.QuoteToken)
			FinishRefundLiabilityRequest(liability.ID)
			quote, err = QuoteRefundFinance(f.user.Id, tc.requested)
			require.NoError(t, err)
			assert.Equal(t, tc.cash, quote.RefundCashCents)
			_, err = ConfirmRefundFinance(f.admin.Id, ticket.Id, tc.requested, quote.QuoteToken)
			require.Error(t, err)
			_, err = ConfirmRefundFinance(f.user.Id, ticket.Id, tc.requested, "stale")
			require.ErrorIs(t, err, ErrInvoiceTaxQuoteChanged)
			refund, err = ConfirmRefundFinance(f.user.Id, ticket.Id, tc.requested, quote.QuoteToken)
			require.NoError(t, err)
			assert.True(t, refund.SettlementVerified)
			assert.Equal(t, tc.requested, refund.FrozenQuota)
			_, err = ConfirmRefundFinance(f.user.Id, ticket.Id, tc.requested, quote.QuoteToken)
			require.NoError(t, err)
			require.Len(t, refund.Finance.InvoiceCorrections, 1)
			correction := refund.Finance.InvoiceCorrections[0]
			assert.Equal(t, tc.reissue, correction.ReissueCents)
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
			require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
			_, err = StartRefundInvoiceCorrection(correction.Id, f.finance.Id)
			require.NoError(t, err)
			blue := ""
			if tc.reissue > 0 {
				blue = "BLUE-REISSUED"
			}
			_, err = CompleteRefundInvoiceCorrection(correction.Id, f.finance.Id, "RED-ORIGINAL", blue, "verified")
			require.NoError(t, err)
			refund, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
			require.NoError(t, err)
			assert.Equal(t, tc.cash, refund.RefundCashCents)
			var owner User
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, tc.quota-tc.used-tc.requested, owner.Quota)
			admitted, err := BeginRefundLiability(f.user.Id, "resumed-request")
			require.NoError(t, err)
			FinishRefundLiabilityRequest(admitted.ID)
		})
	}
}

func TestRefundSettlementConcurrentConfirmation(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.Where("1 = 1").Delete(&RefundLiability{}).Error)
	seedRefundFinanceOrder(t, f.user.Id, 9400, 10000, 600, InvoicePolicyEffectiveAt)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 9400).Error)
	params := financeRefundParams(f.user, 9400)
	start := make(chan struct{})
	var admitted *common.RefundLiability
	var admissionErr, createErr error
	var ticket *Ticket
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		admitted, admissionErr = BeginRefundLiability(f.user.Id, "concurrent-request")
	}()
	go func() { defer workers.Done(); <-start; ticket, _, _, createErr = CreateRefundTicket(params) }()
	close(start)
	workers.Wait()
	require.NoError(t, createErr)
	if admissionErr == nil {
		quote, err := QuoteRefundFinance(f.user.Id, 9400)
		require.NoError(t, err)
		assert.Empty(t, quote.QuoteToken)
		FinishRefundLiabilityRequest(admitted.ID)
	} else {
		require.ErrorIs(t, admissionErr, ErrRefundConsumptionFrozen)
	}
	quote, err := QuoteRefundFinance(f.user.Id, 9400)
	require.NoError(t, err)
	require.NotEmpty(t, quote.QuoteToken)
	start = make(chan struct{})
	results := make(chan error, 2)
	workers.Add(2)
	for range 2 {
		go func() {
			defer workers.Done()
			<-start
			_, err := ConfirmRefundFinance(f.user.Id, ticket.Id, 9400, quote.QuoteToken)
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Zero(t, owner.Quota)
	var count int64
	require.NoError(t, DB.Model(&RefundOrderAllocation{}).Where("refund_ticket_id = ?", ticket.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.NoError(t, err)
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 9400, owner.Quota)
}
