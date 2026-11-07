package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDiscountedUsageAutomaticallyDeterminesInvoiceCorrection(t *testing.T) {
	f := newRefundFinanceFixture(t)
	first := seedRefundFinanceOrder(t, f.user.Id, 100, 9000, 540, 100)
	second := seedRefundFinanceOrder(t, f.user.Id, 100, 6000, 0, 200)
	require.NoError(t, DB.Model(first).Update("discount_rate", 90).Error)
	require.NoError(t, DB.Create(&TopUp{UserId: f.user.Id, TradeNo: uuid.NewString(), Source: "discount_bonus", Status: common.TopUpStatusSuccess, QuotaGranted: 20}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 180, "used_quota": 40}).Error)
	ticket := Ticket{UserId: f.user.Id, Type: TicketTypeInvoice, Status: TicketStatusResolved}
	require.NoError(t, DB.Create(&ticket).Error)
	ids, err := common.Marshal([]int{first.Id, second.Id})
	require.NoError(t, err)
	invoice := TicketInvoice{TicketId: ticket.Id, UserId: f.user.Id, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 150, TopUpOrderIds: string(ids)}
	require.NoError(t, DB.Create(&invoice).Error)
	quote, err := quoteRefundFinanceTx(DB, f.user.Id, 60)
	require.NoError(t, err)
	require.Len(t, quote.Allocations, 1)
	assert.Equal(t, int64(3600), quote.Allocations[0].UsedInvoiceCents)
	assert.Equal(t, int64(5400), quote.RefundCashCents)
	assert.Equal(t, int64(324), quote.RefundTaxCents)
	assert.Equal(t, int64(3600), quote.Allocations[0].RemainingInvoiceCents)
	previousUnit, previousRate := common.QuotaPerUnit, operation_setting.USDExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit, operation_setting.USDExchangeRate = previousUnit, previousRate
	})
	common.QuotaPerUnit, operation_setting.USDExchangeRate = 123456, 9.9
	repriced, err := quoteRefundFinanceTx(DB, f.user.Id, 60)
	require.NoError(t, err)
	assert.Equal(t, quote.RefundCashCents, repriced.RefundCashCents)
	assert.Equal(t, quote.RefundTaxCents, repriced.RefundTaxCents)
	assert.Equal(t, quote.Allocations[0].UsedInvoiceCents, repriced.Allocations[0].UsedInvoiceCents)
	refund := TicketRefund{TicketId: ticket.Id + 1, UserId: f.user.Id, RefundStatus: RefundStatusPending}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return createRefundInvoiceCorrectionsTx(tx, &refund, quote.Allocations) }))
	var correction RefundInvoiceCorrection
	require.NoError(t, DB.Where("refund_ticket_id = ?", refund.TicketId).First(&correction).Error)
	assert.Equal(t, int64(15000), correction.OriginalCents)
	assert.Equal(t, int64(5400), correction.RefundCents)
	assert.Equal(t, int64(9600), correction.ReissueCents, "retain consumed payment and the untouched second order")
}

func TestReconciliationSourceEvidenceIsOptional(t *testing.T) {
	for name, evidence := range map[string]string{"empty": "", "whitespace": " ", "short": "已核对", "long": strings.Repeat("x", 5000)} {
		t.Run(name, func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			order := seedRefundFinanceOrder(t, f.user.Id, 1000, 900, 0, 100)
			require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
			ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 500))
			require.NoError(t, err)
			report, err := GetRefundReconciliationReport(ticket.Id)
			require.NoError(t, err)
			plan := FinanceReconciliationPlan{Payments: []FinancePaymentEvidence{{TopUpID: order.Id, PaidCents: 900, PaidQuota: 1000}}, Evidence: evidence}
			record, err := SubmitFinanceReconciliation(ticket.Id, f.finance.Id, uuid.NewString(), report.Revision, plan)
			require.NoError(t, err)
			assert.Equal(t, strings.TrimSpace(evidence), record.Plan.Evidence)
			_, err = ReviewFinanceReconciliation(record.Id, f.admin.Id, true, "verified")
			require.NoError(t, err)
		})
	}
}

func TestReconciliationUsesCurrentCNYConversionAndInvalidatesStaleReview(t *testing.T) {
	f := newRefundFinanceFixture(t)
	previousRate := operation_setting.USDExchangeRate
	previousUnit := common.QuotaPerUnit
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = previousRate
		common.QuotaPerUnit = previousUnit
	})
	common.QuotaPerUnit = 100
	operation_setting.USDExchangeRate = 7.3
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 900, 0, 100)
	require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 500))
	require.NoError(t, err)
	report, err := GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, "100", report.QuotaPerUnit)
	assert.Equal(t, "7.3", report.USDExchangeRate)
	plan := FinanceReconciliationPlan{Payments: []FinancePaymentEvidence{{TopUpID: order.Id, PaidCents: 900, PaidQuota: 1000}}, Evidence: "Verified original receipt"}
	review, err := SubmitFinanceReconciliation(ticket.Id, f.finance.Id, uuid.NewString(), report.Revision, plan)
	require.NoError(t, err)
	operation_setting.USDExchangeRate = 7
	_, err = ReviewFinanceReconciliation(review.Id, f.admin.Id, true, "Verified receipt")
	require.ErrorIs(t, err, ErrFinanceRevision)
}

func TestReconciliationClassifiesLegacyDiscountWithoutChangingWallet(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 900, 0, 100)
	require.NoError(t, DB.Model(order).Updates(map[string]any{"checkout_version": 0, "payment_total_cents": 0, "invoice_net_cents": 0, "discount_code_id": 1, "discount_rate": 90}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1100).Error)
	ticket, refund, _, err := CreateRefundTicket(financeRefundParams(f.user, 500))
	require.NoError(t, err)
	assert.Zero(t, refund.FrozenQuota)
	report, err := GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, "100", report.Difference)
	assert.Contains(t, report.Blockers, FinanceBlocker{Code: "legacy_payment_snapshot_missing", OrderID: order.Id})
	plan := FinanceReconciliationPlan{Payments: []FinancePaymentEvidence{{TopUpID: order.Id, PaidCents: 900, TaxCents: 0, PaidQuota: 800, BonusQuota: 200}}, NoncashGrantQuota: 100, Evidence: "Synthetic fixture: receipt 900 cents, 800 paid quota + 200 promotional quota; separate documented 100 quota gift"}
	key := uuid.NewString()
	record, err := SubmitFinanceReconciliation(ticket.Id, f.finance.Id, key, report.Revision, plan)
	require.NoError(t, err)
	replay, err := SubmitFinanceReconciliation(ticket.Id, f.finance.Id, key, report.Revision, plan)
	require.NoError(t, err)
	assert.Equal(t, record.Id, replay.Id)
	_, err = ReviewFinanceReconciliation(record.Id, f.finance.Id, true, "verified source receipts")
	require.ErrorIs(t, err, ErrTicketForbidden)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.finance.Id).Update("role", common.RoleAdminUser).Error)
	_, err = ReviewFinanceReconciliation(record.Id, f.finance.Id, true, "verified source receipts")
	require.ErrorIs(t, err, ErrFinanceReviewer)
	_, err = ReviewFinanceReconciliation(record.Id, f.admin.Id, true, "independent receipt verification")
	require.NoError(t, err)
	report, err = GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	assert.Empty(t, report.Blockers)
	assert.Equal(t, "0", report.Difference)
	assert.Equal(t, int64(800), report.PaidQuota)
	assert.Equal(t, int64(300), report.NoncashQuota)
	var original TopUp
	require.NoError(t, DB.First(&original, order.Id).Error)
	assert.Zero(t, original.CheckoutVersion)
	assert.Zero(t, original.PaymentTotalCents)
	assert.Equal(t, int64(1000), original.QuotaGranted)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 1100, owner.Quota)
	quote, err := QuoteRefundFinance(f.user.Id, 500)
	require.NoError(t, err)
	assert.Equal(t, int64(563), quote.RefundCashCents)
	_, err = ConfirmRefundFinance(f.admin.Id, ticket.Id, 500, quote.QuoteToken)
	require.Error(t, err)
	confirmed, err := ConfirmRefundFinance(f.user.Id, ticket.Id, 500, quote.QuoteToken)
	require.NoError(t, err)
	assert.Equal(t, 500, confirmed.FrozenQuota)
	_, err = SubmitFinanceReconciliation(ticket.Id, f.admin.Id, uuid.NewString(), report.Revision, plan)
	require.Error(t, err)
}

func TestPartialRefundsConserveActualCashFeesAndCommission(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 3, 100, 6, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 3, "inviter_id": f.finance.Id}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.finance.Id).Updates(map[string]any{"aff_quota": 101, "aff_history": 101}).Error)
	require.NoError(t, DB.Create(&CommissionRecord{UserId: f.user.Id, InviterId: f.finance.Id, TopUpId: order.Id, TopUpMoney: 1, CommissionQuota: 101, Type: CommissionTypeTopUp}).Error)
	var cash, fees int64
	for i, expected := range []struct {
		cash, fee int64
		remaining int
	}{{33, 2, 68}, {34, 2, 33}, {33, 2, 0}} {
		ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 1))
		require.NoError(t, err)
		quote, err := QuoteRefundFinance(f.user.Id, 1)
		require.NoError(t, err)
		assert.Equal(t, expected.cash, quote.RefundCashCents)
		assert.Equal(t, expected.fee, quote.RefundTaxCents)
		_, err = ConfirmRefundFinance(f.user.Id, ticket.Id, 1, quote.QuoteToken)
		require.NoError(t, err)
		attempt, err := StartRefundPayout(ticket.Id, f.admin.Id, uuid.NewString())
		require.NoError(t, err)
		_, err = CompleteRefundPayout(attempt.Id, f.admin.Id, uuid.NewString(), "Verified actual bank receipt", true)
		require.NoError(t, err)
		var inviter User
		require.NoError(t, DB.First(&inviter, f.finance.Id).Error)
		assert.Equal(t, expected.remaining, inviter.AffQuota, "partial refund %d", i)
		cash += quote.RefundCashCents
		fees += quote.RefundTaxCents
	}
	assert.Equal(t, int64(100), cash)
	assert.Equal(t, int64(6), fees)
}

func TestReconciliationRejectsStaleOrInventedPaymentEvidence(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 900, 0, 100)
	require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 500))
	require.NoError(t, err)
	report, err := GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	plan := FinanceReconciliationPlan{Payments: []FinancePaymentEvidence{{TopUpID: order.Id, PaidCents: 1000, PaidQuota: 1000}}, Evidence: "Receipt verification evidence"}
	_, err = SubmitFinanceReconciliation(ticket.Id, f.finance.Id, uuid.NewString(), report.Revision, plan)
	require.ErrorIs(t, err, ErrFinanceEvidence)
	plan.Payments[0].PaidCents = 900
	review, err := SubmitFinanceReconciliation(ticket.Id, f.finance.Id, uuid.NewString(), report.Revision, plan)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 999).Error)
	_, err = ReviewFinanceReconciliation(review.Id, f.admin.Id, true, "Independently checked receipt")
	require.ErrorIs(t, err, ErrFinanceRevision)
	var unchanged FinanceReconciliation
	require.NoError(t, DB.First(&unchanged, review.Id).Error)
	assert.Equal(t, "submitted", unchanged.Status)
}

func TestIndependentInvoiceCorrectionTracksRedAndReplacementSeparately(t *testing.T) {
	f := newRefundFinanceFixture(t)
	ticket := Ticket{UserId: f.user.Id, Type: TicketTypeInvoice, Subject: "Correction", Status: TicketStatusResolved}
	require.NoError(t, DB.Create(&ticket).Error)
	invoice := TicketInvoice{TicketId: ticket.Id, UserId: f.user.Id, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 41, TopUpOrderIds: "[1,2]"}
	require.NoError(t, DB.Create(&invoice).Error)
	_, _, _, err := UpdateInvoiceStatus(ticket.Id, f.admin.Id, InvoiceStatusRejected)
	require.ErrorIs(t, err, ErrTicketInvoiceStatusInvalid)
	key := uuid.NewString()
	correction, err := CreateIndependentInvoiceCorrection(ticket.Id, f.finance.Id, key, "Correct invoice title", true)
	require.NoError(t, err)
	replay, err := CreateIndependentInvoiceCorrection(ticket.Id, f.finance.Id, key, "Correct invoice title", true)
	require.NoError(t, err)
	assert.Equal(t, correction.Id, replay.Id)
	_, err = CreateIndependentInvoiceCorrection(ticket.Id, f.admin.Id, uuid.NewString(), "Concurrent correction", true)
	require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
	_, err = TransitionInvoiceCorrection(correction.Id, f.finance.Id, "start", nil, "")
	require.Error(t, err)
	docs := []InvoiceCorrectionDocument{{Issuer: "A", OriginalReference: "BLUE-A", OriginalCents: 1100}, {Issuer: "B", OriginalReference: "BLUE-B", OriginalCents: 3000}}
	_, err = TransitionInvoiceCorrection(correction.Id, f.user.Id, "prepare", docs, "Original receipts verified")
	require.ErrorIs(t, err, ErrTicketForbidden)
	_, err = TransitionInvoiceCorrection(correction.Id, f.finance.Id, "prepare", docs, "Original receipts verified")
	require.NoError(t, err)
	_, err = TransitionInvoiceCorrection(correction.Id, f.finance.Id, "start", nil, "")
	require.NoError(t, err)
	_, err = TransitionInvoiceCorrection(correction.Id, f.finance.Id, "cancel", nil, "Cancel after external work")
	require.Error(t, err)
	docs[0].RedReference = "RED-A"
	docs[1].RedReference = "RED-B"
	red, err := TransitionInvoiceCorrection(correction.Id, f.finance.Id, "red", docs, "Actual red invoice receipts")
	require.NoError(t, err)
	assert.Equal(t, "reissue_pending", red.Phase)
	assert.Equal(t, "processing", red.Status)
	_, err = TransitionInvoiceCorrection(correction.Id, f.finance.Id, "reissue", docs, "Replacement receipt missing")
	require.Error(t, err)
	docs[0].ReissueReference = "NEW-A"
	docs[1].ReissueReference = "NEW-B"
	done, err := TransitionInvoiceCorrection(correction.Id, f.finance.Id, "reissue", docs, "Actual replacement receipts")
	require.NoError(t, err)
	assert.Equal(t, "completed", done.Status)
	replayed, err := TransitionInvoiceCorrection(correction.Id, f.finance.Id, "reissue", docs, "Actual replacement receipts")
	require.NoError(t, err)
	assert.Equal(t, done.Id, replayed.Id)
	loaded, err := GetTicketInvoiceByTicketId(ticket.Id)
	require.NoError(t, err)
	require.Len(t, loaded.Corrections, 1)
	assert.Equal(t, "NEW-B", loaded.Corrections[0].Documents[1].ReissueReference)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Zero(t, owner.Quota)
}

func TestRefundPayoutUnknownOutcomeCannotBeCancelledOrPaidTwice(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 1000, 10000, 600, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 500))
	require.NoError(t, err)
	quote, err := QuoteRefundFinance(f.user.Id, 500)
	require.NoError(t, err)
	_, err = ConfirmRefundFinance(f.user.Id, ticket.Id, 500, quote.QuoteToken)
	require.NoError(t, err)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.ErrorIs(t, err, ErrRefundPayout)
	_, err = StartRefundPayout(ticket.Id, f.finance.Id, uuid.NewString())
	require.ErrorIs(t, err, ErrTicketForbidden)
	payout, err := StartRefundPayout(ticket.Id, f.admin.Id, uuid.NewString())
	require.NoError(t, err)
	_, err = RecordRefundPayoutReview(payout.Id, f.admin.Id, "outcome_unknown", "Bank transfer response timed out")
	require.NoError(t, err)
	_, err = StartRefundPayout(ticket.Id, f.admin.Id, uuid.NewString())
	require.ErrorIs(t, err, ErrRefundPayout)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.ErrorIs(t, err, ErrRefundPayout)
	for range 2 {
		refund, err := CompleteRefundPayout(payout.Id, f.admin.Id, "BANK-REFERENCE", "Bank confirms exact 5000 cents paid")
		require.NoError(t, err)
		assert.Equal(t, RefundStatusRefunded, refund.RefundStatus)
	}
	_, err = CompleteRefundPayout(payout.Id, f.admin.Id, "DIFFERENT-REFERENCE", "Bank confirms exact 5000 cents paid")
	require.ErrorIs(t, err, ErrRefundPayout)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 500, owner.Quota)
	second, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 500))
	require.NoError(t, err)
	quote, err = QuoteRefundFinance(f.user.Id, 500)
	require.NoError(t, err)
	_, err = ConfirmRefundFinance(f.user.Id, second.Id, 500, quote.QuoteToken)
	require.NoError(t, err)
	secondAttempt, err := StartRefundPayout(second.Id, f.admin.Id, uuid.NewString())
	require.NoError(t, err)
	_, err = CompleteRefundPayout(secondAttempt.Id, f.admin.Id, "BANK-REFERENCE", "Receipt incorrectly reused for another refund")
	require.Error(t, err)
	var unchanged TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", second.Id).First(&unchanged).Error)
	assert.Equal(t, RefundStatusPending, unchanged.RefundStatus)
	assert.Equal(t, 500, unchanged.FrozenQuota)
}
