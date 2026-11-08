package model

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconciliationReviewPolicy(t *testing.T) {
	for _, scenario := range []string{"default", "quorum", "concurrent", "disabled_pending", "lowered_quorum", "reject", "stale"} {
		t.Run(scenario, func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			config, err := GetInvoiceTaxConfig(DB)
			require.NoError(t, err)
			assert.False(t, config.ReconciliationReviewEnabled)
			if scenario != "default" {
				config.ReconciliationReviewEnabled, config.ReconciliationReviewCount = true, 2
				require.NoError(t, SaveInvoiceTaxConfig(config))
			}
			order := seedRefundFinanceOrder(t, f.user.Id, 1000, 900, 0, 100)
			require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
			ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 500))
			require.NoError(t, err)
			report, err := GetRefundReconciliationReport(ticket.Id)
			require.NoError(t, err)
			plan := FinanceReconciliationPlan{Payments: []FinancePaymentEvidence{{TopUpID: order.Id, PaidCents: 900, PaidQuota: 1000}}}
			if scenario == "default" {
				_, err = SubmitFinanceReconciliation(ticket.Id, f.finance.Id, uuid.NewString(), report.Revision, plan)
				require.ErrorIs(t, err, ErrTicketForbidden)
			}
			key := uuid.NewString()
			record, err := SubmitFinanceReconciliation(ticket.Id, f.admin.Id, key, report.Revision, plan)
			require.NoError(t, err)
			if scenario == "default" {
				assert.Equal(t, "applied", record.Status)
				assert.Equal(t, f.admin.Id, record.ReviewedBy)
				replay, err := SubmitFinanceReconciliation(ticket.Id, f.admin.Id, key, report.Revision, plan)
				require.NoError(t, err)
				assert.Equal(t, record.Id, replay.Id)
			} else {
				assert.Equal(t, "submitted", record.Status)
				_, err = ReviewFinanceReconciliation(record.Id, f.admin.Id, true, "self review")
				require.ErrorIs(t, err, ErrFinanceReviewer)
				_, err = ReviewFinanceReconciliation(record.Id, f.finance.Id, true, "finance review")
				require.ErrorIs(t, err, ErrTicketForbidden)
				partial, err := ReviewFinanceReconciliation(record.Id, f.root.Id, true, "first reviewer")
				require.NoError(t, err)
				assert.Equal(t, "submitted", partial.Status)
				require.Len(t, partial.Approvals, 1)
				partial, err = ReviewFinanceReconciliation(record.Id, f.root.Id, true, "first reviewer")
				require.NoError(t, err)
				assert.Len(t, partial.Approvals, 1)
				waiting, err := GetRefundReconciliationReport(ticket.Id)
				require.NoError(t, err)
				assert.Equal(t, "awaiting_review", waiting.Stage)
				assert.NotEmpty(t, waiting.Blockers)
				switch scenario {
				case "concurrent":
					require.NoError(t, DB.Model(&User{}).Where("id = ?", f.finance.Id).Update("role", common.RoleAdminUser).Error)
					start := make(chan struct{})
					results := make(chan error, 2)
					var workers sync.WaitGroup
					for range 2 {
						workers.Add(1)
						go func() {
							defer workers.Done()
							<-start
							_, reviewErr := ReviewFinanceReconciliation(partial.Id, f.finance.Id, true, "second reviewer")
							results <- reviewErr
						}()
					}
					close(start)
					workers.Wait()
					close(results)
					for reviewErr := range results {
						require.NoError(t, reviewErr)
					}
					err = DB.First(record, record.Id).Error
					require.NoError(t, err)
					err = common.UnmarshalJsonStr(record.ApprovalsJSON, &record.Approvals)
					require.NoError(t, err)
					assert.Len(t, record.Approvals, 2)
				case "disabled_pending":
					config.ReconciliationReviewEnabled = false
					require.NoError(t, SaveInvoiceTaxConfig(config))
					record, err = SubmitFinanceReconciliation(ticket.Id, f.admin.Id, uuid.NewString(), report.Revision, plan)
				case "lowered_quorum":
					config.ReconciliationReviewCount = 1
					require.NoError(t, SaveInvoiceTaxConfig(config))
					record, err = ReviewFinanceReconciliation(record.Id, f.root.Id, true, "first reviewer")
				default:
					require.NoError(t, DB.Model(&User{}).Where("id = ?", f.finance.Id).Update("role", common.RoleAdminUser).Error)
					if scenario == "stale" {
						require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 999).Error)
					}
					record, err = ReviewFinanceReconciliation(record.Id, f.finance.Id, scenario != "reject", "second reviewer")
				}
				if scenario == "stale" {
					require.ErrorIs(t, err, ErrFinanceRevision)
					var unchanged FinanceReconciliation
					require.NoError(t, DB.First(&unchanged, partial.Id).Error)
					assert.Equal(t, partial.ApprovalsJSON, unchanged.ApprovalsJSON)
					assert.Equal(t, "submitted", unchanged.Status)
					return
				}
				require.NoError(t, err)
				if scenario == "reject" {
					assert.Equal(t, "rejected", record.Status)
					return
				}
				assert.Equal(t, "applied", record.Status)
				if scenario == "quorum" {
					assert.Len(t, record.Approvals, 2)
				}
			}
			report, err = GetRefundReconciliationReport(ticket.Id)
			require.NoError(t, err)
			assert.Empty(t, report.Blockers)
			var owner User
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, 500, owner.Quota)
			var publicAudits int64
			require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ? AND content LIKE ?", ticket.Id, "Financial reconciliation #%").Count(&publicAudits).Error)
			assert.Zero(t, publicAudits)
		})
	}
}

func TestReconciliationReviewCountValidation(t *testing.T) {
	newRefundFinanceFixture(t)
	for _, count := range []int{-1, 0} {
		require.ErrorIs(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Mode: "vat", ReconciliationReviewEnabled: true, ReconciliationReviewCount: count}), ErrInvoiceTaxInvalid)
	}
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Mode: "vat"}))
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	assert.False(t, config.ReconciliationReviewEnabled)
	assert.Equal(t, 1, config.ReconciliationReviewCount)
}

func TestReconciliationReviewUpgrade(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 900, 0, 100)
	require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 500))
	require.NoError(t, err)
	report, err := GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	plan := FinanceReconciliationPlan{Payments: []FinancePaymentEvidence{{TopUpID: order.Id, PaidCents: 900, PaidQuota: 1000}}}
	encoded, err := common.Marshal(plan)
	require.NoError(t, err)
	legacy := FinanceReconciliation{UserId: f.user.Id, TicketId: ticket.Id, ProposedBy: f.admin.Id, RequestKey: uuid.NewString(), SourceRevision: report.Revision, PlanJSON: string(encoded), Status: "submitted"}
	require.NoError(t, DB.Create(&legacy).Error)
	// Exercise the nullable extension on a disposable old-schema database.
	require.NoError(t, DB.Migrator().DropColumn(&FinanceReconciliation{}, "ApprovalsJSON"))
	require.NoError(t, DB.AutoMigrate(&FinanceReconciliation{}))
	require.NoError(t, DB.AutoMigrate(&FinanceReconciliation{}))
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		connection, err := DB.DB()
		require.NoError(t, err)
		connection.SetMaxIdleConns(0)
		t.Cleanup(func() { connection.SetMaxIdleConns(2) })
	}
	report, err = GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	require.Len(t, report.Reviews, 1)
	assert.Empty(t, report.Reviews[0].Approvals)
	assert.Equal(t, "submitted", report.Reviews[0].Status)
	assert.False(t, report.ReviewEnabled)
	applied, err := ReviewFinanceReconciliation(legacy.Id, f.admin.Id, true, "Apply existing verified proposal")
	require.NoError(t, err)
	assert.Equal(t, "applied", applied.Status)
	report, err = GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	assert.Empty(t, report.Blockers)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 500, owner.Quota)
}

func TestReconciliationConcurrentDirectSubmission(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 900, 0, 100)
	require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1100).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 500))
	require.NoError(t, err)
	report, err := GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	plan := FinanceReconciliationPlan{Payments: []FinancePaymentEvidence{{TopUpID: order.Id, PaidCents: 900, PaidQuota: 1000}}, NoncashGrantQuota: 100}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, actor := range []int{f.admin.Id, f.root.Id} {
		workers.Add(1)
		go func(actor int) {
			defer workers.Done()
			<-start
			_, err := SubmitFinanceReconciliation(ticket.Id, actor, uuid.NewString(), report.Revision, plan)
			results <- err
		}(actor)
	}
	close(start)
	workers.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrFinanceRevision)
		}
	}
	assert.Equal(t, 1, successes)
	report, err = GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	assert.Empty(t, report.Blockers)
	assert.Equal(t, int64(100), report.NoncashQuota)
	assert.Equal(t, 600, report.WalletQuota)
	require.Len(t, report.Reviews, 1)
}
