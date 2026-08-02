package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenPeriodTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB, oldLogDB := DB, LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Token{}))
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled = oldRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func seedPeriodToken(t *testing.T, db *gorm.DB, periodType string, periodDays int, startAt int64, used int64) *Token {
	t.Helper()
	token := &Token{
		UserId:           1,
		Key:              fmt.Sprintf("%s-%s-%d", strings.ReplaceAll(t.Name(), "/", "-"), periodType, periodDays),
		Status:           common.TokenStatusEnabled,
		Name:             "period token",
		CreatedTime:      1,
		AccessedTime:     1,
		ExpiredTime:      -1,
		RemainQuota:      100,
		PeriodType:       periodType,
		PeriodDays:       periodDays,
		PeriodQuotaLimit: 1000,
		PeriodLimitUnit:  "quota",
		PeriodAnchorAt:   common.TokenPeriodAnchorNow(time.Now()),
		PeriodStartAt:    startAt,
		PeriodUsedQuota:  used,
	}
	require.NoError(t, db.Create(token).Error)
	return token
}

func currentDayPeriod(t *testing.T) (int64, int64) {
	t.Helper()
	now := time.Now()
	anchor := common.TokenPeriodAnchorNow(now)
	start, end, ok := common.TokenPeriodBounds(common.TokenPeriodTypeDays, 1, anchor, now)
	require.True(t, ok)
	return start, end
}

func TestTokenPeriodStateCalculatesEffectiveBucket(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	anchor := common.TokenPeriodAnchorNow(now)
	start, end, ok := common.TokenPeriodBounds(common.TokenPeriodTypeDays, 3, anchor, now)
	require.True(t, ok)
	state := TokenPeriodState{
		Type:      common.TokenPeriodTypeDays,
		Days:      3,
		Limit:     100,
		Unit:      "quota",
		AnchorAt:  anchor,
		StartAt:   start,
		UsedQuota: 40,
	}

	assert.Equal(t, start, state.CurrentStart(now))
	assert.Equal(t, end, state.ResetAt(now))
	assert.Equal(t, int64(40), state.EffectiveUsed(now))

	state.StartAt = start - 3*24*60*60
	assert.Zero(t, state.EffectiveUsed(now))

	invalid := TokenPeriodState{Type: "invalid", Limit: 1}
	assert.Zero(t, invalid.CurrentStart(now))
	assert.Zero(t, invalid.ResetAt(now))
	assert.Zero(t, invalid.EffectiveUsed(now))
}

func TestAdjustTokenQuotaUpdatesPeriodStateAtomically(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	start, _ := currentDayPeriod(t)
	token := seedPeriodToken(t, db, common.TokenPeriodTypeDays, 1, start, 20)

	require.NoError(t, AdjustTokenQuota(token.Id, token.Key, 30, 0))
	var updated Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, 70, updated.RemainQuota)
	assert.Equal(t, 30, updated.UsedQuota)
	assert.Equal(t, start, updated.PeriodStartAt)
	assert.Equal(t, int64(50), updated.PeriodUsedQuota)

	require.NoError(t, AdjustTokenQuota(token.Id, token.Key, -80, start))
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, 150, updated.RemainQuota)
	assert.Equal(t, -50, updated.UsedQuota)
	assert.Zero(t, updated.PeriodUsedQuota)
}

func TestAdjustTokenQuotaResetsStaleBucketAndPreservesCurrentBucketOnOldRefund(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	start, _ := currentDayPeriod(t)
	token := seedPeriodToken(t, db, common.TokenPeriodTypeDays, 1, start-24*60*60, 90)

	require.NoError(t, AdjustTokenQuota(token.Id, token.Key, 7, 0))
	var updated Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, start, updated.PeriodStartAt)
	assert.Equal(t, int64(7), updated.PeriodUsedQuota)

	require.NoError(t, AdjustTokenQuota(token.Id, token.Key, -5, start-24*60*60))
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, start, updated.PeriodStartAt)
	assert.Equal(t, int64(7), updated.PeriodUsedQuota)
	assert.Equal(t, 98, updated.RemainQuota)
	assert.Equal(t, 2, updated.UsedQuota)
}

func TestPeriodLimitedTokenBypassesBatchUpdate(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	start, _ := currentDayPeriod(t)
	limited := seedPeriodToken(t, db, common.TokenPeriodTypeDays, 1, start, 0)
	unlimited := seedPeriodToken(t, db, "", 0, 0, 0)
	unlimited.PeriodQuotaLimit = 0
	require.NoError(t, db.Model(unlimited).Update("period_quota_limit", 0).Error)

	originalBatchEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	batchUpdateStores[BatchUpdateTypeTokenQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	t.Cleanup(func() {
		common.BatchUpdateEnabled = originalBatchEnabled
		batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
		batchUpdateStores[BatchUpdateTypeTokenQuota] = make(map[int]int)
		batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	})

	require.NoError(t, DecreaseTokenQuota(limited.Id, limited.Key, 11))
	require.NoError(t, DecreaseTokenQuota(unlimited.Id, unlimited.Key, 11))

	var limitedAfter, unlimitedAfter Token
	require.NoError(t, db.First(&limitedAfter, limited.Id).Error)
	require.NoError(t, db.First(&unlimitedAfter, unlimited.Id).Error)
	assert.Equal(t, 89, limitedAfter.RemainQuota)
	assert.Equal(t, int64(11), limitedAfter.PeriodUsedQuota)
	assert.Equal(t, 100, unlimitedAfter.RemainQuota)

	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	queued := batchUpdateStores[BatchUpdateTypeTokenQuota][unlimited.Id]
	_, limitedQueued := batchUpdateStores[BatchUpdateTypeTokenQuota][limited.Id]
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	assert.Equal(t, -11, queued)
	assert.False(t, limitedQueued)
}

func TestTokenUpdateCannotOverwriteAtomicPeriodCounters(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	start, _ := currentDayPeriod(t)
	token := seedPeriodToken(t, db, common.TokenPeriodTypeDays, 1, start, 10)

	var stale Token
	require.NoError(t, db.First(&stale, token.Id).Error)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"period_start_at":   start + 24*60*60,
		"period_used_quota": 77,
	}).Error)

	stale.Name = "renamed"
	require.NoError(t, stale.Update())
	var updated Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, "renamed", updated.Name)
	assert.Equal(t, start+24*60*60, updated.PeriodStartAt)
	assert.Equal(t, int64(77), updated.PeriodUsedQuota)
}

func TestLoadTokenPeriodStateReadsDatabaseState(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	start, _ := currentDayPeriod(t)
	token := seedPeriodToken(t, db, common.TokenPeriodTypeDays, 1, start, 23)

	state, err := LoadTokenPeriodState(token.Id)
	require.NoError(t, err)
	assert.Equal(t, common.TokenPeriodTypeDays, state.Type)
	assert.Equal(t, 1, state.Days)
	assert.Equal(t, int64(1000), state.Limit)
	assert.Equal(t, "quota", state.Unit)
	assert.Equal(t, token.PeriodAnchorAt, state.AnchorAt)
	assert.Equal(t, start, state.StartAt)
	assert.Equal(t, int64(23), state.UsedQuota)
}

func TestTokenPeriodLimitEnabledUsesOnlyTypeAndPositiveLimit(t *testing.T) {
	tests := []struct {
		name  string
		token *Token
		want  bool
	}{
		{name: "nil", token: nil, want: false},
		{name: "empty type", token: &Token{PeriodQuotaLimit: 1}, want: false},
		{name: "zero limit", token: &Token{PeriodType: common.TokenPeriodTypeMonth}, want: false},
		{name: "enabled regardless of other fields", token: &Token{PeriodType: "future-type", PeriodQuotaLimit: 1}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.token.PeriodLimitEnabled())
		})
	}
}

func TestUpdatePeriodConfigPreserveStateCannotOverwriteFreshCounters(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	start, _ := currentDayPeriod(t)
	token := seedPeriodToken(t, db, common.TokenPeriodTypeDays, 1, start, 10)

	var stale Token
	require.NoError(t, db.First(&stale, token.Id).Error)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"period_start_at":   start,
		"period_used_quota": 77,
	}).Error)

	stale.PeriodQuotaLimit = 2000
	require.NoError(t, stale.UpdatePeriodConfigPreserveState())
	var updated Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, int64(2000), updated.PeriodQuotaLimit)
	assert.Equal(t, start, updated.PeriodStartAt)
	assert.Equal(t, int64(77), updated.PeriodUsedQuota)
}

func TestAdjustTokenQuotaConcurrentDeltasHaveExactSum(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	start, _ := currentDayPeriod(t)
	token := seedPeriodToken(t, db, common.TokenPeriodTypeDays, 1, start, 0)

	begin := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, delta := range []int{7, 11} {
		wg.Add(1)
		go func(delta int) {
			defer wg.Done()
			<-begin
			errs <- AdjustTokenQuota(token.Id, token.Key, delta, start)
		}(delta)
	}
	close(begin)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var updated Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, 82, updated.RemainQuota)
	assert.Equal(t, 18, updated.UsedQuota)
	assert.Equal(t, int64(18), updated.PeriodUsedQuota)
}

func TestTokenCacheProjectionKeepsConfigButNotCounters(t *testing.T) {
	token := Token{
		Key:              "cache-key",
		PeriodType:       common.TokenPeriodTypeDays,
		PeriodDays:       3,
		PeriodQuotaLimit: 100,
		PeriodLimitUnit:  "quota",
		PeriodAnchorAt:   10,
		PeriodStartAt:    20,
		PeriodUsedQuota:  30,
	}

	projection := tokenCacheProjection(token)
	assert.Equal(t, "cache-key", token.Key)
	assert.Equal(t, int64(20), token.PeriodStartAt)
	assert.Equal(t, int64(30), token.PeriodUsedQuota)
	assert.Empty(t, projection.Key)
	assert.Equal(t, common.TokenPeriodTypeDays, projection.PeriodType)
	assert.Equal(t, 3, projection.PeriodDays)
	assert.Equal(t, int64(100), projection.PeriodQuotaLimit)
	assert.Equal(t, "quota", projection.PeriodLimitUnit)
	assert.Equal(t, int64(10), projection.PeriodAnchorAt)
	assert.Equal(t, int64(-1), projection.PeriodStartAt)
	assert.Equal(t, int64(-1), projection.PeriodUsedQuota)
}

func TestLoadTokenPeriodStateTreatsLegacyNullsAsDisabled(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	token := seedPeriodToken(t, db, "", 0, 0, 0)
	require.NoError(t, db.Exec(`UPDATE tokens SET period_type = NULL, period_days = NULL,
		period_quota_limit = NULL, period_limit_unit = NULL, period_anchor_at = NULL,
		period_start_at = NULL, period_used_quota = NULL WHERE id = ?`, token.Id).Error)

	state, err := LoadTokenPeriodState(token.Id)
	require.NoError(t, err)
	assert.False(t, state.PeriodLimitEnabled())
	assert.Empty(t, state.Type)
	assert.Zero(t, state.Days)
	assert.Zero(t, state.Limit)
	assert.Zero(t, state.AnchorAt)
	assert.Zero(t, state.StartAt)
	assert.Zero(t, state.UsedQuota)
}

// 未启用周期限额的令牌必须走原有算术与批量语义，行为零变化。
func TestAdjustTokenQuotaLegacyPathLeavesPeriodColumnsUntouched(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	oldBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatch })

	token := seedPeriodToken(t, db, "", 0, 0, 0)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).
		Update("period_quota_limit", 0).Error)

	require.NoError(t, AdjustTokenQuota(token.Id, token.Key, 30, 0))
	require.NoError(t, AdjustTokenQuota(token.Id, token.Key, -10, 0))

	var got Token
	require.NoError(t, db.First(&got, token.Id).Error)
	assert.Equal(t, 80, got.RemainQuota, "100 - 30 + 10")
	assert.Equal(t, 20, got.UsedQuota, "30 - 10")
	assert.Zero(t, got.PeriodStartAt, "legacy path must not stamp a period bucket")
	assert.Zero(t, got.PeriodUsedQuota, "legacy path must not touch the period counter")
}

func TestAdjustTokenQuotaRejectsInvalidPeriodConfigAndMissingRows(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	start, _ := currentDayPeriod(t)

	// period_type=days 但 period_days=0：边界无法求解，必须报错而不是静默漏计。
	broken := seedPeriodToken(t, db, common.TokenPeriodTypeDays, 0, start, 0)
	err := AdjustTokenQuota(broken.Id, broken.Key, 10, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "令牌周期配置无效")

	var untouched Token
	require.NoError(t, db.First(&untouched, broken.Id).Error)
	assert.Equal(t, 100, untouched.RemainQuota, "a rejected adjustment must not move the balance")

	// 周期令牌被删除后，UPDATE 影响 0 行，必须冒泡而不是当作成功。
	live := seedPeriodToken(t, db, common.TokenPeriodTypeDays, 1, start, 0)
	require.NoError(t, db.Delete(&Token{}, live.Id).Error)
	assert.ErrorIs(t, AdjustTokenQuota(live.Id, live.Key, 10, 0), gorm.ErrRecordNotFound)

	_, stateErr := LoadTokenPeriodState(live.Id)
	assert.Error(t, stateErr, "a deleted token has no period state to trust")

	_, invalidIdErr := LoadTokenPeriodState(0)
	assert.Error(t, invalidIdErr, "the gate must fail closed on an invalid token id")

	// 跨周期退款同样不得把「影响 0 行」当成功，否则用户余额会静默丢失返还。
	assert.ErrorIs(t, adjustTokenQuotaAttributedPeriod(live.Id, -10), gorm.ErrRecordNotFound)
}
