package service

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelCostReportWorkerPublishesOnlyCompletedWindows(t *testing.T) {
	for _, pause := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "pause-and-resume"}[pause], func(t *testing.T) {
			truncate(t)
			require.NoError(t, model.DB.AutoMigrate(&model.ChannelCostReportJob{}))
			t.Cleanup(func() { model.DB.Where("1 = 1").Delete(&model.ChannelCostReportJob{}) })
			require.NoError(t, model.DB.Create(&model.User{Id: 989, Username: "report-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
			job, err := model.CreateChannelCostReportJob(context.Background(), 989, uuid.NewString(), model.CostReportPolicy{Start: 100, End: 7300, QuotaPerUnit: "500000", FallbackMode: "none"})
			require.NoError(t, err)
			calls := 0
			handler := channelCostReportHandler{aggregate: func(ctx context.Context, policy model.CostReportPolicy, start, end int64) ([]model.ChannelCostReportRow, error) {
				calls++
				if pause && calls == 2 {
					require.NoError(t, model.ControlChannelCostReportJob(ctx, job.ID, "pause"))
				}
				return []model.ChannelCostReportRow{{ChannelID: 1, ConsumeCount: 1, OriginalCount: 1, ConsumeAmount: "2", RefundAmount: "0", OriginalCost: "1", FallbackCost: "0", KnownRevenue: "2", UnknownRevenue: "0"}}, nil
			}}
			run := func() {
				task, err := model.GetSystemTaskByTaskID(job.SystemTaskID)
				require.NoError(t, err)
				claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
				require.NoError(t, err)
				require.True(t, ok)
				handler.Run(context.Background(), claimed, "worker")
			}
			run()
			job, err = model.GetChannelCostReportJob(context.Background(), job.ID)
			require.NoError(t, err)
			if pause {
				assert.Equal(t, "paused", job.Status)
				assert.Equal(t, 1, job.ProcessedWindows)
				require.NoError(t, model.ControlChannelCostReportJob(context.Background(), job.ID, "resume"))
				job, err = model.GetChannelCostReportJob(context.Background(), job.ID)
				require.NoError(t, err)
				run()
				job, err = model.GetChannelCostReportJob(context.Background(), job.ID)
				require.NoError(t, err)
			}
			assert.Equal(t, "completed", job.Status)
			assert.Equal(t, 2, job.ProcessedWindows)
			var rows []model.ChannelCostReportRow
			require.NoError(t, common.UnmarshalJsonStr(job.RowsJSON, &rows))
			require.Len(t, rows, 1)
			assert.Equal(t, int64(2), rows[0].ConsumeCount)
			assert.Equal(t, "2", rows[0].OriginalCost)
		})
	}
}

func TestChannelCostReportCancellationWaitsForQueryBeforeUnlock(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelCostReportJob{}))
	t.Cleanup(func() { model.DB.Where("1 = 1").Delete(&model.ChannelCostReportJob{}) })
	require.NoError(t, model.DB.Create(&model.User{Id: 989, Username: "report-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	job, err := model.CreateChannelCostReportJob(context.Background(), 989, uuid.NewString(), model.CostReportPolicy{Start: 100, End: 3700, QuotaPerUnit: "500000", FallbackMode: "none"})
	require.NoError(t, err)
	task, err := model.GetSystemTaskByTaskID(job.SystemTaskID)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, ok)
	entered, cancelled, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := channelCostReportHandler{aggregate: func(ctx context.Context, _ model.CostReportPolicy, _, _ int64) ([]model.ChannelCostReportRow, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
		return nil, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { defer close(finished); handler.Run(ctx, claimed, "worker") }()
	<-entered
	cancel()
	<-cancelled
	active, err := model.GetActiveSystemTask(model.SystemTaskTypeChannelCostReport)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, model.SystemTaskStatusRunning, active.Status)
	close(release)
	<-finished
	active, err = model.GetActiveSystemTask(model.SystemTaskTypeChannelCostReport)
	require.NoError(t, err)
	assert.Nil(t, active)
	stored, err := model.GetChannelCostReportJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", stored.Status)
	assert.Zero(t, stored.ProcessedWindows)
}
