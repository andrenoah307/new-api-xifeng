package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"
)

func TestInvoiceAdjustmentSQL(t *testing.T) {
	for _, engine := range []struct {
		name, env string
		kind      common.DatabaseType
	}{
		{"mysql", "TEST_INVOICE_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "TEST_INVOICE_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			if dsn == "" {
				t.Skip("disposable SQL database not configured")
			}
			var dialect gorm.Dialector = mysql.Open(dsn)
			if engine.kind == common.DatabaseTypePostgreSQL {
				dialect = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			conn, err := db.DB()
			require.NoError(t, err)
			conn.SetMaxOpenConns(4)
			oldDB, oldLogDB := DB, LOG_DB
			mainKind, logKind := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(engine.kind, engine.kind)
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = oldDB, oldLogDB
				common.SetDatabaseTypes(mainKind, logKind)
				initCol()
				require.NoError(t, conn.Close())
			})
			require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &TopUpInvoiceAdjustment{}, &Ticket{}, &TicketMessage{}, &TicketInvoice{}, &TicketRefund{}, &InvoiceTaxCharge{}, &Option{}, &Log{}, &RefundOrderAllocation{}, &RefundInvoiceExport{}))
			require.NoError(t, db.Unscoped().Where("id IN ?", []int{980601, 980602}).Delete(&User{}).Error)
			t.Run("debit and return", TestInvoiceAdjustmentDebitsConfirmsAndReverses)
			t.Run("guards and rollback", TestInvoiceAdjustmentGuardsAndRollback)
			t.Run("competing invoice", TestInvoiceAdjustmentAndApplicationCannotBothSucceed)
			t.Run("rounding and allowance", TestInvoiceAdjustmentRoundingAndConfiguredAllowance)
		})
	}
}
