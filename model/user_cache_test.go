package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupUserCacheRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldClient, oldEnabled := common.RDB, common.RedisEnabled
	oldFrequency := common.SyncFrequency
	common.RDB = client
	common.RedisEnabled = true
	common.SyncFrequency = 60
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		server.Close()
		common.RDB = oldClient
		common.RedisEnabled = oldEnabled
		common.SyncFrequency = oldFrequency
	})
	return server, client
}

func fullUserBase(id, quota int) *UserBase {
	return &UserBase{
		Id:       id,
		Status:   common.UserStatusEnabled,
		Quota:    quota,
		Group:    "default",
		Username: "cache-user",
		Email:    "cache@example.com",
		Setting:  "",
	}
}

func TestPopulateUserCacheIsMissOnlyAndRebuildsAfterExpiry(t *testing.T) {
	server, client := setupUserCacheRedis(t)
	key := getUserCacheKey(900001)
	oldUser := User{Id: 900001, Status: common.UserStatusEnabled, Quota: 100, Group: "default", Username: "cache-user", Email: "cache@example.com"}
	newUser := oldUser
	newUser.Quota = 40

	require.NoError(t, populateUserCache(oldUser))
	quota, err := client.HGet(context.Background(), key, "Quota").Result()
	require.NoError(t, err)
	assert.Equal(t, "100", quota)
	require.NoError(t, populateUserCache(newUser))
	quota, err = client.HGet(context.Background(), key, "Quota").Result()
	require.NoError(t, err)
	assert.Equal(t, "100", quota)

	server.FastForward(61 * time.Second)
	exists, err := client.Exists(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
	require.NoError(t, populateUserCache(newUser))
	quota, err = client.HGet(context.Background(), key, "Quota").Result()
	require.NoError(t, err)
	assert.Equal(t, "40", quota)
}

func TestUpdateUserQuotaCacheDoesNotCreateOrOverwrite(t *testing.T) {
	_, client := setupUserCacheRedis(t)
	ctx := context.Background()

	require.NoError(t, updateUserQuotaCache(900002, 20))
	count, err := client.Exists(ctx, getUserCacheKey(900002)).Result()
	require.NoError(t, err)
	assert.Zero(t, count)

	user := fullUserBase(900003, 100)
	require.NoError(t, common.RedisHSetObj(getUserCacheKey(user.Id), user, time.Minute))
	require.NoError(t, updateUserQuotaCache(user.Id, 20))
	quota, err := client.HGet(ctx, getUserCacheKey(user.Id), "Quota").Result()
	require.NoError(t, err)
	assert.Equal(t, "100", quota)
}

func TestCacheGetUserBaseRequiresAllFieldsButAcceptsZeroValues(t *testing.T) {
	_, client := setupUserCacheRedis(t)
	ctx := context.Background()
	complete := fullUserBase(900004, 0)
	key := getUserCacheKey(complete.Id)
	require.NoError(t, common.RedisHSetObj(key, complete, time.Minute))
	got, err := cacheGetUserBase(complete.Id)
	require.NoError(t, err)
	assert.Equal(t, complete, got)

	baseFields := map[string]string{
		"Id": "900005", "Status": "1", "Quota": "0", "Group": "default",
		"Username": "cache-user", "Email": "cache@example.com", "Setting": "",
	}
	missingFields := []string{"Status", "Group", "Quota", "Id", "Username", "Email", "Setting"}
	for index, missing := range missingFields {
		id := 900010 + index
		key := getUserCacheKey(id)
		values := make(map[string]interface{}, len(baseFields))
		for field, value := range baseFields {
			if field != missing {
				values[field] = value
			}
		}
		require.NoError(t, client.HSet(ctx, key, values).Err())
		require.NoError(t, client.Expire(ctx, key, time.Minute).Err())
		_, err := cacheGetUserBase(id)
		assert.Error(t, err)
		exists, existsErr := client.Exists(ctx, key).Result()
		require.NoError(t, existsErr)
		assert.Zero(t, exists)
	}

	// A hash containing only the field created by a TOCTOU HINCRBY is also rejected.
	key = getUserCacheKey(900020)
	require.NoError(t, client.HSet(ctx, key, "Quota", "12").Err())
	require.NoError(t, client.Expire(ctx, key, time.Minute).Err())
	_, err = cacheGetUserBase(900020)
	assert.Error(t, err)
	exists, err := client.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
}

func TestGetUserCacheFallsBackToDatabaseAfterMalformedHash(t *testing.T) {
	_, client := setupUserCacheRedis(t)
	const userID = 900021
	user := &User{Id: userID, Username: "cache-fallback-user", Password: "hashed", Status: common.UserStatusEnabled, Quota: 77, Group: "default", Email: "fallback@example.com"}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&User{}, userID) })
	key := getUserCacheKey(userID)
	require.NoError(t, client.HSet(context.Background(), key, "Quota", "1").Err())
	require.NoError(t, client.Expire(context.Background(), key, time.Minute).Err())

	got, err := GetUserCache(userID)
	require.NoError(t, err)
	assert.Equal(t, user.Quota, got.Quota)
	assert.Equal(t, user.Email, got.Email)
}

func TestUpdateUserCacheFieldSkipsMissWithoutCreatingHash(t *testing.T) {
	server, _ := setupUserCacheRedis(t)

	// No cache entry: the update is a no-op, and must not resurrect a hash that
	// would then be served as a complete user.
	require.NoError(t, updateUserSettingCache(940001, `{"lang":"zh"}`))
	assert.False(t, server.Exists(getUserCacheKey(940001)))

	require.NoError(t, populateUserCache(User{Id: 940001, Username: "field-update", Status: common.UserStatusEnabled, Quota: 5, Group: "default"}))
	require.NoError(t, updateUserSettingCache(940001, `{"lang":"en"}`))
	cached, err := cacheGetUserBase(940001)
	require.NoError(t, err)
	assert.Equal(t, `{"lang":"en"}`, cached.Setting)
}
