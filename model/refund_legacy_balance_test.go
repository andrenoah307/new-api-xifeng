package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newLegacyBalanceFixture(t *testing.T) (refundFinanceFixture, *Ticket, *TicketRefund) {
	t.Helper()
	f, ticket, order := newRefundAdjustmentFixture(t)
	require.NoError(t, DB.Model(order).Updates(map[string]interface{}{"quota_granted": 50000000, "money": 100, "payment_total_cents": 10000, "invoice_net_cents": 10000, "invoice_tax_cents": 0}).Error)
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": 150000, "used_quota": 0}).Error)
	require.NoError(t, DB.Model(&TicketRefund{}).Where("ticket_id = ?", ticket.Id).Updates(map[string]interface{}{"refund_quota": 20000000, "frozen_quota": 20000000}).Error)
	oldTicket := Ticket{UserId: f.user.Id, Type: TicketTypeRefund, Status: TicketStatusResolved}
	require.NoError(t, DB.Create(&oldTicket).Error)
	old := &TicketRefund{TicketId: oldTicket.Id, UserId: f.user.Id, RefundStatus: RefundStatusRefunded, RefundQuota: 10000000, ProcessedTime: 100}
	require.NoError(t, DB.Create(old).Error)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.LegacyInvoiceBefore = 1609430400
	require.NoError(t, SaveInvoiceTaxConfig(config))
	return f, ticket, old
}

func TestLegacyBalanceCarryforwardCompletesAndSurvivesCutoffChange(t *testing.T) {
	f, ticket, old := newLegacyBalanceFixture(t)
	preview, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	require.True(t, preview.CanApply, "%+v", preview.Diagnostic)
	require.Equal(t, int64(4000), preview.Calculation.CashCents)
	var count int64
	require.NoError(t, DB.Model(&RefundOrderAllocation{}).Where("refund_ticket_id = ?", old.TicketId).Count(&count).Error)
	assert.Zero(t, count, "preview must not persist historical occupancy")
	input := &RefundAdjustmentInput{CalculationMethod: RefundBasisApplicationBalance, Revision: preview.Revision}
	applied, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, input)
	require.NoError(t, err)
	assert.True(t, applied.Applied)
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, input)
	require.NoError(t, err)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.LegacyInvoiceBefore = 0
	require.NoError(t, SaveInvoiceTaxConfig(config))
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.ErrorIs(t, err, ErrTicketRefundNotPending)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 150000, owner.Quota)
	var historical TicketRefund
	require.NoError(t, DB.First(&historical, old.Id).Error)
	assert.Equal(t, old.RefundQuota, historical.RefundQuota)
	assert.Zero(t, historical.RefundCashCents, "policy occupancy is not a bank receipt")
	var allocations []RefundOrderAllocation
	require.NoError(t, DB.Where("refund_ticket_id = ?", old.TicketId).Find(&allocations).Error)
	require.Len(t, allocations, 1)
	assert.Equal(t, int64(2000), allocations[0].RefundCents)
	// A later refund must consume only the remaining wallet, not reopen old capacity.
	second, _, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 100000, PayeeType: "alipay", PayeeName: "test", PayeeAccount: "test", Contact: "test", RequestKey: uuid.NewString()})
	require.NoError(t, err)
	next, err := ManageRefundAdjustment(second.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	require.True(t, next.CanApply)
	assert.Equal(t, int64(20), next.Calculation.CashCents)
	assert.Equal(t, common.UserStatusEnabled, owner.Status)
}

func TestLegacyBalanceCutoffAndPaymentCapacity(t *testing.T) {
	for _, test := range []struct {
		name      string
		processed int64
		cash      int64
		want      bool
	}{
		{"before cutoff", 1609430399, 0, true},
		{"at cutoff", 1609430400, 0, false},
		{"unknown completion", 0, 0, false},
		{"recorded cash", 100, 2500, true},
		{"cash exceeds funding", 100, 11000, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, ticket, old := newLegacyBalanceFixture(t)
			require.NoError(t, DB.Model(old).Updates(map[string]interface{}{"processed_time": test.processed, "refund_cash_cents": test.cash}).Error)
			preview, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
			require.NoError(t, err)
			assert.Equal(t, test.want, preview.CanApply)
			if test.want {
				require.Len(t, preview.Calculation.LegacyBalances, 1)
				wantCash := test.cash
				if wantCash == 0 {
					wantCash = 2000
				}
				assert.Equal(t, wantCash, preview.Calculation.LegacyBalances[0].CashCents)
			}
		})
	}
}

func TestLegacyBalanceMixedHistoryAndStaleDate(t *testing.T) {
	f, ticket, old := newLegacyBalanceFixture(t)
	var order TopUp
	require.NoError(t, DB.Where("user_id = ?", f.user.Id).First(&order).Error)
	// Existing cash allocations are retained verbatim alongside a missing old refund.
	knownTicket := Ticket{UserId: f.user.Id, Type: TicketTypeRefund, Status: TicketStatusResolved}
	require.NoError(t, DB.Create(&knownTicket).Error)
	known := TicketRefund{TicketId: knownTicket.Id, UserId: f.user.Id, RefundStatus: RefundStatusRefunded, RefundQuota: 5000000, RefundCashCents: 1000, ProcessedTime: 90}
	require.NoError(t, DB.Create(&known).Error)
	allocation := RefundOrderAllocation{RefundTicketID: knownTicket.Id, UserID: f.user.Id, TopUpID: order.Id, OriginalQuota: order.QuotaGranted, OriginalPaidCents: order.PaymentTotalCents, RefundQuota: 5000000, RefundCents: 1000, Status: "committed"}
	require.NoError(t, DB.Create(&allocation).Error)
	preview, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	require.True(t, preview.CanApply)
	require.Len(t, preview.Calculation.LegacyBalances, 1)
	assert.Equal(t, old.TicketId, preview.Calculation.LegacyBalances[0].TicketID)
	assert.Equal(t, int64(3000), preview.Calculation.Payments[0].Calculation.PreviouslyRefundedCents)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.LegacyInvoiceBefore += 86400
	require.NoError(t, SaveInvoiceTaxConfig(config))
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, &RefundAdjustmentInput{CalculationMethod: RefundBasisApplicationBalance, Revision: preview.Revision})
	require.ErrorIs(t, err, ErrFinanceRevision)
	var count int64
	require.NoError(t, DB.Model(&RefundOrderAllocation{}).Where("refund_ticket_id = ?", old.TicketId).Count(&count).Error)
	assert.Zero(t, count)
}

func TestLegacyBalanceRollbackAndInvoiceCommission(t *testing.T) {
	f, ticket, old := newLegacyBalanceFixture(t)
	var order TopUp
	require.NoError(t, DB.Where("user_id = ?", f.user.Id).First(&order).Error)
	inviter := User{Username: "legacy-inviter", AffCode: uuid.NewString()[:20], Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffHistoryQuota: 5000000}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Model(&f.user).Update("inviter_id", inviter.Id).Error)
	require.NoError(t, DB.Create(&CommissionRecord{UserId: f.user.Id, InviterId: inviter.Id, TopUpId: order.Id, CommissionQuota: 5000000, Type: CommissionTypeTopUp}).Error)
	invoice := TicketInvoice{TicketId: ticket.Id + 100, UserId: f.user.Id, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 100, InvoiceType: "ordinary"}
	require.NoError(t, invoice.SetTopUpOrderIDs([]int{order.Id}))
	require.NoError(t, DB.Create(&invoice).Error)
	preview, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	require.True(t, preview.CanApply)
	require.Len(t, preview.Calculation.Invoices, 1)
	input := &RefundAdjustmentInput{CalculationMethod: RefundBasisApplicationBalance, Revision: preview.Revision}
	failure := errors.New("audit unavailable")
	const callback = "legacy_balance_audit_failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "ticket_messages" {
			tx.AddError(failure)
		}
	}))
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, input)
	require.NoError(t, DB.Callback().Create().Remove(callback))
	require.ErrorIs(t, err, failure)
	var count int64
	require.NoError(t, DB.Model(&RefundOrderAllocation{}).Where("refund_ticket_id = ?", old.TicketId).Count(&count).Error)
	assert.Zero(t, count)
	var refund TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&refund).Error)
	assert.Empty(t, refund.CalculationBasis)
	assert.Equal(t, 20000000, refund.FrozenQuota)
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, input)
	require.NoError(t, err)
	require.NoError(t, DB.First(&inviter, inviter.Id).Error)
	assert.Zero(t, inviter.Quota, "historical carryforward does not claw rewards back")
	_, _, _, clawback, err := UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	require.NotNil(t, clawback)
	assert.Equal(t, 2000000, clawback.ClawedQuota, "only the current cash refund recovers rewards")
	require.NoError(t, DB.First(&inviter, inviter.Id).Error)
	assert.Equal(t, -2000000, inviter.Quota)
	var oldCorrections int64
	require.NoError(t, DB.Model(&RefundInvoiceCorrection{}).Where("refund_ticket_id = ?", old.TicketId).Count(&oldCorrections).Error)
	assert.Zero(t, oldCorrections, "carryforward must not reverse old invoices again")
}
