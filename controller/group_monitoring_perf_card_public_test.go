package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getPublicPerfCardData(t *testing.T) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/monitoring/public/model-performance", nil)
	GetPublicMonitoringGroupModels(c)
	require.Equal(t, http.StatusOK, w.Code)

	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(w.Body.String(), &payload))
	require.Equal(t, true, payload["success"], w.Body.String())
	return payload["data"].(map[string]any)
}

func seedPerfCardMetric(t *testing.T, group, modelName string, requests, successes int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: modelName, Group: group, BucketTs: time.Now().Unix() - 1800,
		RequestCount: requests, SuccessCount: successes, TotalLatencyMs: 400,
		TtftSumMs: 200 * requests, TtftCount: requests,
	}).Error)
}

// 放开给普通用户是管理员的显式动作，不是升级的副作用。
// perf_card_public 缺省为假时，即便总开关和 perf_card 开关都开着、
// 管理员端点有数据，公开端点也必须什么都不给。
func TestGetPublicMonitoringGroupModels_HiddenUntilAdminOptsIn(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)
	seedPerfCardMetric(t, "picked", "m", 4, 4)

	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["picked"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_groups":  `["picked"]`,
	})

	assert.NotEmpty(t, getPerfCardData(t)["groups"], "管理员侧应当有数据，否则本用例无法证伪")

	data := getPublicPerfCardData(t)
	assert.Equal(t, false, data["enabled"])
	assert.Empty(t, data["groups"])
	assert.Empty(t, data["enabled_groups"])
}

// 脱敏契约：request_count 是唯一直接暴露真实业务量的字段，公开侧必须没有这个键；
// 其余七个字段是服务质量指标，必须完整保留，否则公开卡片会退化成空壳。
func TestGetPublicMonitoringGroupModels_StripsRequestCountKeepsQualityFields(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)
	seedPerfCardMetric(t, "picked", "m", 4, 3)

	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["picked"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_public":  "true",
		"group_monitoring_setting.perf_card_groups":  `["picked"]`,
	})

	data := getPublicPerfCardData(t)
	items := data["groups"].(map[string]any)["picked"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)

	assert.NotContains(t, item, "request_count", "真实业务量不得对普通用户暴露")
	for _, key := range []string{
		"model_name", "success_rate", "avg_latency_ms",
		"avg_ttft_ms", "has_ttft", "avg_tps", "series", "ttft_series",
	} {
		assert.Contains(t, item, key, "服务质量字段 %s 必须保留", key)
	}
	// 槽数随窗口与 bucket 宽度变化，公开侧同样必须自洽：两条序列的长度
	// 只能对着响应自己下发的槽数断言，不能硬编码。
	assert.Len(t, item["series"].([]any), int(data["series_slots"].(float64)))
	assert.Len(t, item["ttft_series"].([]any), int(data["series_slots"].(float64)))
}

// 回归护栏：脱敏只针对公开端点，管理员仍要看到请求量，否则运营失去容量判断依据。
func TestGetAdminMonitoringGroupModels_KeepsRequestCount(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)
	seedPerfCardMetric(t, "picked", "m", 4, 3)

	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["picked"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_groups":  `["picked"]`,
	})

	items := getPerfCardData(t)["groups"].(map[string]any)["picked"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, float64(4), items[0].(map[string]any)["request_count"])
}

// 公开端点复用同一份取数与白名单过滤，不得出现"公开侧多给了一个分组/模型"的漂移。
func TestGetPublicMonitoringGroupModels_HonorsGroupAndModelWhitelist(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)
	seedPerfCardMetric(t, "picked", "shown", 4, 4)
	seedPerfCardMetric(t, "picked", "hidden", 4, 4)
	seedPerfCardMetric(t, "skipped", "shown", 4, 4)

	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":                "true",
		"group_monitoring_setting.monitoring_groups":      `["picked","skipped"]`,
		"group_monitoring_setting.perf_card_enabled":      "true",
		"group_monitoring_setting.perf_card_public":       "true",
		"group_monitoring_setting.perf_card_groups":       `["picked"]`,
		"group_monitoring_setting.perf_card_group_models": `{"picked":["shown"]}`,
	})

	groups := getPublicPerfCardData(t)["groups"].(map[string]any)
	assert.NotContains(t, groups, "skipped", "未加入白名单的分组不得对普通用户返回")
	items := groups["picked"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, "shown", items[0].(map[string]any)["model_name"])
}

// 快照按配置签名生效：管理员改白名单后必须立刻可见，
// 不能被 TTL 挡住 30 秒——否则管理员会以为设置没保存。
func TestPerfCardSnapshot_ConfigChangeBypassesTTL(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)
	seedPerfCardMetric(t, "a", "m", 4, 4)
	seedPerfCardMetric(t, "b", "m", 4, 4)

	base := map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["a","b"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_groups":  `["a"]`,
	}
	applyMonitoringSettings(t, base)
	first := getPerfCardData(t)["groups"].(map[string]any)
	require.Contains(t, first, "a")
	require.NotContains(t, first, "b")

	base["group_monitoring_setting.perf_card_groups"] = `["a","b"]`
	applyMonitoringSettings(t, base)
	second := getPerfCardData(t)["groups"].(map[string]any)
	assert.Contains(t, second, "a")
	assert.Contains(t, second, "b", "白名单变更必须绕过 TTL 立即生效")
}

// TTL 内重复请求不再打库：公开后该端点的并发量是管理员端点的数百倍，
// 缓存是放开的前置条件而非优化。用"两次调用之间新增数据"来观测是否重查。
func TestPerfCardSnapshot_ServesCachedResultWithinTTL(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	saveMonitoringSettings(t)
	seedPerfCardMetric(t, "picked", "first", 4, 4)

	applyMonitoringSettings(t, map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": `["picked"]`,
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_public":  "true",
		"group_monitoring_setting.perf_card_groups":  `["picked"]`,
	})

	require.Len(t, getPerfCardData(t)["groups"].(map[string]any)["picked"].([]any), 1)

	seedPerfCardMetric(t, "picked", "second", 4, 4)

	assert.Len(t, getPerfCardData(t)["groups"].(map[string]any)["picked"].([]any), 1,
		"TTL 内不得重新查询数据库")
	assert.Len(t, getPublicPerfCardData(t)["groups"].(map[string]any)["picked"].([]any), 1,
		"公开端点与管理员端点共用同一份快照")
}
