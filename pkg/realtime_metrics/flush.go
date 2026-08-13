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

const (
	// flushInterval is also the dashboard's poll interval, so a refresh always
	// lands on data at most one window old.
	flushInterval = 10 * time.Second
	// instanceTTL outlives three flush windows: one missed flush must not make a
	// healthy instance vanish from the dashboard.
	instanceTTL = 45 * time.Second
	// staleAfterSeconds is the read side's own cutoff, for keys that outlived the
	// process that wrote them.
	staleAfterSeconds = 45
	// seriesRetention keeps one hour of minute buckets. Anything longer belongs in
	// the persisted perf metrics, not in Redis.
	seriesRetention = 65 * time.Minute
	// channelRetention is deliberately short: the per-channel hash is the widest
	// key in the set (two fields per channel), so it holds a rolling few minutes
	// rather than a full hour.
	channelRetention  = 5 * time.Minute
	channelWindowMins = 5
	// nodeRetention drops instances that have not reported for long enough that
	// they are gone rather than restarting.
	nodeRetention = 10 * time.Minute

	redisTimeout = 200 * time.Millisecond

	keyPrefix        = "nak:rt:"
	keyNodes         = keyPrefix + "nodes"
	keyInstanceFmt   = keyPrefix + "inst:%s"
	keyMinuteFmt     = keyPrefix + "min:%d"
	keyChannelFmt    = keyPrefix + "chconc:%s"
	keyChannelMinFmt = keyPrefix + "chmin:%d"

	fieldRequests         = "requests"
	fieldSuccess          = "success"
	fieldErrors           = "errors"
	fieldClientGone       = "client_gone"
	fieldPromptTokens     = "prompt_tokens"
	fieldCompletionTokens = "completion_tokens"
	fieldQuota            = "quota"
)

var (
	initOnce      sync.Once
	gaugeProvider func() InstanceGauges
	nodeName      string
)

// Init starts the flush loop. The gauge provider is supplied by main.go because
// its sources (relay admission, the cgroup breaker, the runtime) live in packages
// that import this one.
func Init(provider func() InstanceGauges) {
	initOnce.Do(func() {
		gaugeProvider = provider
		nodeName = common.GetNodeIdentity().Name
		if nodeName == "" {
			nodeName = "unknown"
		}
		go flushLoop()
	})
}

// NodeName is the identity this process publishes under, so the read path can
// tell the local instance apart from its peers.
func NodeName() string {
	return nodeName
}

// LocalGauges reports this process's own gauges, used by the read path when Redis
// is unavailable.
func LocalGauges() InstanceGauges {
	if gaugeProvider == nil {
		return InstanceGauges{}
	}
	return gaugeProvider()
}

func flushLoop() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for range ticker.C {
		flushOnce()
	}
}

func flushOnce() {
	if !redisAvailable() {
		// Without Redis the read path reports this process's atomics directly, so
		// draining them here would throw away the only data the dashboard has.
		return
	}
	drained := globalDeltas.drain()
	channelDrained := drainChannelDeltas()
	if err := writeToRedis(drained, channelDrained); err != nil {
		globalDeltas.restore(drained)
		restoreChannelDeltas(channelDrained)
		common.SysError(fmt.Sprintf("realtime metrics flush failed: %s", err.Error()))
	}
}

func writeToRedis(drained drainedCounters, channelDrained []channelDelta) error {
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()

	nowUnix := time.Now().Unix()
	minute := nowUnix - nowUnix%60
	gauges := InstanceGauges{}
	if gaugeProvider != nil {
		gauges = gaugeProvider()
	}

	// One MULTI/EXEC for the whole window. The per-node gauge hash is rewritten
	// with DEL+HSET so a channel that fell to zero stops appearing, and that pair
	// must not be observable half-applied.
	pipe := common.RDB.TxPipeline()

	pipe.ZAdd(ctx, keyNodes, &redis.Z{Score: float64(nowUnix), Member: nodeName})
	// Retired instances would otherwise sit in the set forever. Pruning here keeps
	// the read path a pure read.
	pipe.ZRemRangeByScore(ctx, keyNodes, "-inf", strconv.FormatInt(nowUnix-int64(nodeRetention.Seconds()), 10))

	instanceKey := fmt.Sprintf(keyInstanceFmt, nodeName)
	pipe.HSet(ctx, instanceKey,
		"active_requests", gauges.ActiveRequests,
		"active_body_bytes", gauges.ActiveBodyBytes,
		"max_concurrent", gauges.MaxConcurrent,
		"max_body_bytes", gauges.MaxBodyBytes,
		"cgroup_permille", gauges.CgroupPermille,
		"cgroup_tripped", boolField(gauges.CgroupTripped),
		"trip_count", gauges.TripCount,
		"forced_reset_count", gauges.ForcedResetCount,
		"goroutines", gauges.Goroutines,
		"last_seen_unix", nowUnix,
	)
	pipe.Expire(ctx, instanceKey, instanceTTL)

	minuteKey := fmt.Sprintf(keyMinuteFmt, minute)
	wroteMinute := false
	for field, value := range drained {
		if value == 0 {
			continue
		}
		// HINCRBY, never HSET: every instance writes this same key.
		pipe.HIncrBy(ctx, minuteKey, field, value)
		wroteMinute = true
	}
	if wroteMinute {
		pipe.Expire(ctx, minuteKey, seriesRetention)
	}

	channelKey := fmt.Sprintf(keyChannelFmt, nodeName)
	concurrency := snapshotChannelConcurrency()
	pipe.Del(ctx, channelKey)
	if len(concurrency) > 0 {
		fields := make([]any, 0, len(concurrency)*2)
		for channelID, value := range concurrency {
			fields = append(fields, strconv.Itoa(channelID), value)
		}
		pipe.HSet(ctx, channelKey, fields...)
		pipe.Expire(ctx, channelKey, instanceTTL)
	}

	if len(channelDrained) > 0 {
		channelMinuteKey := fmt.Sprintf(keyChannelMinFmt, minute)
		for _, item := range channelDrained {
			id := strconv.Itoa(item.channelID)
			if item.requests != 0 {
				pipe.HIncrBy(ctx, channelMinuteKey, id+":r", item.requests)
			}
			if item.errors != 0 {
				pipe.HIncrBy(ctx, channelMinuteKey, id+":e", item.errors)
			}
		}
		pipe.Expire(ctx, channelMinuteKey, channelRetention)
	}

	_, err := pipe.Exec(ctx)
	return err
}

func boolField(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func redisAvailable() bool {
	return common.RedisEnabled && common.RDB != nil
}
