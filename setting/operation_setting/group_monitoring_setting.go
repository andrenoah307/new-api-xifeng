package operation_setting

import (
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/setting/config"
)

type GroupMonitoringSetting struct {
	Enabled                        bool                `json:"enabled"`
	MonitoringGroups               []string            `json:"monitoring_groups"`
	ExcludedChannelIDs             []int               `json:"excluded_channel_ids"`
	ExcludedUserIDs                []int               `json:"excluded_user_ids"`
	AvailabilityPeriodMinutes      int                 `json:"availability_period_minutes"`
	CacheHitPeriodMinutes          int                 `json:"cache_hit_period_minutes"`
	AvailabilityExcludeModels      []string            `json:"availability_exclude_models"`
	CacheHitExcludeModels          []string            `json:"cache_hit_exclude_models"`
	CacheHitExcludeNonStream       bool                `json:"cache_hit_exclude_non_stream"`
	AvailabilityExcludeKeywords    []string            `json:"availability_exclude_keywords"`
	AvailabilityExcludeStatusCodes []int               `json:"availability_exclude_status_codes"`
	GroupDisplayOrder              []string            `json:"group_display_order"`
	AggregationIntervalMinutes     int                 `json:"aggregation_interval_minutes"`
	CacheTokensSeparateGroups      []string            `json:"cache_tokens_separate_groups"`
	FRTExcludeThresholdSeconds     float64             `json:"frt_exclude_threshold_seconds"`
	PerfCardEnabled                bool                `json:"perf_card_enabled"`
	PerfCardPublic                 bool                `json:"perf_card_public"`
	PerfCardShowAllModels          bool                `json:"perf_card_show_all_models"`
	PerfCardTopN                   int                 `json:"perf_card_top_n"`
	PerfCardGroups                 []string            `json:"perf_card_groups"`
	PerfCardGroupModels            map[string][]string `json:"perf_card_group_models"`
}

var groupMonitoringSetting = GroupMonitoringSetting{
	Enabled:                        true,
	MonitoringGroups:               []string{},
	ExcludedChannelIDs:             []int{},
	ExcludedUserIDs:                []int{},
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
	PerfCardEnabled:                true,
	PerfCardPublic:                 false,
	PerfCardShowAllModels:          false,
	PerfCardTopN:                   6,
	PerfCardGroups:                 []string{},
	PerfCardGroupModels:            map[string][]string{},
}

func (s GroupMonitoringSetting) IsPerfCardGroupEnabled(group string) bool {
	for _, value := range s.PerfCardGroups {
		if value == group {
			return true
		}
	}
	return false
}

func (s GroupMonitoringSetting) PerfCardModelsForGroup(group string) []string {
	if len(s.PerfCardGroupModels[group]) == 0 {
		return nil
	}
	return s.PerfCardGroupModels[group]
}

func (s GroupMonitoringSetting) PerfCardTopNOrDefault() int {
	if s.PerfCardTopN <= 0 {
		return 6
	}
	if s.PerfCardTopN > 50 {
		return 50
	}
	return s.PerfCardTopN
}

func init() {
	config.GlobalConfig.Register("group_monitoring_setting", &groupMonitoringSetting)
}

func GetGroupMonitoringSetting() GroupMonitoringSetting {
	return groupMonitoringSetting
}

type monitoringExclusions struct {
	channels map[int]struct{}
	users    map[int]struct{}
}

var monitoringExclusionIndex atomic.Pointer[monitoringExclusions]

// Rebuild on option updates, never on the request path. Published maps are immutable.
func RebuildMonitoringExclusionIndex() {
	index := &monitoringExclusions{
		channels: make(map[int]struct{}, len(groupMonitoringSetting.ExcludedChannelIDs)),
		users:    make(map[int]struct{}, len(groupMonitoringSetting.ExcludedUserIDs)),
	}
	for _, id := range groupMonitoringSetting.ExcludedChannelIDs {
		if id > 0 {
			index.channels[id] = struct{}{}
		}
	}
	for _, id := range groupMonitoringSetting.ExcludedUserIDs {
		if id > 0 {
			index.users[id] = struct{}{}
		}
	}
	monitoringExclusionIndex.Store(index)
}

func IsMonitoringExcluded(channelID, userID int) bool {
	index := monitoringExclusionIndex.Load()
	if index == nil {
		return false
	}
	_, channelExcluded := index.channels[channelID]
	_, userExcluded := index.users[userID]
	return channelExcluded || userExcluded
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
