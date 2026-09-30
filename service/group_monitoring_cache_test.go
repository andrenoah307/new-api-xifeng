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
	RecordMonitoringMetric("hourly", 1, true, 100, 25, 100, 0, "", 0, "")
	keys := server.Keys()
	require.Len(t, keys, 1)
	server.FastForward(59 * time.Minute)
	assert.True(t, server.Exists(keys[0]), "a short configured window must not expire hourly samples")
	assert.Equal(t, "100", server.HGet(keys[0], "pt"))
	assert.Equal(t, "25", server.HGet(keys[0], "ct"))
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
