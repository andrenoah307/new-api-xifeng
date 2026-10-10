package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitoringFallbackExcludesChannelsAndUsers(t *testing.T) {
	truncateTables(t)
	logs := []Log{
		{CreatedAt: 100, Type: 2, Group: "test", ChannelId: 1, UserId: 1, UseTime: 2},
		{CreatedAt: 101, Type: 5, Group: "test", ChannelId: 1, UserId: 1, UseTime: 3},
		{CreatedAt: 102, Type: 2, Group: "test", ChannelId: 7, UserId: 1, UseTime: 4},
		{CreatedAt: 103, Type: 5, Group: "test", ChannelId: 1, UserId: 8, UseTime: 5},
		{CreatedAt: 99, Type: 2, Group: "test", ChannelId: 1, UserId: 1, UseTime: 6},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)
	rows, err := AggregateLogsForMonitoring(100, []int{7}, []int{8})
	require.NoError(t, err)
	assert.ElementsMatch(t, []MonitoringLogAggRow{
		{Group: "test", ChannelId: 1, LogType: 2, Count: 1, SumTime: 2},
		{Group: "test", ChannelId: 1, LogType: 5, Count: 1, SumTime: 3},
	}, rows)
	rows, err = AggregateLogsForMonitoring(100, nil, nil)
	require.NoError(t, err)
	var total int64
	for _, row := range rows {
		total += row.Count
	}
	assert.EqualValues(t, 4, total, "empty settings preserve the original sample population")
}

func TestMonitoringExclusionOptionsRefreshAndClear(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	keys := []string{"group_monitoring_setting.excluded_channel_ids", "group_monitoring_setting.excluded_user_ids"}
	before := operation_setting.GetGroupMonitoringSetting()
	t.Cleanup(func() {
		for i, ids := range [][]int{before.ExcludedChannelIDs, before.ExcludedUserIDs} {
			value, err := common.Marshal(ids)
			require.NoError(t, err)
			require.NoError(t, updateOptionMap(keys[i], string(value)))
		}
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, updateOptionMap(keys[0], "[7]"))
	require.NoError(t, updateOptionMap(keys[1], "[8]"))
	assert.True(t, operation_setting.IsMonitoringExcluded(7, 1))
	assert.True(t, operation_setting.IsMonitoringExcluded(1, 8))
	assert.False(t, operation_setting.IsMonitoringExcluded(1, 1))
	require.NoError(t, updateOptionMap(keys[0], "[]"))
	assert.False(t, operation_setting.IsMonitoringExcluded(7, 1))
	assert.True(t, operation_setting.IsMonitoringExcluded(1, 8))
	require.NoError(t, updateOptionMap(keys[1], "[]"))
	assert.False(t, operation_setting.IsMonitoringExcluded(1, 8))
}
