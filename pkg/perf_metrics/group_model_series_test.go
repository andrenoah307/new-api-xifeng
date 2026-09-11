package perfmetrics

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
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

// 槽宽同时受两条约束支配，缺一条都会让走势条失真：
//   - 槽宽 >= bucket 宽度：否则一个 bucket 只能落进一个槽，其余槽必然空洞
//     （1h 窗口 + 1h 桶按 window/24 推算得 150s，24 槽只有 1 槽有值）；
//   - 槽宽是 bucket 宽度的整数倍：否则两套网格错位，落槽呈梳齿状
//     （3h 窗口 + 5min 桶，450s 槽宽会让相邻桶时而同槽时而跨槽）。
//
// 槽数由 window/槽宽 推出，因此天然不超过 GroupModelSeriesMaxSlots——
// 这条上界是载荷保护：按 window/bucket 直接推算的话 bucket_time="minute"
// 会产出 1440 个点，单次响应里 73 模型 × 20 分组会放大成百万级数组。
func TestGroupModelSeriesLayout(t *testing.T) {
	cases := []struct {
		name            string
		bucketTime      string
		hours           int
		wantSlotSeconds int64
		wantSlots       int
	}{
		{name: "24h/hour 与改动前逐字相同", bucketTime: "hour", hours: 24, wantSlotSeconds: 3600, wantSlots: 24},
		{name: "24h/5min 折叠 12 个底层桶", bucketTime: "5min", hours: 24, wantSlotSeconds: 3600, wantSlots: 24},
		{name: "24h/minute 守住载荷上界", bucketTime: "minute", hours: 24, wantSlotSeconds: 3600, wantSlots: 24},
		{name: "1h/5min 无空洞", bucketTime: "5min", hours: 1, wantSlotSeconds: 300, wantSlots: 12},
		{name: "1h/hour 诚实暴露没有小时内分辨率", bucketTime: "hour", hours: 1, wantSlotSeconds: 3600, wantSlots: 1},
		{name: "1h/minute 槽宽向上取整到桶宽整数倍", bucketTime: "minute", hours: 1, wantSlotSeconds: 180, wantSlots: 20},
		{name: "3h/5min 450s 必须抬到 600s", bucketTime: "5min", hours: 3, wantSlotSeconds: 600, wantSlots: 18},
		{name: "720h/minute 上限窗口仍是 24 槽", bucketTime: "minute", hours: 720, wantSlotSeconds: 108000, wantSlots: 24},
		{name: "hours=0 回落 24h", bucketTime: "hour", hours: 0, wantSlotSeconds: 3600, wantSlots: 24},
		{name: "hours 为负回落 24h", bucketTime: "hour", hours: -1, wantSlotSeconds: 3600, wantSlots: 24},
		{name: "hours 超过 720 夹到 720", bucketTime: "hour", hours: 1000, wantSlotSeconds: 108000, wantSlots: 24},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setupGroupModelSummaryTest(t)
			setBucketTime(t, c.bucketTime)

			slotSeconds, slots := GroupModelSeriesLayout(c.hours)
			assert.Equal(t, c.wantSlotSeconds, slotSeconds)
			assert.Equal(t, c.wantSlots, slots)
			assert.LessOrEqual(t, slots, GroupModelSeriesMaxSlots, "载荷上界不可突破")
			assert.Zero(t, slotSeconds%perf_metrics_setting.GetBucketSeconds(), "槽宽必须是桶宽的整数倍")
		})
	}
}

// 槽位对齐到墙钟网格（而不是从"此刻"往回推），这是两件事的前提：
//   - 每个 bucket 恰好落进一个槽，不会因为起点在桶内部而被切成两半；
//   - 同一段数据在连续两次刷新之间停在同一个槽，色块不会左右横跳。
func TestQueryGroupModelSummary_SlotsAlignToWallClockGrid(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	now := time.Now().Unix()
	newestSlot := now - now%300
	for i := 0; i < 12; i++ {
		require.NoError(t, model.DB.Create(&model.PerfMetric{
			ModelName: "m", Group: "g", BucketTs: newestSlot - int64(11-i)*300,
			RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 1000,
		}).Error)
	}

	got, err := QueryGroupModelSummary(1, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	series := got["g"][0].Series
	require.Len(t, series, 12, "1 小时窗口 + 5 分钟桶 = 12 槽")
	for idx, point := range series {
		require.NotNil(t, point, "槽位 %d 必须有值——12 个连续桶不允许出现任何空洞", idx)
		assert.Equal(t, 100.0, *point)
	}
}

// 直接把窗口常量改成 1 曾经的第二重失效：startTs 落在整点桶内部，
// 上一个完整桶被 `bucket_ts >= startTs` 整条排除，1 小时窗口在库里命中 0 行。
// 对齐后窗口起点必须正好压在桶边界上，最老的那个桶要被完整纳入。
func TestQueryGroupModelSummary_OldestBucketInWindowIsNotDropped(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	now := time.Now().Unix()
	newestSlot := now - now%300
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: newestSlot - 11*300,
		RequestCount: 8, SuccessCount: 4, TotalLatencyMs: 800,
	}).Error)

	got, err := QueryGroupModelSummary(1, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1, "窗口内最老的那个桶不能被排除")
	assert.Equal(t, int64(8), got["g"][0].RequestCount)
	require.NotNil(t, got["g"][0].Series[0])
}

// 窗口再往前一个桶就出界了，必须被排除——否则"近 1 小时"会悄悄变成 65 分钟。
func TestQueryGroupModelSummary_BucketBeforeWindowIsExcluded(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	now := time.Now().Unix()
	newestSlot := now - now%300
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: newestSlot - 12*300,
		RequestCount: 8, SuccessCount: 4, TotalLatencyMs: 800,
	}).Error)

	got, err := QueryGroupModelSummary(1, []string{"g"})
	require.NoError(t, err)
	assert.Empty(t, got["g"])
}

// 无数据的槽位必须是 nil（JSON null），不能是 0。
// 0 会被前端红黄绿色阶渲染成"成功率 0%"的红块，把"没请求"谎报成"全挂了"。
func TestQueryGroupModelSummary_EmptySlotsAreNull(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "hour")

	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: now - now%3600,
		RequestCount: 4, SuccessCount: 2, TotalLatencyMs: 400,
	}).Error)

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	series := got["g"][0].Series
	require.Len(t, series, 24)
	filled := 0
	for _, point := range series {
		if point != nil {
			filled++
		}
	}
	require.Equal(t, 1, filled, "只有一个桶有数据，其余槽位必须为 null")
	require.NotNil(t, series[23], "最新的数据必须落在时间轴末尾（从旧到新排列）")
	assert.Equal(t, 50.0, *series[23])
}

// 回归（承接热桶归一化缺陷）：热桶必须按它自己的 bucketTs 落到对应槽位。
// 汇总数值需要跨桶合并，但时序不能——一旦为了合并汇总而先丢掉 bucketTs，
// 热桶的实时数据会被整段涂到错误的时间点上。
func TestQueryGroupModelSummary_HotBucketLandsInItsOwnSlot(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "hour")

	now := time.Now().Unix()
	newestSlot := now - now%3600
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: newestSlot - 22*3600, // 槽位 1
		RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 1000,
	}).Error)
	seedHotBucket("g", "m", newestSlot, counters{ // 当前桶 -> 末尾槽位
		requestCount: 10, successCount: 0, totalLatencyMs: 1000,
	})

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	item := got["g"][0]
	assert.Equal(t, int64(20), item.RequestCount, "汇总仍然要跨桶合并")
	assert.Equal(t, 50.0, item.SuccessRate)

	series := item.Series
	require.Len(t, series, 24)
	require.NotNil(t, series[1], "数据库行必须落在它自己的时间槽位")
	assert.Equal(t, 100.0, *series[1])
	require.NotNil(t, series[23], "热桶必须落在它自己的时间槽位")
	assert.Equal(t, 0.0, *series[23])
	for i := 2; i < 23; i++ {
		assert.Nil(t, series[i], "槽位 %d 无数据", i)
	}
}

// 同一槽位内的多个桶按请求数加权，而不是把各桶成功率再平均。
func TestQueryGroupModelSummary_SeriesMergesWithinSlotByWeight(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "minute")

	now := time.Now().Unix()
	// 两个桶都必须落在"已经整段过去"的槽里。若播在末尾槽 newestSlot 上，
	// 第二个桶 newestSlot+60 在每小时的前 60 秒会大于 now 而被窗口右界排除，
	// 于是这条用例只在一天里 1/60 的时刻失败。取次新槽即与时刻无关。
	slot := now - now%3600 - 3600
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: slot,
		RequestCount: 99, SuccessCount: 99, TotalLatencyMs: 990,
	}).Error)
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: slot + 60,
		RequestCount: 1, SuccessCount: 0, TotalLatencyMs: 10,
	}).Error)

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	series := got["g"][0].Series
	require.Len(t, series, 24)
	require.NotNil(t, series[22])
	// 加权 99/100 = 99；若错误地对两桶成功率取平均则是 (100 + 0) / 2 = 50。
	assert.Equal(t, 99.0, *series[22])
}

// 走势条的颜色与左侧时间轴共用同一条色阶：可用率达标时再看首字延迟，
// 超过阈值降级为「健康但慢」。左侧拿得到逐段 FRT，右侧此前只有成功率，
// 于是同一时刻左黄右绿。TtftSeries 把逐槽首字补齐——数据在 DB 行与热桶里
// 本就带着 ttft_sum_ms/ttft_count，此前在填充循环里被丢弃，补它不增加任何查询。
func TestQueryGroupModelSummary_TtftSeriesCarriesPerSlotLatency(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	now := time.Now().Unix()
	newestSlot := now - now%300
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: newestSlot - 11*300,
		RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 10000,
		TtftSumMs: 5000, TtftCount: 10, // 500ms
	}).Error)
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: newestSlot,
		RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 160000,
		TtftSumMs: 150000, TtftCount: 10, // 15000ms
	}).Error)

	got, err := QueryGroupModelSummary(1, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	item := got["g"][0]
	require.Len(t, item.TtftSeries, 12)
	require.NotNil(t, item.TtftSeries[0])
	assert.Equal(t, int64(500), *item.TtftSeries[0])
	require.NotNil(t, item.TtftSeries[11])
	assert.Equal(t, int64(15000), *item.TtftSeries[11])
	for i := 1; i < 11; i++ {
		assert.Nil(t, item.TtftSeries[i], "槽位 %d 无样本", i)
	}
	// 窗口级均值把尖峰抹平成 7750ms（低于 10s 阈值），这正是只看它无法给出
	// 逐槽颜色的原因：末槽实际 15s 必须单独可见。
	assert.Equal(t, int64(7750), item.AvgTtftMs)
	assert.True(t, item.HasTtft)
}

// 非流式请求不产生首字样本（hasTtft = IsStream && HasSendResponse），
// 于是一个槽可以「有请求但 ttft_count == 0」。这种槽必须是 null 而不是 0：
// 0 会被前端读成「首字 0ms」，把慢模型涂成绿色，正好抵消这次修复。
func TestQueryGroupModelSummary_SlotWithoutTtftSampleIsNullNotZero(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	now := time.Now().Unix()
	newestSlot := now - now%300
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: newestSlot,
		RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 10000,
		TtftSumMs: 0, TtftCount: 0,
	}).Error)
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: newestSlot - 300,
		RequestCount: 1, SuccessCount: 1, TotalLatencyMs: 15000,
		TtftSumMs: 12000, TtftCount: 1,
	}).Error)

	got, err := QueryGroupModelSummary(1, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	item := got["g"][0]
	require.Len(t, item.Series, 12)
	require.NotNil(t, item.Series[11], "有请求的槽仍要有成功率")
	require.Len(t, item.TtftSeries, 12)
	assert.Nil(t, item.TtftSeries[11], "没有首字样本的槽必须是 null，不能是 0")
	require.NotNil(t, item.TtftSeries[10])
	assert.Equal(t, int64(12000), *item.TtftSeries[10])
	assert.True(t, item.HasTtft)
	assert.Equal(t, int64(12000), item.AvgTtftMs)
}

// 整个窗口都没有流式请求时，逐槽数组整体省略，不在载荷里留 N 个 null。
func TestQueryGroupModelSummary_TtftSeriesOmittedWhenModelNeverStreamed(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	now := time.Now().Unix()
	newestSlot := now - now%300
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: newestSlot,
		RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 10000,
	}).Error)

	got, err := QueryGroupModelSummary(1, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)
	assert.Nil(t, got["g"][0].TtftSeries, "窗口内零首字样本时整条省略")
}

// 热桶与 DB 行走同一套归属规则：当前桶的首字必须落进它自己的槽，
// 否则「刚刚变慢」这件事要等到下一次 flush 才会被染色。
func TestQueryGroupModelSummary_HotBucketTtftLandsInItsOwnSlot(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	now := time.Now().Unix()
	newestSlot := now - now%300
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: newestSlot - 11*300,
		RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 10000,
		TtftSumMs: 5000, TtftCount: 10,
	}).Error)
	seedHotBucket("g", "m", newestSlot, counters{
		requestCount: 4, successCount: 4, totalLatencyMs: 80000,
		ttftSumMs: 48000, ttftCount: 4, // 12000ms
	})

	got, err := QueryGroupModelSummary(1, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	series := got["g"][0].TtftSeries
	require.Len(t, series, 12)
	require.NotNil(t, series[0])
	assert.Equal(t, int64(500), *series[0])
	require.NotNil(t, series[11], "热桶的首字必须实时可见")
	assert.Equal(t, int64(12000), *series[11])
}

// 同一槽内多个桶按样本数加权求均值，而不是把各桶均值再平均。
func TestQueryGroupModelSummary_TtftSeriesMergesWithinSlotByWeight(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "minute")

	now := time.Now().Unix()
	// 取次新槽，避免末尾半开槽在每小时前 60 秒把 slot+60 排除到窗口外。
	slot := now - now%3600 - 3600
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: slot,
		RequestCount: 99, SuccessCount: 99, TotalLatencyMs: 99000,
		TtftSumMs: 99000, TtftCount: 99, // 1000ms
	}).Error)
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: slot + 60,
		RequestCount: 1, SuccessCount: 1, TotalLatencyMs: 21000,
		TtftSumMs: 21000, TtftCount: 1, // 21000ms
	}).Error)

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	series := got["g"][0].TtftSeries
	require.Len(t, series, 24)
	require.NotNil(t, series[22])
	// 加权 (99000+21000)/100 = 1200；若对两桶均值取平均则是 (1000+21000)/2 = 11000。
	assert.Equal(t, int64(1200), *series[22])
}
