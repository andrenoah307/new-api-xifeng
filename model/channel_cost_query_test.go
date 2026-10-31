package model

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelCostQueryCheckpointDifferenceAndMissingEdges(t *testing.T) {
	job, task := costReportJobFixture(t)
	_, claimed, err := ClaimSystemTask(task.ID, task.Type, "query-worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, BeginChannelCostReportJob(context.Background(), job, "query-worker"))
	row := ChannelCostReportRow{ChannelID: 1, ConsumeCount: 1, OriginalCount: 1, ConsumeAmount: "10.123456789", KnownRevenue: "10.123456789", OriginalCost: "4.25"}
	require.NoError(t, PublishChannelCostReportWindow(context.Background(), job, "query-worker", 3700, []ChannelCostReportRow{row}))
	job, err = GetChannelCostReportJob(context.Background(), job.ID)
	require.NoError(t, err)
	row.ConsumeAmount, row.KnownRevenue, row.OriginalCost = "7", "7", "8"
	require.NoError(t, PublishChannelCostReportWindow(context.Background(), job, "query-worker", 7300, []ChannelCostReportRow{row}))
	result, err := QueryChannelCostReports(context.Background(), 3700, 7300, 0)
	require.NoError(t, err)
	require.True(t, result.Complete)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "7", result.Rows[0].KnownRevenue)
	assert.Equal(t, "8", result.Rows[0].OriginalCost)
	assert.EqualValues(t, 1, result.Rows[0].ConsumeCount)
	result, err = QueryChannelCostReports(context.Background(), 200, 7300, 0)
	require.NoError(t, err)
	assert.False(t, result.Complete)
	assert.Empty(t, result.Rows)
	assert.Equal(t, []CostQueryRange{{200, 3700}}, result.Missing)
	result, err = QueryChannelCostReports(context.Background(), 3700, 7300, 99)
	require.NoError(t, err)
	assert.True(t, result.Complete)
	assert.Empty(t, result.Rows)
	budget, err := GetCostCheckpointBudget(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 3, budget.StoredRows)
	require.ErrorIs(t, PublishChannelCostReportWindow(context.Background(), job, "query-worker", 7300, nil), ErrChannelCostReportConflict)
	after, err := GetCostCheckpointBudget(context.Background())
	require.NoError(t, err)
	assert.Equal(t, budget, after)
}

func TestChannelCostQueryLegacyOverlapPolicyAndSource(t *testing.T) {
	job, _ := costReportJobFixture(t)
	ctx := context.Background()
	identity := job.SourceIdentity
	require.NoError(t, DB.Delete(job).Error)
	makeJob := func(start, end int64, amount, unit string, created int64) {
		policy := CostReportPolicy{Start: start, End: end, QuotaPerUnit: unit, FallbackMode: "none", CostBasisVersion: 1}
		p, err := common.Marshal(policy)
		require.NoError(t, err)
		r, err := common.Marshal([]ChannelCostReportRow{{ChannelID: 1, ConsumeCount: 1, OriginalCount: 1, ConsumeAmount: amount, KnownRevenue: amount}})
		require.NoError(t, err)
		require.NoError(t, DB.Create(&ChannelCostReportJob{ID: uuid.NewString(), RequestKey: uuid.NewString(), SourceIdentity: identity, PolicyJSON: string(p), RowsJSON: string(r), Cursor: end, Status: "completed", CreatedAt: created}).Error)
	}
	// Greedy farthest-end selection would choose [100,4100) and miss the tail.
	makeJob(100, 4100, "100", "500000", 3)
	makeJob(100, 3100, "10", "500000", 1)
	makeJob(3100, 6100, "20", "500000", 2)
	makeJob(6100, 7100, "999", "1000000", 4)
	result, err := QueryChannelCostReports(ctx, 100, 6100, 0)
	require.NoError(t, err)
	require.True(t, result.Complete)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "30", result.Rows[0].KnownRevenue)
	assert.Len(t, result.Sources, 2)
	result, err = QueryChannelCostReports(ctx, 100, 7100, 0)
	require.NoError(t, err)
	assert.False(t, result.Complete)
	assert.Empty(t, result.Rows)
	assert.Equal(t, []CostQueryRange{{6100, 7100}}, result.Missing)
	result, err = QueryChannelCostReports(ctx, 200, 3000, 0)
	require.NoError(t, err)
	assert.False(t, result.Complete, "legacy totals must not be prorated")
	require.NoError(t, DB.Model(&ChannelCostReportJob{}).Where("source_identity = ?", identity).Update("source_identity", "different-source").Error)
	result, err = QueryChannelCostReports(ctx, 100, 6100, 0)
	require.NoError(t, err)
	assert.False(t, result.Complete)
}

func TestChannelCostQueryEmptySpanAndEvictedBaseline(t *testing.T) {
	job, task := costReportJobFixture(t)
	_, claimed, err := ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, BeginChannelCostReportJob(context.Background(), job, "worker"))
	_, err = SkipChannelCostReportEmptyWindows(context.Background(), job, "worker")
	require.NoError(t, err)
	result, err := QueryChannelCostReports(context.Background(), 200, 6000, 0)
	require.NoError(t, err)
	assert.True(t, result.Complete)
	assert.Empty(t, result.Rows)
	// Lost baselines exclude occupied windows, never treating them as zero.
	require.NoError(t, DB.Model(&ChannelCostReportCheckpoint{}).Where("job_id = ? AND start_timestamp < end_timestamp", job.ID).Update("is_empty", false).Error)
	require.NoError(t, DB.Where("job_id = ? AND end_timestamp = ?", job.ID, 100).Delete(&ChannelCostReportCheckpoint{}).Error)
	result, err = QueryChannelCostReports(context.Background(), 3700, 7300, 0)
	require.NoError(t, err)
	assert.False(t, result.Complete)
}

func TestChannelCostCheckpointRollingLimitAndAtomicRollback(t *testing.T) {
	job, _ := costReportJobFixture(t)
	// Fill the real 10,000-row boundary, then publish two checkpoints atomically.
	seed := make([]ChannelCostReportCheckpoint, CostCheckpointMaxRows)
	for i := range seed {
		seed[i] = ChannelCostReportCheckpoint{JobID: "old", Start: int64(i), End: int64(i + 1), StoredBytes: 256}
	}
	require.NoError(t, DB.CreateInBatches(seed, 200).Error)
	require.NoError(t, DB.Create(&ChannelCostCheckpointBudget{ID: 1, StoredRows: CostCheckpointMaxRows, StoredBytes: CostCheckpointMaxRows * 256}).Error)
	err := DB.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, saveCostReportCheckpoint(tx, job, 3700, "[]", true))
		return errors.New("rollback publication")
	})
	require.Error(t, err)
	budget, err := GetCostCheckpointBudget(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, CostCheckpointMaxRows, budget.StoredRows)
	assert.Zero(t, budget.EvictedRows)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return saveCostReportCheckpoint(tx, job, 3700, "[]", true) }))
	budget, err = GetCostCheckpointBudget(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, CostCheckpointMaxRows, budget.StoredRows)
	assert.EqualValues(t, 2, budget.EvictedRows)
	var count int64
	require.NoError(t, DB.Model(&ChannelCostReportCheckpoint{}).Count(&count).Error)
	assert.Equal(t, CostCheckpointMaxRows, count)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return deleteCostReportCheckpoints(tx, []string{job.ID}) }))
	budget, err = GetCostCheckpointBudget(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, CostCheckpointMaxRows-2, budget.StoredRows)
}
