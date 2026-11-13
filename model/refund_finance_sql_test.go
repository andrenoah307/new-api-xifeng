package model

import (
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These DSNs must identify disposable test databases, never an application database.
func TestRefundFinanceSQLTransactions(t *testing.T) {
	for _, engine := range []struct {
		name, env string
		kind      common.DatabaseType
	}{
		{"mysql", "TEST_REFUND_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "TEST_REFUND_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			if dsn == "" {
				t.Skip("isolated SQL integration DSN not configured")
			}
			var dialect gorm.Dialector = mysql.Open(dsn)
			if engine.kind == common.DatabaseTypePostgreSQL {
				dialect = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			connection, err := db.DB()
			require.NoError(t, err)
			connection.SetMaxOpenConns(4)
			previousDB, previousLogDB := DB, LOG_DB
			mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(engine.kind, engine.kind)
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(mainType, logType)
				initCol()
				require.NoError(t, connection.Close())
			})
			models := []any{&User{}, &TopUp{}, &Ticket{}, &TicketMessage{}, &TicketInvoice{}, &InvoiceTaxCharge{}, &TicketRefund{}, &RefundOrderAllocation{}, &RefundInvoiceCorrection{}, &FinanceReconciliation{}, &RefundPayoutAttempt{}, &RefundLiability{}, &Option{}, &Log{}, &CommissionRecord{}}
			require.NoError(t, db.AutoMigrate(models...))
			require.NoError(t, db.AutoMigrate(&RefundInvoiceExport{}))
			require.NoError(t, db.AutoMigrate(models...))
			// Simulate the previous release's unique constraint before upgrading.
			require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_refund_invoice_correction ON refund_invoice_corrections (refund_ticket_id, invoice_ticket_id)").Error)
			require.NoError(t, migrateFinanceCorrectionIndex())
			require.NoError(t, migrateFinanceCorrectionIndex())
			require.NoError(t, db.AutoMigrate(models...))
			var version string
			require.NoError(t, db.Raw("SELECT version()").Scan(&version).Error)
			t.Logf("transaction verification on %s", version)
			require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "vat", OrdinaryRateBps: 600}))
			owner := User{Username: uuid.NewString()[:20], AffCode: uuid.NewString()[:20], Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 5000, UsedQuota: 5000}
			finance := User{Username: uuid.NewString()[:20], AffCode: uuid.NewString()[:20], Role: common.RoleFinanceUser, Status: common.UserStatusEnabled}
			admin := User{Username: uuid.NewString()[:20], AffCode: uuid.NewString()[:20], Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
			for _, user := range []*User{&owner, &finance, &admin} {
				require.NoError(t, db.Create(user).Error)
			}
			order := seedRefundFinanceOrder(t, owner.Id, 10000, 1060000, 60000, 100)
			invoiceTicket := Ticket{UserId: owner.Id, Type: TicketTypeInvoice, Status: TicketStatusResolved}
			require.NoError(t, db.Create(&invoiceTicket).Error)
			ids, err := common.Marshal([]int{order.Id})
			require.NoError(t, err)
			require.NoError(t, db.Create(&TicketInvoice{TicketId: invoiceTicket.Id, UserId: owner.Id, TopUpOrderIds: string(ids), TotalMoney: 10600, InvoiceStatus: InvoiceStatusIssued, InvoiceType: "ordinary"}).Error)

			params := financeRefundParams(owner, 5000)
			historicalQuote, err := quoteRefundFinanceTx(DB, owner.Id, 5000)
			require.NoError(t, err)
			assert.Equal(t, int64(530000), historicalQuote.RefundCashCents)
			quote, err := QuoteRefundFinance(owner.Id, 5000)
			require.NoError(t, err)
			assert.Equal(t, RefundSettlementWaiting, quote.SettlementStatus)
			assert.Zero(t, quote.RefundCashCents)
			ticket, refund, _, err := CreateRefundTicket(params)
			require.NoError(t, err)
			assert.Equal(t, 5000, refund.FrozenQuota)
			assert.Empty(t, refund.Finance.InvoiceCorrections)
			require.NoError(t, DecreaseUserQuota(owner.Id, 1000, true))
			repeated, _, _, err := CreateRefundTicket(params)
			require.NoError(t, err)
			assert.Equal(t, ticket.Id, repeated.Id)
			for _, mode := range []string{"", RefundQuotaModeWriteOff, RefundQuotaModeSubtract, RefundQuotaModeOverride} {
				_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: admin.Id, RefundStatus: RefundStatusRefunded, QuotaMode: mode, ActualRefundQuota: 1})
				require.ErrorIs(t, err, ErrRefundSettlementUnconfirmed)
			}
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: admin.Id, RefundStatus: RefundStatusRejected})
			require.NoError(t, err)
			require.NoError(t, db.First(&owner, owner.Id).Error)
			assert.Equal(t, 4000, owner.Quota)
			// Simulate an already frozen legacy refund and verify only its actual reserve is returned once.
			legacyTicket := Ticket{UserId: owner.Id, Type: TicketTypeRefund, Status: TicketStatusOpen}
			require.NoError(t, db.Create(&legacyTicket).Error)
			require.NoError(t, db.Create(&TicketRefund{UserId: owner.Id, TicketId: legacyTicket.Id, RefundQuota: 1000, FrozenQuota: 200, RefundStatus: RefundStatusPending}).Error)
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: legacyTicket.Id, AdminId: admin.Id, RefundStatus: RefundStatusRejected})
			require.NoError(t, err)
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: legacyTicket.Id, AdminId: admin.Id, RefundStatus: RefundStatusRejected})
			require.ErrorIs(t, err, ErrTicketRefundNotPending)
			require.NoError(t, db.First(&owner, owner.Id).Error)
			assert.Equal(t, 4200, owner.Quota)
			other := User{Username: uuid.NewString()[:20], AffCode: uuid.NewString()[:20], Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 10000}
			require.NoError(t, db.Create(&other).Error)
			seedRefundFinanceOrder(t, other.Id, 10000, 1060000, 60000, 100)
			failedParams := financeRefundParams(other, 5000)
			injected := errors.New("test message write rejected")
			callback := "test_refund_atomic_message_failure"
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "ticket_messages" {
					tx.AddError(injected)
				}
			}))
			_, _, _, err = CreateRefundTicket(failedParams)
			require.NoError(t, db.Callback().Create().Remove(callback))
			require.ErrorIs(t, err, injected)
			require.NoError(t, db.First(&other, other.Id).Error)
			assert.Equal(t, 10000, other.Quota)
			var count int64
			require.NoError(t, db.Model(&TicketRefund{}).Where("user_id = ?", other.Id).Count(&count).Error)
			assert.Zero(t, count)
			require.NoError(t, db.Model(&RefundOrderAllocation{}).Where("user_id = ?", other.Id).Count(&count).Error)
			assert.Zero(t, count)

			start := make(chan struct{})
			results := make(chan error, 2)
			var workers sync.WaitGroup
			for range 2 {
				request := financeRefundParams(other, 5000)
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					_, _, _, err := CreateRefundTicket(request)
					results <- err
				}()
			}
			close(start)
			workers.Wait()
			close(results)
			successes := 0
			for err := range results {
				if err == nil {
					successes++
				} else {
					assert.ErrorIs(t, err, ErrRefundFinancePending)
				}
			}
			assert.Equal(t, 1, successes)
			require.NoError(t, db.First(&other, other.Id).Error)
			assert.Equal(t, 5000, other.Quota)
			t.Run("published_policy", TestRefundSettlementPolicyFlow)
			t.Run("admission_and_confirmation_races", TestRefundSettlementConcurrentConfirmation)
			t.Run("historical_reconciliation", TestReconciliationClassifiesLegacyDiscountWithoutChangingWallet)
			t.Run("reconciliation_policy", TestReconciliationReviewPolicy)
			t.Run("provisional_hold", TestRefundProvisionalHoldAdjustment)
			t.Run("view_hold_sync", TestRefundViewSynchronizesExistingHolds)
			t.Run("view_pending_review", TestRefundViewPreservesPendingReconciliation)
			t.Run("reconciliation_upgrade", TestReconciliationReviewUpgrade)
			t.Run("reconciliation_concurrent_submit", TestReconciliationConcurrentDirectSubmission)
			t.Run("invoice_correction_phases", TestIndependentInvoiceCorrectionTracksRedAndReplacementSeparately)
			t.Run("payout_uncertainty", TestRefundPayoutUnknownOutcomeCannotBeCancelledOrPaidTwice)
			t.Run("proportional_refunds", TestPartialRefundsConserveActualCashFeesAndCommission)
			t.Run("gift_hold_and_release", TestRefundCreditLotCashFirstAndCancellation)
			t.Run("gift_fifo_expiry", TestRefundCreditLotTwoDiscountOrdersExpireEarlierGift)
			t.Run("gift_rounding", TestRefundCreditLotPartialPayoutConservesGift)
			t.Run("gift_legacy_upgrade", TestRefundGiftUpgradePreservesExistingAllocations)
			t.Run("original_exports", TestOriginalRefundInvoiceExportsAcrossOrdersAndInvoices)
			t.Run("original_gift_policies", TestOriginalRefundCashAndGiftRecovery)
			t.Run("issuer_identity_exports", TestRefundExportsPreserveInvoiceTitlesAndIssuerAmounts)
			t.Run("issuer_partial_exports", TestRefundExportsPreserveIssuersAcrossPartialRefunds)
			t.Run("consumed_replacement_invoices", TestRefundReplacementInvoicesContainOnlyConsumedCash)
			t.Run("pending_invoice_refund_lock", TestPendingInvoiceRefundLock)
			t.Run("original_adjustment", TestOriginalRefundRejectionAndAdjustedApproval)
			t.Run("recovery_unissued", TestRefundRecoveryUnissuedCorrection)
			t.Run("recovery_close_restart", TestAdminCloseUnissuedRefundAllowsNewApplication)
			t.Run("recovery_issued", TestRefundRecoveryIssuedCorrectionCompletesOnce)
			t.Run("manual_adjustment", TestRefundAdjustmentPreviewApplyReplayAndComplete)
			t.Run("manual_adjustment_rollback", TestRefundAdjustmentRefreshConfirmationAndRollback)
			t.Run("manual_adjustment_evidence", TestRefundAdjustmentRejectsChangedEvidence)
			t.Run("manual_adjustment_settlement", TestRefundAdjustmentSettlementRequiresExplicitEvidence)
			t.Run("manual_adjustment_cancellation", TestRefundAdjustmentSettlementOnlyAndCancellation)
			t.Run("manual_adjustment_concurrency", TestRefundAdjustmentConcurrentConfirmation)
			t.Run("application_balance", TestApplicationBalanceRefundPreservesReferralAndWallet)
			t.Run("application_balance_grace", TestApplicationBalanceGraceRecheckAndCancellation)
			t.Run("application_balance_atomic", TestApplicationBalanceAtomicConfirmation)
			t.Run("application_balance_bonus", TestApplicationBalancePreservesBonusAndPaymentCaps)
			t.Run("application_balance_tax", TestApplicationBalanceKeepsTaxRatioAndLedgerDifference)
			t.Run("calculation_method", TestRefundCalculationMethodSelection)
			t.Run("calculation_method_guards", TestRefundCalculationMethodCannotBypassTaskOrRevision)
			t.Run("calculation_method_referral", TestRefundCalculationMethodPreservesReferralWithoutArtificialDebt)
			t.Run("referral_real_debt", TestRefundReferralGraceStillRequiresConsentForRealDebt)
		})
	}
}
