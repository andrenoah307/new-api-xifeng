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

func newApplicationBalanceFixture(t *testing.T) (refundFinanceFixture, *Ticket) {
	t.Helper()
	f, oldTicket, _ := newRefundAdjustmentFixture(t)
	require.NoError(t, DB.Where("ticket_id = ?", oldTicket.Id).Delete(&TicketRefund{}).Error)
	require.NoError(t, DB.Where("user_id = ?", f.user.Id).Delete(&TopUp{}).Error)
	seedRefundFinanceOrder(t, f.user.Id, 10375000000, 2075000, 0, 1)
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": 637398767, "used_quota": 10074136882, "aff_history": 105000000, "aff_quota": 0}).Error)
	ticket, _, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 500000000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()})
	require.NoError(t, err)
	require.NoError(t, DB.Model(&f.user).Update("quota", 85619144).Error)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.RefundOverdraftLimitCents = 3877
	require.NoError(t, SaveInvoiceTaxConfig(config))
	return f, ticket
}

func TestApplicationBalanceRefundPreservesReferralAndWallet(t *testing.T) {
	f, ticket := newApplicationBalanceFixture(t)
	liability := RefundLiability{ID: uuid.NewString(), UserID: f.user.Id, RequestID: "verified-stopped-request", TaskDone: true}
	require.NoError(t, DB.Create(&liability).Error)
	preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	assert.True(t, preview.CanApply)
	assert.Equal(t, "0", preview.DebitAmount)
	assert.Equal(t, "171.238288", preview.BalanceAfter)
	require.NotNil(t, preview.ApplicationBalance)
	assert.Equal(t, "210", preview.ApplicationBalance.NoncashAmount)
	assert.Equal(t, "-38.761712", preview.ApplicationBalance.CashAmount)
	require.NotNil(t, preview.Calculation)
	assert.Equal(t, int64(100000), preview.Calculation.CashCents)
	assert.Zero(t, preview.Calculation.RefundGiftQuota)
	assert.Equal(t, int64(19380856), preview.Calculation.Overdraft.Quota)
	assert.NotEqual(t, preview.ApplicationBalance.ProjectedSpentQuota, preview.ApplicationBalance.RecordedSpentQuota)
	input := &RefundAdjustmentInput{CalculationBasis: RefundBasisApplicationBalance, Revision: preview.Revision, Reason: "Verified application balance and settled request", ConfirmApplicationBalance: true}
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.ErrorIs(t, err, ErrRefundAdjustmentConfirmation)
	input.ConfirmNegative = true
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.ErrorIs(t, err, ErrRefundAdjustmentConfirmation)
	input.ConfirmSettlement = true
	applied, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.NoError(t, err)
	assert.True(t, applied.Applied)
	replayed, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.NoError(t, err)
	appliedJSON, err := common.Marshal(applied)
	require.NoError(t, err)
	replayedJSON, err := common.Marshal(replayed)
	require.NoError(t, err)
	assert.JSONEq(t, string(appliedJSON), string(replayedJSON))
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 85619144, owner.Quota)
	assert.Equal(t, 105000000, owner.AffHistoryQuota)
	assert.Equal(t, 10074136882, owner.UsedQuota)
	assert.Zero(t, owner.AffQuota)
	// A newly admitted request must still prevent final completion.
	liability.ID = uuid.NewString()
	require.NoError(t, DB.Create(&liability).Error)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.finance.Id, RefundStatus: RefundStatusRefunded})
	require.Error(t, err)
	require.NoError(t, DB.Delete(&liability).Error)
	data, err := SaveRefundInvoiceExport(ticket.Id, f.finance.Id)
	require.NoError(t, err)
	assert.Equal(t, preview.Calculation, data)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.finance.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 85619144, owner.Quota)
	assert.Equal(t, 105000000, owner.AffHistoryQuota)
	assert.Equal(t, 10074136882, owner.UsedQuota)
	var refund TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&refund).Error)
	assert.Equal(t, RefundStatusRefunded, refund.RefundStatus)
	assert.Zero(t, refund.FrozenQuota)
	assert.Equal(t, 500000000, refund.RefundQuota)
	assert.Equal(t, int64(100000), refund.RefundCashCents)
	// Subsequent funding repays the cash deficit without cashing out referrals.
	seedRefundFinanceOrder(t, f.user.Id, 50000000, 10000, 0, 2)
	require.NoError(t, increaseUserQuota(f.user.Id, 50000000))
	eligibility, err := OriginalRefundEligibility(f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 30619144, eligibility.MaxRefundableQuota)
	assert.Equal(t, 105000000, eligibility.NonRefundableQuota)
	second, _, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 30615000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()})
	require.NoError(t, err)
	secondPreview, err := ManageRefundAdjustment(second.Id, f.finance.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	assert.Equal(t, int64(6123), secondPreview.Calculation.CashCents)
	assert.Equal(t, "0.008288", secondPreview.ApplicationBalance.CashAmount)
	_, err = ManageRefundAdjustment(second.Id, f.finance.Id, &RefundAdjustmentInput{CalculationBasis: RefundBasisApplicationBalance, Revision: secondPreview.Revision, Reason: "Verified remaining paid balance", ConfirmApplicationBalance: true})
	require.NoError(t, err)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: second.Id, AdminId: f.finance.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 105004144, owner.Quota)
	assert.Equal(t, 105000000, owner.AffHistoryQuota)
}

func TestApplicationBalanceGraceRecheckAndCancellation(t *testing.T) {
	f, ticket := newApplicationBalanceFixture(t)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.RefundOverdraftLimitCents = 3876
	require.NoError(t, SaveInvoiceTaxConfig(config))
	blocked, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	assert.False(t, blocked.CanApply)
	require.NotNil(t, blocked.Diagnostic)
	assert.Equal(t, "grace_exceeded", blocked.Diagnostic.Code)
	assert.Equal(t, "-38.761712", blocked.ApplicationBalance.CashAmount)
	config.RefundOverdraftLimitCents = 3877
	require.NoError(t, SaveInvoiceTaxConfig(config))
	preview, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	input := &RefundAdjustmentInput{CalculationBasis: RefundBasisApplicationBalance, Revision: preview.Revision, Reason: "Confirmed original application basis", ConfirmApplicationBalance: true, ConfirmNegative: true}
	require.NoError(t, DB.Model(&f.user).Update("aff_history", 105000001).Error)
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, input)
	require.ErrorIs(t, err, ErrFinanceRevision)
	require.NoError(t, DB.Model(&f.user).Update("aff_history", 105000000).Error)
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, input)
	require.NoError(t, err)
	config.RefundOverdraftLimitCents = 0
	require.NoError(t, SaveInvoiceTaxConfig(config))
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.Error(t, err)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 585619144, owner.Quota, "only the original frozen amount is returned")
	assert.Equal(t, 105000000, owner.AffHistoryQuota)
	assert.Equal(t, common.RoleCommonUser, owner.Role)
}

func TestApplicationBalanceAtomicConfirmation(t *testing.T) {
	f, ticket := newApplicationBalanceFixture(t)
	liability := RefundLiability{ID: uuid.NewString(), UserID: f.user.Id, RequestID: "verified-request", TaskDone: true}
	require.NoError(t, DB.Create(&liability).Error)
	preview, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	input := RefundAdjustmentInput{CalculationBasis: RefundBasisApplicationBalance, Revision: preview.Revision, Reason: "Verified cash deficit and settlement", ConfirmApplicationBalance: true, ConfirmNegative: true, ConfirmSettlement: true}
	injected := errors.New("receipt write failed")
	const callback = "test_application_receipt_failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "refund_adjustments" {
			tx.AddError(injected)
		}
	}))
	_, err = ManageRefundAdjustment(ticket.Id, f.admin.Id, &input)
	require.NoError(t, DB.Callback().Create().Remove(callback))
	require.ErrorIs(t, err, injected)
	var refund TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&refund).Error)
	assert.Empty(t, refund.CalculationBasis)
	require.NoError(t, DB.First(&liability, "id = ?", liability.ID).Error)
	var count int64
	require.NoError(t, DB.Model(&RefundInvoiceExport{}).Where("ticket_id = ?", ticket.Id).Count(&count).Error)
	assert.Zero(t, count)
	var workers sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			request := input
			_, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, &request)
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.NoError(t, DB.Model(&RefundAdjustment{}).Where("ticket_id = ?", ticket.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ? AND role = ?", ticket.Id, common.RoleAdminUser).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 85619144, owner.Quota)
}

func TestApplicationBalancePreservesBonusAndPaymentCaps(t *testing.T) {
	f, ticket := newApplicationBalanceFixture(t)
	// A genuine bonus remains excluded even when the configured old policy recovers gifts.
	bonus := TopUp{UserId: f.user.Id, TradeNo: uuid.NewString(), Source: "discount_bonus", QuotaGranted: 5000000, Status: common.TopUpStatusSuccess}
	require.NoError(t, DB.Create(&bonus).Error)
	require.NoError(t, increaseUserQuota(f.user.Id, 5000000))
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.RefundGiftRecoveryPolicy = RefundGiftProportional
	require.NoError(t, SaveInvoiceTaxConfig(config))
	preview, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	assert.True(t, preview.CanApply)
	assert.Equal(t, "220", preview.ApplicationBalance.NoncashAmount)
	assert.Equal(t, "-38.761712", preview.ApplicationBalance.CashAmount)
	assert.Zero(t, preview.Calculation.RefundGiftQuota)
	assert.Equal(t, int64(100000), preview.Calculation.CashCents)
	// Extra wallet credits cannot manufacture refundable cash beyond payment capacity.
	require.NoError(t, DB.Model(&TopUp{}).Where("user_id = ? AND source != ?", f.user.Id, "discount_bonus").Updates(map[string]interface{}{"quota_granted": 50000000, "money": 100, "payment_total_cents": 10000, "invoice_net_cents": 10000}).Error)
	blocked, err := ManageRefundAdjustment(ticket.Id, f.admin.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	assert.False(t, blocked.CanApply)
	require.NotNil(t, blocked.Diagnostic)
	assert.Equal(t, "grace_exceeded", blocked.Diagnostic.Code)
}

func TestApplicationBalanceKeepsTaxRatioAndLedgerDifference(t *testing.T) {
	f, ticket, _ := newRefundAdjustmentFixture(t)
	preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	assert.True(t, preview.CanApply)
	assert.Equal(t, "0", preview.DebitAmount)
	assert.Equal(t, "1", preview.BalanceAfter)
	assert.Equal(t, "18", preview.AccountedAmount)
	assert.Equal(t, "12", preview.KnownAmount)
	assert.Equal(t, int64(5500000), preview.ApplicationBalance.ProjectedSpentQuota)
	assert.Equal(t, int64(8500000), preview.ApplicationBalance.RecordedSpentQuota)
	assert.Equal(t, int64(3723), preview.Calculation.CashCents)
	require.Len(t, preview.Calculation.Payments, 1)
	assert.Equal(t, int64(223), preview.Calculation.Payments[0].RefundTaxCents)
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, &RefundAdjustmentInput{CalculationBasis: RefundBasisApplicationBalance, Revision: preview.Revision, Reason: "Confirmed application basis and original tax ratio", ConfirmApplicationBalance: true})
	require.NoError(t, err)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.finance.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 500000, owner.Quota)
	assert.Equal(t, 8500000, owner.UsedQuota)
}
