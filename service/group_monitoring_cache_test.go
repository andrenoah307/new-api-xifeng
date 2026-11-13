package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMonitoringBucketsRetainHourlyCacheWithShortWindows(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedis, oldEnabled := common.RDB, common.RedisEnabled
	saved := make(map[string]string)
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "group_monitoring_setting.") {
			saved[key] = value
		}
		return nil
	}))
	monitoredGroupsMu.Lock()
	oldGroups := monitoredGroupsCache
	monitoredGroupsCache = map[string]struct{}{"hourly": {}}
	monitoredGroupsMu.Unlock()
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = oldRedis, oldEnabled
		monitoredGroupsMu.Lock()
		monitoredGroupsCache = oldGroups
		monitoredGroupsMu.Unlock()
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		_ = client.Close()
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_monitoring_setting.enabled":                      "true",
		"group_monitoring_setting.availability_period_minutes":  "5",
		"group_monitoring_setting.cache_hit_period_minutes":     "5",
		"group_monitoring_setting.aggregation_interval_minutes": "5",
	}))
	RecordMonitoringMetric("hourly", 1, 1, true, 100, 25, 100, 0, "", 0, "", true, false)
	keys := server.Keys()
	require.Len(t, keys, 1)
	server.FastForward(59 * time.Minute)
	assert.True(t, server.Exists(keys[0]), "a short configured window must not expire hourly samples")
	assert.Equal(t, "100", server.HGet(keys[0], "pt"))
	assert.Equal(t, "25", server.HGet(keys[0], "ct"))
}

func TestGroupMonitoringCachePopulationToggles(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	oldDB, oldRedis, oldEnabled := model.DB, common.RDB, common.RedisEnabled
	saved := make(map[string]string)
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "group_monitoring_setting.") {
			saved[key] = value
		}
		return nil
	}))
	monitoredGroupsMu.Lock()
	oldGroups := monitoredGroupsCache
	monitoredGroupsCache = map[string]struct{}{"mixed": {}, "nonstream": {}, "legacy": {}}
	monitoredGroupsMu.Unlock()
	model.DB, common.RDB, common.RedisEnabled = db, client, true
	t.Cleanup(func() {
		model.DB, common.RDB, common.RedisEnabled = oldDB, oldRedis, oldEnabled
		monitoredGroupsMu.Lock()
		monitoredGroupsCache = oldGroups
		monitoredGroupsMu.Unlock()
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		_ = client.Close()
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.GroupMonitoringStat{}, &model.ChannelMonitoringStat{}, &model.MonitoringHistory{}))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_monitoring_setting.enabled":                       "true",
		"group_monitoring_setting.cache_hit_exclude_models":      `["excluded"]`,
		"group_monitoring_setting.frt_exclude_threshold_seconds": "0",
	}))
	RecordMonitoringMetric("mixed", 1, 1, true, 100, 80, 100, 50, "model", 0, "", true, false)
	RecordMonitoringMetric("mixed", 1, 1, true, 900, 450, 100, 50, "model", 0, "", false, false)
	RecordMonitoringMetric("mixed", 1, 1, true, 200, 100, 100, 50, "model", 0, "", true, true)
	RecordMonitoringMetric("mixed", 1, 1, true, 100, 0, 100, 50, "model", 0, "", true, true)
	RecordMonitoringMetric("mixed", 1, 1, true, 700, 630, 100, 50, "model", 0, "", false, true)
	RecordMonitoringMetric("mixed", 1, 1, true, 1000, 0, 100, 50, "excluded", 0, "", true, true)
	RecordMonitoringMetric("nonstream", 1, 1, true, 100, 20, 100, 50, "model", 0, "", false, false)
	key := fmt.Sprintf("gm:b:%d:legacy:1", time.Now().Unix()-60)
	require.NoError(t, client.HSet(context.Background(), key, "pt", 100, "ct", 50, "t", 1, "s", 1).Err())
	cfg := operation_setting.GetGroupMonitoringSetting()
	cfg.MonitoringGroups = []string{"mixed", "nonstream", "legacy"}
	for _, tc := range []struct {
		streamOnly, affinityOnly bool
		rate                     float64
	}{
		{false, false, 63},
		{true, false, 45},
		{false, true, 73},
		{true, true, 100.0 / 3},
		{false, false, 63},
	} {
		cfg.CacheHitExcludeNonStream = tc.streamOnly
		cfg.CacheHitExcludeNonAffinity = tc.affinityOnly
		runRedisAggregation(cfg, false)
		stat, err := model.GetGroupMonitoringStatByName("mixed")
		require.NoError(t, err)
		require.NotNil(t, stat.CacheHitRate1h)
		assert.InDelta(t, tc.rate, *stat.CacheHitRate1h, 0.001)
		assert.InDelta(t, tc.rate, stat.CacheHitRate, 0.001)
		assert.InDelta(t, 100, stat.AvailabilityRate, 0.001)
		assert.Equal(t, 100, stat.AvgResponseTime)
		assert.Equal(t, 50, stat.AvgFRT)
		channels, err := model.GetChannelMonitoringStatsByGroup("mixed")
		require.NoError(t, err)
		require.Len(t, channels, 1)
		assert.InDelta(t, tc.rate, channels[0].CacheHitRate, 0.001)
		var history model.MonitoringHistory
		require.NoError(t, db.Where("group_name = ?", "mixed").Order("recorded_at DESC").First(&history).Error)
		assert.InDelta(t, tc.rate, history.CacheHitRate, 0.001)
		assert.Equal(t, 6, history.RequestCount, "cache filters must not remove requests from availability statistics")
		for _, group := range []string{"nonstream", "legacy"} {
			stat, err = model.GetGroupMonitoringStatByName(group)
			require.NoError(t, err)
			if tc.streamOnly || tc.affinityOnly {
				assert.Nil(t, stat.CacheHitRate1h)
			} else {
				assert.NotNil(t, stat.CacheHitRate1h)
			}
		}
	}
}

func TestGroupMonitoringHourlyCacheRate(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	oldDB, oldRedis := model.DB, common.RDB
	model.DB, common.RDB = db, client
	t.Cleanup(func() {
		model.DB, common.RDB = oldDB, oldRedis
		_ = client.Close()
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.GroupMonitoringStat{}, &model.ChannelMonitoringStat{}, &model.MonitoringHistory{}))
	cfg := operation_setting.GroupMonitoringSetting{
		MonitoringGroups:          []string{"weighted", "zero", "empty", "capped"},
		AvailabilityPeriodMinutes: 5, CacheHitPeriodMinutes: 10, AggregationIntervalMinutes: 5,
	}
	now := time.Now().Unix()
	for _, bucket := range []struct {
		group          string
		channel        int
		age            int64
		prompt, cached int
	}{
		{"weighted", 1, 120, 100, 100},
		{"weighted", 2, 1800, 900, 0}, // Outside both configurable windows; still part of 1h.
		{"weighted", 3, 3700, 10000, 10000},
		{"weighted", 4, -300, 10000, 10000}, // Future buckets must not count.
		{"zero", 1, 120, 100, 0},
		{"capped", 1, 120, 100, 150},
	} {
		key := fmt.Sprintf("gm:b:%d:%s:%d", now-bucket.age, bucket.group, bucket.channel)
		require.NoError(t, client.HSet(context.Background(), key, "pt", bucket.prompt, "ct", bucket.cached, "t", 1, "s", 1).Err())
	}
	runRedisAggregation(cfg, false)
	for group, expected := range map[string]float64{"weighted": 10, "zero": 0, "capped": 100} {
		stat, err := model.GetGroupMonitoringStatByName(group)
		require.NoError(t, err)
		require.NotNil(t, stat.CacheHitRate1h, group)
		assert.InDelta(t, expected, *stat.CacheHitRate1h, 0.0001, group)
	}
	stat, err := model.GetGroupMonitoringStatByName("empty")
	require.NoError(t, err)
	assert.Nil(t, stat.CacheHitRate1h)
	// An expired window must clear the persisted value, rather than keep a stale rate.
	server.FlushAll()
	runRedisAggregation(cfg, false)
	stat, err = model.GetGroupMonitoringStatByName("weighted")
	require.NoError(t, err)
	assert.Nil(t, stat.CacheHitRate1h)
}

func TestGroupMonitoringCacheFilterScopes(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	oldDB, oldRedis := model.DB, common.RDB
	model.DB, common.RDB = db, client
	t.Cleanup(func() {
		model.DB, common.RDB = oldDB, oldRedis
		_ = client.Close()
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.GroupMonitoringStat{}, &model.ChannelMonitoringStat{}, &model.MonitoringHistory{}))
	groups := []string{"both", "stream", "affinity", "neither", "no-affinity"}
	for _, group := range groups {
		key := fmt.Sprintf("gm:b:%d:%s:1", time.Now().Unix()-60, group)
		require.NoError(t, client.HSet(context.Background(), key,
			"pt", 1000, "ct", 600, "spt", 500, "sct", 200,
			"apt", 400, "act", 100, "sapt", 100, "sact", 0,
			"t", 10, "s", 10).Err())
		if group == "no-affinity" {
			require.NoError(t, client.HDel(context.Background(), key, "apt", "act", "sapt", "sact").Err())
		}
	}
	cfg := operation_setting.GroupMonitoringSetting{
		MonitoringGroups: groups, AvailabilityPeriodMinutes: 60, CacheHitPeriodMinutes: 60,
		AggregationIntervalMinutes: 5, CacheHitExcludeNonStream: true, CacheHitExcludeNonAffinity: true,
		CacheHitExcludeNonStreamGroups:   []string{"both", "stream"},
		CacheHitExcludeNonAffinityGroups: []string{"both", "affinity"},
	}
	for _, tc := range []struct {
		name, streamScope, affinityScope string
		disabled, empty                  bool
		want                             []float64
	}{
		{"legacy applies globally", "", "", false, false, []float64{0, 0, 0, 0, -1}},
		{"independent selections", "selected", "selected", false, false, []float64{0, 40, 25, 60, 60}},
		{"stream global", "all", "selected", false, false, []float64{0, 40, 0, 40, 40}},
		{"affinity global", "selected", "all", false, false, []float64{0, 0, 25, 25, -1}},
		{"disabled retains selection", "selected", "selected", true, false, []float64{60, 60, 60, 60, 60}},
		{"empty selection applies nowhere", "selected", "selected", false, true, []float64{60, 60, 60, 60, 60}},
		{"restore global", "all", "all", false, false, []float64{0, 0, 0, 0, -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := cfg
			current.CacheHitExcludeNonStreamScope = tc.streamScope
			current.CacheHitExcludeNonAffinityScope = tc.affinityScope
			current.CacheHitExcludeNonStream = !tc.disabled
			current.CacheHitExcludeNonAffinity = !tc.disabled
			if tc.empty {
				current.CacheHitExcludeNonStreamGroups = nil
				current.CacheHitExcludeNonAffinityGroups = []string{}
			}
			runRedisAggregation(current, false)
			for i, group := range groups {
				stat, err := model.GetGroupMonitoringStatByName(group)
				require.NoError(t, err)
				assert.InDelta(t, tc.want[i], stat.CacheHitRate, 0.001, group)
				if tc.want[i] < 0 {
					assert.Nil(t, stat.CacheHitRate1h, group)
				} else {
					require.NotNil(t, stat.CacheHitRate1h)
					assert.InDelta(t, tc.want[i], *stat.CacheHitRate1h, 0.001, group)
				}
				assert.Equal(t, float64(100), stat.AvailabilityRate)
				channels, err := model.GetChannelMonitoringStatsByGroup(group)
				require.NoError(t, err)
				require.Len(t, channels, 1)
				assert.InDelta(t, tc.want[i], channels[0].CacheHitRate, 0.001, group)
				var history model.MonitoringHistory
				require.NoError(t, db.Where("group_name = ?", group).Order("recorded_at DESC").First(&history).Error)
				assert.InDelta(t, tc.want[i], history.CacheHitRate, 0.001, group)
				assert.Equal(t, 10, history.RequestCount)
			}
		})
	}
}
