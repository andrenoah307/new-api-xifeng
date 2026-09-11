package perfmetrics

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadPerfMetric(t *testing.T, group, modelName string, bucketTs int64) *model.PerfMetric {
	t.Helper()
	var rows []model.PerfMetric
	require.NoError(t, model.DB.Where("model_name = ? AND "+"`group`"+" = ? AND bucket_ts = ?", modelName, group, bucketTs).Find(&rows).Error)
	if len(rows) == 0 {
		return nil
	}
	require.Len(t, rows, 1)
	return &rows[0]
}

// 周期 flush 刻意跳过当前桶（它还在累加）。代价是进程退出时，
// 当前桶（0–5 分钟）连同一个已完成但还没轮到的桶一起丢失——
// 1 小时窗口共 12 槽，等于每次重启抹掉最新的最多 2 槽。
// 退出前强制 flush 必须把当前桶也写进去。
func TestFlushHotBuckets_PersistsTheInProgressBucket(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	current := bucketStart(time.Now().Unix())
	seedHotBucket("g", "m", current, counters{
		requestCount: 7, successCount: 6, totalLatencyMs: 7000,
		ttftSumMs: 3500, ttftCount: 7, outputTokens: 700, generationMs: 3500,
	})

	flushCompletedBuckets()
	assert.Nil(t, loadPerfMetric(t, "g", "m", current), "周期 flush 不碰当前桶")

	FlushHotBuckets()
	row := loadPerfMetric(t, "g", "m", current)
	require.NotNil(t, row, "退出前强制 flush 必须落库当前桶")
	assert.Equal(t, int64(7), row.RequestCount)
	assert.Equal(t, int64(6), row.SuccessCount)
	assert.Equal(t, int64(3500), row.TtftSumMs)
	assert.Equal(t, int64(7), row.TtftCount)
}

// UpsertPerfMetric 是 `col = col + ?` 的累加式 upsert，drain() 又是
// 原子取走并清零。两者相配意味着重复 flush 不会双计——这条是强制 flush
// 敢于写入「还在累加中的桶」的唯一依据，必须有用例钉住。
func TestFlushHotBuckets_DoesNotDoubleCountOnSecondCall(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	current := bucketStart(time.Now().Unix())
	seedHotBucket("g", "m", current, counters{
		requestCount: 5, successCount: 5, totalLatencyMs: 5000,
		ttftSumMs: 2500, ttftCount: 5,
	})

	FlushHotBuckets()
	FlushHotBuckets()

	row := loadPerfMetric(t, "g", "m", current)
	require.NotNil(t, row)
	assert.Equal(t, int64(5), row.RequestCount, "第二次 flush 取到的是清零后的空桶")
	assert.Equal(t, int64(2500), row.TtftSumMs)
}

// 强制 flush 之后桶被清零，但桶本身还留在 hotBuckets 里（24 小时地平线才删壳）。
// 若清零没生效，这次查询会把同一批样本连同刚落库的行一起再算一遍。
func TestFlushHotBuckets_SummaryDoesNotCountFlushedSamplesTwice(t *testing.T) {
	setupGroupModelSummaryTest(t)
	setBucketTime(t, "5min")

	current := bucketStart(time.Now().Unix())
	seedHotBucket("g", "m", current, counters{
		requestCount: 9, successCount: 9, totalLatencyMs: 9000,
		ttftSumMs: 90000, ttftCount: 9,
	})

	FlushHotBuckets()

	got, err := QueryGroupModelSummary(1, []string{"g"})
	require.NoError(t, err)
	require.Len(t, got["g"], 1)
	assert.Equal(t, int64(9), got["g"][0].RequestCount)
	assert.Equal(t, int64(10000), got["g"][0].AvgTtftMs)
}
