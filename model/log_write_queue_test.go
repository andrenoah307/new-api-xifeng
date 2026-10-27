package model

import (
	"context"
	"os"
	"testing"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLogWriteQueueTest(t *testing.T) (*logWriteQueue, *miniredis.Miniredis) {
	t.Helper()
	s := miniredis.RunT(t)
	r := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { assert.NoError(t, r.Close()) })
	q := &logWriteQueue{redis: r, prefix: "logs:{test}:", target: "test", maxRows: 10, maxBytes: 100000, batchRows: 2}
	return q, s
}

func TestLogQueueCapacityPreservesAcceptedRecords(t *testing.T) {
	q, _ := setupLogWriteQueueTest(t)
	q.maxRows = 1
	ctx := context.Background()
	a := &Log{EventID: "a", CreatedAt: 1700000000, Content: "original"}
	require.NoError(t, q.enqueue(ctx, a))
	require.NoError(t, q.enqueue(ctx, a))
	assert.ErrorIs(t, q.enqueue(ctx, &Log{EventID: "b"}), errLogQueueFull)
	a.Content = "changed"
	assert.Error(t, q.enqueue(ctx, a))
	batch, err := q.claim(ctx, "owner")
	require.NoError(t, err)
	require.Len(t, batch.rows, 1)
	assert.Equal(t, "original", batch.rows[0].Content)
	assert.ErrorIs(t, q.enqueue(ctx, &Log{EventID: "c"}), errLogQueueFull, "in-flight rows count toward capacity")
	require.NoError(t, q.ack(ctx, "owner", batch.token))
	q.maxBytes = 1
	assert.ErrorIs(t, q.enqueue(ctx, &Log{EventID: "d"}), errLogQueueFull)
	assert.Equal(t, "0", q.redis.HGet(ctx, q.prefix+"state", "rows").Val())
}

func TestLogQueueExpiresEachEventWithoutRenewingOlderLogs(t *testing.T) {
	q, s := setupLogWriteQueueTest(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.SetTime(start)
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "old"}))
	assert.Equal(t, 24*time.Hour, q.redis.TTL(ctx, q.prefix+"payload").Val())
	s.SetTime(start.Add(12 * time.Hour))
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "new"}))
	s.SetTime(start.Add(24 * time.Hour))
	n, err := q.expire(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.False(t, q.redis.HExists(ctx, q.prefix+"payload", "old").Val())
	assert.True(t, q.redis.HExists(ctx, q.prefix+"payload", "new").Val())
	assert.Equal(t, "1", q.redis.HGet(ctx, q.prefix+"state", "rows").Val())
	assert.Equal(t, "1", q.redis.HGet(ctx, q.prefix+"state", "expired").Val())
	batch, err := q.claim(ctx, "owner")
	require.NoError(t, err)
	require.Len(t, batch.rows, 1)
	assert.Equal(t, "new", batch.rows[0].EventID)
	assert.Equal(t, 12*time.Hour, q.redis.TTL(ctx, q.prefix+"inflight").Val())
}

func TestLogQueueExpiryPreservesActiveLeaseAndChangesBatchToken(t *testing.T) {
	q, s := setupLogWriteQueueTest(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.SetTime(start)
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "old"}))
	s.SetTime(start.Add(12 * time.Hour))
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "new"}))
	batch, err := q.claim(ctx, "old-owner")
	require.NoError(t, err)
	require.Len(t, batch.rows, 2)
	s.SetTime(start.Add(24 * time.Hour))
	n, err := q.expire(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "active batches must not be changed underneath their owner")
	s.FastForward(time.Minute)
	n, err = q.expire(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	recovered, err := q.claim(ctx, "new-owner")
	require.NoError(t, err)
	require.Len(t, recovered.rows, 1)
	assert.NotEqual(t, batch.token, recovered.token, "a smaller batch must not reuse an insert dedup token")
	assert.Error(t, q.ack(ctx, "old-owner", batch.token))
	require.NoError(t, q.ack(ctx, "new-owner", recovered.token))
	assert.Equal(t, "0", q.redis.HGet(ctx, q.prefix+"state", "bytes").Val())
}

func TestLogQueueExpiresWithoutRunningWorker(t *testing.T) {
	q, s := setupLogWriteQueueTest(t)
	ctx := context.Background()
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "a"}))
	_, err := q.claim(ctx, "owner")
	require.NoError(t, err)
	s.FastForward(24 * time.Hour)
	keys, err := q.redis.Keys(ctx, q.prefix+"*").Result()
	require.NoError(t, err)
	assert.Empty(t, keys)
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "b"}))
	assert.Equal(t, "1", q.redis.HGet(ctx, q.prefix+"state", "rows").Val())
}

func TestLogQueueLegacyBacklogGetsOneDayGraceEvenWithActiveLease(t *testing.T) {
	q, s := setupLogWriteQueueTest(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.SetTime(start)
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "legacy"}))
	_, err := q.claim(ctx, "old-worker")
	require.NoError(t, err)
	// Reproduce the previous release's keys, which had no expiry metadata/TTL.
	require.NoError(t, q.redis.Del(ctx, q.prefix+"expires").Err())
	require.NoError(t, q.redis.HDel(ctx, q.prefix+"state", "expires_at", "legacy_expires_at").Err())
	for _, suffix := range []string{"payload", "inflight", "state"} {
		require.NoError(t, q.redis.Persist(ctx, q.prefix+suffix).Err())
	}
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "new"}))
	deadline, err := q.redis.HGet(ctx, q.prefix+"state", "legacy_expires_at").Int64()
	require.NoError(t, err)
	assert.Equal(t, start.Add(24*time.Hour).UnixMilli(), deadline)
	s.FastForward(time.Minute)
	s.SetTime(start.Add(time.Hour))
	n, err := q.expire(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, 23*time.Hour, q.redis.TTL(ctx, q.prefix+"payload").Val())
}

func TestLogQueueRecoveryAndStaleAcknowledgement(t *testing.T) {
	q, s := setupLogWriteQueueTest(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, q.enqueue(ctx, &Log{EventID: id, CreatedAt: 1700000000, Quota: 42}))
	}
	first, err := q.claim(ctx, "old")
	require.NoError(t, err)
	require.Len(t, first.rows, 2)
	busy, err := q.claim(ctx, "other")
	require.NoError(t, err)
	assert.Nil(t, busy)
	s.FastForward(2 * time.Minute)
	restarted := *q
	recovered, err := restarted.claim(ctx, "new")
	require.NoError(t, err)
	assert.Equal(t, first, recovered, "crash recovery must preserve exact token, payload and order")
	assert.Error(t, q.ack(ctx, "old", first.token))
	require.NoError(t, restarted.ack(ctx, "new", recovered.token))
	last, err := restarted.claim(ctx, "last")
	require.NoError(t, err)
	require.Len(t, last.rows, 1)
	assert.Equal(t, "c", last.rows[0].EventID)
	assert.Error(t, q.ack(ctx, "last", first.token))
	require.NoError(t, restarted.ack(ctx, "last", last.token))
	assert.Equal(t, "0", q.redis.HGet(ctx, q.prefix+"state", "bytes").Val())
}

func TestLogQueueRejectsInvalidPayloadWithoutRemovingIt(t *testing.T) {
	q, _ := setupLogWriteQueueTest(t)
	ctx := context.Background()
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "a"}))
	require.NoError(t, q.redis.HSet(ctx, q.prefix+"payload", "a", "invalid").Err())
	_, err := q.claim(ctx, "owner")
	require.Error(t, err)
	assert.True(t, q.redis.HExists(ctx, q.prefix+"payload", "a").Val())
}

func TestLogQueueDoesNotFallBackToDatabaseOnRedisFailure(t *testing.T) {
	q, s := setupLogWriteQueueTest(t)
	s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	assert.Error(t, q.enqueue(ctx, &Log{EventID: "a"}))
}

func TestLogQueueEnvelopePreservesPrivateEventIdentity(t *testing.T) {
	q, _ := setupLogWriteQueueTest(t)
	row := &Log{EventID: "private-id", Content: "content", Other: `{"admin_info":{"x":1}}`, Quota: 2147483647}
	require.NoError(t, q.enqueue(context.Background(), row))
	batch, err := q.claim(context.Background(), "owner")
	require.NoError(t, err)
	require.Len(t, batch.rows, 1)
	assert.Equal(t, *row, batch.rows[0].Log)
	public, err := common.Marshal(row)
	require.NoError(t, err)
	assert.NotContains(t, string(public), "private-id")
}

func TestLogQueueClickHouseAcknowledgementLoss(t *testing.T) {
	db := setupClickHouseLogTest(t)
	q, s := setupLogWriteQueueTest(t)
	q.db = db
	ctx := context.Background()
	require.NoError(t, q.prepare(ctx))
	for _, id := range []string{"one", "two"} {
		require.NoError(t, q.enqueue(ctx, &Log{EventID: id, CreatedAt: 1700000000, Quota: 42, RequestId: "same"}))
	}
	batch, err := q.claim(ctx, "first")
	require.NoError(t, err)
	// Simulate loss of the Redis acknowledgement after a successful insert.
	require.NoError(t, db.Callback().Create().After("gorm:create").Register("test:expire_queue_lease", func(tx *gorm.DB) {
		s.FastForward(2 * time.Minute)
	}))
	require.Error(t, q.deliver(ctx, "first", batch))
	require.NoError(t, db.Callback().Create().Remove("test:expire_queue_lease"))
	var count int64
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(2), count)
	// A concurrent retry before reconciliation must also be deduplicated.
	duplicateCtx := chdriver.Context(ctx, chdriver.WithSettings(chdriver.Settings{
		"async_insert": 0, "insert_deduplicate": 1, "insert_deduplication_token": batch.token,
	}))
	require.NoError(t, db.WithContext(duplicateCtx).Table("logs").Create(&batch.rows).Error)
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(2), count)
	// Reconciliation also protects retries after the finite dedup log is evicted.
	require.NoError(t, db.Exec("ALTER TABLE logs MODIFY SETTING non_replicated_deduplication_window=1").Error)
	require.NoError(t, db.Exec("INSERT INTO logs (event_id,created_at) VALUES ('unrelated',1700000001)").Error)
	recovered, err := q.claim(ctx, "second")
	require.NoError(t, err)
	assert.Equal(t, batch.token, recovered.token)
	require.NoError(t, q.deliver(ctx, "second", recovered))
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(3), count, "retry must not double count usage")
	var stored []Log
	require.NoError(t, db.Where("event_id IN ?", []string{"one", "two"}).Order("event_id").Find(&stored).Error)
	require.Len(t, stored, 2)
	assert.Equal(t, "one", stored[0].EventID)
	assert.Equal(t, "two", stored[1].EventID)
	assert.Equal(t, 42, stored[0].Quota)
	assert.Equal(t, "same", stored[0].RequestId)
	assert.Equal(t, "0", q.redis.HGet(ctx, q.prefix+"state", "rows").Val())
}

func TestLogQueueClickHouseOutageRetainsBatch(t *testing.T) {
	db := setupClickHouseLogTest(t)
	q, s := setupLogWriteQueueTest(t)
	q.db = db
	ctx := context.Background()
	require.NoError(t, q.prepare(ctx))
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "outage", CreatedAt: 1700000000}))
	batch, err := q.claim(ctx, "first")
	require.NoError(t, err)
	require.NoError(t, db.Exec("RENAME TABLE logs TO offline_logs").Error)
	require.Error(t, q.deliver(ctx, "first", batch))
	assert.Equal(t, "1", q.redis.HGet(ctx, q.prefix+"state", "rows").Val())
	require.NoError(t, db.Exec("RENAME TABLE offline_logs TO logs").Error)
	s.FastForward(2 * time.Minute)
	recovered, err := q.claim(ctx, "second")
	require.NoError(t, err)
	require.NoError(t, q.deliver(ctx, "second", recovered))
	var count int64
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestLogQueueConnectsLazilyOnlyWhenEnabled(t *testing.T) {
	oldDB, oldType, oldMaster := LOG_DB, common.LogDatabaseType(), common.IsMasterNode
	t.Cleanup(func() {
		LOG_DB = oldDB
		common.SetLogDatabaseType(oldType)
		common.IsMasterNode = oldMaster
		initCol()
	})
	common.IsMasterNode = true
	t.Setenv("LOG_SQL_DSN", "clickhouse://127.0.0.1:1/unavailable?dial_timeout=100ms")
	t.Setenv("LOG_QUEUE_ENABLED", "true")
	require.NoError(t, InitLogDB(), "master startup must not attempt ClickHouse migrations while offline")
	assert.Equal(t, common.DatabaseTypeClickHouse, common.LogDatabaseType())
	conn, err := LOG_DB.DB()
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	t.Setenv("LOG_QUEUE_ENABLED", "false")
	_, _, err = chooseDB("LOG_SQL_DSN", true)
	assert.Error(t, err)
}

func TestLogQueueRefusesRecreatedDestination(t *testing.T) {
	db := setupClickHouseLogTest(t)
	q, _ := setupLogWriteQueueTest(t)
	q.db = db
	ctx := context.Background()
	require.NoError(t, q.prepare(ctx))
	require.NoError(t, q.enqueue(ctx, &Log{EventID: "pending"}))
	require.NoError(t, db.Exec("DROP TABLE logs SYNC").Error)
	require.ErrorContains(t, q.prepare(ctx), "destination table changed")
	assert.Equal(t, "1", q.redis.HGet(ctx, q.prefix+"state", "rows").Val())
}

func TestLogQueuePartialPublicationRequiresReconciliation(t *testing.T) {
	db := setupClickHouseLogTest(t)
	q, _ := setupLogWriteQueueTest(t)
	q.db = db
	ctx := context.Background()
	require.NoError(t, q.prepare(ctx))
	for _, id := range []string{"a", "b"} {
		require.NoError(t, q.enqueue(ctx, &Log{EventID: id, CreatedAt: 1700000000}))
	}
	batch, err := q.claim(ctx, "owner")
	require.NoError(t, err)
	require.NoError(t, db.Table("logs").Create(&batch.rows[0]).Error)
	require.ErrorContains(t, q.deliver(ctx, "owner", batch), "partially present")
	assert.Equal(t, "2", q.redis.HGet(ctx, q.prefix+"state", "rows").Val())
	var count int64
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestLogQueueRejectsChangedPreviouslyDeliveredEvent(t *testing.T) {
	db := setupClickHouseLogTest(t)
	q, _ := setupLogWriteQueueTest(t)
	q.db = db
	ctx := context.Background()
	require.NoError(t, q.prepare(ctx))
	row := &Log{EventID: "a", CreatedAt: 1700000000, Quota: 10, ChannelName: "historical-name", Other: `{"admin_info":{"x":1}}`}
	require.NoError(t, q.enqueue(ctx, row))
	first, err := q.claim(ctx, "first")
	require.NoError(t, err)
	require.NoError(t, q.deliver(ctx, "first", first))
	// Re-enqueue after acknowledgement is reconciled by identity, not token.
	require.NoError(t, q.enqueue(ctx, row))
	repeat, err := q.claim(ctx, "repeat")
	require.NoError(t, err)
	require.NoError(t, q.deliver(ctx, "repeat", repeat))
	row.Quota = 20
	require.NoError(t, q.enqueue(ctx, row))
	changed, err := q.claim(ctx, "changed")
	require.NoError(t, err)
	require.ErrorContains(t, q.deliver(ctx, "changed", changed), "content mismatch")
	var stored []Log
	require.NoError(t, db.Find(&stored).Error)
	require.Len(t, stored, 1)
	assert.Equal(t, 10, stored[0].Quota)
	assert.Equal(t, "historical-name", stored[0].ChannelName)
	assert.Equal(t, "1", q.redis.HGet(ctx, q.prefix+"state", "rows").Val())
}

// The two phases are run around a forced restart of an isolated AOF Redis.
func TestLogQueueRedisPersistence(t *testing.T) {
	phase, redisURL := os.Getenv("TEST_QUEUE_PERSISTENCE_PHASE"), os.Getenv("TEST_REDIS_URL")
	if phase == "" || redisURL == "" {
		t.Skip("requires isolated Redis and explicit persistence test phase")
	}
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	r := redis.NewClient(options)
	t.Cleanup(func() { assert.NoError(t, r.Close()) })
	q := &logWriteQueue{redis: r, prefix: "logqueue:{isolated-persistence-test}:", target: "test", maxRows: 10, maxBytes: 100000, batchRows: 2}
	ctx := context.Background()
	if phase == "enqueue" {
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, q.enqueue(ctx, &Log{EventID: id, CreatedAt: 1700000000, Content: "survives crash"}))
		}
		batch, err := q.claim(ctx, "before-crash")
		require.NoError(t, err)
		require.Len(t, batch.rows, 2)
		require.NoError(t, r.Set(ctx, q.prefix+"test-token", batch.token, 0).Err())
		require.NoError(t, r.PExpire(ctx, q.prefix+"lease", time.Millisecond).Err())
		return
	}
	require.Equal(t, "recover", phase)
	batch, err := q.claim(ctx, "after-crash")
	require.NoError(t, err)
	require.NotNil(t, batch)
	assert.Equal(t, r.Get(ctx, q.prefix+"test-token").Val(), batch.token)
	require.Len(t, batch.rows, 2)
	assert.Equal(t, "survives crash", batch.rows[0].Content)
	require.NoError(t, q.ack(ctx, "after-crash", batch.token))
	remaining, err := q.claim(ctx, "last")
	require.NoError(t, err)
	require.Len(t, remaining.rows, 1)
	assert.Equal(t, "c", remaining.rows[0].EventID)
	require.NoError(t, q.ack(ctx, "last", remaining.token))
	keys, err := r.Keys(ctx, q.prefix+"*").Result()
	require.NoError(t, err)
	require.NoError(t, r.Del(ctx, keys...).Err())
}

func TestLogQueueStartupAcceptsLogsWithOfflineClickHouse(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("requires isolated persistent Redis")
	}
	oldDB, oldType, oldQueue := LOG_DB, common.LogDatabaseType(), activeLogWriteQueue
	t.Cleanup(func() {
		LOG_DB, activeLogWriteQueue = oldDB, oldQueue
		common.SetLogDatabaseType(oldType)
		initCol()
	})
	t.Setenv("LOG_QUEUE_ENABLED", "true")
	t.Setenv("LOG_QUEUE_REDIS_URL", redisURL)
	t.Setenv("LOG_SQL_DSN", "clickhouse://127.0.0.1:1/queue_startup_test?dial_timeout=100ms")
	t.Setenv("LOG_SQL_CLICKHOUSE_TTL_DAYS", "0")
	require.NoError(t, InitLogDB())
	conn, err := LOG_DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, conn.Close()) })
	require.NoError(t, InitLogWriteQueue())
	q := activeLogWriteQueue
	t.Cleanup(StopLogWriteQueue)
	row := &Log{CreatedAt: 1700000000, Quota: 42}
	require.NoError(t, createLog(row))
	assert.NotEmpty(t, row.EventID)
	assert.Equal(t, "1", q.redis.HGet(context.Background(), q.prefix+"state", "rows").Val())
	// No worker shutdown drain is necessary: accepted data already lives in Redis.
	keys, err := q.redis.Keys(context.Background(), q.prefix+"*").Result()
	require.NoError(t, err)
	require.NoError(t, q.redis.Del(context.Background(), keys...).Err())
}
