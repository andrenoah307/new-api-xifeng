package perfmetrics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGroupModelSummaryTest(t *testing.T) {
	t.Helper()

	oldDB := model.DB
	oldMainType := common.MainDatabaseType()
	oldLogType := common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	model.DB = db
	// commonGroupCol 只在 InitDB/InitLogDB 里赋值；LOG_SQL_DSN 为空时 InitLogDB 只做列名初始化，无任何 I/O。
	t.Setenv("LOG_SQL_DSN", "")
	oldLogDB := model.LOG_DB
	require.NoError(t, model.InitLogDB())

	hotBuckets.Range(func(key, _ any) bool {
		hotBuckets.Delete(key)
		return true
	})

	t.Cleanup(func() {
		hotBuckets.Range(func(key, _ any) bool {
			hotBuckets.Delete(key)
			return true
		})
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled = oldRedisEnabled
	})
}

func seedHotBucket(group, modelName string, bucketTs int64, c counters) {
	b := &atomicBucket{}
	b.addCounters(c)
	hotBuckets.Store(bucketKey{model: modelName, group: group, bucketTs: bucketTs}, b)
}

// 回归：同一 (group, model) 同时存在于数据库聚合行与内存热桶时，只能产出一条记录。
// 曾经的缺陷是热桶键带 bucketTs、数据库行键 bucketTs 为 0，两者永远合并不到一起，
// 导致同一个模型在同一分组下被输出多条、数值互不相同。
func TestQueryGroupModelSummary_MergesDatabaseRowWithHotBucket(t *testing.T) {
	setupGroupModelSummaryTest(t)

	nowBucket := time.Now().Unix() - 60
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "gpt-5.6", Group: "default", BucketTs: time.Now().Unix() - 3600,
		RequestCount: 10, SuccessCount: 8, TotalLatencyMs: 2000,
		TtftSumMs: 500, TtftCount: 10, OutputTokens: 1000, GenerationMs: 2000,
	}).Error)
	seedHotBucket("default", "gpt-5.6", nowBucket, counters{
		requestCount: 30, successCount: 30, totalLatencyMs: 6000,
		ttftSumMs: 900, ttftCount: 30, outputTokens: 3000, generationMs: 6000,
	})

	got, err := QueryGroupModelSummary(24, []string{"default"})
	require.NoError(t, err)

	items := got["default"]
	require.Len(t, items, 1, "同一 (group, model) 必须只产出一条记录")

	item := items[0]
	assert.Equal(t, "gpt-5.6", item.ModelName)
	assert.Equal(t, int64(40), item.RequestCount, "计数必须是数据库行与热桶之和")
	assert.Equal(t, 95.0, item.SuccessRate, "成功率必须按合并后的分子分母计算")
	assert.Equal(t, int64(200), item.AvgLatencyMs, "8000ms / 40 次")
	assert.True(t, item.HasTtft)
	assert.Equal(t, int64(35), item.AvgTtftMs, "1400ms / 40 次")
	assert.Equal(t, 500.0, item.AvgTps, "4000 tokens / 8s")
}

// 加权平均：请求数悬殊的两个桶必须按请求数加权，而不是把各桶平均值再平均。
func TestQueryGroupModelSummary_WeightsByRequestCount(t *testing.T) {
	setupGroupModelSummaryTest(t)

	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: time.Now().Unix() - 3600,
		RequestCount: 99, SuccessCount: 99, TotalLatencyMs: 99 * 100,
	}).Error)
	seedHotBucket("g", "m", time.Now().Unix()-60, counters{
		requestCount: 1, successCount: 0, totalLatencyMs: 10000,
	})

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	// 加权：(9900 + 10000) / 100 = 199。若错误地对两桶均值取平均则是 (100 + 10000) / 2 = 5050。
	assert.Equal(t, int64(199), got["g"][0].AvgLatencyMs)
	assert.Equal(t, 99.0, got["g"][0].SuccessRate)
}

func TestQueryGroupModelSummary_MissingTtftAndGenerationTime(t *testing.T) {
	setupGroupModelSummaryTest(t)

	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "no-ttft", Group: "g", BucketTs: time.Now().Unix() - 600,
		RequestCount: 5, SuccessCount: 5, TotalLatencyMs: 500,
		TtftCount: 0, TtftSumMs: 0, OutputTokens: 100, GenerationMs: 0,
	}).Error)

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)

	item := got["g"][0]
	assert.False(t, item.HasTtft, "ttft_count 为 0 时不得声称有首字数据")
	assert.Equal(t, int64(0), item.AvgTtftMs)
	assert.Equal(t, 0.0, item.AvgTps, "generation_ms 为 0 时不得出现除零或 Inf")
}

func TestQueryGroupModelSummary_SortsByRequestCountThenName(t *testing.T) {
	setupGroupModelSummaryTest(t)

	ts := time.Now().Unix() - 600
	for _, m := range []struct {
		name  string
		count int64
	}{{"b-model", 5}, {"a-model", 5}, {"busy", 100}} {
		require.NoError(t, model.DB.Create(&model.PerfMetric{
			ModelName: m.name, Group: "g", BucketTs: ts,
			RequestCount: m.count, SuccessCount: m.count, TotalLatencyMs: m.count,
		}).Error)
	}

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 3)

	names := []string{got["g"][0].ModelName, got["g"][1].ModelName, got["g"][2].ModelName}
	assert.Equal(t, []string{"busy", "a-model", "b-model"}, names, "按请求数降序，同数按模型名升序保证稳定")
}

// groups 为 nil 表示不过滤；长度为 0 表示可见集合为空，必须直接返回空结果。
func TestQueryGroupModelSummary_EmptyGroupsMeansNothingVisible(t *testing.T) {
	setupGroupModelSummaryTest(t)

	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ModelName: "m", Group: "g", BucketTs: time.Now().Unix() - 600,
		RequestCount: 5, SuccessCount: 5, TotalLatencyMs: 500,
	}).Error)
	seedHotBucket("g", "m", time.Now().Unix()-60, counters{requestCount: 1, successCount: 1})

	got, err := QueryGroupModelSummary(24, []string{})
	require.NoError(t, err)
	assert.Empty(t, got, "可见分组集合为空时热桶也不得泄漏进结果")
}

func TestQueryGroupModelSummary_ExcludesHotBucketsOutsideWindow(t *testing.T) {
	setupGroupModelSummaryTest(t)

	seedHotBucket("g", "stale", time.Now().Unix()-48*3600, counters{requestCount: 7, successCount: 7})
	seedHotBucket("g", "fresh", time.Now().Unix()-60, counters{requestCount: 3, successCount: 3})

	got, err := QueryGroupModelSummary(24, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)
	assert.Equal(t, "fresh", got["g"][0].ModelName)
}
