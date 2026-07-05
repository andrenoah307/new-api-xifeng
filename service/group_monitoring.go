package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var (
	aggregationRunning int32
	triggerCh          = make(chan struct{}, 1)
)

func StartGroupMonitoringAggregation() {
	if !common.IsMasterNode {
		return
	}
	common.GroupMonitoringHook = func(group string, channelId int, isSuccess bool, promptTokens, cacheTokens, useTimeMs, frtMs int, modelName string, statusCode int, content string) {
		RecordMonitoringMetric(group, channelId, isSuccess, promptTokens, cacheTokens, useTimeMs, frtMs, modelName, statusCode, content)
	}
	refreshMonitoredGroupsCache()
	go monitoringLoop()
}

func monitoringLoop() {
	runAggregationCycle()

	cfg := operation_setting.GetGroupMonitoringSetting()
	interval := cfg.AggregationIntervalMinutes
	if interval <= 0 {
		interval = 5
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			refreshMonitoredGroupsCache()
			runAggregationCycle()
		case <-triggerCh:
			refreshMonitoredGroupsCache()
			runAggregationCycle()
		}
	}
}

func TriggerAggregationRefresh() bool {
	if !atomic.CompareAndSwapInt32(&aggregationRunning, 0, 1) {
		return false
	}
	atomic.StoreInt32(&aggregationRunning, 0)
	select {
	case triggerCh <- struct{}{}:
	default:
	}
	return true
}

type bucketData struct {
	Total        int64
	Success      int64
	Error        int64
	CacheTokens  int64
	PromptTokens int64
	RespTimeMs   int64
	FRTSumMs     int64
	FRTCount     int64
}

type channelKey struct {
	Group     string
	ChannelId int
}

func runAggregationCycle() {
	if !atomic.CompareAndSwapInt32(&aggregationRunning, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&aggregationRunning, 0)

	cfg := operation_setting.GetGroupMonitoringSetting()
	if len(cfg.MonitoringGroups) == 0 {
		return
	}

	if common.RedisEnabled {
		runRedisAggregation(cfg)
	} else {
		runDBFallbackAggregation(cfg)
	}
}

func runRedisAggregation(cfg operation_setting.GroupMonitoringSetting) {
	now := time.Now().Unix()
	bucketSec := int64(cfg.AggregationIntervalMinutes * 60)
	if bucketSec <= 0 {
		bucketSec = 300
	}

	availStart := now - int64(cfg.AvailabilityPeriodMinutes*60)
	cacheStart := now - int64(cfg.CacheHitPeriodMinutes*60)

	monitoredSet := make(map[string]struct{}, len(cfg.MonitoringGroups))
	for _, g := range cfg.MonitoringGroups {
		monitoredSet[g] = struct{}{}
	}

	ctx := context.Background()
	var cursor uint64
	channelAvailData := make(map[channelKey]*bucketData)
	channelCacheData := make(map[channelKey]*bucketData)
	channelLatestBucket := make(map[channelKey]int64)

	for {
		keys, nextCursor, err := common.RDB.Scan(ctx, cursor, "gm:b:*", 200).Result()
		if err != nil {
			common.SysError("group monitoring: redis scan error: " + err.Error())
			break
		}

		for _, key := range keys {
			bucketTs, group, chId, ok := parseMonitoringKey(key)
			if !ok {
				continue
			}
			if _, monitored := monitoredSet[group]; !monitored {
				continue
			}

			vals, err := common.RDB.HGetAll(ctx, key).Result()
			if err != nil || len(vals) == 0 {
				continue
			}

			bd := parseBucketValues(vals)
			ck := channelKey{Group: group, ChannelId: chId}

			if bucketTs >= availStart {
				agg, ok := channelAvailData[ck]
				if !ok {
					agg = &bucketData{}
					channelAvailData[ck] = agg
				}
				agg.Total += bd.Total
				agg.Success += bd.Success
				agg.Error += bd.Error
				agg.RespTimeMs += bd.RespTimeMs
				agg.FRTSumMs += bd.FRTSumMs
				agg.FRTCount += bd.FRTCount
			}

			if bucketTs >= cacheStart {
				agg, ok := channelCacheData[ck]
				if !ok {
					agg = &bucketData{}
					channelCacheData[ck] = agg
				}
				agg.CacheTokens += bd.CacheTokens
				agg.PromptTokens += bd.PromptTokens
			}

			if bucketTs > channelLatestBucket[ck] {
				channelLatestBucket[ck] = bucketTs
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	groupRatios := ratio_setting.GetGroupRatioCopy()

	type groupAgg struct {
		totalRequests  int64
		totalSuccess   int64
		totalCacheTok  int64
		totalPromptTok int64
		totalRespMs    int64
		totalFRTMs     int64
		totalFRTCount  int64
		onlineChannels int
		totalChannels  int
		lastTestModel  string
	}
	groupStats := make(map[string]*groupAgg)

	channelStats := make([]*model.ChannelMonitoringStat, 0)

	for ck, avail := range channelAvailData {
		stat := &model.ChannelMonitoringStat{
			GroupName: ck.Group,
			ChannelId: ck.ChannelId,
		}

		if avail.Total > 0 {
			stat.AvailabilityRate = float64(avail.Success) / float64(avail.Total) * 100
		} else {
			stat.AvailabilityRate = -1
		}

		if cache, ok := channelCacheData[ck]; ok && cache.PromptTokens > 0 {
			stat.CacheHitRate = float64(cache.CacheTokens) / float64(cache.PromptTokens) * 100
		} else {
			stat.CacheHitRate = -1
		}

		if avail.Total > 0 {
			stat.AvgResponseTime = int(avail.RespTimeMs / avail.Total)
		}
		if avail.FRTCount > 0 {
			stat.AvgFRT = int(avail.FRTSumMs / avail.FRTCount)
		}

		latestBucket := channelLatestBucket[ck]
		latestKeys := fmt.Sprintf("gm:b:%d:%s:%d", latestBucket, ck.Group, ck.ChannelId)
		latestVals, _ := common.RDB.HGetAll(context.Background(), latestKeys).Result()
		latestBD := parseBucketValues(latestVals)
		stat.IsOnline = latestBD.Total > 0 && (latestBD.Error == 0 || latestBD.Success > 0)

		channelStats = append(channelStats, stat)

		ga, ok := groupStats[ck.Group]
		if !ok {
			ga = &groupAgg{}
			groupStats[ck.Group] = ga
		}
		ga.totalRequests += avail.Total
		ga.totalSuccess += avail.Success
		if cache, ok := channelCacheData[ck]; ok {
			ga.totalCacheTok += cache.CacheTokens
			ga.totalPromptTok += cache.PromptTokens
		}
		ga.totalRespMs += avail.RespTimeMs
		ga.totalFRTMs += avail.FRTSumMs
		ga.totalFRTCount += avail.FRTCount
		ga.totalChannels++
		if stat.IsOnline {
			ga.onlineChannels++
		}
	}

	if len(channelStats) > 0 {
		if err := model.BatchUpsertChannelMonitoringStats(channelStats); err != nil {
			common.SysError("group monitoring: upsert channel stats error: " + err.Error())
		}
	}

	for _, groupName := range cfg.MonitoringGroups {
		ga, ok := groupStats[groupName]
		if !ok {
			ga = &groupAgg{}
		}

		stat := &model.GroupMonitoringStat{
			GroupName:      groupName,
			OnlineChannels: ga.onlineChannels,
			TotalChannels:  ga.totalChannels,
			GroupRatio:     groupRatios[groupName],
		}

		if ga.totalRequests > 0 {
			stat.AvailabilityRate = float64(ga.totalSuccess) / float64(ga.totalRequests) * 100
		} else {
			stat.AvailabilityRate = -1
		}

		if ga.totalPromptTok > 0 {
			stat.CacheHitRate = float64(ga.totalCacheTok) / float64(ga.totalPromptTok) * 100
		} else {
			stat.CacheHitRate = -1
		}

		if ga.totalRequests > 0 {
			stat.AvgResponseTime = int(ga.totalRespMs / ga.totalRequests)
		}
		if ga.totalFRTCount > 0 {
			stat.AvgFRT = int(ga.totalFRTMs / ga.totalFRTCount)
		}

		if err := model.UpsertGroupMonitoringStat(stat); err != nil {
			common.SysError("group monitoring: upsert group stat error: " + err.Error())
		}

		_ = model.InsertMonitoringHistory(&model.MonitoringHistory{
			GroupName:        groupName,
			AvailabilityRate: stat.AvailabilityRate,
			CacheHitRate:     stat.CacheHitRate,
			RecordedAt:       time.Now().Unix(),
		})
	}

	cleanupThreshold := time.Now().Add(-30 * 24 * time.Hour).Unix()
	_ = model.CleanupOldMonitoringHistory(cleanupThreshold)
}

func runDBFallbackAggregation(cfg operation_setting.GroupMonitoringSetting) {
	now := time.Now().Unix()
	availStart := now - int64(cfg.AvailabilityPeriodMinutes*60)

	monitoredSet := make(map[string]struct{}, len(cfg.MonitoringGroups))
	for _, g := range cfg.MonitoringGroups {
		monitoredSet[g] = struct{}{}
	}

	groupRatios := ratio_setting.GetGroupRatioCopy()

	type dbAgg struct {
		Total   int64
		Success int64
		RespMs  int64
	}

	rows, err := model.AggregateLogsForMonitoring(availStart)
	if err != nil {
		common.SysError("group monitoring: db fallback query error: " + err.Error())
		return
	}

	channelAgg := make(map[channelKey]*dbAgg)
	for _, row := range rows {
		if _, ok := monitoredSet[row.Group]; !ok {
			continue
		}
		ck := channelKey{Group: row.Group, ChannelId: row.ChannelId}
		agg, ok := channelAgg[ck]
		if !ok {
			agg = &dbAgg{}
			channelAgg[ck] = agg
		}
		agg.Total += row.Count
		if row.LogType == 2 {
			agg.Success += row.Count
		}
		agg.RespMs += row.SumTime * 1000
	}

	type groupFallbackAgg struct {
		totalReq     int64
		totalSuccess int64
		totalRespMs  int64
		online       int
		total        int
	}
	groupAggs := make(map[string]*groupFallbackAgg)

	var channelStats []*model.ChannelMonitoringStat
	for ck, agg := range channelAgg {
		stat := &model.ChannelMonitoringStat{
			GroupName:    ck.Group,
			ChannelId:    ck.ChannelId,
			CacheHitRate: -1,
		}
		if agg.Total > 0 {
			stat.AvailabilityRate = float64(agg.Success) / float64(agg.Total) * 100
			stat.AvgResponseTime = int(agg.RespMs / agg.Total)
			stat.IsOnline = agg.Success > 0
		} else {
			stat.AvailabilityRate = -1
		}
		channelStats = append(channelStats, stat)

		ga, ok := groupAggs[ck.Group]
		if !ok {
			ga = &groupFallbackAgg{}
			groupAggs[ck.Group] = ga
		}
		ga.totalReq += agg.Total
		ga.totalSuccess += agg.Success
		ga.totalRespMs += agg.RespMs
		ga.total++
		if stat.IsOnline {
			ga.online++
		}
	}

	if len(channelStats) > 0 {
		_ = model.BatchUpsertChannelMonitoringStats(channelStats)
	}

	for _, groupName := range cfg.MonitoringGroups {
		ga := groupAggs[groupName]
		stat := &model.GroupMonitoringStat{
			GroupName:  groupName,
			GroupRatio: groupRatios[groupName],
		}
		if ga != nil {
			stat.TotalChannels = ga.total
			stat.OnlineChannels = ga.online
			if ga.totalReq > 0 {
				stat.AvailabilityRate = float64(ga.totalSuccess) / float64(ga.totalReq) * 100
				stat.AvgResponseTime = int(ga.totalRespMs / ga.totalReq)
			} else {
				stat.AvailabilityRate = -1
			}
		} else {
			stat.AvailabilityRate = -1
		}
		stat.CacheHitRate = -1

		_ = model.UpsertGroupMonitoringStat(stat)
		_ = model.InsertMonitoringHistory(&model.MonitoringHistory{
			GroupName:        groupName,
			AvailabilityRate: stat.AvailabilityRate,
			CacheHitRate:     stat.CacheHitRate,
			RecordedAt:       time.Now().Unix(),
		})
	}

	cleanupThreshold := time.Now().Add(-30 * 24 * time.Hour).Unix()
	_ = model.CleanupOldMonitoringHistory(cleanupThreshold)
}

func parseMonitoringKey(key string) (bucketTs int64, group string, channelId int, ok bool) {
	// key format: gm:b:{bucket_ts}:{group}:{channel_id}
	parts := strings.SplitN(key, ":", 5)
	if len(parts) != 5 || parts[0] != "gm" || parts[1] != "b" {
		return 0, "", 0, false
	}
	var err error
	bucketTs, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, "", 0, false
	}
	group = parts[3]
	channelId, err = strconv.Atoi(parts[4])
	if err != nil {
		return 0, "", 0, false
	}
	return bucketTs, group, channelId, true
}

func parseBucketValues(vals map[string]string) bucketData {
	var bd bucketData
	if v, ok := vals["t"]; ok {
		bd.Total, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := vals["s"]; ok {
		bd.Success, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := vals["e"]; ok {
		bd.Error, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := vals["ct"]; ok {
		bd.CacheTokens, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := vals["pt"]; ok {
		bd.PromptTokens, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := vals["rt"]; ok {
		bd.RespTimeMs, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := vals["fs"]; ok {
		bd.FRTSumMs, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := vals["fc"]; ok {
		bd.FRTCount, _ = strconv.ParseInt(v, 10, 64)
	}
	return bd
}

// Exported for testing
var ParseMonitoringKey = parseMonitoringKey
var ParseBucketValues = parseBucketValues

// MonitoringAggregationLoopStarted tracks whether the loop was started (for tests)
var monitoringLoopOnce sync.Once
