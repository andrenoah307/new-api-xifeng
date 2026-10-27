package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var errLogQueueFull = errors.New("log write queue capacity exceeded; log was not accepted")
var activeLogWriteQueue *logWriteQueue

const logQueueBatchBytes = 4 << 20
const logQueueDedupWindow = 100000

// Same-named fields override Log's read-only GORM fields as well as JSON tags.
type queuedLog struct {
	Log
	EventID     string `json:"event_key" gorm:"column:event_id"`
	ChannelName string `json:"channel_name" gorm:"column:channel_name"`
}

type logWriteBatch struct {
	token string
	rows  []queuedLog
}

type logWriteQueue struct {
	redis     *redis.Client
	db        *gorm.DB
	prefix    string
	target    string
	maxRows   int64
	maxBytes  int64
	batchRows int
	sinkID    string
	cancel    context.CancelFunc
	done      chan struct{}
}

func logWriteQueueEnabled() bool {
	return os.Getenv("LOG_QUEUE_ENABLED") == "true"
}

// Queue keys share a hash tag. No pending or in-flight data has a TTL.
var enqueueLogScript = redis.NewScript(`
local target = redis.call('HGET', KEYS[3], 'target')
if target and target ~= ARGV[5] then return redis.error_reply('queue target mismatch') end
local existing = redis.call('HGET', KEYS[2], ARGV[1])
if existing then
  if existing ~= ARGV[2] then return redis.error_reply('event payload changed') end
  return 1
end
local rows = tonumber(redis.call('HGET', KEYS[3], 'rows') or '0')
local bytes = tonumber(redis.call('HGET', KEYS[3], 'bytes') or '0')
if rows >= tonumber(ARGV[3]) or bytes + string.len(ARGV[2]) > tonumber(ARGV[4]) then
  redis.call('HINCRBY', KEYS[3], 'rejected', 1)
  return 0
end
redis.call('HSET', KEYS[2], ARGV[1], ARGV[2])
redis.call('RPUSH', KEYS[1], ARGV[1])
redis.call('HSET', KEYS[3], 'target', ARGV[5], 'rows', rows + 1, 'bytes', bytes + string.len(ARGV[2]))
return 1
`)

func (q *logWriteQueue) enqueue(ctx context.Context, row *Log) error {
	if row.CreatedAt == 0 {
		row.CreatedAt = time.Now().Unix()
	}
	payload, err := common.Marshal(queuedLog{Log: *row, EventID: row.EventID, ChannelName: row.ChannelName})
	if err != nil {
		return err
	}
	if len(payload) > logQueueBatchBytes || row.EventID == "" {
		return errors.New("log queue event missing identity or exceeds 4 MiB")
	}
	accepted, err := enqueueLogScript.Run(ctx, q.redis, []string{q.prefix + "pending", q.prefix + "payload", q.prefix + "state"}, row.EventID, payload, q.maxRows, q.maxBytes, q.target).Int()
	if err != nil {
		return fmt.Errorf("log queue enqueue failed: %w", err)
	}
	if accepted == 0 {
		return errLogQueueFull
	}
	return nil
}

var claimLogScript = redis.NewScript(`
if not redis.call('SET', KEYS[5], ARGV[1], 'NX', 'PX', 60000) then return {} end
local ids = redis.call('LRANGE', KEYS[3], 0, -1)
local fresh = #ids == 0
if fresh then ids = redis.call('LRANGE', KEYS[1], 0, tonumber(ARGV[2]) - 1) end
local payloads = {}
local selected = {}
local bytes = 0
for _, id in ipairs(ids) do
  local payload = redis.call('HGET', KEYS[2], id)
  if not payload then return redis.error_reply('queue payload missing; retained for repair') end
  if fresh and bytes + string.len(payload) > tonumber(ARGV[3]) then break end
  table.insert(selected, id)
  table.insert(payloads, payload)
  bytes = bytes + string.len(payload)
end
if #selected == 0 then redis.call('DEL', KEYS[5]); return {} end
if fresh then
  redis.call('RPUSH', KEYS[3], unpack(selected))
  redis.call('LTRIM', KEYS[1], #selected, -1)
  redis.call('HSET', KEYS[4], 'token', ARGV[4])
end
local token = redis.call('HGET', KEYS[4], 'token')
if not token then return redis.error_reply('queue batch token missing') end
table.insert(payloads, 1, token)
return payloads
`)

func (q *logWriteQueue) claim(ctx context.Context, owner string) (*logWriteBatch, error) {
	values, err := claimLogScript.Run(ctx, q.redis, []string{q.prefix + "pending", q.prefix + "payload", q.prefix + "inflight", q.prefix + "state", q.prefix + "lease"}, owner, q.batchRows, logQueueBatchBytes, uuid.NewString()).StringSlice()
	if err != nil || len(values) == 0 {
		return nil, err
	}
	batch := &logWriteBatch{token: values[0], rows: make([]queuedLog, 0, len(values)-1)}
	for _, payload := range values[1:] {
		var row queuedLog
		if err := common.UnmarshalJsonStr(payload, &row); err != nil || row.EventID == "" {
			return nil, errors.New("invalid queued log payload; batch retained for repair")
		}
		row.Log.EventID = row.EventID
		row.Log.ChannelName = row.ChannelName
		batch.rows = append(batch.rows, row)
	}
	return batch, nil
}

var ackLogScript = redis.NewScript(`
if redis.call('GET', KEYS[4]) ~= ARGV[1] or redis.call('HGET', KEYS[3], 'token') ~= ARGV[2] then return 0 end
local ids = redis.call('LRANGE', KEYS[1], 0, -1)
local bytes = 0
for _, id in ipairs(ids) do
  local payload = redis.call('HGET', KEYS[2], id)
  if not payload then return redis.error_reply('queue payload missing; retained for repair') end
  bytes = bytes + string.len(payload)
end
for _, id in ipairs(ids) do redis.call('HDEL', KEYS[2], id) end
redis.call('HINCRBY', KEYS[3], 'rows', -#ids)
redis.call('HINCRBY', KEYS[3], 'bytes', -bytes)
redis.call('HINCRBY', KEYS[3], 'delivered', #ids)
redis.call('HDEL', KEYS[3], 'token')
redis.call('DEL', KEYS[1], KEYS[4])
return 1
`)

func (q *logWriteQueue) ack(ctx context.Context, owner, token string) error {
	ok, err := ackLogScript.Run(ctx, q.redis, []string{q.prefix + "inflight", q.prefix + "payload", q.prefix + "state", q.prefix + "lease"}, owner, token).Int()
	if err != nil {
		return err
	}
	if ok != 1 {
		return errors.New("log queue lease or batch changed; acknowledgement refused")
	}
	return nil
}

// Check schema/settings before claiming. Never discard a batch on sink failure.
func (q *logWriteQueue) prepare(ctx context.Context) error {
	db := q.db.WithContext(ctx)
	if err := db.Exec(ClickHouseLogCreateTableSQL(0)).Error; err != nil {
		return err
	}
	if err := validateClickHouseLogSchema(db); err != nil {
		return err
	}
	var ddl string
	if err := db.Raw("SHOW CREATE TABLE logs").Scan(&ddl).Error; err != nil {
		return err
	}
	if clickHouseCreateTableHasTTL(ddl) {
		return errors.New("log queue requires a logs table without TTL")
	}
	var tableID string
	if err := db.Raw("SELECT toString(uuid) FROM system.tables WHERE database = currentDatabase() AND name = 'logs'").Scan(&tableID).Error; err != nil {
		return err
	}
	if _, err := q.redis.HSetNX(ctx, q.prefix+"state", "sink_uuid", tableID).Result(); err != nil {
		return err
	}
	bound, err := q.redis.HGet(ctx, q.prefix+"state", "sink_uuid").Result()
	if err != nil || bound != tableID {
		return errors.New("log queue destination table changed; operator reconciliation required")
	}
	q.sinkID = tableID
	return db.Exec(fmt.Sprintf("ALTER TABLE logs MODIFY SETTING non_replicated_deduplication_window = %d", logQueueDedupWindow)).Error
}

func (q *logWriteQueue) deliver(ctx context.Context, owner string, batch *logWriteBatch) error {
	lease, err := q.redis.Get(ctx, q.prefix+"lease").Result()
	if err != nil || lease != owner {
		return errors.New("log queue lease lost before insert")
	}
	ctx = chdriver.Context(ctx, chdriver.WithSettings(chdriver.Settings{
		"async_insert": 0, "insert_deduplicate": 1,
		"insert_deduplication_token": batch.token,
		"max_threads":                2, "max_memory_usage": 536870912,
		"max_execution_time": 15,
	}))
	var tableID string
	if err := q.db.WithContext(ctx).Raw("SELECT toString(uuid) FROM system.tables WHERE database = currentDatabase() AND name = 'logs'").Scan(&tableID).Error; err != nil {
		return err
	}
	if tableID == "" || tableID != q.sinkID {
		return errors.New("log queue destination table changed; operator reconciliation required")
	}
	// The finite ClickHouse deduplication window is not a recovery ledger.
	// Reconcile persisted identities/content before retrying an uncertain batch.
	ids := make([]string, 0, len(batch.rows))
	expected := make(map[string]string, len(batch.rows))
	start, end := batch.rows[0].CreatedAt, batch.rows[0].CreatedAt
	for _, row := range batch.rows {
		ids = append(ids, row.EventID)
		payload, err := common.Marshal(row)
		if err != nil {
			return err
		}
		expected[row.EventID] = string(payload)
		start, end = min(start, row.CreatedAt), max(end, row.CreatedAt)
	}
	var stored []queuedLog
	if err := q.db.WithContext(ctx).Table("logs").Where("created_at >= ? AND created_at <= ? AND event_id IN ?", start, end, ids).Limit(len(batch.rows) + 1).Find(&stored).Error; err != nil {
		return err
	}
	if len(stored) > 0 {
		if len(stored) != len(batch.rows) {
			return errors.New("log queue batch partially present or duplicated; retained for reconciliation")
		}
		for _, row := range stored {
			payload, err := common.Marshal(row)
			if err != nil || expected[row.EventID] != string(payload) {
				return errors.New("log queue persisted content mismatch; retained for reconciliation")
			}
			delete(expected, row.EventID)
		}
		return q.ack(ctx, owner, batch.token)
	}
	// Insert the complete immutable batch in one statement; splitting a retry
	// differently would change block identities and defeat deduplication.
	if err := q.db.WithContext(ctx).Table("logs").Create(&batch.rows).Error; err != nil {
		return err
	}
	return q.ack(ctx, owner, batch.token)
}

func (q *logWriteQueue) run(ctx context.Context) {
	defer close(q.done)
	ready := false
	delay := time.Second
	nextReport := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
		var err error
		if !ready {
			err = q.prepare(attempt)
			ready = err == nil
		}
		if err == nil {
			owner := uuid.NewString()
			var batch *logWriteBatch
			batch, err = q.claim(attempt, owner)
			if err == nil && batch != nil {
				err = q.deliver(attempt, owner, batch)
			}
		}
		if time.Now().After(nextReport) {
			stats, statsErr := q.redis.HMGet(attempt, q.prefix+"state", "rows", "bytes", "delivered", "rejected").Result()
			if statsErr == nil {
				common.SysLog(fmt.Sprintf("log queue status: rows=%v payload_bytes=%v delivered=%v rejected=%v", stats[0], stats[1], stats[2], stats[3]))
			}
			nextReport = time.Now().Add(time.Minute)
		}
		cancel()
		if err != nil {
			common.SysError("log queue delivery failed; retained pending data: " + err.Error())
			delay = min(delay*2, 30*time.Second)
			ready = false
		} else {
			delay = time.Second
		}
	}
}

// InitLogWriteQueue runs before HTTP starts. Dedicated Redis is recommended;
// persistence settings are validated, never changed by the application.
func InitLogWriteQueue() error {
	if value := os.Getenv("LOG_QUEUE_ENABLED"); value != "" && value != "true" && value != "false" {
		return errors.New("LOG_QUEUE_ENABLED must be true or false")
	}
	if !logWriteQueueEnabled() {
		return nil
	}
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) || clickHouseLogTTLDays() != 0 {
		return errors.New("LOG_QUEUE_ENABLED requires ClickHouse and disabled log TTL")
	}
	redisURL := os.Getenv("LOG_QUEUE_REDIS_URL")
	if redisURL == "" {
		redisURL = os.Getenv("REDIS_CONN_STRING")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return errors.New("invalid log queue Redis URL")
	}
	options.DialTimeout, options.ReadTimeout, options.WriteTimeout = time.Second, time.Second, time.Second
	options.MaxRetries = -1
	options.PoolSize = 10
	client := redis.NewClient(options)
	valid := false
	defer func() {
		if !valid {
			_ = client.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for key, allowed := range map[string]string{"appendonly": "yes", "maxmemory-policy": "noeviction", "appendfsync": "always,everysec", "no-appendfsync-on-rewrite": "no"} {
		values, err := client.ConfigGet(ctx, key).Result()
		if err != nil || len(values) != 2 || !strings.Contains(","+allowed+",", ","+fmt.Sprint(values[1])+",") {
			return fmt.Errorf("log queue requires Redis %s=%s and CONFIG GET permission", key, allowed)
		}
		if key == "appendfsync" && values[1] == "everysec" {
			common.SysLog("log queue Redis uses appendfsync=everysec; a host crash can lose recent acknowledgements")
		}
	}
	targetURL, err := url.Parse(os.Getenv("LOG_SQL_DSN"))
	if err != nil {
		return errors.New("invalid log queue ClickHouse target")
	}
	target := targetURL.Host + targetURL.Path
	prefix := fmt.Sprintf("logqueue:{%x}:", sha256.Sum256([]byte(target)))
	q := &logWriteQueue{redis: client, db: LOG_DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}), prefix: prefix, target: target, maxRows: 1000000, maxBytes: 1 << 30, batchRows: 1000, done: make(chan struct{})}
	for name, dst := range map[string]*int64{"LOG_QUEUE_MAX_ROWS": &q.maxRows, "LOG_QUEUE_MAX_BYTES": &q.maxBytes} {
		if raw := os.Getenv(name); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 1 || value > 1<<40 {
				return fmt.Errorf("invalid %s: expected integer in [1, 2^40]", name)
			}
			*dst = value
		}
	}
	workerCtx, stop := context.WithCancel(context.Background())
	q.cancel = stop
	activeLogWriteQueue = q
	valid = true
	go q.run(workerCtx)
	return nil
}

func StopLogWriteQueue() {
	if q := activeLogWriteQueue; q != nil {
		q.cancel()
		<-q.done
		_ = q.redis.Close()
	}
}
