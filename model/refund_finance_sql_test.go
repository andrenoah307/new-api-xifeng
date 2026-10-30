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
			models := []any{&User{}, &TopUp{}, &Ticket{}, &TicketMessage{}, &TicketInvoice{}, &InvoiceTaxCharge{}, &TicketRefund{}, &RefundOrderAllocation{}, &RefundInvoiceCorrection{}, &Option{}, &Log{}, &CommissionRecord{}}
			require.NoError(t, db.AutoMigrate(models...))
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
			quote, err := QuoteRefundFinance(owner.Id, 5000)
			require.NoError(t, err)
			params.FinanceQuote = quote.QuoteToken
			ticket, refund, _, err := CreateRefundTicket(params)
			require.NoError(t, err)
			require.Equal(t, int64(530000), refund.RefundCashCents)
			require.Len(t, refund.Finance.InvoiceCorrections, 1)
			repeated, _, _, err := CreateRefundTicket(params)
			require.NoError(t, err)
			assert.Equal(t, ticket.Id, repeated.Id)
			correction := refund.Finance.InvoiceCorrections[0]
			_, err = StartRefundInvoiceCorrection(correction.Id, finance.Id)
			require.NoError(t, err)
			_, err = CloseUserTicket(ticket.Id, owner.Id)
			require.Error(t, err)
			_, err = CompleteRefundInvoiceCorrection(correction.Id, finance.Id, "red-sql", "blue-sql", "verified")
			require.NoError(t, err)
			approve := UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: admin.Id, RefundStatus: RefundStatusRefunded}
			settled, _, _, _, err := UpdateRefundStatus(approve)
			require.NoError(t, err)
			assert.Equal(t, RefundStatusRefunded, settled.RefundStatus)
			_, _, _, _, err = UpdateRefundStatus(approve)
			require.ErrorIs(t, err, ErrTicketRefundNotPending)
			require.NoError(t, db.First(&owner, owner.Id).Error)
			assert.Zero(t, owner.Quota)
			require.NoError(t, db.First(&correction, correction.Id).Error)
			assert.Equal(t, int64(1060000), correction.OriginalCents)
			assert.Equal(t, int64(530000), correction.ReissueCents)

			other := User{Username: uuid.NewString()[:20], AffCode: uuid.NewString()[:20], Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 10000}
			require.NoError(t, db.Create(&other).Error)
			seedRefundFinanceOrder(t, other.Id, 10000, 1060000, 60000, 100)
			failedParams := financeRefundParams(other, 5000)
			quote, err = QuoteRefundFinance(other.Id, 5000)
			require.NoError(t, err)
			failedParams.FinanceQuote = quote.QuoteToken
			require.NoError(t, db.Model(&User{}).Where("id = ?", other.Id).Updates(map[string]any{"quota": 9900, "used_quota": 100}).Error)
			_, _, _, err = CreateRefundTicket(failedParams)
			require.ErrorIs(t, err, ErrInvoiceTaxQuoteChanged)
			require.NoError(t, db.First(&other, other.Id).Error)
			assert.Equal(t, 9900, other.Quota, "stale preview must not reserve additional wallet quota")
			var staleCount int64
			require.NoError(t, db.Model(&TicketRefund{}).Where("user_id = ?", other.Id).Count(&staleCount).Error)
			assert.Zero(t, staleCount)
			require.NoError(t, db.Model(&RefundOrderAllocation{}).Where("user_id = ?", other.Id).Count(&staleCount).Error)
			assert.Zero(t, staleCount)
			require.NoError(t, db.Model(&User{}).Where("id = ?", other.Id).Updates(map[string]any{"quota": 10000, "used_quota": 0}).Error)
			quote, err = QuoteRefundFinance(other.Id, 5000)
			require.NoError(t, err)
			failedParams.FinanceQuote = quote.QuoteToken
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
				request.FinanceQuote = quote.QuoteToken
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
		})
	}
}
