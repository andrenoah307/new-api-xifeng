package perfmetrics

import (
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelaySampleExclusionsApplyToSharedPerformance(t *testing.T) {
	setupGroupModelSummaryTest(t)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "group_monitoring_setting.") {
			saved[key] = value
		}
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		operation_setting.RebuildMonitoringExclusionIndex()
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_monitoring_setting.excluded_channel_ids": "[7]",
		"group_monitoring_setting.excluded_user_ids":    "[8]",
	}))
	operation_setting.RebuildMonitoringExclusionIndex()
	for _, success := range []bool{true, false} {
		for _, identity := range [][2]int{{7, 1}, {1, 8}} {
			RecordRelaySample(&relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelId: identity[0]},
				UserId:      identity[1], OriginModelName: "excluded", UsingGroup: "test", StartTime: time.Now(),
			}, success, 10, 503, "")
		}
	}
	got, err := QueryGroupModelSummary(1, []string{"test"})
	require.NoError(t, err)
	assert.Empty(t, got["test"], "excluded requests cannot reach either shared performance view")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1}, UserId: 1, OriginModelName: "kept", UsingGroup: "test", StartTime: time.Now()}
	RecordRelaySample(info, true, 10, 0, "")
	RecordRelaySample(info, false, 0, 503, "")
	got, err = QueryGroupModelSummary(1, []string{"test"})
	require.NoError(t, err)
	require.Len(t, got["test"], 1)
	assert.EqualValues(t, 2, got["test"][0].RequestCount)
	assert.Equal(t, 50.0, got["test"][0].SuccessRate)
}
