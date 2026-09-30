package realtimemetrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const keyChannelCacheMinFmt = keyPrefix + "chcache:%d"

type channelCacheKey struct {
	minute    int64
	channelID int
}

type cacheUsage struct{ prompt, cached int64 }

var channelCachePending = struct {
	sync.Mutex
	values   map[channelCacheKey]cacheUsage
	prunedAt int64
}{values: make(map[channelCacheKey]cacheUsage)}

// RecordChannelCacheUsage receives the same inclusive input denominator used by
// group monitoring, but covers every channel on every node without its filters.
// Keep event minutes through retries so a Redis outage cannot move old usage
// into the current hour. The request path performs no network or database I/O.
func RecordChannelCacheUsage(channelID, prompt, cached int) {
	if channelID <= 0 || prompt <= 0 || !redisAvailable() {
		return
	}
	minute := time.Now().Unix() / 60 * 60
	channelCachePending.Lock()
	defer channelCachePending.Unlock()
	if channelCachePending.prunedAt != minute {
		for key := range channelCachePending.values {
			if key.minute <= minute-3600 {
				delete(channelCachePending.values, key)
			}
		}
		channelCachePending.prunedAt = minute
	}
	key := channelCacheKey{minute: minute, channelID: channelID}
	value := channelCachePending.values[key]
	value.prompt += int64(prompt)
	value.cached += int64(max(0, min(cached, prompt)))
	channelCachePending.values[key] = value
}

func flushChannelCacheUsage() {
	channelCachePending.Lock()
	drained := channelCachePending.values
	channelCachePending.values = make(map[channelCacheKey]cacheUsage)
	channelCachePending.Unlock()
	if len(drained) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	minute := time.Now().Unix() / 60 * 60
	pipe := common.RDB.TxPipeline()
	for key, value := range drained {
		if key.minute <= minute-3600 {
			continue
		}
		redisKey := fmt.Sprintf(keyChannelCacheMinFmt, key.minute)
		id := strconv.Itoa(key.channelID)
		pipe.HIncrBy(ctx, redisKey, id+":p", value.prompt)
		pipe.HIncrBy(ctx, redisKey, id+":c", value.cached)
		pipe.Expire(ctx, redisKey, seriesRetention)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		channelCachePending.Lock()
		for key, value := range drained {
			if key.minute <= minute-3600 {
				continue
			}
			pending := channelCachePending.values[key]
			pending.prompt += value.prompt
			pending.cached += value.cached
			channelCachePending.values[key] = pending
		}
		channelCachePending.Unlock()
		common.SysError("channel cache metrics flush failed: " + err.Error())
	}
}

// ReadChannelCacheRates batches the whole channel table into one bounded Redis
// pipeline. An absent channel means no measured input tokens, not zero cache hits.
func ReadChannelCacheRates(ctx context.Context) (map[int]float64, error) {
	rates := make(map[int]float64)
	if !redisAvailable() {
		return rates, nil
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	minute := time.Now().Unix() / 60 * 60
	pipe := common.RDB.Pipeline()
	commands := make([]*redis.StringStringMapCmd, 0, seriesMinutes)
	for i := 0; i < seriesMinutes; i++ {
		commands = append(commands, pipe.HGetAll(ctx, fmt.Sprintf(keyChannelCacheMinFmt, minute-int64(i)*60)))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	totals := make(map[int]cacheUsage)
	for _, command := range commands {
		for field, raw := range command.Val() {
			id, kind, ok := strings.Cut(field, ":")
			channelID, err := strconv.Atoi(id)
			if !ok || err != nil || channelID <= 0 {
				continue
			}
			value := totals[channelID]
			switch kind {
			case "p":
				value.prompt += max(0, parseInt(raw))
			case "c":
				value.cached += max(0, parseInt(raw))
			}
			totals[channelID] = value
		}
	}
	for id, value := range totals {
		if value.prompt > 0 {
			rates[id] = min(100.0, float64(value.cached)/float64(value.prompt)*100)
		}
	}
	return rates, nil
}
