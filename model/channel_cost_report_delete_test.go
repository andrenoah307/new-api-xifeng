package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteCostReportOnlyStoppedExecutors(t *testing.T) {
	for _, test := range []struct {
		name, status                    string
		executor                        SystemTaskStatus
		active, locked, single, allowed bool
	}{
		{"completed single", "completed", SystemTaskStatusSucceeded, false, false, true, true},
		{"completed bulk retained", "completed", SystemTaskStatusSucceeded, false, false, false, false},
		{"paused", "paused", SystemTaskStatusSucceeded, false, false, false, true},
		{"cancelled", "cancelled", SystemTaskStatusSucceeded, false, false, false, true},
		{"failed", "failed", SystemTaskStatusFailed, false, false, false, true},
		{"executor failed before report updated", "running", SystemTaskStatusFailed, false, false, false, true},
		{"pending", "pending", SystemTaskStatusPending, true, false, true, false},
		{"running", "running", SystemTaskStatusRunning, true, true, true, false},
		{"paused still executing", "paused", SystemTaskStatusRunning, true, true, true, false},
		{"terminal with active key", "cancelled", SystemTaskStatusSucceeded, true, false, true, false},
		{"terminal lease not released", "cancelled", SystemTaskStatusSucceeded, false, true, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			job, task := costReportJobFixture(t)
			require.NoError(t, DB.Model(job).Update("status", test.status).Error)
			updates := map[string]any{"status": test.executor}
			if !test.active {
				updates["active_key"] = nil
			}
			require.NoError(t, DB.Model(task).Updates(updates).Error)
			if test.locked {
				require.NoError(t, DB.Create(&SystemTaskLock{Type: task.Type, TaskID: task.TaskID, LockedBy: "worker", LockedUntil: common.GetTimestamp() + 60}).Error)
			}
			oldLogDB := LOG_DB
			LOG_DB = nil
			t.Cleanup(func() { LOG_DB = oldLogDB })
			id := ""
			if test.single {
				id = job.ID
			}
			deleted, err := DeleteChannelCostReportJobs(context.Background(), 989, id)
			if !test.allowed && test.single {
				require.ErrorIs(t, err, ErrChannelCostReportConflict)
			} else {
				require.NoError(t, err)
			}
			var remaining int64
			require.NoError(t, DB.Model(&ChannelCostReportJob{}).Where("id = ?", job.ID).Count(&remaining).Error)
			if test.allowed {
				assert.Equal(t, []string{job.ID}, deleted)
				assert.Zero(t, remaining)
			} else {
				assert.Empty(t, deleted)
				assert.EqualValues(t, 1, remaining)
			}
			var stored SystemTask
			require.NoError(t, DB.First(&stored, task.ID).Error)
			assert.Equal(t, test.executor, stored.Status, "executor audit history is preserved")
			var root User
			require.NoError(t, DB.First(&root, 989).Error)
			assert.Zero(t, root.Quota)
		})
	}
}

func TestDeleteCostReportRequiresFreshRootAndOnlyTarget(t *testing.T) {
	job, task := costReportJobFixture(t)
	require.NoError(t, DB.Model(job).Update("status", "completed").Error)
	require.NoError(t, DB.Model(task).Updates(map[string]any{"status": SystemTaskStatusSucceeded, "active_key": nil}).Error)
	other := *job
	other.ID, other.RequestKey = "other-report", "other-key"
	require.NoError(t, DB.Create(&other).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 989).Update("role", common.RoleAdminUser).Error)
	_, err := DeleteChannelCostReportJobs(context.Background(), 989, job.ID)
	require.Error(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 989).Update("role", common.RoleRootUser).Error)
	deleted, err := DeleteChannelCostReportJobs(context.Background(), 989, job.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{job.ID}, deleted)
	require.NoError(t, DB.First(&ChannelCostReportJob{}, "id = ?", other.ID).Error)
	_, err = DeleteChannelCostReportJobs(context.Background(), 989, job.ID)
	require.ErrorIs(t, err, ErrChannelCostReportConflict)
}
