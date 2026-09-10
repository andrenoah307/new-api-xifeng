package perfmetrics

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setBucketTime(t *testing.T, value string) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, current string) error {
		if key == "perf_metrics_setting.bucket_time" {
			saved[key] = current
		}
		return nil
	}))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"perf_metrics_setting.bucket_time": value}))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
}

// 时序槽位数必须是固定的 GroupModelSeriesSlots，与管理员配置的 bucket 宽度无关。
// 若按 window/bucketWidth 推算，bucket_time="minute" 会产出 1440 个点，
// 单次响应里 73 个模型 × 20 个分组会瞬间放大成百万级数组。
func TestQueryGroupModelSummary_SeriesSlotCountIsFixed(t *testing.T) {
	for _, bucketTime := range []string{"hour", "5min", "minute"} {
		t.Run(bucketTime, func(t *testing.T) {
			setupGroupModelSummaryTest(t)
			setBucketTime(t, bucketTime)

			require.NoError(t, model.DB.Create(&model.PerfMetric{
				ModelName: "m", Group: "g", BucketTs: time.Now().Unix() - 1800,
				RequestCount: 4, SuccessCount: 4, TotalLatencyMs: 400,
			}).Error)

			got, err := QueryGroupModelSummary(24, []string{"g"})
			require.NoError(t, err)
			require.Len(t, got["g"], 1)
			assert.Len(t, got["g"][0].Series, GroupModelSeriesSlots)
		})
	}
}

// 无数据的槽位必须是 nil（JSON null），不能是 0。
// 0 会被前端红黄绿色阶渲染成"成功率 0%"的红块，把"没请求"谎报成"全挂了"。
func TestQueryGroupModelSummary_EmptySlotsAreNull(t *testing.T) {
	setupGroupModelSummaryTest(t)

	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: time.Now().Unix() - 1800,
		RequestCount: 4, SuccessCount: 2, TotalLatencyMs: 400,
	}).Error)

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	series := got["g"][0].Series
	require.Len(t, series, GroupModelSeriesSlots)
	filled := 0
	for _, point := range series {
		if point != nil {
			filled++
		}
	}
	require.Equal(t, 1, filled, "只有一个桶有数据，其余槽位必须为 null")
	require.NotNil(t, series[GroupModelSeriesSlots-1], "最新的数据必须落在时间轴末尾（从旧到新排列）")
	assert.Equal(t, 50.0, *series[GroupModelSeriesSlots-1])
}

// 回归（承接热桶归一化缺陷）：热桶必须按它自己的 bucketTs 落到对应槽位。
// 汇总数值需要跨桶合并，但时序不能——一旦为了合并汇总而先丢掉 bucketTs，
// 热桶的实时数据会被整段涂到错误的时间点上。
func TestQueryGroupModelSummary_HotBucketLandsInItsOwnSlot(t *testing.T) {
	setupGroupModelSummaryTest(t)

	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: now - 81000, // 22.5 小时前 -> 槽位 1
		RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 1000,
	}).Error)
	seedHotBucket("g", "m", now-1800, counters{ // 0.5 小时前 -> 末尾槽位
		requestCount: 10, successCount: 0, totalLatencyMs: 1000,
	})

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	item := got["g"][0]
	assert.Equal(t, int64(20), item.RequestCount, "汇总仍然要跨桶合并")
	assert.Equal(t, 50.0, item.SuccessRate)

	series := item.Series
	require.Len(t, series, GroupModelSeriesSlots)
	require.NotNil(t, series[1], "数据库行必须落在它自己的时间槽位")
	assert.Equal(t, 100.0, *series[1])
	require.NotNil(t, series[GroupModelSeriesSlots-1], "热桶必须落在它自己的时间槽位")
	assert.Equal(t, 0.0, *series[GroupModelSeriesSlots-1])
	for i := 2; i < GroupModelSeriesSlots-1; i++ {
		assert.Nil(t, series[i], "槽位 %d 无数据", i)
	}
}

// 同一槽位内的多个桶按请求数加权，而不是把各桶成功率再平均。
func TestQueryGroupModelSummary_SeriesMergesWithinSlotByWeight(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "minute")

	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: now - 1800,
		RequestCount: 99, SuccessCount: 99, TotalLatencyMs: 990,
	}).Error)
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: now - 1740,
		RequestCount: 1, SuccessCount: 0, TotalLatencyMs: 10,
	}).Error)

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	series := got["g"][0].Series
	require.Len(t, series, GroupModelSeriesSlots)
	require.NotNil(t, series[GroupModelSeriesSlots-1])
	// 加权 99/100 = 99；若错误地对两桶成功率取平均则是 (100 + 0) / 2 = 50。
	assert.Equal(t, 99.0, *series[GroupModelSeriesSlots-1])
}

// 槽位宽度只由查询窗口决定，与管理员配置的 bucket 宽度无关。
// 若误用 GetBucketSeconds() 当槽宽，bucket_time="minute" 时同一小时的数据会全部
// 挤进第一个槽，时序缩略图会退化成"只有开头有色块"。
func TestGroupModelSeriesSlotSeconds(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "minute")

	cases := []struct {
		hours int
		want  int64
	}{
		{hours: 24, want: 3600},
		{hours: 1, want: 150},
		{hours: 720, want: 108000},
		{hours: 0, want: 3600},
		{hours: -1, want: 3600},
		{hours: 1000, want: 108000},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, GroupModelSeriesSlotSeconds(c.hours), "hours=%d", c.hours)
	}
}
