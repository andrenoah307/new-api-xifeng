package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func applyMonitoringSettings(t *testing.T, values map[string]string) {
	t.Helper()
	require.NoError(t, config.GlobalConfig.LoadFromDB(values))
}

func getPerfCardData(t *testing.T) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/monitoring/admin/model-performance", nil)
	GetAdminMonitoringGroupModels(c)
	require.Equal(t, http.StatusOK, w.Code)

	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(w.Body.String(), &payload))
	require.Equal(t, true, payload["success"], w.Body.String())
	return payload["data"].(map[string]any)
}

// 白名单语义的端到端契约：只有被勾选的分组才返回模型性能数据。
// 未勾选的分组即便在监控列表里、即便有数据，也必须完全不出现。
func TestGetAdminMonitoringGroupModels_WhitelistOnly(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)

	ts := time.Now().Unix() - 1800
	for _, group := range []string{"picked", "skipped"} {
		require.NoError(t, db.Create(&model.PerfMetric{
			ModelName: "m", Group: group, BucketTs: ts,
			RequestCount: 4, SuccessCount: 4, TotalLatencyMs: 400,
		}).Error)
	}

	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["picked","skipped"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_groups":  `["picked"]`,
	})

	data := getPerfCardData(t)
	assert.Equal(t, true, data["enabled"])
	groups := data["groups"].(map[string]any)
	assert.Contains(t, groups, "picked")
	assert.NotContains(t, groups, "skipped", "未加入白名单的分组不得返回")
}

// 白名单为空 = 一个分组都不展示。黑名单时代空集合意味着"全部放行"，
// 语义反转后如果沿用旧判断，管理员一开总开关就会把全部分组暴露出来。
func TestGetAdminMonitoringGroupModels_EmptyWhitelistReturnsNothing(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)

	require.NoError(t, db.Create(&model.PerfMetric{
		ModelName: "m", Group: "picked", BucketTs: time.Now().Unix() - 1800,
		RequestCount: 4, SuccessCount: 4, TotalLatencyMs: 400,
	}).Error)

	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["picked"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_groups":  `[]`,
	})

	data := getPerfCardData(t)
	assert.Empty(t, data["groups"])
}

// 前端要按槽位画缩略图，必须同时知道每个槽位代表多长时间、一共有几个槽位。
// 槽数不再是定长 24：它随窗口与 bucket 宽度一起变化，所以必须随响应下发，
// 且 series 的实际长度必须与下发的槽数一致——否则前端只能靠猜。
func TestGetAdminMonitoringGroupModels_ExposesSeriesLayout(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)

	require.NoError(t, db.Create(&model.PerfMetric{
		ModelName: "m", Group: "picked", BucketTs: time.Now().Unix() - 1800,
		RequestCount: 4, SuccessCount: 3, TotalLatencyMs: 400,
	}).Error)

	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["picked"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_groups":  `["picked"]`,
		"perf_metrics_setting.bucket_time":           "minute",
	})

	data := getPerfCardData(t)
	assert.Equal(t, float64(1), data["window_hours"])
	// 1 小时窗口 / 上限 24 槽 = 150 秒，但槽宽必须是 bucket 宽度（60 秒）的整数倍，
	// 于是向上取整到 180 秒，槽数随之变成 20。
	assert.Equal(t, float64(180), data["series_slot_seconds"])
	assert.Equal(t, float64(20), data["series_slots"])

	items := data["groups"].(map[string]any)["picked"].([]any)
	require.Len(t, items, 1)
	series := items[0].(map[string]any)["series"].([]any)
	assert.Len(t, series, 20, "series 长度必须与下发的槽数一致")
}

// 前端要靠 enabled_groups 决定哪些分组置顶成单列宽卡片。
// 如果只能从 groups 的键名反推，那么一个"已加入白名单但当前窗口没有流量"的分组
// 会悄悄掉回多列网格，管理员会以为白名单没生效。
func TestGetAdminMonitoringGroupModels_ExposesEnabledGroupsIncludingSilentOnes(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)

	require.NoError(t, db.Create(&model.PerfMetric{
		ModelName: "m", Group: "busy", BucketTs: time.Now().Unix() - 1800,
		RequestCount: 4, SuccessCount: 4, TotalLatencyMs: 400,
	}).Error)

	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["busy","silent","excluded"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_groups":  `["busy","silent"]`,
	})

	data := getPerfCardData(t)
	assert.Equal(t, []any{"busy", "silent"}, data["enabled_groups"], "白名单顺序即置顶顺序，没有流量的分组也要在列表里")
	assert.NotContains(t, data["groups"].(map[string]any), "silent", "没有样本的分组不应伪造出模型行")
}

// 白名单为空时 enabled_groups 必须是空数组而不是 null：
// 前端 Object/数组解构拿到 null 会直接抛错，整块监控页白屏。
func TestGetAdminMonitoringGroupModels_EmptyWhitelistReturnsEmptyEnabledGroups(t *testing.T) {
	setupMonitoringControllerTestDB(t, false)
	saveMonitoringSettings(t)
	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["a"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_groups":  `[]`,
	})

	data := getPerfCardData(t)
	assert.Equal(t, []any{}, data["enabled_groups"])
}
