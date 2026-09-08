package common

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type redisCacheTestObject struct {
	Id        int
	Name      string
	Enabled   bool
	DeletedAt gorm.DeletedAt
}

func setupRedisCacheTest(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldClient, oldEnabled := RDB, RedisEnabled
	RDB = client
	RedisEnabled = true
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		server.Close()
		RDB = oldClient
		RedisEnabled = oldEnabled
	})
	return server
}

func TestRedisHashMutationReturnsKeyMissAndPreservesTTL(t *testing.T) {
	server := setupRedisCacheTest(t)
	ctx := context.Background()

	err := RedisHIncrBy("missing", "Quota", 1)
	assert.True(t, errors.Is(err, ErrRedisKeyMiss))
	err = RedisHSetField("missing", "Quota", 1)
	assert.True(t, errors.Is(err, ErrRedisKeyMiss))

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	require.NoError(t, client.HSet(ctx, "persistent", "Quota", 1).Err())
	assert.True(t, errors.Is(RedisHIncrBy("persistent", "Quota", 1), ErrRedisKeyMiss))
	assert.True(t, errors.Is(RedisHSetField("persistent", "Quota", 2), ErrRedisKeyMiss))

	require.NoError(t, client.HSet(ctx, " expiring ", "Quota", 1).Err())
	require.NoError(t, client.Expire(ctx, " expiring ", 90*time.Second).Err())
	before, err := client.TTL(ctx, " expiring ").Result()
	require.NoError(t, err)
	require.NoError(t, RedisHIncrBy(" expiring ", "Quota", 2))
	after, err := client.TTL(ctx, " expiring ").Result()
	require.NoError(t, err)
	assert.Equal(t, before, after)
	require.NoError(t, RedisHSetField(" expiring ", "Quota", 4))
	after, err = client.TTL(ctx, " expiring ").Result()
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestRedisHSetObjIfAbsent(t *testing.T) {
	server := setupRedisCacheTest(t)
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	object := &redisCacheTestObject{Id: 7, Name: "new", Enabled: true}

	wrote, err := RedisHSetObjIfAbsent("new", object, 75*time.Second)
	require.NoError(t, err)
	assert.True(t, wrote)
	values, err := client.HGetAll(ctx, "new").Result()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Id": "7", "Name": "new", "Enabled": "true"}, values)
	ttl, err := client.TTL(ctx, "new").Result()
	require.NoError(t, err)
	assert.Equal(t, 75*time.Second, ttl)

	require.NoError(t, client.HSet(ctx, "existing", "Original", "yes").Err())
	require.NoError(t, client.Expire(ctx, "existing", 41*time.Second).Err())
	before, err := client.TTL(ctx, "existing").Result()
	require.NoError(t, err)
	wrote, err = RedisHSetObjIfAbsent("existing", object, 75*time.Second)
	require.NoError(t, err)
	assert.False(t, wrote)
	values, err = client.HGetAll(ctx, "existing").Result()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Original": "yes"}, values)
	after, err := client.TTL(ctx, "existing").Result()
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestRedisHSetFieldIfAbsent(t *testing.T) {
	server := setupRedisCacheTest(t)
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	wrote, err := RedisHSetFieldIfAbsent("missing", "Quota", 12)
	assert.False(t, wrote)
	assert.True(t, errors.Is(err, ErrRedisKeyMiss))
	require.NoError(t, client.HSet(ctx, "user", "Quota", 1).Err())
	require.NoError(t, client.Expire(ctx, "user", time.Minute).Err())
	wrote, err = RedisHSetFieldIfAbsent("user", "Quota", 12)
	require.NoError(t, err)
	assert.False(t, wrote)
	value, err := client.HGet(ctx, "user", "Quota").Result()
	require.NoError(t, err)
	assert.Equal(t, "1", value)
	wrote, err = RedisHSetFieldIfAbsent("user", "Group", "default")
	require.NoError(t, err)
	assert.True(t, wrote)
	value, err = client.HGet(ctx, "user", "Group").Result()
	require.NoError(t, err)
	assert.Equal(t, "default", value)
}

// The partial-hash shape produced by the historical HINCRBY race must be
// reported as "present but incomplete", not as a miss, so callers can tell a
// missing field apart from a legitimate zero value.
func TestRedisHGetObjWithFieldsReportsPresentFields(t *testing.T) {
	server := setupRedisCacheTest(t)
	server.HSet("user:1", "Quota", "42")

	type userBase struct {
		Id     int
		Quota  int
		Status int
	}
	var partial userBase
	fields, err := RedisHGetObjWithFields("user:1", &partial)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Quota": "42"}, fields)
	assert.Equal(t, 42, partial.Quota)
	assert.Equal(t, 0, partial.Status, "a missing field stays at the Go zero value")

	var full userBase
	require.NoError(t, RedisHGetObj("user:1", &full))
	assert.Equal(t, 42, full.Quota)

	_, err = RedisHGetObjWithFields("user:missing", &full)
	assert.Error(t, err)
}
