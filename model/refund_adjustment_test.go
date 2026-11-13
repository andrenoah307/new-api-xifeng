package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newRefundAdjustmentFixture(t *testing.T) (refundFinanceFixture, *Ticket, *TopUp) {
	t.Helper()
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundAdjustment{}, &RefundInvoiceExport{}))
	for _, table := range []interface{}{&RefundAdjustment{}, &RefundInvoiceExport{}, &RefundLiability{}} {
		require.NoError(t, DB.Where("1=1").Delete(table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []interface{}{&RefundAdjustment{}, &RefundInvoiceExport{}, &RefundLiability{}} {
			require.NoError(t, DB.Where("1=1").Delete(table).Error)
		}
	})
	unit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = unit })
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.RefundOverdraftLimitCents, config.RefundGiftRecoveryPolicy = 2000, RefundGiftNone
	require.NoError(t, SaveInvoiceTaxConfig(config))
	order := seedRefundFinanceOrder(t, f.user.Id, 23500000, 5000, 300, 1)
	// 47 paid credits + 6 manually classified promotional credits - 17 consumed.
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": 18000000, "used_quota": 8500000, "remark": "Keep this note"}).Error)
	ticket, _, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 17500000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()})
	require.NoError(t, err)
	return f, ticket, order
}

func TestRefundAdjustmentPreviewApplyReplayAndComplete(t *testing.T) {
	f, ticket, order := newRefundAdjustmentFixture(t)
	preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil)
	require.NoError(t, err)
	assert.True(t, preview.CanApply)
	assert.Equal(t, "1", preview.BalanceBefore)
	assert.Equal(t, "6", preview.DebitAmount)
	assert.Equal(t, "18", preview.AccountedAmount)
	assert.Equal(t, "12", preview.KnownAmount)
	assert.Equal(t, "-5", preview.BalanceAfter)
	assert.Equal(t, "35", preview.Requested)
	assert.Equal(t, "20", preview.GraceAmount)
	require.NotNil(t, preview.Calculation)
	assert.Equal(t, int64(3723), preview.Calculation.CashCents)
	assert.Equal(t, int64(223), preview.Calculation.Payments[0].RefundTaxCents)
	var count int64
	require.NoError(t, DB.Model(&RefundInvoiceExport{}).Count(&count).Error)
	assert.Zero(t, count, "preview must not save a calculation")
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 500000, owner.Quota)
	input := &RefundAdjustmentInput{Revision: preview.Revision, Reason: "Confirmed historical promotional credits", ConfirmRecovery: true}
	result, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, -2500000, owner.Quota)
	assert.Equal(t, 8500000, owner.UsedQuota)
	assert.Contains(t, owner.Remark, "Keep this note")
	assert.Contains(t, owner.Remark, "因退款工单计算扣回历史优惠赠送额度")
	var refund TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&refund).Error)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
	assert.Equal(t, 17500000, refund.FrozenQuota)
	assert.Zero(t, refund.ProcessedTime)
	var receipt RefundAdjustment
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&receipt).Error)
	assert.Equal(t, int64(3000000), receipt.DebitQuota)
	assert.NotEmpty(t, receipt.BeforeJSON)
	assert.Contains(t, receipt.ResultJSON, "3723")
	for _, actor := range []int{f.finance.Id, f.admin.Id} {
		replay, err := ManageRefundAdjustment(ticket.Id, actor, input)
		require.NoError(t, err)
		assert.Equal(t, result.Revision, replay.Revision)
		assert.Equal(t, result.AppliedAt, replay.AppliedAt)
	}
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, -2500000, owner.Quota)
	require.NoError(t, DB.Model(&RefundAdjustment{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.finance.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, -2500000, owner.Quota, "completion must not debit the held quota again")
	var allocations []RefundOrderAllocation
	require.NoError(t, DB.Where("refund_ticket_id = ?", ticket.Id).Find(&allocations).Error)
	require.Len(t, allocations, 1)
	assert.Equal(t, int64(3723), allocations[0].RefundCents)
	newOrder := seedRefundFinanceOrder(t, f.user.Id, 5000000, 1000, 0, 2)
	lots, err := projectRefundCreditLots(int64(owner.UsedQuota), []TopUp{*order, *newOrder}, allocations)
	require.NoError(t, err)
	assert.Equal(t, int64(2500000), lots[newOrder.Id].PaidRemaining, "later recharge repays deferred consumption")
}

func TestRefundAdjustmentRefreshConfirmationAndRollback(t *testing.T) {
	f, ticket, _ := newRefundAdjustmentFixture(t)
	preview, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil)
	require.NoError(t, err)
	input := &RefundAdjustmentInput{Revision: preview.Revision, Reason: "Historical credits verified"}
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, input)
	require.ErrorIs(t, err, ErrRefundAdjustmentConfirmation)
	input.ConfirmRecovery = true
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.RefundOverdraftLimitCents = 100
	require.NoError(t, SaveInvoiceTaxConfig(config))
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, input)
	require.ErrorIs(t, err, ErrFinanceRevision)
	blocked, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil)
	require.NoError(t, err)
	assert.False(t, blocked.CanApply)
	require.NotNil(t, blocked.Diagnostic)
	assert.Equal(t, "grace_exceeded", blocked.Diagnostic.Code)
	config.RefundOverdraftLimitCents = 2000
	require.NoError(t, SaveInvoiceTaxConfig(config))
	injected := errors.New("receipt could not be persisted")
	callback := "test_refund_adjustment_rollback"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "refund_adjustments" {
			tx.AddError(injected)
		}
	}))
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, input)
	require.NoError(t, DB.Callback().Create().Remove(callback))
	require.ErrorIs(t, err, injected)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 500000, owner.Quota)
	assert.Equal(t, "Keep this note", owner.Remark)
	var count int64
	require.NoError(t, DB.Model(&RefundInvoiceExport{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&RefundAdjustment{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRefundAdjustmentSettlementRequiresExplicitEvidence(t *testing.T) {
	f, ticket, _ := newRefundAdjustmentFixture(t)
	liability := RefundLiability{ID: uuid.NewString(), UserID: f.user.Id, RequestID: "historic-request", NodeName: "retired-node", CreatedAt: 1, TaskDone: false}
	require.NoError(t, DB.Create(&liability).Error)
	preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil)
	require.NoError(t, err)
	assert.Equal(t, "unsettled_tasks", preview.Blocker)
	assert.False(t, preview.CanApply)
	require.NoError(t, DB.Model(&liability).Update("task_done", true).Error)
	preview, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, nil)
	require.NoError(t, err)
	assert.True(t, preview.CanApply)
	input := &RefundAdjustmentInput{Revision: preview.Revision, Reason: "Request ended and all debits checked", ConfirmRecovery: true}
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.ErrorIs(t, err, ErrRefundAdjustmentConfirmation)
	input.ConfirmSettlement = true
	result, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	var count int64
	require.NoError(t, DB.Model(&RefundLiability{}).Where("user_id = ?", f.user.Id).Count(&count).Error)
	assert.Zero(t, count)
	var receipt RefundAdjustment
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&receipt).Error)
	assert.Contains(t, receipt.ResultJSON, "historic-request")
	assert.Contains(t, receipt.BeforeJSON, "retired-node")
}

func TestRefundAdjustmentAlreadyDebitedAndAuthorization(t *testing.T) {
	f, ticket, _ := newRefundAdjustmentFixture(t)
	_, err := ManageRefundAdjustment(ticket.Id, f.user.Id, nil)
	require.ErrorIs(t, err, ErrTicketForbidden)
	require.NoError(t, DB.Model(&f.finance).Update("status", common.UserStatusDisabled).Error)
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, nil)
	require.ErrorIs(t, err, ErrTicketForbidden)
	// A previously executed manual debit must not be inferred and deducted again.
	require.NoError(t, DB.Model(&f.user).Update("quota", -2500000).Error)
	preview, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil)
	require.NoError(t, err)
	assert.Equal(t, "0", preview.DebitAmount)
	assert.Equal(t, "no_adjustment", preview.Blocker)
	assert.False(t, preview.CanApply)
	require.NoError(t, DB.Model(&f.user).Update("quota", -2500001).Error)
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, nil)
	require.ErrorIs(t, err, ErrRefundAdjustment, "negative discrepancy must never become a wallet credit")
}

func TestRefundAdjustmentRejectsChangedEvidence(t *testing.T) {
	for _, change := range []string{"wallet", "usage", "order", "liability"} {
		t.Run(change, func(t *testing.T) {
			f, ticket, order := newRefundAdjustmentFixture(t)
			preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil)
			require.NoError(t, err)
			switch change {
			case "wallet":
				require.NoError(t, DB.Model(&f.user).Update("quota", 500001).Error)
			case "usage":
				require.NoError(t, DB.Model(&f.user).Update("used_quota", 8500001).Error)
			case "order":
				require.NoError(t, DB.Model(order).Update("trade_no", "changed-order-evidence").Error)
			case "liability":
				require.NoError(t, DB.Create(&RefundLiability{ID: uuid.NewString(), UserID: f.user.Id, RequestID: "new-request", TaskDone: true}).Error)
			}
			_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, &RefundAdjustmentInput{Revision: preview.Revision, Reason: "Historical credits verified", ConfirmRecovery: true, ConfirmSettlement: true})
			require.ErrorIs(t, err, ErrFinanceRevision)
			var count int64
			require.NoError(t, DB.Model(&RefundAdjustment{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestRefundAdjustmentSettlementOnlyAndCancellation(t *testing.T) {
	f, ticket, _ := newRefundAdjustmentFixture(t)
	require.NoError(t, DB.Model(&f.user).Update("quota", -2500000).Error)
	liability := RefundLiability{ID: uuid.NewString(), UserID: f.user.Id, RequestID: "already-debited-request", TaskDone: true}
	require.NoError(t, DB.Create(&liability).Error)
	unrelated := RefundLiability{ID: uuid.NewString(), UserID: f.admin.Id, RequestID: "other-user", TaskDone: true}
	require.NoError(t, DB.Create(&unrelated).Error)
	preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil)
	require.NoError(t, err)
	assert.Equal(t, "0", preview.DebitAmount)
	assert.True(t, preview.CanApply)
	input := &RefundAdjustmentInput{Revision: preview.Revision, Reason: "Verified stopped request and drained settlement queue", ConfirmSettlement: true}
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, -2500000, owner.Quota)
	require.NoError(t, DB.First(&unrelated, "id = ?", unrelated.ID).Error)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.NoError(t, err)
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 15000000, owner.Quota, "cancellation returns the held request only")
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.NoError(t, err, "receipt remains available without another debit after cancellation")
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 15000000, owner.Quota)
}

func TestRefundAdjustmentConcurrentConfirmation(t *testing.T) {
	f, ticket, _ := newRefundAdjustmentFixture(t)
	preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, &RefundAdjustmentInput{Revision: preview.Revision, Reason: "Verified historical promotional credits", ConfirmRecovery: true})
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
	assert.Equal(t, -2500000, owner.Quota)
	var count int64
	require.NoError(t, DB.Model(&RefundAdjustment{}).Where("ticket_id = ?", ticket.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ? AND role = ?", ticket.Id, common.RoleFinanceUser).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
