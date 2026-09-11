package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/pkg/requestip"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"

	"github.com/gin-gonic/gin"
)

func GetAdminMonitoringGroups(c *gin.Context) {
	setting := operation_setting.GetGroupMonitoringSetting()
	monitoringGroups := filterRegionBlockedGroupNames(c, setting.MonitoringGroups)
	if len(monitoringGroups) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    []interface{}{},
		})
		return
	}

	stats, err := model.GetGroupMonitoringStatsByNames(monitoringGroups)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取监控数据失败: " + err.Error(),
		})
		return
	}

	orderedStats := orderGroupStats(stats, setting.GroupDisplayOrder)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    orderedStats,
	})
}

func GetAdminMonitoringGroupModels(c *gin.Context) {
	cfg := operation_setting.GetGroupMonitoringSetting()
	if !cfg.Enabled || !cfg.PerfCardEnabled {
		respondPerfCardDisabled(c)
		return
	}
	respondPerfCard(c, cfg, false)
}

// GetPublicMonitoringGroupModels 是模型性能卡片的公开端点。
// 与管理员端点共用取数快照与白名单语义，只在写出前多一层脱敏投影；
// 是否放开由管理员在设置里显式勾选，缺省关闭。
func GetPublicMonitoringGroupModels(c *gin.Context) {
	cfg := operation_setting.GetGroupMonitoringSetting()
	if !cfg.Enabled || !cfg.PerfCardEnabled || !cfg.PerfCardPublic {
		respondPerfCardDisabled(c)
		return
	}
	respondPerfCard(c, cfg, true)
}

// perf_card 的统计窗口。1 小时要求底层 bucket 宽度细于 1 小时才有意义——
// 槽宽与槽数由 GroupModelSeriesLayout 从窗口和 bucket 宽度共同推出。
const perfCardWindowHours = 1

func respondPerfCardDisabled(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"enabled": false, "enabled_groups": []string{}, "groups": gin.H{}}})
}

func perfCardEnvelope(cfg operation_setting.GroupMonitoringSetting, visible []string, groups any) gin.H {
	// 槽数不再是定长 24：它随窗口与 bucket 宽度变化，必须随响应一起下发，
	// 前端不得再假定 series 的长度。
	slotSeconds, slots := perfmetrics.GroupModelSeriesLayout(perfCardWindowHours)
	return gin.H{
		"enabled":             true,
		"window_hours":        perfCardWindowHours,
		"bucket_seconds":      perf_metrics_setting.GetBucketSeconds(),
		"series_slot_seconds": slotSeconds,
		"series_slots":        slots,
		"enabled_groups":      visible,
		"show_all_models":     cfg.PerfCardShowAllModels,
		"top_n":               cfg.PerfCardTopNOrDefault(),
		"groups":              groups,
	}
}

// respondPerfCard 是两个端点唯一的取数与组装路径。
// 白名单过滤先于快照、地区过滤后于快照：两者都是分组名集合求交，顺序不影响结果，
// 但这样快照与访问者地区无关，缓存不会按地区分裂。
func respondPerfCard(c *gin.Context, cfg operation_setting.GroupMonitoringSetting, public bool) {
	whitelisted := perfCardWhitelistedGroups(cfg)
	visible := filterRegionBlockedGroupNames(c, whitelisted)
	if len(visible) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": perfCardEnvelope(cfg, visible, gin.H{})})
		return
	}

	snapshot, err := perfCardGroupModels(cfg, whitelisted)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取模型性能数据失败"})
		return
	}

	groups := make(map[string]any, len(visible))
	for _, group := range visible {
		items, ok := snapshot[group]
		if !ok {
			continue
		}
		if public {
			groups[group] = desensitizeGroupModelPerf(items)
		} else {
			groups[group] = items
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": perfCardEnvelope(cfg, visible, groups)})
}

func GetAdminMonitoringGroupsHistoryBatch(c *gin.Context) {
	cfg := operation_setting.GetGroupMonitoringSetting()
	groups := filterRegionBlockedGroupNames(c, cfg.MonitoringGroups)
	end := time.Now().Unix()
	start := end - int64(cfg.AvailabilityPeriodMinutes*60)
	history, err := model.GetMonitoringHistoryBatch(groups, start, end)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取历史数据失败"})
		return
	}
	seeds, err := model.GetLastMonitoringHistoryBeforeBatch(groups, start)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取历史数据失败"})
		return
	}
	for _, group := range groups {
		if seed, ok := seeds[group]; ok {
			seed.RecordedAt = start
			history[group] = append([]model.MonitoringHistory{seed}, history[group]...)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": history, "period_minutes": cfg.AvailabilityPeriodMinutes, "aggregation_interval_minutes": cfg.AggregationIntervalMinutes})
}

func GetAdminMonitoringGroupDetail(c *gin.Context) {
	groupName := c.Param("group")
	if groupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "分组名称不能为空",
		})
		return
	}

	groupStat, err := model.GetGroupMonitoringStatByName(groupName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "分组不存在或无监控数据",
		})
		return
	}

	channelStats, err := model.GetChannelMonitoringStatsByGroup(groupName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取渠道监控数据失败: " + err.Error(),
		})
		return
	}

	activeChannels, err := model.GetAllChannelsByGroup(groupName)
	if err == nil {
		activeSet := make(map[int]bool, len(activeChannels))
		channelNameMap := make(map[int]string, len(activeChannels))
		channelStatusMap := make(map[int]int, len(activeChannels))
		for _, ch := range activeChannels {
			activeSet[ch.Id] = true
			channelNameMap[ch.Id] = ch.Name
			channelStatusMap[ch.Id] = ch.Status
		}
		filtered := make([]model.ChannelMonitoringStat, 0, len(channelStats))
		seenChannels := make(map[int]bool, len(channelStats))
		for _, cs := range channelStats {
			if activeSet[cs.ChannelId] {
				cs.ChannelName = channelNameMap[cs.ChannelId]
				cs.ChannelStatus = channelStatusMap[cs.ChannelId]
				filtered = append(filtered, cs)
				seenChannels[cs.ChannelId] = true
			}
		}
		for _, ch := range activeChannels {
			if !seenChannels[ch.Id] {
				filtered = append(filtered, model.ChannelMonitoringStat{
					GroupName:        groupName,
					ChannelId:        ch.Id,
					ChannelName:      ch.Name,
					ChannelStatus:    ch.Status,
					AvailabilityRate: -1,
					CacheHitRate:     -1,
				})
			}
		}
		channelStats = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "",
		"data":          groupStat,
		"channel_stats": channelStats,
	})
}

func GetAdminMonitoringGroupHistory(c *gin.Context) {
	groupName := c.Param("group")
	if groupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "分组名称不能为空",
		})
		return
	}

	setting := operation_setting.GetGroupMonitoringSetting()
	endTime := time.Now().Unix()
	startTime := endTime - int64(setting.AvailabilityPeriodMinutes*60)

	history, err := model.GetMonitoringHistory(groupName, startTime, endTime)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取历史数据失败: " + err.Error(),
		})
		return
	}

	history = prependSeedRecord(groupName, startTime, history)

	c.JSON(http.StatusOK, gin.H{
		"success":                      true,
		"message":                      "",
		"data":                         history,
		"period_minutes":               setting.AvailabilityPeriodMinutes,
		"aggregation_interval_minutes": setting.AggregationIntervalMinutes,
	})
}

func RefreshMonitoringData(c *gin.Context) {
	if !operation_setting.GetGroupMonitoringSetting().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "分组监控已关闭，无法刷新",
		})
		return
	}
	ok := service.RebuildAggregationFromBuckets()
	if !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "聚合正在运行中，请稍后再试",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "刷新成功，历史数据已按段重建",
	})
}

func DeleteMonitoringGroupRecords(c *gin.Context) {
	groupName := c.Param("group")
	if groupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "分组名称不能为空",
		})
		return
	}

	totalDeleted, err := model.DeleteAllMonitoringDataForGroup(groupName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "清空记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "清空成功",
		"data": gin.H{
			"deleted_rows": totalDeleted,
		},
	})
}

func GetPublicMonitoringGroups(c *gin.Context) {
	setting := operation_setting.GetGroupMonitoringSetting()
	if !setting.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "分组监控功能未启用",
		})
		return
	}

	monitoringGroups := filterRegionBlockedGroupNames(c, setting.MonitoringGroups)
	if len(monitoringGroups) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    []interface{}{},
		})
		return
	}

	stats, err := model.GetGroupMonitoringStatsForPublic(monitoringGroups)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取监控数据失败",
		})
		return
	}

	desensitized := make([]gin.H, 0, len(stats))
	for _, s := range stats {
		desensitized = append(desensitized, desensitizeGroupStat(&s))
	}

	orderedData := orderDesensitizedStats(desensitized, setting.GroupDisplayOrder)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    orderedData,
	})
}

func GetPublicMonitoringGroupHistory(c *gin.Context) {
	setting := operation_setting.GetGroupMonitoringSetting()
	if !setting.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "分组监控功能未启用",
		})
		return
	}

	groupName := c.Param("group")
	if groupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "分组名称不能为空",
		})
		return
	}

	monitoringGroups := filterRegionBlockedGroupNames(c, setting.MonitoringGroups)
	found := false
	for _, g := range monitoringGroups {
		if g == groupName {
			found = true
			break
		}
	}
	if !found {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "该分组不在监控列表中",
		})
		return
	}

	endTime := time.Now().Unix()
	startTime := endTime - int64(setting.AvailabilityPeriodMinutes*60)

	history, err := model.GetMonitoringHistory(groupName, startTime, endTime)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取历史数据失败",
		})
		return
	}

	history = prependSeedRecord(groupName, startTime, history)
	records := make([]gin.H, 0, len(history))
	for _, h := range history {
		records = append(records, gin.H{
			"recorded_at":       h.RecordedAt,
			"availability_rate": h.AvailabilityRate,
			"cache_hit_rate":    h.CacheHitRate,
			"avg_frt":           h.AvgFRT,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":                      true,
		"message":                      "",
		"data":                         records,
		"period_minutes":               setting.AvailabilityPeriodMinutes,
		"aggregation_interval_minutes": setting.AggregationIntervalMinutes,
	})
}

func desensitizeGroupStat(stat *model.GroupMonitoringStat) gin.H {
	return gin.H{
		"group_name":        stat.GroupName,
		"availability_rate": stat.AvailabilityRate,
		"cache_hit_rate":    stat.CacheHitRate,
		"avg_response_time": stat.AvgResponseTime,
		"avg_frt":           stat.AvgFRT,
		"is_online":         stat.OnlineChannels > 0,
		"group_ratio":       stat.GroupRatio,
		"last_test_model":   stat.LastTestModel,
		"updated_at":        stat.UpdatedAt,
	}
}

func orderGroupStats(stats []model.GroupMonitoringStat, order []string) []model.GroupMonitoringStat {
	if len(order) == 0 {
		return stats
	}

	statMap := make(map[string]model.GroupMonitoringStat)
	for _, s := range stats {
		statMap[s.GroupName] = s
	}

	ordered := make([]model.GroupMonitoringStat, 0, len(stats))
	for _, name := range order {
		if s, ok := statMap[name]; ok {
			ordered = append(ordered, s)
			delete(statMap, name)
		}
	}

	for _, s := range statMap {
		ordered = append(ordered, s)
	}

	return ordered
}

func orderDesensitizedStats(stats []gin.H, order []string) []gin.H {
	if len(order) == 0 {
		return stats
	}

	statMap := make(map[string]gin.H)
	for _, s := range stats {
		if name, ok := s["group_name"].(string); ok {
			statMap[name] = s
		}
	}

	ordered := make([]gin.H, 0, len(stats))
	for _, name := range order {
		if s, ok := statMap[name]; ok {
			ordered = append(ordered, s)
			delete(statMap, name)
		}
	}

	for _, s := range statMap {
		ordered = append(ordered, s)
	}

	return ordered
}

func prependSeedRecord(groupName string, startTime int64, history []model.MonitoringHistory) []model.MonitoringHistory {
	seed, err := model.GetLastMonitoringHistoryBefore(groupName, startTime)
	if err != nil || seed == nil {
		return history
	}
	seed.RecordedAt = startTime
	return append([]model.MonitoringHistory{*seed}, history...)
}

func filterRegionBlockedGroupNames(c *gin.Context, groups []string) []string {
	rs := operation_setting.GetRegionRestrictionSetting()
	if !rs.Enabled || !rs.FilterConsole {
		return groups
	}
	cc := requestip.GetClientCountry(c)
	if cc == "" {
		return groups
	}
	filtered := make([]string, 0, len(groups))
	for _, g := range groups {
		if !operation_setting.IsGroupBlockedForCountry(cc, g) {
			filtered = append(filtered, g)
		}
	}
	return filtered
}
