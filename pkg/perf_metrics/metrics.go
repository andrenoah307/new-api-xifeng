package perfmetrics

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

var hotBuckets sync.Map

// seriesSchema is a stable client cache/schema marker. Do not change it when
// hiding fields or making response-only privacy hardening changes.
const seriesSchema = "dbcd0a3c01b55203"

func Init() {
	if setting := perf_metrics_setting.GetSetting(); setting.RetentionDays <= 0 {
		common.SysLog("perf_metrics 未配置保留期，数据将无限增长")
	}
	go flushLoop()
}

type GroupModelPerf struct {
	ModelName    string     `json:"model_name"`
	RequestCount int64      `json:"request_count"`
	SuccessRate  float64    `json:"success_rate"`
	AvgLatencyMs int64      `json:"avg_latency_ms"`
	AvgTtftMs    int64      `json:"avg_ttft_ms"`
	HasTtft      bool       `json:"has_ttft"`
	AvgTps       float64    `json:"avg_tps"`
	Series       []*float64 `json:"series"`
	// TtftSeries 与 Series 逐槽一一对应。走势条与左侧时间轴共用同一条色阶，
	// 而那条色阶在可用率达标时要再看首字延迟才能判「健康但慢」。
	// 数据本就随 DB 行与热桶一起到手，只是此前在填充循环里被丢弃，补它不增加查询。
	// 没有首字样本的槽是 nil 而不是 0——0 会被读成「首字 0ms」，把慢模型涂绿。
	TtftSeries []*int64 `json:"ttft_series,omitempty"`
}

// GroupModelSeriesMaxSlots 是走势条点数的上界，不是定长。
// 按 window/bucket 直接推算的话 bucket_time="minute" 会产出 1440 个点，
// 单次响应里 73 个模型 × 20 个分组会放大成百万级数组，所以必须封顶。
const GroupModelSeriesMaxSlots = 24

// GroupModelSeriesLayout 给出窗口的槽宽与槽数。槽宽同时受两条约束支配：
//   - 不小于 bucket 宽度——否则一个 bucket 只能落进一个槽，其余槽必然空洞；
//   - 是 bucket 宽度的整数倍——否则两套网格错位，落槽呈梳齿状。
//
// 槽数由 window/槽宽 推出，因而天然不超过 GroupModelSeriesMaxSlots。
// 全程整数秒运算，不引入浮点。
func GroupModelSeriesLayout(hours int) (slotSeconds int64, slots int) {
	if hours <= 0 {
		hours = 24
	}
	if hours > 720 {
		hours = 720
	}
	window := int64(hours) * 3600
	bucketSeconds := perf_metrics_setting.GetBucketSeconds()
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	buckets := ceilDiv(ceilDiv(window, GroupModelSeriesMaxSlots), bucketSeconds)
	if buckets < 1 {
		buckets = 1
	}
	slotSeconds = bucketSeconds * buckets
	return slotSeconds, int(ceilDiv(window, slotSeconds))
}

func ceilDiv(a, b int64) int64 {
	return (a + b - 1) / b
}

func QueryGroupModelSummary(hours int, groups []string) (map[string][]GroupModelPerf, error) {
	if hours <= 0 {
		hours = 24
	}
	if hours > 720 {
		hours = 720
	}
	slotSeconds, slots := GroupModelSeriesLayout(hours)
	endTs := time.Now().Unix()
	// 槽位对齐到墙钟网格，而不是从"此刻"往回推。不对齐会同时坏两件事：
	// 起点落在桶内部时，窗口里最老的那个完整桶被 `bucket_ts >= startTs` 整条排除；
	// 且 startTs 随请求时刻滑动，同一段数据会在连续刷新之间左右横跳。
	startTs := endTs - endTs%slotSeconds - int64(slots-1)*slotSeconds
	allowed := allowedGroupSet(groups)
	rows, err := model.GetPerfMetricsGroupModelBuckets(startTs, endTs, groups)
	if err != nil {
		return nil, err
	}
	totals := map[bucketKey]counters{}
	series := map[bucketKey]counters{}
	for _, row := range rows {
		value := counters{requestCount: row.RequestCount, successCount: row.SuccessCount, totalLatencyMs: row.TotalLatencyMs, ttftSumMs: row.TtftSumMs, ttftCount: row.TtftCount, outputTokens: row.OutputTokens, generationMs: row.GenerationMs}
		key := bucketKey{model: row.ModelName, group: row.Group}
		mergeCounters(totals, key, value)
		idx := int((row.BucketTs - startTs) / slotSeconds)
		if idx < 0 {
			idx = 0
		}
		if idx >= slots {
			idx = slots - 1
		}
		key.bucketTs = int64(idx)
		mergeCounters(series, key, value)
	}
	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.bucketTs < startTs || k.bucketTs > endTs {
			return true
		}
		if allowed != nil {
			if _, ok := allowed[k.group]; !ok {
				return true
			}
		}
		// 归一到 (model, group)：数据库行已跨桶聚合，热桶必须丢弃 bucketTs 才能并入同一条目
		snapshot := value.(*atomicBucket).snapshot()
		baseKey := bucketKey{model: k.model, group: k.group}
		mergeCounters(totals, baseKey, snapshot)
		idx := int((k.bucketTs - startTs) / slotSeconds)
		if idx < 0 {
			idx = 0
		}
		if idx >= slots {
			idx = slots - 1
		}
		slotKey := baseKey
		slotKey.bucketTs = int64(idx)
		mergeCounters(series, slotKey, snapshot)
		return true
	})
	result := map[string][]GroupModelPerf{}
	for key, total := range totals {
		if total.requestCount == 0 {
			continue
		}
		rate := float64(total.successCount) / float64(total.requestCount) * 100
		tps := 0.0
		if total.generationMs > 0 {
			tps = float64(total.outputTokens) / (float64(total.generationMs) / 1000)
		}
		item := GroupModelPerf{ModelName: key.model, RequestCount: total.requestCount, SuccessRate: math.Round(rate*100) / 100, AvgLatencyMs: total.totalLatencyMs / total.requestCount, AvgTps: math.Round(tps*100) / 100, Series: make([]*float64, slots)}
		if total.ttftCount > 0 {
			item.HasTtft = true
			item.AvgTtftMs = total.ttftSumMs / total.ttftCount
			item.TtftSeries = make([]*int64, slots)
		}
		for idx := 0; idx < slots; idx++ {
			slotKey := key
			slotKey.bucketTs = int64(idx)
			value := series[slotKey]
			if value.requestCount > 0 {
				rate := math.Round(successRate(value)*100) / 100
				item.Series[idx] = &rate
			}
			if item.TtftSeries != nil && value.ttftCount > 0 {
				ttft := value.ttftSumMs / value.ttftCount
				item.TtftSeries[idx] = &ttft
			}
		}
		result[key.group] = append(result[key.group], item)
	}
	for group := range result {
		sort.Slice(result[group], func(i, j int) bool {
			if result[group][i].RequestCount == result[group][j].RequestCount {
				return result[group][i].ModelName < result[group][j].ModelName
			}
			return result[group][i].RequestCount > result[group][j].RequestCount
		})
	}
	return result, nil
}
func RecordRelaySample(info *relaycommon.RelayInfo, success bool, outputTokens int64, statusCode int, errContent string) {
	if info == nil {
		return
	}
	// 复用「分组监控设置」的排除关键词/状态码：命中的失败视为用户参数问题，整条不入桶，
	// 既不拉低成功率也不进分母（与分组监控口径一致）。仅对失败样本判定。
	if !success && operation_setting.GetGroupMonitoringSetting().IsUserParamFailure(statusCode, errContent) {
		return
	}
	now := time.Now()
	hasTtft := info.IsStream && info.HasSendResponse()
	ttftMs := int64(0)
	if hasTtft {
		ttftMs = info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
	}
	latencyMs := now.Sub(info.StartTime).Milliseconds()
	generationMs := latencyMs
	if hasTtft {
		generationMs = now.Sub(info.FirstResponseTime).Milliseconds()
	}
	if generationMs <= 0 {
		generationMs = latencyMs
	}
	Record(Sample{
		Model:        info.OriginModelName,
		Group:        info.UsingGroup,
		LatencyMs:    latencyMs,
		TtftMs:       ttftMs,
		HasTtft:      hasTtft,
		Success:      success,
		OutputTokens: outputTokens,
		GenerationMs: generationMs,
	})
}

func Record(sample Sample) {
	setting := perf_metrics_setting.GetSetting()
	if !setting.Enabled || sample.Model == "" {
		return
	}
	if sample.Group == "" {
		sample.Group = "default"
	}
	if sample.LatencyMs < 0 {
		sample.LatencyMs = 0
	}

	key := bucketKey{
		model:    sample.Model,
		group:    sample.Group,
		bucketTs: bucketStart(time.Now().Unix()),
	}
	actual, _ := hotBuckets.LoadOrStore(key, &atomicBucket{})
	actual.(*atomicBucket).add(sample)
}

func Query(params QueryParams) (QueryResult, error) {
	if params.Hours <= 0 {
		params.Hours = 24
	}
	if params.Hours > 24*30 {
		params.Hours = 24 * 30
	}
	endTs := time.Now().Unix()
	startTs := endTs - int64(params.Hours)*3600

	merged := map[bucketKey]counters{}
	rows, err := model.GetPerfMetrics(params.Model, params.Group, startTs, endTs)
	if err != nil {
		return QueryResult{}, err
	}
	for _, row := range rows {
		mergeCounters(merged, bucketKey{
			model:    row.ModelName,
			group:    row.Group,
			bucketTs: row.BucketTs,
		}, counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			ttftSumMs:      row.TtftSumMs,
			ttftCount:      row.TtftCount,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		})
	}

	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.model != params.Model || k.bucketTs < startTs || k.bucketTs > endTs {
			return true
		}
		if params.Group != "" && k.group != params.Group {
			return true
		}
		mergeCounters(merged, k, value.(*atomicBucket).snapshot())
		return true
	})

	return buildQueryResult(params.Model, merged), nil
}

func QuerySummaryAll(hours int, groups []string) (SummaryAllResult, error) {
	if hours <= 0 {
		hours = 24
	}
	if hours > 24*30 {
		hours = 24 * 30
	}
	endTs := time.Now().Unix()
	startTs := endTs - int64(hours)*3600
	allowedGroups := allowedGroupSet(groups)

	rows, err := model.GetPerfMetricsSummaryBucketsAll(startTs, endTs, groups)
	if err != nil {
		return SummaryAllResult{}, err
	}

	totals := map[string]counters{}
	modelBuckets := map[string]map[int64]counters{}
	for _, row := range rows {
		value := counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		}
		mergeModelTotals(totals, row.ModelName, value)
		mergeModelBucket(modelBuckets, row.ModelName, row.BucketTs, value)
	}

	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.bucketTs < startTs || k.bucketTs > endTs {
			return true
		}
		if allowedGroups != nil {
			if _, ok := allowedGroups[k.group]; !ok {
				return true
			}
		}
		snap := value.(*atomicBucket).snapshot()
		if snap.requestCount == 0 {
			return true
		}
		mergeModelTotals(totals, k.model, snap)
		mergeModelBucket(modelBuckets, k.model, k.bucketTs, snap)
		return true
	})

	models := make([]ModelSummary, 0, len(totals))
	for name, total := range totals {
		if total.requestCount == 0 {
			continue
		}
		avgLatency := total.totalLatencyMs / total.requestCount
		successRate := float64(total.successCount) / float64(total.requestCount) * 100
		avgTps := 0.0
		if total.generationMs > 0 {
			avgTps = float64(total.outputTokens) / (float64(total.generationMs) / 1000.0)
		}
		models = append(models, ModelSummary{
			ModelName:          name,
			AvgLatencyMs:       avgLatency,
			SuccessRate:        math.Round(successRate*100) / 100,
			AvgTps:             math.Round(avgTps*100) / 100,
			RecentSuccessRates: recentSuccessRates(modelBuckets[name], 3),
			RequestCount:       total.requestCount,
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].RequestCount > models[j].RequestCount
	})

	return SummaryAllResult{Models: models}, nil
}

func mergeModelTotals(totals map[string]counters, modelName string, value counters) {
	if value.requestCount == 0 {
		return
	}
	current := totals[modelName]
	current.requestCount += value.requestCount
	current.successCount += value.successCount
	current.totalLatencyMs += value.totalLatencyMs
	current.ttftSumMs += value.ttftSumMs
	current.ttftCount += value.ttftCount
	current.outputTokens += value.outputTokens
	current.generationMs += value.generationMs
	totals[modelName] = current
}

func mergeModelBucket(modelBuckets map[string]map[int64]counters, modelName string, bucketTs int64, value counters) {
	if value.requestCount == 0 {
		return
	}
	if _, ok := modelBuckets[modelName]; !ok {
		modelBuckets[modelName] = map[int64]counters{}
	}
	current := modelBuckets[modelName][bucketTs]
	current.requestCount += value.requestCount
	current.successCount += value.successCount
	current.totalLatencyMs += value.totalLatencyMs
	current.ttftSumMs += value.ttftSumMs
	current.ttftCount += value.ttftCount
	current.outputTokens += value.outputTokens
	current.generationMs += value.generationMs
	modelBuckets[modelName][bucketTs] = current
}

func recentSuccessRates(buckets map[int64]counters, limit int) []float64 {
	if len(buckets) == 0 || limit <= 0 {
		return nil
	}
	timestamps := make([]int64, 0, len(buckets))
	for ts := range buckets {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})
	if len(timestamps) > limit {
		timestamps = timestamps[len(timestamps)-limit:]
	}
	rates := make([]float64, 0, len(timestamps))
	for _, ts := range timestamps {
		rates = append(rates, math.Round(successRate(buckets[ts])*100)/100)
	}
	return rates
}

func allowedGroupSet(groups []string) map[string]struct{} {
	if groups == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		allowed[group] = struct{}{}
	}
	return allowed
}

func bucketStart(ts int64) int64 {
	bucketSeconds := perf_metrics_setting.GetBucketSeconds()
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	return ts - (ts % bucketSeconds)
}

func mergeCounters(merged map[bucketKey]counters, key bucketKey, value counters) {
	if value.requestCount == 0 {
		return
	}
	current := merged[key]
	current.requestCount += value.requestCount
	current.successCount += value.successCount
	current.totalLatencyMs += value.totalLatencyMs
	current.ttftSumMs += value.ttftSumMs
	current.ttftCount += value.ttftCount
	current.outputTokens += value.outputTokens
	current.generationMs += value.generationMs
	merged[key] = current
}

func buildQueryResult(modelName string, merged map[bucketKey]counters) QueryResult {
	groupBuckets := map[string]map[int64]counters{}
	for key, value := range merged {
		if value.requestCount == 0 {
			continue
		}
		if _, ok := groupBuckets[key.group]; !ok {
			groupBuckets[key.group] = map[int64]counters{}
		}
		groupBuckets[key.group][key.bucketTs] = value
	}

	groups := make([]string, 0, len(groupBuckets))
	for group := range groupBuckets {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	results := make([]GroupResult, 0, len(groups))
	for _, group := range groups {
		buckets := groupBuckets[group]
		timestamps := make([]int64, 0, len(buckets))
		for ts := range buckets {
			timestamps = append(timestamps, ts)
		}
		sort.Slice(timestamps, func(i, j int) bool {
			return timestamps[i] < timestamps[j]
		})

		total := counters{}
		series := make([]BucketPoint, 0, len(timestamps))
		for _, ts := range timestamps {
			value := buckets[ts]
			total.requestCount += value.requestCount
			total.successCount += value.successCount
			total.totalLatencyMs += value.totalLatencyMs
			total.ttftSumMs += value.ttftSumMs
			total.ttftCount += value.ttftCount
			total.outputTokens += value.outputTokens
			total.generationMs += value.generationMs
			series = append(series, bucketPoint(ts, value))
		}

		results = append(results, GroupResult{
			Group:        group,
			AvgTtftMs:    avg(total.ttftSumMs, total.ttftCount),
			AvgLatencyMs: avg(total.totalLatencyMs, total.requestCount),
			SuccessRate:  successRate(total),
			AvgTps:       avgTps(total),
			Series:       series,
		})
	}

	return QueryResult{
		ModelName:    modelName,
		SeriesSchema: seriesSchema,
		Groups:       results,
	}
}

func bucketPoint(ts int64, value counters) BucketPoint {
	return BucketPoint{
		Ts:           ts,
		AvgTtftMs:    avg(value.ttftSumMs, value.ttftCount),
		AvgLatencyMs: avg(value.totalLatencyMs, value.requestCount),
		SuccessRate:  successRate(value),
		AvgTps:       avgTps(value),
	}
}

func avg(sum int64, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return sum / count
}

func successRate(value counters) float64 {
	if value.requestCount <= 0 {
		return 0
	}
	return float64(value.successCount) / float64(value.requestCount) * 100
}

func avgTps(value counters) float64 {
	if value.outputTokens <= 0 || value.generationMs <= 0 {
		return 0
	}
	return float64(value.outputTokens) / (float64(value.generationMs) / 1000)
}
