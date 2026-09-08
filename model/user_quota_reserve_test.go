package model

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetUserQuotaWithSourceReportsCacheAndDatabase(t *testing.T) {
	_, client := setupUserCacheRedis(t)
	user := &User{Id: 990001, Username: "quota-source", Password: "hashed", Status: common.UserStatusEnabled, Quota: 100, Group: "default", AffCode: "quota-source-aff"}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&User{}, user.Id) })
	require.NoError(t, populateUserCache(*user))

	quota, source, err := GetUserQuotaWithSource(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 100, quota)
	assert.Equal(t, UserQuotaSourceCache, source)

	require.NoError(t, client.Del(context.Background(), getUserCacheKey(user.Id)).Err())
	oldEnabled := common.RedisEnabled
	common.RedisEnabled = false
	quota, source, err = GetUserQuotaWithSource(user.Id, false)
	common.RedisEnabled = oldEnabled
	require.NoError(t, err)
	assert.Equal(t, 100, quota)
	assert.Equal(t, UserQuotaSourceDB, source)

	quota, source, err = GetUserQuotaWithSource(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 100, quota)
	assert.Equal(t, UserQuotaSourceDB, source)
}

func TestReserveUserQuotaConditionsAndErrors(t *testing.T) {
	oldBatchUpdate := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatchUpdate })

	user := seedQuotaCacheUser(t, "reserve-user", 10)
	reserved, err := ReserveUserQuota(user.Id, 3)
	require.NoError(t, err)
	assert.True(t, reserved)
	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, 7, got.Quota)

	reserved, err = ReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	assert.False(t, reserved)
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, 7, got.Quota)

	reserved, err = ReserveUserQuota(user.Id, 0)
	assert.False(t, reserved)
	assert.Error(t, err)
	reserved, err = ReserveUserQuota(user.Id, -1)
	assert.False(t, reserved)
	assert.Error(t, err)

	reserved, err = ReserveUserQuota(999991, 1)
	assert.False(t, reserved)
	assert.Error(t, err)
}

func TestReserveUserQuotaDatabaseErrorIsNotInsufficientBalance(t *testing.T) {
	oldDB := DB
	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = brokenDB
	t.Cleanup(func() {
		DB = oldDB
		sqlDB, dbErr := brokenDB.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	reserved, err := ReserveUserQuota(1, 1)
	assert.False(t, reserved)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrUserQuotaInsufficient))
}

func TestReserveUserQuotaBatchUpdateDegradesWithoutNewReject(t *testing.T) {
	oldBatchUpdate := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchUpdate
		batchUpdate()
	})

	user := seedQuotaCacheUser(t, "reserve-batch", 1)
	reserved, err := ReserveUserQuota(user.Id, 100)
	require.NoError(t, err)
	assert.True(t, reserved)
}

func TestDecreaseUserQuotaStillAllowsSettlementBelowZero(t *testing.T) {
	oldBatchUpdate := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatchUpdate })

	user := seedQuotaCacheUser(t, "settlement-negative", 1)
	require.NoError(t, DecreaseUserQuota(user.Id, 3, true))
	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, -2, got.Quota)
}
