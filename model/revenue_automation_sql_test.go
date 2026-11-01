package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRevenueAutomationSQLTransactions(t *testing.T) {
	for _, backend := range []string{"MYSQL", "POSTGRES"} {
		t.Run(backend, func(t *testing.T) {
			dsn := os.Getenv("TEST_REVENUE_" + backend + "_DSN")
			if dsn == "" {
				t.Skip("isolated SQL database not configured")
			}
			var dialect gorm.Dialector = mysql.Open(dsn)
			if backend == "POSTGRES" {
				dialect = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			pool, err := db.DB()
			require.NoError(t, err)
			defer pool.Close()
			require.NoError(t, db.AutoMigrate(&User{}, &Channel{}, &RevenueAutomation{}, &RevenueAutomationRun{}, &RevenueAutomationJob{}))
			old := DB
			DB = db
			t.Cleanup(func() { DB = old })
			for _, item := range []any{&RevenueAutomation{}, &RevenueAutomationRun{}, &RevenueAutomationJob{}, &User{}, &Channel{}} {
				require.NoError(t, db.Unscoped().Where("1 = 1").Delete(item).Error)
			}
			root := User{Username: "sql-automation-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&root).Error)
			channel := Channel{Name: "sql-cost", CostSettings: `{"mode":"manual","ratio":0.5}`}
			require.NoError(t, db.Create(&channel).Error)
			ctx := context.Background()
			preview, err := PreviewChannelCostReportConfig(ctx, 0)
			require.NoError(t, err)
			c := DefaultRevenueAutomationConfig()
			c.Enabled = true
			c.Fingerprint = preview.Fingerprint
			require.NoError(t, SaveRevenueAutomation(ctx, root.Id, 0, c))
			row, _, err := ClaimRevenueAutomation(ctx)
			require.NoError(t, err)
			require.NotNil(t, row)
			other, _, err := ClaimRevenueAutomation(ctx)
			require.NoError(t, err)
			assert.Nil(t, other)
			// MySQL affected-rows must succeed even for two writes in the same second.
			run := RevenueAutomationRun{ID: "sql-run", ScheduledAt: time.Now().Unix(), Status: "completed", Data: strings.Repeat("x", 100000)}
			require.NoError(t, SaveRevenueAutomationRun(ctx, row, &run))
			require.NoError(t, SaveRevenueAutomationRun(ctx, row, &run))
			var saved RevenueAutomationRun
			require.NoError(t, db.First(&saved, "id = ?", run.ID).Error)
			assert.Equal(t, run.Data, saved.Data)
			ReleaseRevenueAutomation(ctx, row)
			assert.ErrorIs(t, SaveRevenueAutomationRun(ctx, row, &run), ErrRevenueAutomationConflict)
		})
	}
}
