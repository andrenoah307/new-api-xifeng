package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMonitoringControllerTestDB(t *testing.T, migrate bool) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMainType := common.MainDatabaseType()
	oldLogType := common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	if migrate {
		require.NoError(t, db.AutoMigrate(&model.MonitoringHistory{}, &model.GroupMonitoringStat{}))
	}

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled = oldRedisEnabled
	})

	return db
}

func saveMonitoringSettings(t *testing.T) map[string]string {
	t.Helper()
	saved := make(map[string]string)
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "group_monitoring_setting.") || strings.HasPrefix(key, "region_restriction.") {
			saved[key] = value
		}
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		operation_setting.RebuildRegionRestrictionIndex()
	})
	return saved
}

func configureMonitoringSettings(t *testing.T, enabled bool, groups []string, regionEnabled bool, filterConsole bool, blockedGroups map[string][]string) {
	t.Helper()
	groupsJSON, err := common.Marshal(groups)
	require.NoError(t, err)
	blockedGroupsJSON, err := common.Marshal(blockedGroups)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_monitoring_setting.enabled":                      fmt.Sprintf("%t", enabled),
		"group_monitoring_setting.monitoring_groups":            string(groupsJSON),
		"group_monitoring_setting.availability_period_minutes":  "60",
		"group_monitoring_setting.aggregation_interval_minutes": "5",
		"region_restriction.enabled":                            fmt.Sprintf("%t", regionEnabled),
		"region_restriction.filter_console":                     fmt.Sprintf("%t", filterConsole),
		"region_restriction.blocked_groups":                     string(blockedGroupsJSON),
	}))
	operation_setting.RebuildRegionRestrictionIndex()
}

func monitoringTestContext(method, path, group, country string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, nil)
	if group != "" {
		c.Params = gin.Params{{Key: "group", Value: group}}
	}
	if country != "" {
		c.Request.Header.Set("Cf-Ipcountry", country)
	}
	return c, recorder
}

func decodeMonitoringResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func TestPublicMonitoringHistoryRegionBlockedMatchesUnknownGroup(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, true)
	saveMonitoringSettings(t)
	configureMonitoringSettings(t, true, []string{"blocked-cn", "visible-group"}, true, true, map[string][]string{
		"CN": {"blocked-cn"},
	})
	require.NoError(t, db.Create(&model.MonitoringHistory{
		GroupName:        "blocked-cn",
		AvailabilityRate: 0.9,
		CacheHitRate:     0.8,
		AvgFRT:           120,
		RequestCount:     3,
		RecordedAt:       time.Now().Unix(),
	}).Error)

	blockedContext, blockedRecorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups/blocked-cn/history", "blocked-cn", "CN")
	GetPublicMonitoringGroupHistory(blockedContext)

	unknownContext, unknownRecorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups/not-configured/history", "not-configured", "CN")
	GetPublicMonitoringGroupHistory(unknownContext)

	require.Equal(t, http.StatusForbidden, blockedRecorder.Code)
	require.Equal(t, http.StatusForbidden, unknownRecorder.Code)
	require.Equal(t, unknownRecorder.Body.String(), blockedRecorder.Body.String())
}

func TestMonitoringHistoryPublicProjectionKeepsAdminFields(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, true)
	saveMonitoringSettings(t)
	configureMonitoringSettings(t, true, []string{"visible-group"}, false, true, nil)
	require.NoError(t, db.Create(&model.MonitoringHistory{
		GroupName:        "visible-group",
		AvailabilityRate: 0.91,
		CacheHitRate:     0.73,
		AvgFRT:           88,
		RequestCount:     12,
		RecordedAt:       time.Now().Unix(),
	}).Error)

	publicContext, publicRecorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups/visible-group/history", "visible-group", "")
	GetPublicMonitoringGroupHistory(publicContext)
	require.Equal(t, http.StatusOK, publicRecorder.Code)
	publicPayload := decodeMonitoringResponse(t, publicRecorder)
	publicData, ok := publicPayload["data"].([]any)
	require.True(t, ok)
	require.Len(t, publicData, 1)
	publicRecord, ok := publicData[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, publicRecord, "recorded_at")
	assert.Contains(t, publicRecord, "availability_rate")
	assert.Contains(t, publicRecord, "cache_hit_rate")
	assert.Contains(t, publicRecord, "avg_frt")
	assert.NotContains(t, publicRecord, "id")
	assert.NotContains(t, publicRecord, "group_name")
	assert.NotContains(t, publicRecord, "request_count")

	adminContext, adminRecorder := monitoringTestContext(http.MethodGet, "/api/monitoring/admin/groups/visible-group/history", "visible-group", "")
	GetAdminMonitoringGroupHistory(adminContext)
	require.Equal(t, http.StatusOK, adminRecorder.Code)
	adminPayload := decodeMonitoringResponse(t, adminRecorder)
	adminData, ok := adminPayload["data"].([]any)
	require.True(t, ok)
	require.Len(t, adminData, 1)
	adminRecord, ok := adminData[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, adminRecord, "id")
	assert.Contains(t, adminRecord, "group_name")
	assert.Contains(t, adminRecord, "request_count")
}

func TestPublicMonitoringDisabledShortCircuitsBeforeDatabase(t *testing.T) {
	setupMonitoringControllerTestDB(t, false)
	saveMonitoringSettings(t)
	configureMonitoringSettings(t, false, []string{"configured-group"}, false, true, nil)

	groupsContext, groupsRecorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups", "", "")
	require.NotPanics(t, func() {
		GetPublicMonitoringGroups(groupsContext)
	})
	require.Equal(t, http.StatusServiceUnavailable, groupsRecorder.Code)
	groupsPayload := decodeMonitoringResponse(t, groupsRecorder)
	assert.Equal(t, false, groupsPayload["success"])
	assert.Equal(t, "分组监控功能未启用", groupsPayload["message"])

	historyContext, historyRecorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups/configured-group/history", "configured-group", "")
	require.NotPanics(t, func() {
		GetPublicMonitoringGroupHistory(historyContext)
	})
	require.Equal(t, http.StatusServiceUnavailable, historyRecorder.Code)
	historyPayload := decodeMonitoringResponse(t, historyRecorder)
	assert.Equal(t, false, historyPayload["success"])
	assert.Equal(t, "分组监控功能未启用", historyPayload["message"])
}

func TestPublicMonitoringGroupsRegionBlockedGroupsAreDesensitizedAndFiltered(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, true)
	saveMonitoringSettings(t)
	configureMonitoringSettings(t, true, []string{"blocked-cn", "visible-group"}, true, true, map[string][]string{
		"CN": {"blocked-cn"},
	})
	require.NoError(t, db.Create(&model.GroupMonitoringStat{
		GroupName:        "blocked-cn",
		AvailabilityRate: 0.91,
		CacheHitRate:     0.82,
		AvgFRT:           120,
		OnlineChannels:   2,
		TotalChannels:    3,
	}).Error)
	require.NoError(t, db.Create(&model.GroupMonitoringStat{
		GroupName:        "visible-group",
		AvailabilityRate: 0.93,
		CacheHitRate:     0.84,
		AvgFRT:           100,
		OnlineChannels:   2,
		TotalChannels:    3,
	}).Error)

	context, recorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups", "", "CN")
	GetPublicMonitoringGroups(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeMonitoringResponse(t, recorder)
	data, ok := payload["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	record, ok := data[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "visible-group", record["group_name"])
	assert.Contains(t, record, "is_online")
	assert.Equal(t, true, record["is_online"])
	assert.NotContains(t, record, "online_channels")
	assert.NotContains(t, record, "total_channels")
}

func TestPublicMonitoringGroupsWithEmptyCountryKeepsAllGroups(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, true)
	saveMonitoringSettings(t)
	configureMonitoringSettings(t, true, []string{"visible-group", "blocked-group"}, true, true, map[string][]string{
		"CN": {"blocked-group"},
	})
	require.NoError(t, db.Create(&model.GroupMonitoringStat{
		GroupName:        "visible-group",
		AvailabilityRate: 0.93,
		CacheHitRate:     0.84,
		AvgFRT:           100,
		OnlineChannels:   2,
		TotalChannels:    3,
	}).Error)
	require.NoError(t, db.Create(&model.GroupMonitoringStat{
		GroupName:        "blocked-group",
		AvailabilityRate: 0.91,
		CacheHitRate:     0.82,
		AvgFRT:           120,
		OnlineChannels:   2,
		TotalChannels:    3,
	}).Error)

	context, recorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups", "", "")
	GetPublicMonitoringGroups(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeMonitoringResponse(t, recorder)
	data, ok := payload["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 2)
	groupNames := make([]string, 0, len(data))
	for _, item := range data {
		record, ok := item.(map[string]any)
		require.True(t, ok)
		name, ok := record["group_name"].(string)
		require.True(t, ok)
		groupNames = append(groupNames, name)
	}
	assert.ElementsMatch(t, []string{"visible-group", "blocked-group"}, groupNames)
}

func TestPublicMonitoringGroupsAllRegionBlockedReturnsEmptyData(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, true)
	saveMonitoringSettings(t)
	configureMonitoringSettings(t, true, []string{"blocked-one", "blocked-two"}, true, true, map[string][]string{
		"CN": {"*"},
	})
	require.NoError(t, db.Create(&model.GroupMonitoringStat{
		GroupName:        "blocked-one",
		AvailabilityRate: 0.91,
		CacheHitRate:     0.82,
		AvgFRT:           120,
		OnlineChannels:   2,
		TotalChannels:    3,
	}).Error)
	require.NoError(t, db.Create(&model.GroupMonitoringStat{
		GroupName:        "blocked-two",
		AvailabilityRate: 0.93,
		CacheHitRate:     0.84,
		AvgFRT:           100,
		OnlineChannels:   2,
		TotalChannels:    3,
	}).Error)

	context, recorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups", "", "CN")
	GetPublicMonitoringGroups(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeMonitoringResponse(t, recorder)
	data, ok := payload["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 0)
}

func TestPublicMonitoringGroupsReturnsFailureWhenStatsQueryFails(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, true)
	saveMonitoringSettings(t)
	configureMonitoringSettings(t, true, []string{"visible-group"}, false, true, nil)
	require.NoError(t, db.Migrator().DropTable(&model.GroupMonitoringStat{}))

	context, recorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups", "", "")
	GetPublicMonitoringGroups(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeMonitoringResponse(t, recorder)
	assert.Equal(t, false, payload["success"])
	assert.Equal(t, "获取监控数据失败", payload["message"])
}

func TestPublicMonitoringHistoryReturnsFailureWhenHistoryQueryFails(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, true)
	saveMonitoringSettings(t)
	configureMonitoringSettings(t, true, []string{"visible-group"}, false, true, nil)
	require.NoError(t, db.Create(&model.GroupMonitoringStat{GroupName: "visible-group"}).Error)
	require.NoError(t, db.Migrator().DropTable(&model.MonitoringHistory{}))

	context, recorder := monitoringTestContext(http.MethodGet, "/api/monitoring/public/groups/visible-group/history", "visible-group", "")
	GetPublicMonitoringGroupHistory(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeMonitoringResponse(t, recorder)
	assert.Equal(t, false, payload["success"])
	assert.Equal(t, "获取历史数据失败", payload["message"])
}
