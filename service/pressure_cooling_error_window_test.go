package service

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetPressureCoolingErrorWindowMemory() {
	pressureCoolingErrorMemStore.Range(func(key, _ interface{}) bool {
		pressureCoolingErrorMemStore.Delete(key)
		return true
	})
}

func TestPressureCoolingErrorWindowBucketsAndCounters(t *testing.T) {
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		resetPressureCoolingErrorWindowMemory()
	})
	resetPressureCoolingErrorWindowMemory()

	attempts, errors := incrPressureCoolingErrorWindowAt(801, 60, true, 120)
	assert.Equal(t, int64(1), attempts)
	assert.Equal(t, int64(1), errors)
	attempts, errors = incrPressureCoolingErrorWindowAt(801, 60, false, 121)
	assert.Equal(t, int64(2), attempts)
	assert.Equal(t, int64(1), errors)
	attempts, errors = loadPressureCoolingErrorWindowAt(801, 60, 121)
	assert.Equal(t, int64(2), attempts)
	assert.Equal(t, int64(1), errors)

	attempts, errors = incrPressureCoolingErrorWindowAt(801, 60, true, 180)
	assert.Equal(t, int64(1), attempts, "a new time bucket starts from zero")
	assert.Equal(t, int64(1), errors)
}

func TestPressureCoolingErrorWindowNonErrorOnlyIncrementsAttempts(t *testing.T) {
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		resetPressureCoolingErrorWindowMemory()
	})
	resetPressureCoolingErrorWindowMemory()

	attempts, errors := incrPressureCoolingErrorWindowAt(802, 60, false, 240)
	require.Equal(t, int64(1), attempts)
	assert.Equal(t, int64(0), errors)
	attempts, errors = incrPressureCoolingErrorWindowAt(802, 60, false, 241)
	assert.Equal(t, int64(2), attempts)
	assert.Equal(t, int64(0), errors)
}

func TestPressureCoolingErrorWindowMemoryConcurrentIncrements(t *testing.T) {
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		resetPressureCoolingErrorWindowMemory()
	})
	resetPressureCoolingErrorWindowMemory()

	const workers = 16
	const perWorker = 125
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				incrPressureCoolingErrorWindowAt(803, 60, true, 300)
			}
		}()
	}
	wg.Wait()

	attempts, errors := loadPressureCoolingErrorWindowAt(803, 60, 300)
	assert.Equal(t, int64(workers*perWorker), attempts)
	assert.Equal(t, int64(workers*perWorker), errors)
}

func TestPressureCoolingErrorWindowNonPositiveWindowIsSafe(t *testing.T) {
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		resetPressureCoolingErrorWindowMemory()
	})
	resetPressureCoolingErrorWindowMemory()

	assert.NotPanics(t, func() {
		attempts, errors := incrPressureCoolingErrorWindowAt(804, 0, true, 400)
		assert.Equal(t, int64(1), attempts)
		assert.Equal(t, int64(1), errors)
		attempts, errors = loadPressureCoolingErrorWindowAt(804, -1, 400)
		assert.Equal(t, int64(1), attempts)
		assert.Equal(t, int64(1), errors)
	})
}

func TestPressureCoolingErrorWindowRedisAtomicCountersAndTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedis, oldClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedis, oldClient
	})

	attempts, errors := incrPressureCoolingErrorWindowAt(805, 30, true, 600)
	assert.Equal(t, int64(1), attempts)
	assert.Equal(t, int64(1), errors)
	attempts, errors = loadPressureCoolingErrorWindowAt(805, 30, 600)
	assert.Equal(t, int64(1), attempts)
	assert.Equal(t, int64(1), errors)
	attempts, errors = incrPressureCoolingErrorWindowAt(805, 30, false, 601)
	assert.Equal(t, int64(2), attempts)
	assert.Equal(t, int64(1), errors)
	assert.Greater(t, server.TTL(pressureCoolingErrorWindowKey(805, 30, 600)), time.Duration(0))

	server.FastForward(61 * time.Second)
	attempts, errors = loadPressureCoolingErrorWindowAt(805, 30, 661)
	assert.Equal(t, int64(0), attempts)
	assert.Equal(t, int64(0), errors)
}

func TestPressureCoolingErrorWindowConvenienceWrappersAndMemoryExpiry(t *testing.T) {
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		resetPressureCoolingErrorWindowMemory()
	})
	resetPressureCoolingErrorWindowMemory()

	attempts, errors := incrPressureCoolingErrorWindow(806, 60, true)
	assert.Equal(t, int64(1), attempts)
	assert.Equal(t, int64(1), errors)
	attempts, errors = loadPressureCoolingErrorWindow(806, 60)
	assert.Equal(t, int64(1), attempts)
	assert.Equal(t, int64(1), errors)

	incrPressureCoolingErrorWindowAt(807, 30, true, 900)
	cleanupPressureCoolingErrorWindows(961)
	attempts, errors = loadPressureCoolingErrorWindowAt(807, 30, 900)
	assert.Equal(t, int64(0), attempts)
	assert.Equal(t, int64(0), errors)
}

func TestPressureCoolingErrorWindowRedisFailureDoesNotTrigger(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedis, oldClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedis, oldClient
	})
	server.Close()

	attempts, errors := incrPressureCoolingErrorWindowAt(808, 30, true, 600)
	assert.Equal(t, int64(0), attempts)
	assert.Equal(t, int64(0), errors)
	attempts, errors = loadPressureCoolingErrorWindowAt(808, 30, 600)
	assert.Equal(t, int64(0), attempts)
	assert.Equal(t, int64(0), errors)
}
