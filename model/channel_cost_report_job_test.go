package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"path/filepath"
	"sync"
	"testing"
)

func costReportJobFixture(t *testing.T, policies ...CostReportPolicy) (*ChannelCostReportJob, *SystemTask) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&ChannelCostReportJob{}))
	t.Cleanup(func() { DB.Where("1 = 1").Delete(&ChannelCostReportJob{}) })
	require.NoError(t, DB.Create(&User{Id: 989, Username: "cost-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	policy := CostReportPolicy{Start: 100, End: 7300, QuotaPerUnit: "500000", FallbackMode: "none"}
	if len(policies) > 0 {
		policy = policies[0]
	}
	job, err := CreateChannelCostReportJob(context.Background(), 989, uuid.NewString(), policy)
	require.NoError(t, err)
	task, err := GetSystemTaskByTaskID(job.SystemTaskID)
	require.NoError(t, err)
	return job, task
}

func TestChannelCostReportMultiYearHistoryKeepsBoundedWindows(t *testing.T) {
	policy := CostReportPolicy{Start: 100, End: 100 + 10*365*86400, QuotaPerUnit: "500000", FallbackMode: "none"}
	job, task := costReportJobFixture(t, policy)
	assert.Equal(t, 10*365*24, job.TotalWindows)
	_, claimed, err := ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, BeginChannelCostReportJob(context.Background(), job, "worker"))
	require.Error(t, PublishChannelCostReportWindow(context.Background(), job, "worker", policy.End, nil))
	require.NoError(t, PublishChannelCostReportWindow(context.Background(), job, "worker", policy.Start+3600, nil))
	stored, err := GetChannelCostReportJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, policy.Start+3600, stored.Cursor)
	assert.Equal(t, 1, stored.ProcessedWindows)
}

func TestChannelCostReportIdempotencyAndExclusiveActiveJob(t *testing.T) {
	job, _ := costReportJobFixture(t)
	var policy CostReportPolicy
	require.NoError(t, common.UnmarshalJsonStr(job.PolicyJSON, &policy))
	again, err := CreateChannelCostReportJob(context.Background(), job.OwnerID, job.RequestKey, policy)
	require.NoError(t, err)
	assert.Equal(t, job.ID, again.ID)
	_, err = CreateChannelCostReportJob(context.Background(), job.OwnerID, uuid.NewString(), policy)
	require.ErrorIs(t, err, ErrChannelCostReportConflict)
	policy.End++
	_, err = CreateChannelCostReportJob(context.Background(), job.OwnerID, job.RequestKey, policy)
	require.ErrorIs(t, err, ErrChannelCostReportConflict)
}

func TestChannelCostReportWindowPublicationFences(t *testing.T) {
	for _, rejection := range []string{"replay", "pause", "owner", "lease", "runner"} {
		t.Run(rejection, func(t *testing.T) {
			job, task := costReportJobFixture(t)
			_, claimed, err := ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
			require.NoError(t, err)
			require.True(t, claimed)
			require.NoError(t, BeginChannelCostReportJob(context.Background(), job, "worker"))
			runner := "worker"
			switch rejection {
			case "replay":
				require.NoError(t, PublishChannelCostReportWindow(context.Background(), job, runner, 3700, nil))
			case "pause":
				require.NoError(t, ControlChannelCostReportJob(context.Background(), job.ID, "pause"))
			case "owner":
				require.NoError(t, DB.Model(&User{}).Where("id = ?", job.OwnerID).Update("role", common.RoleAdminUser).Error)
			case "lease":
				require.NoError(t, DB.Model(&SystemTaskLock{}).Where("task_id = ?", job.SystemTaskID).Update("locked_until", common.GetTimestamp()-1).Error)
			case "runner":
				runner = "stale-worker"
			}
			require.ErrorIs(t, PublishChannelCostReportWindow(context.Background(), job, runner, 3700, nil), ErrChannelCostReportConflict)
			stored, err := GetChannelCostReportJob(context.Background(), job.ID)
			require.NoError(t, err)
			if rejection == "replay" {
				assert.Equal(t, 1, stored.ProcessedWindows)
			} else {
				assert.Zero(t, stored.ProcessedWindows)
				assert.Equal(t, int64(100), stored.Cursor)
			}
		})
	}
}

func TestChannelCostReportResumeRequiresReleasedLeaseAndPreservesProgress(t *testing.T) {
	job, task := costReportJobFixture(t)
	_, claimed, err := ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, BeginChannelCostReportJob(context.Background(), job, "worker"))
	require.NoError(t, PublishChannelCostReportWindow(context.Background(), job, "worker", 3700, nil))
	require.NoError(t, ControlChannelCostReportJob(context.Background(), job.ID, "pause"))
	require.NoError(t, FinishChannelCostReportJob(context.Background(), job, "worker", "paused"))
	require.ErrorIs(t, ControlChannelCostReportJob(context.Background(), job.ID, "resume"), ErrChannelCostReportConflict)
	require.NoError(t, FinishSystemTask(job.SystemTaskID, "worker", SystemTaskStatusSucceeded, nil, ""))
	require.NoError(t, ControlChannelCostReportJob(context.Background(), job.ID, "resume"))
	resumed, err := GetChannelCostReportJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.NotEqual(t, job.SystemTaskID, resumed.SystemTaskID)
	assert.Equal(t, int64(3700), resumed.Cursor)
	assert.Equal(t, 1, resumed.ProcessedWindows)
	assert.Equal(t, "pending", resumed.Status)
	require.ErrorIs(t, PublishChannelCostReportWindow(context.Background(), job, "worker", 3700, nil), ErrChannelCostReportConflict)
}

func TestChannelCostReportConcurrentPublicationAddsOnce(t *testing.T) {
	job, task := costReportJobFixture(t)
	_, claimed, err := ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, BeginChannelCostReportJob(context.Background(), job, "worker"))
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- PublishChannelCostReportWindow(context.Background(), job, "worker", 3700, nil)
		}()
	}
	workers.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, ErrChannelCostReportConflict)
		}
	}
	assert.Equal(t, 1, success)
	stored, err := GetChannelCostReportJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, stored.ProcessedWindows)
}

func TestChannelCostReportExpiredLeaseReconcilesAndResumes(t *testing.T) {
	job, task := costReportJobFixture(t)
	_, claimed, err := ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, BeginChannelCostReportJob(context.Background(), job, "worker"))
	require.NoError(t, DB.Model(&SystemTaskLock{}).Where("task_id = ?", task.TaskID).Update("locked_until", common.GetTimestamp()-1).Error)
	require.NoError(t, ExpireStaleSystemTaskLocks(common.GetTimestamp()))
	stored, err := GetChannelCostReportJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", stored.Status)
	jobs, err := ListChannelCostReportJobs(context.Background(), 50)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Equal(t, "failed", jobs[0].Status)
	assert.Empty(t, jobs[0].RowsJSON)
	require.NoError(t, ControlChannelCostReportJob(context.Background(), job.ID, "resume"))
	resumed, err := GetChannelCostReportJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.NotEqual(t, job.SystemTaskID, resumed.SystemTaskID)
	require.ErrorIs(t, BeginChannelCostReportJob(context.Background(), job, "worker"), ErrChannelCostReportConflict)
}

func TestChannelCostReportLargeJSONColumnsSupportAllDialects(t *testing.T) {
	parsed, err := schema.Parse(&ChannelCostReportJob{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	for _, name := range []string{"PolicyJSON", "RowsJSON"} {
		field := parsed.LookUpField(name)
		require.NotNil(t, field)
		assert.Equal(t, "mediumtext", mysql.New(mysql.Config{}).DataTypeOf(field))
		assert.Equal(t, "varchar(10485760)", postgres.New(postgres.Config{}).DataTypeOf(field))
	}
}

func TestChannelCostReportSourceChangeBlocksPublicationAndResume(t *testing.T) {
	job, task := costReportJobFixture(t)
	_, claimed, err := ClaimSystemTask(task.ID, task.Type, "worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, BeginChannelCostReportJob(context.Background(), job, "worker"))
	require.NoError(t, PublishChannelCostReportWindow(context.Background(), job, "worker", 3700, nil))
	stored, err := GetChannelCostReportJob(context.Background(), job.ID)
	require.NoError(t, err)
	original := LOG_DB
	replacement, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "different-source.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := replacement.DB()
	require.NoError(t, err)
	t.Cleanup(func() { LOG_DB = original; _ = sqlDB.Close() })
	require.NoError(t, replacement.AutoMigrate(&Log{}))
	LOG_DB = replacement
	require.ErrorIs(t, PublishChannelCostReportWindow(context.Background(), stored, "worker", 7300, nil), ErrChannelCostReportSourceChanged)
	require.NoError(t, FinishChannelCostReportJob(context.Background(), stored, "worker", "paused"))
	require.NoError(t, FinishSystemTask(task.TaskID, "worker", SystemTaskStatusSucceeded, nil, ""))
	require.ErrorIs(t, ControlChannelCostReportJob(context.Background(), job.ID, "resume"), ErrChannelCostReportSourceChanged)
	unchanged, err := GetChannelCostReportJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, unchanged.ProcessedWindows)
	assert.Equal(t, int64(3700), unchanged.Cursor)
	assert.Equal(t, stored.SourceIdentity, unchanged.SourceIdentity)
}

func TestChannelCostReportSourceIdentitySeparatesMemoryDatabases(t *testing.T) {
	original := LOG_DB
	t.Cleanup(func() { LOG_DB = original })
	identities := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		pool, err := db.DB()
		require.NoError(t, err)
		pool.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = pool.Close() })
		require.NoError(t, db.AutoMigrate(&Log{}))
		LOG_DB = db
		identity, err := ChannelCostReportSourceIdentity(context.Background())
		require.NoError(t, err)
		again, err := ChannelCostReportSourceIdentity(context.Background())
		require.NoError(t, err)
		assert.Equal(t, identity, again)
		identities = append(identities, identity)
	}
	assert.NotEqual(t, identities[0], identities[1])
}
