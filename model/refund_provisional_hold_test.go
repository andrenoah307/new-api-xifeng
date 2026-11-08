package model

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRefundViewSynchronizesExistingHolds(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		held, gift, wallet, expected     int
		verified, terminal, insufficient bool
	}{
		{"legacy_unfrozen", 0, 0, 1000, 400, false, false, false},
		{"under_frozen", 100, 0, 900, 400, false, false, false},
		{"over_frozen", 600, 0, 400, 400, false, false, false},
		{"verified_with_gift", 400, 50, 600, 450, true, false, false},
		{"terminal", 0, 0, 1000, 0, false, true, false},
		{"insufficient", 100, 0, 200, 100, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			seedRefundFinanceOrder(t, f.user.Id, 1000, 10000, 0, 100)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
			ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 400))
			require.NoError(t, err)
			status := RefundStatusPending
			if tc.terminal {
				status = RefundStatusRejected
			}
			require.NoError(t, DB.Model(&TicketRefund{}).Where("ticket_id = ?", ticket.Id).Updates(map[string]any{"frozen_quota": tc.held, "refund_gift_quota": tc.gift, "settlement_verified": tc.verified, "refund_status": status}).Error)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", tc.wallet).Error)
			var workers sync.WaitGroup
			results := make(chan error, 2)
			for range 2 {
				workers.Add(1)
				go func() { defer workers.Done(); results <- SynchronizeRefundHold(ticket.Id) }()
			}
			workers.Wait()
			close(results)
			for err := range results {
				if tc.insufficient {
					require.ErrorIs(t, err, ErrTicketRefundQuotaExceed)
				} else {
					require.NoError(t, err)
				}
			}
			var refund TicketRefund
			require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&refund).Error)
			assert.Equal(t, tc.expected, refund.FrozenQuota)
			var owner User
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, tc.wallet+tc.held-tc.expected, owner.Quota)
		})
	}
}

func TestRefundViewPreservesPendingReconciliation(t *testing.T) {
	f := newRefundFinanceFixture(t)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.ReconciliationReviewEnabled, config.ReconciliationReviewCount = true, 1
	require.NoError(t, SaveInvoiceTaxConfig(config))
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 900, 0, 100)
	require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	ticket, refund, _, err := CreateRefundTicket(financeRefundParams(f.user, 400))
	require.NoError(t, err)
	require.NoError(t, DB.Model(refund).Update("frozen_quota", 0).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	report, err := refundReconciliationReportTx(DB, *refund)
	require.NoError(t, err)
	plan := FinanceReconciliationPlan{Payments: []FinancePaymentEvidence{{TopUpID: order.Id, PaidCents: 900, PaidQuota: 1000}}}
	record, err := SubmitFinanceReconciliation(ticket.Id, f.admin.Id, uuid.NewString(), report.Revision, plan)
	require.NoError(t, err)
	view, err := GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, "awaiting_review", view.Stage)
	require.NoError(t, DB.First(record, record.Id).Error)
	assert.Equal(t, "submitted", record.Status)
	assert.Equal(t, view.Revision, record.SourceRevision)
	approved, err := ReviewFinanceReconciliation(record.Id, f.root.Id, true, "checked original payment")
	require.NoError(t, err)
	assert.Equal(t, "applied", approved.Status)
}

func TestRefundProvisionalHoldAdjustment(t *testing.T) {
	for _, tc := range []struct {
		name                                                       string
		request, final, gift, consumed, initialWallet, finalWallet int
	}{
		{"unchanged", 400, 400, 0, 0, 1000, 600},
		{"smaller", 600, 400, 0, 0, 1000, 600},
		{"larger", 200, 400, 0, 0, 1000, 600},
		{"gift_recovery", 400, 400, 100, 0, 1100, 660},
		{"settled_after_submission", 1000, 800, 0, 200, 1000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			order := seedRefundFinanceOrder(t, f.user.Id, 1000, 10000, 0, 100)
			if tc.gift > 0 {
				seedRefundGift(t, order, int64(tc.gift))
			}
			require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", tc.initialWallet).Error)
			params := financeRefundParams(f.user, tc.request)
			ticket, refund, _, err := CreateRefundTicket(params)
			require.NoError(t, err)
			assert.Equal(t, tc.request, refund.FrozenQuota)
			assert.False(t, refund.SettlementVerified)
			var owner User
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, tc.initialWallet-tc.request, owner.Quota)
			_, _, _, err = CreateRefundTicket(params)
			require.NoError(t, err)
			if tc.consumed > 0 {
				require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": gorm.Expr("quota - ?", tc.consumed), "used_quota": tc.consumed}).Error)
			}
			quote, err := QuoteRefundFinance(f.user.Id, tc.final)
			require.NoError(t, err)
			require.NotEmpty(t, quote.QuoteToken)
			assert.Equal(t, int64(tc.final*10), quote.RefundCashCents)
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, tc.initialWallet-tc.request-tc.consumed, owner.Quota, "quoting cannot release the provisional hold")
			confirmed, err := ConfirmRefundFinance(f.user.Id, ticket.Id, tc.final, quote.QuoteToken)
			require.NoError(t, err)
			assert.Equal(t, tc.final+int(quote.RefundGiftQuota), confirmed.FrozenQuota)
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, tc.finalWallet, owner.Quota)
			_, err = ConfirmRefundFinance(f.user.Id, ticket.Id, tc.final, quote.QuoteToken)
			require.NoError(t, err)
			report, err := GetRefundReconciliationReport(ticket.Id)
			require.NoError(t, err)
			assert.Equal(t, "0", report.Difference)
			require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return CancelPendingRefund(tx, ticket.Id, f.user.Id) }))
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, tc.initialWallet-tc.consumed, owner.Quota)
		})
	}
}

func TestRefundLegacyWaitingWithoutHoldStillConfirms(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 1000, 10000, 0, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 400))
	require.NoError(t, err)
	// Simulate an older release's unverified ticket without a provisional hold.
	require.NoError(t, DB.Model(&TicketRefund{}).Where("ticket_id = ?", ticket.Id).Update("frozen_quota", 0).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	quote, err := QuoteRefundFinance(f.user.Id, 400)
	require.NoError(t, err)
	_, err = ConfirmRefundFinance(f.user.Id, ticket.Id, 400, quote.QuoteToken)
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 600, owner.Quota)
}

func TestRefundQuoteBindsRequestedTicket(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 1000, 10000, 0, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 400))
	require.NoError(t, err)
	quote, err := QuoteRefundFinance(f.user.Id, 1, ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(4000), quote.RefundCashCents, "ticket views quote the stored request, not arbitrary query amounts")
	_, err = QuoteRefundFinance(f.user.Id, 400, ticket.Id+1)
	assert.ErrorIs(t, err, ErrTicketRefundNotPending)
}
