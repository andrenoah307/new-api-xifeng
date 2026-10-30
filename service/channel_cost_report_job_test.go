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

func TestChannelCostReportSparseYearAndWorkflow(t *testing.T) {
	for _, mode := range []string{"none", "billed_basis", "empty"} {
		t.Run(mode, func(t *testing.T) {
			truncate(t)
			require.NoError(t, model.DB.AutoMigrate(&model.ChannelCostReportJob{}))
			t.Cleanup(func() { model.DB.Where("1 = 1").Delete(&model.ChannelCostReportJob{}) })
			require.NoError(t, model.DB.Create(&model.User{Id: 989, Username: "sparse-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
			const recent = int64(100 + 358*86400)
			policy := model.CostReportPolicy{Start: 100, End: 100 + 365*86400 + 10, QuotaPerUnit: "100", FallbackMode: "none"}
			if mode == "billed_basis" {
				policy.FallbackMode = mode
				policy.Rates = []model.CostReportRate{{ChannelID: 1, Ratio: "0.5"}}
			}
			if mode != "empty" {
				require.NoError(t, model.LOG_DB.Create(&[]model.Log{
					{CreatedAt: recent + 3599, Type: 2, ChannelId: 1, Quota: 200, Other: `{"root_cost":{"status":"estimated","amount_usd":"1"}}`},
					{CreatedAt: recent + 7200, Type: 2, ChannelId: 1, Quota: 200, Other: `{"group_ratio":1}`},
				}).Error)
			}
			job, err := model.CreateChannelCostReportJob(context.Background(), 989, uuid.NewString(), policy)
			require.NoError(t, err)
			task, err := model.GetSystemTaskByTaskID(job.SystemTaskID)
			require.NoError(t, err)
			claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
			require.NoError(t, err)
			require.True(t, ok)
			var windows [][2]int64
			handler := channelCostReportHandler{
				skipEmpty: model.SkipChannelCostReportEmptyWindows,
				aggregate: func(ctx context.Context, p model.CostReportPolicy, start, end int64) ([]model.ChannelCostReportRow, error) {
					windows = append(windows, [2]int64{start, end})
					return model.AggregateChannelCostWindow(ctx, p, start, end)
				},
			}
			handler.Run(context.Background(), claimed, "worker")
			stored, err := model.GetChannelCostReportJob(context.Background(), job.ID)
			require.NoError(t, err)
			assert.Equal(t, "completed", stored.Status)
			assert.Equal(t, policy.End, stored.Cursor)
			assert.Equal(t, stored.TotalWindows, stored.ProcessedWindows)
			var rows []model.ChannelCostReportRow
			require.NoError(t, common.UnmarshalJsonStr(stored.RowsJSON, &rows))
			if mode == "empty" {
				assert.Empty(t, windows, "an empty year needs no hourly aggregation")
				assert.Empty(t, rows)
				return
			}
			assert.Equal(t, [][2]int64{{recent, recent + 3600}, {recent + 3600, recent + 7200}, {recent + 7200, recent + 10800}, {recent + 10800, recent + 14400}}, windows)
			require.Len(t, rows, 1)
			assert.EqualValues(t, 2, rows[0].ConsumeCount)
			assert.Equal(t, "4", rows[0].ConsumeAmount)
			assert.Equal(t, "1", rows[0].OriginalCost)
			if mode == "none" {
				assert.EqualValues(t, 1, rows[0].MissingCount)
				assert.Equal(t, "0", rows[0].FallbackCost)
			} else {
				assert.Zero(t, rows[0].MissingCount)
				assert.EqualValues(t, 1, rows[0].FallbackCount)
				assert.Equal(t, "1", rows[0].FallbackCost)
			}
		})
	}
}
