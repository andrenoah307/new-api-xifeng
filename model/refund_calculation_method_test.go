package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundCalculationMethodSelection(t *testing.T) {
	for _, method := range []string{RefundBasisApplicationBalance, "discount_recovery"} {
		t.Run(method, func(t *testing.T) {
			f, ticket, _ := newRefundAdjustmentFixture(t)
			liability := RefundLiability{ID: "method-selection", UserID: f.user.Id, RequestID: "residual-request", TaskDone: true}
			require.NoError(t, DB.Create(&liability).Error)
			basis := ""
			if method == RefundBasisApplicationBalance {
				basis = method
			}
			preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil, basis)
			require.NoError(t, err)
			require.True(t, preview.CanApply)
			assert.Equal(t, "6", preview.DifferenceAmount)
			input := &RefundAdjustmentInput{CalculationMethod: method, Revision: preview.Revision}
			if method == "discount_recovery" {
				_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
				require.ErrorIs(t, err, ErrRefundAdjustmentConfirmation)
			}
			input.ConfirmNegative = true
			result, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
			require.NoError(t, err)
			assert.Equal(t, method, result.CalculationMethod)
			assert.Contains(t, result.Reason, "no manual settlement verification claimed")
			assert.Empty(t, input.Reason, "request must not be mutated")
			var remaining int64
			require.NoError(t, DB.Model(&RefundLiability{}).Where("user_id = ?", f.user.Id).Count(&remaining).Error)
			assert.Zero(t, remaining)
			var owner User
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			wantBalance := 500000
			if method == "discount_recovery" {
				wantBalance = -2500000
			}
			assert.Equal(t, wantBalance, owner.Quota)
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.finance.Id, RefundStatus: RefundStatusRefunded})
			require.NoError(t, err)
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, wantBalance, owner.Quota)
		})
	}
}

func TestRefundCalculationMethodCannotBypassTaskOrRevision(t *testing.T) {
	f, ticket, _ := newRefundAdjustmentFixture(t)
	preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, &RefundAdjustmentInput{CalculationMethod: "invalid", Revision: preview.Revision})
	require.ErrorIs(t, err, ErrRefundAdjustment)
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, &RefundAdjustmentInput{CalculationMethod: "discount_recovery", Revision: preview.Revision, ConfirmNegative: true})
	require.ErrorIs(t, err, ErrFinanceRevision)
	require.NoError(t, DB.Create(&RefundLiability{ID: "running-task", UserID: f.user.Id, TaskDone: false}).Error)
	preview, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	assert.False(t, preview.CanApply)
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, &RefundAdjustmentInput{CalculationMethod: RefundBasisApplicationBalance, Revision: preview.Revision, ConfirmNegative: true})
	require.ErrorIs(t, err, ErrRefundAdjustment)
}

func TestRefundCalculationMethodPreservesReferralWithoutArtificialDebt(t *testing.T) {
	f, ticket := newApplicationBalanceFixture(t)
	preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	input := &RefundAdjustmentInput{CalculationMethod: RefundBasisApplicationBalance, Revision: preview.Revision}
	applied, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.NoError(t, err)
	assert.Equal(t, "210", applied.ApplicationBalance.NoncashAmount)
	assert.Equal(t, "0", applied.DebitAmount)
	assert.Equal(t, "210", applied.ApplicationBalance.ReferralGraceAmount)
	assert.False(t, applied.RequiresNegativeConfirmation)
	replayed, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.NoError(t, err)
	assert.Equal(t, applied.AppliedAt, replayed.AppliedAt)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 85619144, owner.Quota)
	assert.Equal(t, 105000000, owner.AffHistoryQuota)
	assert.Zero(t, owner.AffQuota)
}

func TestRefundReferralGraceStillRequiresConsentForRealDebt(t *testing.T) {
	f, ticket := newApplicationBalanceFixture(t)
	require.NoError(t, DB.Model(&f.user).Update("quota", -5000).Error)
	blocked, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	assert.False(t, blocked.CanApply, "referral allowance cannot erase real wallet debt")
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.RefundOverdraftLimitCents = 1
	require.NoError(t, SaveInvoiceTaxConfig(config))
	preview, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil, RefundBasisApplicationBalance)
	require.NoError(t, err)
	require.True(t, preview.CanApply)
	assert.True(t, preview.RequiresNegativeConfirmation)
	input := &RefundAdjustmentInput{CalculationMethod: RefundBasisApplicationBalance, Revision: preview.Revision}
	_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.ErrorIs(t, err, ErrRefundAdjustmentConfirmation)
	input.ConfirmNegative = true
	applied, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, input)
	require.NoError(t, err)
	assert.Equal(t, int64(100000), applied.Calculation.CashCents)
	assert.Equal(t, "-0.01", applied.BalanceAfter)
}
