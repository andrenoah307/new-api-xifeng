package realtimemetrics

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const affinityCacheBucketSeconds int64 = 300

type ChannelAffinityCacheUsage struct {
	PromptTokens int64
	CachedTokens int64
	Samples      int64
}

var affinityCachePending = struct {
	sync.Mutex
	values   map[channelCacheKey]ChannelAffinityCacheUsage
	prunedAt int64
}{values: make(map[channelCacheKey]ChannelAffinityCacheUsage)}

var affinityCacheFlush = struct {
	sync.Mutex
	sequence int64
	batch    map[channelCacheKey]ChannelAffinityCacheUsage
}{}
var affinityCacheWriter = common.GetUUID()

// A per-process sequence makes retries idempotent even when Redis committed but
// its response was lost. Each bucket stores one sequence per writer, not per flush.
const affinityCacheFlushScript = `
local previous = tonumber(redis.call('HGET', KEYS[1], ARGV[1]) or '0')
if previous >= tonumber(ARGV[2]) then return 0 end
redis.call('HINCRBY', KEYS[1], 'p', ARGV[3])
redis.call('HINCRBY', KEYS[1], 'c', ARGV[4])
redis.call('HINCRBY', KEYS[1], 'n', ARGV[5])
redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])
redis.call('EXPIRE', KEYS[1], 90000)
return 1
`

// Match the log's affinity-hit marker, including zero-cache usage. If a marked
// request retries elsewhere, attribute its usage to the final billed channel.
func RecordChannelAffinityCacheUsage(selectedChannelID, channelID, prompt, cached int) {
	if channelID <= 0 || selectedChannelID <= 0 || prompt <= 0 || !redisAvailable() {
		return
	}
	key := channelCacheKey{minute: time.Now().Unix() / affinityCacheBucketSeconds * affinityCacheBucketSeconds, channelID: channelID}
	affinityCachePending.Lock()
	defer affinityCachePending.Unlock()
	value := affinityCachePending.values[key]
	value.PromptTokens += int64(prompt)
	value.CachedTokens += int64(max(0, min(prompt, cached)))
	value.Samples++
	affinityCachePending.values[key] = value
	if affinityCachePending.prunedAt != key.minute {
		for old := range affinityCachePending.values {
			if old.minute < key.minute-3600 {
				delete(affinityCachePending.values, old)
			}
		}
		affinityCachePending.prunedAt = key.minute
	}
}

func flushChannelAffinityCacheUsage() {
	affinityCacheFlush.Lock()
	defer affinityCacheFlush.Unlock()
	if len(affinityCacheFlush.batch) == 0 {
		affinityCachePending.Lock()
		affinityCacheFlush.batch = affinityCachePending.values
		affinityCachePending.values = make(map[channelCacheKey]ChannelAffinityCacheUsage)
		affinityCachePending.Unlock()
		affinityCacheFlush.sequence++
	}
	if len(affinityCacheFlush.batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	pipe := common.RDB.TxPipeline()
	for key, value := range affinityCacheFlush.batch {
		if key.minute < time.Now().Unix()-24*3600 {
			continue
		}
		name := fmt.Sprintf("nak:rt:affcache:%d:%d", key.channelID, key.minute)
		pipe.Eval(ctx, affinityCacheFlushScript, []string{name}, "w:"+affinityCacheWriter, affinityCacheFlush.sequence, value.PromptTokens, value.CachedTokens, value.Samples)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		common.SysError("affinity cache metrics flush failed: " + err.Error())
		return
	}
	affinityCacheFlush.batch = nil
}

// ReadChannelAffinityCacheUsage excludes partial oldest buckets and pre-recovery
// samples. No Redis means unknown, never a measured zero-percent cache rate.
func ReadChannelAffinityCacheUsage(ctx context.Context, channelID, hours int, since, now int64) (ChannelAffinityCacheUsage, error) {
	var total ChannelAffinityCacheUsage
	if !redisAvailable() {
		return total, fmt.Errorf("affinity cache metrics unavailable")
	}
	if hours < 1 || hours > 24 || channelID <= 0 {
		return total, fmt.Errorf("invalid affinity cache window")
	}
	start := max(now-int64(hours)*3600, since)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	pipe := common.RDB.Pipeline()
	commands := make([]*redis.StringStringMapCmd, 0, hours*12)
	for bucket := now / affinityCacheBucketSeconds * affinityCacheBucketSeconds; bucket >= start; bucket -= affinityCacheBucketSeconds {
		commands = append(commands, pipe.HGetAll(ctx, fmt.Sprintf("nak:rt:affcache:%d:%d", channelID, bucket)))
	}
	if len(commands) == 0 {
		return total, nil
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return total, err
	}
	for _, command := range commands {
		value := command.Val()
		if len(value) == 0 {
			continue
		}
		p, pe := strconv.ParseInt(value["p"], 10, 64)
		c, ce := strconv.ParseInt(value["c"], 10, 64)
		n, ne := strconv.ParseInt(value["n"], 10, 64)
		if pe != nil || ce != nil || ne != nil || p <= 0 || c < 0 || c > p || n <= 0 {
			return ChannelAffinityCacheUsage{}, fmt.Errorf("invalid affinity cache counters")
		}
		total.PromptTokens += p
		total.CachedTokens += c
		total.Samples += n
	}
	return total, nil
}
