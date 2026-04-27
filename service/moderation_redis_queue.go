package service

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

// moderationRedisQueue persists pending moderation events in Redis using
// the simple LIST primitive (DEV_GUIDE §14 — we deliberately stay below
// asynq's complexity until throughput justifies it). Layout:
//
//	rc:mod:queue                      LIST of event_json strings
//	rc:mod:processing:<worker_id>     LIST holding events a worker
//	                                  has popped but not yet finished.
//
// Workflow:
//   - enqueue(): LPUSH to rc:mod:queue with LTRIM cap to maintain bounded
//     storage. When storage is at the cap LTRIM drops the OLDEST event,
//     mirroring the in-memory ring buffer semantics so the two stores
//     agree on overflow behaviour.
//   - takeForWorker(): RPOPLPUSH rc:mod:queue → rc:mod:processing:<wid>.
//     Returns the JSON the caller must hand back via complete() once done.
//   - complete(): LREM the matching JSON from rc:mod:processing:<wid>.
//   - recoverProcessing(): on startup move every left-over processing
//     entry back to the main queue so a previously-crashed worker's
//     in-flight events get retried.
//
// All Redis operations use short timeouts; failure causes the engine to
// fall back to the in-memory channel for that call. Persistence is
// best-effort, never blocking the relay path.
type moderationRedisQueue struct {
	rdb     *redis.Client
	maxSize int
}

func newModerationRedisQueue(maxSize int) *moderationRedisQueue {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}
	if maxSize <= 0 {
		maxSize = 4096
	}
	return &moderationRedisQueue{
		rdb:     common.RDB,
		maxSize: maxSize,
	}
}

func (q *moderationRedisQueue) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 200*time.Millisecond)
}

const (
	moderationRedisMainQueueKey       = "rc:mod:queue"
	moderationRedisProcessingKeyPrefix = "rc:mod:processing:"
)

// enqueue appends a JSON-encoded event to the main queue with a bound on
// the queue length. When the queue is full LTRIM drops the oldest entry.
func (q *moderationRedisQueue) enqueue(eventJSON string) error {
	if q == nil || eventJSON == "" {
		return nil
	}
	ctx, cancel := q.ctx()
	defer cancel()
	pipe := q.rdb.TxPipeline()
	pipe.LPush(ctx, moderationRedisMainQueueKey, eventJSON)
	// Keep only the newest maxSize entries — LTRIM 0 (maxSize-1) preserves
	// indices [0, maxSize-1] on a list where index 0 is the most recent.
	pipe.LTrim(ctx, moderationRedisMainQueueKey, 0, int64(q.maxSize-1))
	_, err := pipe.Exec(ctx)
	return err
}

// takeForWorker pops the oldest queued event and reserves it under the
// worker's processing list atomically. Returns "" + nil when the queue is
// empty. Returns an error only on Redis failure; callers should fall back
// to the in-memory queue on error.
func (q *moderationRedisQueue) takeForWorker(workerID int) (string, error) {
	if q == nil {
		return "", nil
	}
	ctx, cancel := q.ctx()
	defer cancel()
	res, err := q.rdb.RPopLPush(ctx, moderationRedisMainQueueKey, q.processingKey(workerID)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return res, nil
}

// complete removes the JSON entry from the worker's processing list.
// LREM count=1 ensures we only delete one occurrence even if the same
// payload happened to appear twice (which is unlikely but harmless).
func (q *moderationRedisQueue) complete(workerID int, eventJSON string) error {
	if q == nil || eventJSON == "" {
		return nil
	}
	ctx, cancel := q.ctx()
	defer cancel()
	return q.rdb.LRem(ctx, q.processingKey(workerID), 1, eventJSON).Err()
}

// recoverProcessing moves any events left in worker processing lists back
// onto the main queue. Called once at startup so events in flight when
// the previous instance crashed get retried.
func (q *moderationRedisQueue) recoverProcessing() {
	if q == nil {
		return
	}
	ctx, cancel := q.ctx()
	defer cancel()
	keys, err := q.rdb.Keys(ctx, moderationRedisProcessingKeyPrefix+"*").Result()
	if err != nil {
		common.SysError("moderation redis recover keys failed: " + err.Error())
		return
	}
	for _, key := range keys {
		for {
			ctx2, cancel2 := q.ctx()
			val, popErr := q.rdb.RPopLPush(ctx2, key, moderationRedisMainQueueKey).Result()
			cancel2()
			if popErr == redis.Nil || val == "" {
				break
			}
			if popErr != nil {
				common.SysError("moderation redis recover pop failed: " + popErr.Error())
				break
			}
		}
	}
}

func (q *moderationRedisQueue) processingKey(workerID int) string {
	return moderationRedisProcessingKeyPrefix + strconv.Itoa(workerID)
}

// depth returns the live queue length for the UI status card. Returns 0
// when Redis is unreachable so the card stays informative even if the
// persistence layer is down.
func (q *moderationRedisQueue) depth() int64 {
	if q == nil {
		return 0
	}
	ctx, cancel := q.ctx()
	defer cancel()
	n, err := q.rdb.LLen(ctx, moderationRedisMainQueueKey).Result()
	if err != nil {
		return 0
	}
	return n
}
