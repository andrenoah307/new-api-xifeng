package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const defaultPressureCoolingErrorWindowSeconds = 60

type pressureCoolingErrorBucket struct {
	attempts atomic.Int64
	errors   atomic.Int64
	expireAt int64
}

var pressureCoolingErrorMemStore sync.Map

func normalizePressureCoolingErrorWindow(windowSeconds int) int {
	if windowSeconds <= 0 {
		return defaultPressureCoolingErrorWindowSeconds
	}
	return windowSeconds
}

func pressureCoolingErrorWindowKey(channelId, windowSeconds int, now int64) string {
	windowSeconds = normalizePressureCoolingErrorWindow(windowSeconds)
	return fmt.Sprintf("pc:ew:%d:%d", channelId, now/int64(windowSeconds))
}

func incrPressureCoolingErrorWindow(channelId, windowSeconds int, isError bool) (attempts, errors int64) {
	return incrPressureCoolingErrorWindowAt(channelId, windowSeconds, isError, time.Now().Unix())
}

func incrPressureCoolingErrorWindowAt(channelId, windowSeconds int, isError bool, now int64) (attempts, errors int64) {
	windowSeconds = normalizePressureCoolingErrorWindow(windowSeconds)
	if common.RedisEnabled && common.RDB != nil {
		key := pressureCoolingErrorWindowKey(channelId, windowSeconds, now)
		ctx := context.Background()
		pipe := common.RDB.Pipeline()
		attemptCmd := pipe.HIncrBy(ctx, key, "a", 1)
		errorIncrement := int64(0)
		if isError {
			errorIncrement = 1
		}
		errorCmd := pipe.HIncrBy(ctx, key, "e", errorIncrement)
		pipe.Expire(ctx, key, time.Duration(2*windowSeconds)*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			return 0, 0
		}
		attempts = attemptCmd.Val()
		errors = errorCmd.Val()
		return attempts, errors
	}

	key := pressureCoolingErrorWindowKey(channelId, windowSeconds, now)
	expireAt := now + int64(2*windowSeconds)
	for {
		value, loaded := pressureCoolingErrorMemStore.Load(key)
		if loaded {
			bucket := value.(*pressureCoolingErrorBucket)
			if bucket.expireAt <= now {
				pressureCoolingErrorMemStore.Delete(key)
				continue
			}
			attempts = bucket.attempts.Add(1)
			if isError {
				bucket.errors.Add(1)
			}
			errors = bucket.errors.Load()
			return attempts, errors
		}
		bucket := &pressureCoolingErrorBucket{expireAt: expireAt}
		actual, _ := pressureCoolingErrorMemStore.LoadOrStore(key, bucket)
		if actual != bucket {
			continue
		}
		attempts = bucket.attempts.Add(1)
		if isError {
			bucket.errors.Add(1)
		}
		errors = bucket.errors.Load()
		return attempts, errors
	}
}

func loadPressureCoolingErrorWindow(channelId, windowSeconds int) (attempts, errors int64) {
	return loadPressureCoolingErrorWindowAt(channelId, windowSeconds, time.Now().Unix())
}

func loadPressureCoolingErrorWindowAt(channelId, windowSeconds int, now int64) (attempts, errors int64) {
	windowSeconds = normalizePressureCoolingErrorWindow(windowSeconds)
	if common.RedisEnabled && common.RDB != nil {
		key := pressureCoolingErrorWindowKey(channelId, windowSeconds, now)
		vals, err := common.RDB.HGetAll(context.Background(), key).Result()
		if err != nil || len(vals) == 0 {
			return 0, 0
		}
		attempts, _ = strconv.ParseInt(vals["a"], 10, 64)
		errors, _ = strconv.ParseInt(vals["e"], 10, 64)
		return attempts, errors
	}

	key := pressureCoolingErrorWindowKey(channelId, windowSeconds, now)
	value, ok := pressureCoolingErrorMemStore.Load(key)
	if !ok {
		return 0, 0
	}
	bucket := value.(*pressureCoolingErrorBucket)
	if bucket.expireAt <= now {
		pressureCoolingErrorMemStore.Delete(key)
		return 0, 0
	}
	return bucket.attempts.Load(), bucket.errors.Load()
}

func cleanupPressureCoolingErrorWindows(now int64) {
	pressureCoolingErrorMemStore.Range(func(key, value interface{}) bool {
		if value.(*pressureCoolingErrorBucket).expireAt <= now {
			pressureCoolingErrorMemStore.Delete(key)
		}
		return true
	})
}
