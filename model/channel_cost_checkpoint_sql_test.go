package model

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestChannelCostCheckpointSQLTransactions(t *testing.T) {
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
			require.NoError(t, db.AutoMigrate(&ChannelCostReportCheckpoint{}, &ChannelCostCheckpointBudget{}))
			require.NoError(t, db.Where("1 = 1").Delete(&ChannelCostReportCheckpoint{}).Error)
			require.NoError(t, db.Where("1 = 1").Delete(&ChannelCostCheckpointBudget{}).Error)
			oldDB := DB
			DB = db
			t.Cleanup(func() { DB = oldDB })
			job := &ChannelCostReportJob{ID: "sql-checkpoints", Cursor: 100, RowsJSON: "[]"}
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return saveCostReportCheckpoint(tx, job, 3700, "[]", true) }))
			budget, err := GetCostCheckpointBudget(context.Background())
			require.NoError(t, err)
			assert.EqualValues(t, 2, budget.StoredRows)
			job.Cursor = 3700
			require.Error(t, db.Transaction(func(tx *gorm.DB) error {
				require.NoError(t, saveCostReportCheckpoint(tx, job, 7300, "[]", true))
				return errors.New("rollback")
			}))
			after, err := GetCostCheckpointBudget(context.Background())
			require.NoError(t, err)
			assert.Equal(t, budget, after)
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return saveCostReportCheckpoint(tx, job, 7300, "[]", true) }))
			// Exercise the portable baseline/boolean coverage predicate on real SQL.
			var points []ChannelCostReportCheckpoint
			require.NoError(t, db.Where("is_empty = ? OR EXISTS (SELECT 1 FROM channel_cost_report_checkpoints baseline WHERE baseline.job_id = channel_cost_report_checkpoints.job_id AND baseline.end_timestamp = channel_cost_report_checkpoints.start_timestamp)", true).Find(&points).Error)
			assert.Len(t, points, 3)
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return deleteCostReportCheckpoints(tx, []string{job.ID}) }))
			after, err = GetCostCheckpointBudget(context.Background())
			require.NoError(t, err)
			assert.Zero(t, after.StoredRows)
			assert.Zero(t, after.StoredBytes)
			var writers sync.WaitGroup
			failures := make(chan error, 2)
			for _, id := range []string{"concurrent-a", "concurrent-b"} {
				writers.Add(1)
				go func(id string) {
					defer writers.Done()
					failures <- db.Transaction(func(tx *gorm.DB) error {
						return saveCostReportCheckpoint(tx, &ChannelCostReportJob{ID: id, Cursor: 100, RowsJSON: "[]"}, 3700, "[]", true)
					})
				}(id)
			}
			writers.Wait()
			close(failures)
			for err := range failures {
				require.NoError(t, err)
			}
			after, err = GetCostCheckpointBudget(context.Background())
			require.NoError(t, err)
			assert.EqualValues(t, 4, after.StoredRows, "concurrent writers must not lose storage reservations")
			require.NoError(t, db.AutoMigrate(&Log{}, &ChannelCostReportJob{}))
			oldLog := LOG_DB
			LOG_DB = db
			t.Cleanup(func() { LOG_DB = oldLog })
			identity, err := ChannelCostReportSourceIdentity(context.Background())
			require.NoError(t, err)
			require.NoError(t, db.Where("1 = 1").Delete(&ChannelCostReportJob{}).Error)
			require.NoError(t, db.Create(&ChannelCostReportJob{ID: "sql-query", RequestKey: "sql-query", SourceIdentity: identity, Cursor: 3700, Status: "completed", PolicyJSON: `{"start":100,"end":3700,"quota_per_unit":"500000","fallback_mode":"none"}`, RowsJSON: `[{"channel_id":1,"consume_count":1,"original_count":1,"consume_amount":"10","known_revenue":"10","original_cost":"12"}]`}).Error)
			result, err := QueryChannelCostReports(context.Background(), 100, 3700, 0)
			require.NoError(t, err)
			require.True(t, result.Complete)
			require.Len(t, result.Rows, 1)
			assert.Equal(t, "12", result.Rows[0].OriginalCost)
		})
	}
}
