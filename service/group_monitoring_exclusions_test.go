package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitoringExclusionsBeforeRedis(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedis, oldEnabled := common.RDB, common.RedisEnabled
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "group_monitoring_setting.") {
			saved[key] = value
		}
		return nil
	}))
	monitoredGroupsMu.Lock()
	oldGroups := monitoredGroupsCache
	monitoredGroupsCache = map[string]struct{}{"test": {}}
	monitoredGroupsMu.Unlock()
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = oldRedis, oldEnabled
		monitoredGroupsMu.Lock()
		monitoredGroupsCache = oldGroups
		monitoredGroupsMu.Unlock()
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		operation_setting.RebuildMonitoringExclusionIndex()
		require.NoError(t, client.Close())
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_monitoring_setting.enabled":              "true",
		"group_monitoring_setting.excluded_channel_ids": "[7]",
		"group_monitoring_setting.excluded_user_ids":    "[8]",
	}))
	operation_setting.RebuildMonitoringExclusionIndex()
	for _, success := range []bool{true, false} {
		RecordMonitoringMetric("test", 7, 1, success, 100, 25, 200, 50, "model", 503, "", true)
		RecordMonitoringMetric("test", 1, 8, success, 100, 25, 200, 50, "model", 503, "", true)
	}
	assert.Empty(t, server.Keys(), "excluded samples must not write any Redis fields")
	RecordMonitoringMetric("test", 1, 1, true, 100, 25, 200, 50, "model", 0, "", true)
	RecordMonitoringMetric("test", 1, 1, false, 0, 0, 300, 0, "model", 503, "", true)
	keys := server.Keys()
	require.Len(t, keys, 1)
	for field, want := range map[string]string{"t": "2", "s": "1", "e": "1", "pt": "100", "ct": "25", "rt": "500", "fs": "50", "fc": "1"} {
		assert.Equal(t, want, server.HGet(keys[0], field), field)
	}
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_monitoring_setting.excluded_channel_ids": "[]",
		"group_monitoring_setting.excluded_user_ids":    "[]",
	}))
	operation_setting.RebuildMonitoringExclusionIndex()
	RecordMonitoringMetric("test", 7, 8, true, 100, 25, 200, 50, "model", 0, "", true)
	assert.Len(t, server.Keys(), 2, "clearing exclusions resumes collection")
}
