package controller

import (
	"sort"
	"strings"
	"sync/atomic"
	"time"

	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"golang.org/x/sync/singleflight"
)

// perf_card 取数 = 一次聚合 SQL + 一次热桶全量遍历，原本每个请求重算一遍。
// 该端点对普通用户放开后并发量是管理员侧的数百倍，所以取数结果在进程内缓存
// 一小段时间，并用 singleflight 合并 TTL 失效瞬间的并发回源。
const perfCardSnapshotTTL = 30 * time.Second

// 快照按"配置签名"生效而不是纯按时间：管理员改动白名单后签名立即变化，
// 新配置不会被上一份快照的 TTL 挡住。同一时刻只可能有一份有效配置，
// 因此单槽位即可，不存在按签名累积的缓存条目。
type perfCardSnapshot struct {
	signature string
	groups    map[string][]perfmetrics.GroupModelPerf
	expireAt  time.Time
}

var (
	perfCardCache  atomic.Pointer[perfCardSnapshot]
	perfCardFlight singleflight.Group
)

// perfCardWhitelistedGroups 返回配置里勾选的分组，不做地区过滤。
// 地区过滤留到快照之后按请求做，否则缓存键会按访问者地区爆炸。
func perfCardWhitelistedGroups(cfg operation_setting.GroupMonitoringSetting) []string {
	groups := make([]string, 0, len(cfg.MonitoringGroups))
	for _, group := range cfg.MonitoringGroups {
		if cfg.IsPerfCardGroupEnabled(group) {
			groups = append(groups, group)
		}
	}
	return groups
}

func perfCardSignature(cfg operation_setting.GroupMonitoringSetting, groups []string) string {
	var sb strings.Builder
	for _, group := range groups {
		sb.WriteString(group)
		sb.WriteByte('\x1f')
		models := append([]string(nil), cfg.PerfCardModelsForGroup(group)...)
		sort.Strings(models)
		sb.WriteString(strings.Join(models, ","))
		sb.WriteByte('\x1e')
	}
	return sb.String()
}

// perfCardGroupModels 取一份已应用分组与模型白名单的模型性能数据。
// 返回的 map 与其中的切片是共享快照，调用方只读；需要裁剪或脱敏时必须另建容器。
func perfCardGroupModels(cfg operation_setting.GroupMonitoringSetting, groups []string) (map[string][]perfmetrics.GroupModelPerf, error) {
	signature := perfCardSignature(cfg, groups)
	if snap := perfCardCache.Load(); snap != nil && snap.signature == signature && time.Now().Before(snap.expireAt) {
		return snap.groups, nil
	}

	loaded, err, _ := perfCardFlight.Do(signature, func() (any, error) {
		data, err := perfmetrics.QueryGroupModelSummary(perfCardWindowHours, groups)
		if err != nil {
			return nil, err
		}
		for group, models := range data {
			allowed := cfg.PerfCardModelsForGroup(group)
			if allowed == nil {
				continue
			}
			set := make(map[string]bool, len(allowed))
			for _, name := range allowed {
				set[name] = true
			}
			filtered := models[:0]
			for _, item := range models {
				if set[item.ModelName] {
					filtered = append(filtered, item)
				}
			}
			data[group] = filtered
		}
		perfCardCache.Store(&perfCardSnapshot{
			signature: signature,
			groups:    data,
			expireAt:  time.Now().Add(perfCardSnapshotTTL),
		})
		return data, nil
	})
	if err != nil {
		return nil, err
	}
	return loaded.(map[string][]perfmetrics.GroupModelPerf), nil
}

// publicGroupModelPerf 是模型性能对普通用户的投影。
// 它刻意写成独立结构体而非复用上游类型：往 perfmetrics.GroupModelPerf 加字段时
// 不会自动流向公开端点，必须有人显式在这里加一行。
type publicGroupModelPerf struct {
	ModelName    string     `json:"model_name"`
	SuccessRate  float64    `json:"success_rate"`
	AvgLatencyMs int64      `json:"avg_latency_ms"`
	AvgTtftMs    int64      `json:"avg_ttft_ms"`
	HasTtft      bool       `json:"has_ttft"`
	AvgTps       float64    `json:"avg_tps"`
	Series       []*float64 `json:"series"`
	TtftSeries   []*int64   `json:"ttft_series,omitempty"`
}

// desensitizeGroupModelPerf 剥离 request_count：它是唯一直接暴露真实业务量的字段，
// 与 desensitizeGroupStat 把渠道数折成布尔同一判据——服务质量可公开，生意规模不可。
func desensitizeGroupModelPerf(items []perfmetrics.GroupModelPerf) []publicGroupModelPerf {
	projected := make([]publicGroupModelPerf, 0, len(items))
	for _, item := range items {
		projected = append(projected, publicGroupModelPerf{
			ModelName:    item.ModelName,
			SuccessRate:  item.SuccessRate,
			AvgLatencyMs: item.AvgLatencyMs,
			AvgTtftMs:    item.AvgTtftMs,
			HasTtft:      item.HasTtft,
			AvgTps:       item.AvgTps,
			Series:       item.Series,
			TtftSeries:   item.TtftSeries,
		})
	}
	return projected
}
