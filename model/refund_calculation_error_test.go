package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundShortfallDiagnosticAmountsAndPrivacy(t *testing.T) {
	oldUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	orders := []TopUp{{Id: 1, QuotaGranted: 75500000, PaymentTotalCents: 15100}}
	lots := map[int]RefundCreditLot{1: {UsedPaid: 60424223, PaidRemaining: 15075777}}
	refund := &TicketRefund{TicketId: 15481, RefundQuota: 25000000, FrozenQuota: 25000000, RefundStatus: RefundStatusPending}
	_, _, err := applyRefundConsumptionOverdraft(orders, lots, refund, &User{Quota: 75777}, 1000)
	require.ErrorIs(t, err, ErrRefundExport)
	var diagnostic *RefundCalculationError
	require.ErrorAs(t, err, &diagnostic)
	assert.Equal(t, "grace_exceeded", diagnostic.Code)
	assert.Equal(t, map[string]string{"recharged": "151", "consumed": "120.848446", "refunded": "0", "available": "30.151554", "requested": "50", "shortfall": "19.848446", "wallet": "0.151554", "limit": "10"}, diagnostic.Amounts)
	assert.Equal(t, ErrRefundExport.Error(), diagnostic.Error())
	assert.NotContains(t, diagnostic.Error(), "15481")
	assert.NotContains(t, diagnostic.Error(), "19.848446")
	assert.Equal(t, int64(60424223), lots[1].UsedPaid)
}

func TestRefundShortfallDistinguishesDebtFromUnbackedCredit(t *testing.T) {
	oldUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	for _, tc := range []struct {
		name   string
		wallet int
		limit  int64
		frozen int
		code   string
	}{
		{"positive_wallet", 1, 1000, 25000000, "unbacked_credit"},
		{"disabled", -12876, 0, 25000000, "grace_unavailable"},
		{"partial_hold", -12876, 1000, 1, "hold_not_eligible"},
		{"debt_exceeds_limit", -5000001, 1000, 25000000, "grace_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orders := []TopUp{{Id: 1, QuotaGranted: 25000000, PaymentTotalCents: 5000}}
			lots := map[int]RefundCreditLot{1: {UsedPaid: 12876, PaidRemaining: 24987124}}
			refund := &TicketRefund{RefundQuota: 25000000, FrozenQuota: tc.frozen, RefundStatus: RefundStatusPending}
			_, _, err := applyRefundConsumptionOverdraft(orders, lots, refund, &User{Quota: tc.wallet}, tc.limit)
			var diagnostic *RefundCalculationError
			require.ErrorAs(t, err, &diagnostic)
			assert.Equal(t, tc.code, diagnostic.Code)
			assert.Equal(t, "0.025752", diagnostic.Amounts["shortfall"])
		})
	}
}

func TestRefundMissingHistoryDiagnosticRollsBack(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error)
	old := TicketRefund{TicketId: 9001, UserId: f.user.Id, RefundStatus: RefundStatusRefunded, RefundQuota: 100}
	current := TicketRefund{TicketId: 9002, UserId: f.user.Id, RefundStatus: RefundStatusPending, RefundQuota: 100, FrozenQuota: 100}
	require.NoError(t, DB.Create(&old).Error)
	require.NoError(t, DB.Create(&current).Error)
	_, err := SaveRefundInvoiceExport(current.TicketId, f.finance.Id)
	var diagnostic *RefundCalculationError
	require.ErrorAs(t, err, &diagnostic)
	assert.Equal(t, "missing_history", diagnostic.Code)
	assert.Equal(t, old.TicketId, diagnostic.TicketID)
	var count int64
	require.NoError(t, DB.Model(&RefundInvoiceExport{}).Where("ticket_id = ?", current.TicketId).Count(&count).Error)
	assert.Zero(t, count)
	var after TicketRefund
	require.NoError(t, DB.First(&after, current.Id).Error)
	assert.Equal(t, current.FrozenQuota, after.FrozenQuota)
	assert.Equal(t, current.RefundStatus, after.RefundStatus)
}
