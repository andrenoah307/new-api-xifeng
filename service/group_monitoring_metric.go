package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

var (
	monitoredGroupsMu    sync.RWMutex
	monitoredGroupsCache map[string]struct{}
)

func refreshMonitoredGroupsCache() {
	cfg := operation_setting.GetGroupMonitoringSetting()
	m := make(map[string]struct{}, len(cfg.MonitoringGroups))
	for _, g := range cfg.MonitoringGroups {
		m[g] = struct{}{}
	}
	monitoredGroupsMu.Lock()
	monitoredGroupsCache = m
	monitoredGroupsMu.Unlock()
}

func isGroupMonitored(group string) bool {
	monitoredGroupsMu.RLock()
	defer monitoredGroupsMu.RUnlock()
	if monitoredGroupsCache == nil {
		return false
	}
	_, ok := monitoredGroupsCache[group]
	return ok
}

func RecordMonitoringMetric(group string, channelId int, isSuccess bool, promptTokens, cacheTokens, useTimeMs, frtMs int, modelName string, statusCode int, content string) {
	if !common.RedisEnabled || group == "" || group == "auto" {
		return
	}
	if !isGroupMonitored(group) {
		return
	}

	cfg := operation_setting.GetGroupMonitoringSetting()
	if !cfg.Enabled {
		return
	}

	excludeAvail := false
	excludeCache := false

	if modelName != "" {
		for _, m := range cfg.AvailabilityExcludeModels {
			if m == modelName {
				excludeAvail = true
				break
			}
		}
		for _, m := range cfg.CacheHitExcludeModels {
			if m == modelName {
				excludeCache = true
				break
			}
		}
	}

	if !isSuccess && !excludeAvail {
		if statusCode > 0 {
			for _, sc := range cfg.AvailabilityExcludeStatusCodes {
				if sc == statusCode {
					excludeAvail = true
					break
				}
			}
		}
		if !excludeAvail && content != "" && len(cfg.AvailabilityExcludeKeywords) > 0 {
			lc := strings.ToLower(content)
			for _, kw := range cfg.AvailabilityExcludeKeywords {
				if strings.Contains(lc, strings.ToLower(kw)) {
					excludeAvail = true
					break
				}
			}
		}
	}

	bucketSec := int64(cfg.AggregationIntervalMinutes * 60)
	if bucketSec <= 0 {
		bucketSec = 300
	}
	now := time.Now().Unix()
	bucket := (now / bucketSec) * bucketSec
	key := fmt.Sprintf("gm:b:%d:%s:%d", bucket, group, channelId)

	maxPeriod := cfg.AvailabilityPeriodMinutes
	if cfg.CacheHitPeriodMinutes > maxPeriod {
		maxPeriod = cfg.CacheHitPeriodMinutes
	}
	ttl := time.Duration(maxPeriod*60*2+int(bucketSec)) * time.Second

	ctx := context.Background()
	pipe := common.RDB.TxPipeline()

	if !excludeAvail {
		pipe.HIncrBy(ctx, key, "t", 1)
		if isSuccess {
			pipe.HIncrBy(ctx, key, "s", 1)
		} else {
			pipe.HIncrBy(ctx, key, "e", 1)
		}
	}

	if !excludeCache {
		pipe.HIncrBy(ctx, key, "ct", int64(cacheTokens))
		pipe.HIncrBy(ctx, key, "pt", int64(promptTokens))
	}

	pipe.HIncrBy(ctx, key, "rt", int64(useTimeMs))
	if frtMs > 0 {
		pipe.HIncrBy(ctx, key, "fs", int64(frtMs))
		pipe.HIncrBy(ctx, key, "fc", 1)
	}
	pipe.Expire(ctx, key, ttl)
	_, _ = pipe.Exec(ctx)
}
