package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type GroupMonitoringSetting struct {
	Enabled                        bool     `json:"enabled"`
	MonitoringGroups               []string `json:"monitoring_groups"`
	AvailabilityPeriodMinutes      int      `json:"availability_period_minutes"`
	CacheHitPeriodMinutes          int      `json:"cache_hit_period_minutes"`
	AvailabilityExcludeModels      []string `json:"availability_exclude_models"`
	CacheHitExcludeModels          []string `json:"cache_hit_exclude_models"`
	AvailabilityExcludeKeywords    []string `json:"availability_exclude_keywords"`
	AvailabilityExcludeStatusCodes []int    `json:"availability_exclude_status_codes"`
	GroupDisplayOrder              []string `json:"group_display_order"`
	AggregationIntervalMinutes     int      `json:"aggregation_interval_minutes"`
	CacheTokensSeparateGroups      []string `json:"cache_tokens_separate_groups"`
	FRTExcludeThresholdSeconds     float64  `json:"frt_exclude_threshold_seconds"`
}

var groupMonitoringSetting = GroupMonitoringSetting{
	Enabled:                        true,
	MonitoringGroups:               []string{},
	AvailabilityPeriodMinutes:      60,
	CacheHitPeriodMinutes:          60,
	AvailabilityExcludeModels:      []string{},
	CacheHitExcludeModels:          []string{},
	AvailabilityExcludeKeywords:    []string{},
	AvailabilityExcludeStatusCodes: []int{},
	GroupDisplayOrder:              []string{},
	AggregationIntervalMinutes:     5,
	CacheTokensSeparateGroups:      []string{},
	FRTExcludeThresholdSeconds:     0,
}

func init() {
	config.GlobalConfig.Register("group_monitoring_setting", &groupMonitoringSetting)
}

func GetGroupMonitoringSetting() GroupMonitoringSetting {
	return groupMonitoringSetting
}

// IsUserParamFailure 判断一次失败请求是否命中"排除状态码/关键词"（视为用户参数问题，
// 不应计入可用率/成功率的分母）。仅对失败请求有意义。分组监控与模型广场性能页共用此判定。
func (s GroupMonitoringSetting) IsUserParamFailure(statusCode int, content string) bool {
	if statusCode > 0 {
		for _, sc := range s.AvailabilityExcludeStatusCodes {
			if sc == statusCode {
				return true
			}
		}
	}
	if content != "" && len(s.AvailabilityExcludeKeywords) > 0 {
		lc := strings.ToLower(content)
		for _, kw := range s.AvailabilityExcludeKeywords {
			if strings.Contains(lc, strings.ToLower(kw)) {
				return true
			}
		}
	}
	return false
}
