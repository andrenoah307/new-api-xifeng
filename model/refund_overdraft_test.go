package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundConsumptionOverdraftBounds(t *testing.T) {
	oldUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	for _, tc := range []struct {
		name   string
		debt   int64
		wallet int
		limit  int64
		paid   int64
		want   bool
	}{
		{"15364", 12876, -12876, 1000, 5000, true},
		{"exact_ten_yuan", 5000000, -5000000, 1000, 5000, true},
		{"one_quota_over_limit", 5000001, -5000001, 1000, 5000, false},
		{"disabled", 12876, -12876, 0, 5000, false},
		{"positive_noncash_wallet", 12876, 1, 1000, 5000, false},
		{"cash_gap_exceeds_real_debt", 12876, -1, 1000, 5000, false},
		{"payment_value_exceeds_limit", 5000000, -5000000, 1000, 5001, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orders := []TopUp{{Id: 1, QuotaGranted: 25000000, PaymentTotalCents: tc.paid}}
			lots := map[int]RefundCreditLot{1: {UsedPaid: tc.debt, PaidRemaining: 25000000 - tc.debt}}
			refund := &TicketRefund{RefundQuota: 25000000, FrozenQuota: 25000000, RefundStatus: RefundStatusPending}
			debt, deferred, err := applyRefundConsumptionOverdraft(orders, lots, refund, &User{Quota: tc.wallet}, tc.limit)
			if !tc.want {
				require.ErrorIs(t, err, ErrRefundExport)
				assert.Equal(t, tc.debt, lots[1].UsedPaid)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, debt)
			assert.Equal(t, tc.debt, debt.Quota)
			assert.Equal(t, tc.debt, deferred[1])
			assert.Equal(t, int64(25000000), lots[1].PaidRemaining)
			assert.Zero(t, lots[1].UsedPaid)
		})
	}
}

func TestRefundOverdraftCompletesAndNewRechargeRepaysConsumption(t *testing.T) {
	f := newRefundFinanceFixture(t)
	oldUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error) })
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.RefundOverdraftLimitCents = 1000
	require.NoError(t, SaveInvoiceTaxConfig(config))
	for _, invalid := range []int64{-1, 1001} {
		invalidConfig := config
		invalidConfig.RefundOverdraftLimitCents = invalid
		require.Error(t, SaveInvoiceTaxConfig(invalidConfig))
	}
	order := seedRefundFinanceOrder(t, f.user.Id, 25000000, 5000, 0, 1)
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": 25000000, "used_quota": 0}).Error)
	params := CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 25000000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()}
	ticket, _, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": -12876, "used_quota": 12876}).Error)
	data, err := SaveRefundInvoiceExport(ticket.Id, f.finance.Id)
	require.NoError(t, err)
	require.NotNil(t, data.Overdraft)
	assert.Equal(t, int64(12876), data.Overdraft.Quota)
	assert.Equal(t, "3219/1250", data.Overdraft.DeferredValueCentsRatio)
	assert.Equal(t, int64(5000), data.CashCents)
	require.Len(t, data.Payments, 1)
	assert.Equal(t, int64(12876), data.Payments[0].DeferredConsumptionQuota)
	// Completion must recheck the floor after further concurrent consumption.
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": -5000001, "used_quota": 5000001}).Error)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.finance.Id, RefundStatus: RefundStatusRefunded})
	require.ErrorIs(t, err, ErrRefundExport)
	var pending TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&pending).Error)
	assert.Equal(t, RefundStatusPending, pending.RefundStatus)
	assert.Equal(t, 25000000, pending.FrozenQuota)
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": -12876, "used_quota": 12876}).Error)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.finance.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, -12876, owner.Quota)
	assert.Equal(t, 12876, owner.UsedQuota)
	var rows []RefundOrderAllocation
	require.NoError(t, DB.Where("user_id = ? AND status = ?", f.user.Id, "committed").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(25000000), rows[0].RefundQuota)
	assert.Equal(t, int64(5000), rows[0].RefundCents)
	newOrder := seedRefundFinanceOrder(t, f.user.Id, 500000, 100, 0, 2)
	require.NoError(t, increaseUserQuota(f.user.Id, 500000))
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 487124, owner.Quota)
	lots, err := projectRefundCreditLots(int64(owner.UsedQuota), []TopUp{*order, *newOrder}, rows)
	require.NoError(t, err)
	assert.Zero(t, lots[order.Id].PaidRemaining)
	assert.Equal(t, int64(487124), lots[newOrder.Id].PaidRemaining)
	assert.Equal(t, int64(12876), lots[newOrder.Id].UsedPaid)
}

func TestRefundOverdraftCannotRefundOriginalCashTwice(t *testing.T) {
	orders := []TopUp{{Id: 1, QuotaGranted: 25000000, PaymentTotalCents: 5000}}
	lots := map[int]RefundCreditLot{1: {RefundedPaid: 25000000, RefundedCash: 5000}}
	_, _, err := applyRefundConsumptionOverdraft(orders, lots, &TicketRefund{RefundQuota: 1, FrozenQuota: 1, RefundStatus: RefundStatusPending}, &User{Quota: -1}, 1000)
	require.ErrorIs(t, err, ErrRefundExport)
	assert.Zero(t, lots[1].PaidRemaining)
}
