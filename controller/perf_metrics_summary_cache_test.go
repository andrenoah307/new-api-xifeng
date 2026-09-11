package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedSummaryMetric(t *testing.T, group, modelName string, ageSeconds int64, requests, successes int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: modelName, Group: group, BucketTs: time.Now().Unix() - ageSeconds,
		RequestCount: requests, SuccessCount: successes, TotalLatencyMs: requests * 100,
	}).Error)
}

func summarySuccessRate(t *testing.T, hours int, groups []string) float64 {
	t.Helper()
	result, err := cachedPerfMetricsSummary(hours, groups)
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	return result.Models[0].SuccessRate
}

// /api/perf-metrics/summary 挂在公开的模型广场页面级加载上，每次访问都做一次
// 全量聚合。TTL 内重复调用必须直接吃快照，不得回源——这是该端点抗住公开流量的
// 唯一机制。判据：回源之后改库，若下一次调用看见了新数据，就说明它又查了一遍。
func TestCachedPerfMetricsSummary_ServesSnapshotWithinTTL(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	seedSummaryMetric(t, "a", "m", 1800, 4, 4)

	assert.Equal(t, 100.0, summarySuccessRate(t, 24, []string{"a"}))

	seedSummaryMetric(t, "a", "m", 2400, 4, 0)
	assert.Equal(t, 100.0, summarySuccessRate(t, 24, []string{"a"}), "TTL 内必须命中快照，不得回源")
}

// 分组维度在 SQL 内部（GROUP BY model_name, bucket_ts）就被聚掉了，
// 响应里根本没有分组这一维。所以 perf_card 那套「先查全集、按请求做事后过滤」
// 在这里不成立——可见分组集合改变的是聚合值本身，必须进缓存键。
// 若漏掉这一维，跨分组访问者之间会互相读到对方口径的数据。
func TestCachedPerfMetricsSummary_KeyedByVisibleGroupSet(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	seedSummaryMetric(t, "a", "m", 1800, 4, 4)
	seedSummaryMetric(t, "b", "m", 1800, 4, 0)

	assert.Equal(t, 100.0, summarySuccessRate(t, 24, []string{"a"}))
	assert.Equal(t, 50.0, summarySuccessRate(t, 24, []string{"a", "b"}),
		"可见分组集合不同就是另一份聚合，不能复用快照")
	assert.Equal(t, 100.0, summarySuccessRate(t, 24, []string{"a"}),
		"切回原集合同样必须重新聚合，不能拿到上一次的口径")
}

// hours 同样改变聚合范围，必须进键。
func TestCachedPerfMetricsSummary_KeyedByWindow(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	seedSummaryMetric(t, "a", "m", 1800, 4, 4)

	assert.Equal(t, 100.0, summarySuccessRate(t, 24, []string{"a"}))

	// 落在 24 小时窗口之外、48 小时窗口之内
	seedSummaryMetric(t, "a", "m", 30*3600, 4, 0)
	assert.Equal(t, 50.0, summarySuccessRate(t, 48, []string{"a"}),
		"换窗口就是另一份聚合，必须回源")
}

// 回源失败时不能把仍然有效的快照抹掉：那会让一次瞬时 DB 抖动
// 直接把公开端点的缓存击穿成逐请求查询。
func TestCachedPerfMetricsSummary_KeepsSnapshotWhenRefreshFails(t *testing.T) {
	db := setupMonitoringControllerTestDB(t, false)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	seedSummaryMetric(t, "a", "m", 1800, 4, 4)

	assert.Equal(t, 100.0, summarySuccessRate(t, 24, []string{"a"}))

	require.NoError(t, db.Migrator().DropTable(&model.PerfMetric{}))
	_, err := cachedPerfMetricsSummary(48, []string{"a"})
	require.Error(t, err, "换键回源必须真的报错，否则这条用例证明不了任何事")

	assert.Equal(t, 100.0, summarySuccessRate(t, 24, []string{"a"}), "原快照必须还在")
}
