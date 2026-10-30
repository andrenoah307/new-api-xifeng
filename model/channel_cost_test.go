package model

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCostSnapshot(t *testing.T) {
	ratio := 0.3
	config := ChannelCostSettings{Mode: "manual", Ratio: &ratio, Version: "v1", UpdatedAt: 123}
	cost := BuildChannelCost(config, decimal.NewFromFloat(2.5), true)
	require.NotNil(t, cost)
	assert.Equal(t, "0.75", cost.AmountUSD)
	assert.Equal(t, "2.5", cost.BaseUSD)
	assert.Equal(t, "v1", cost.ConfigVersion)
	ratio = 0.8
	assert.Equal(t, 0.3, *cost.Ratio, "a saved snapshot must not alias configuration")
	assert.Nil(t, BuildChannelCost(ChannelCostSettings{}, decimal.Zero, true))
	unknown := BuildChannelCost(ChannelCostSettings{Mode: "auto"}, decimal.NewFromInt(1), true)
	assert.Empty(t, unknown.AmountUSD)
	assert.Equal(t, "unknown", unknown.Status)
	assert.Empty(t, BuildChannelCost(config, decimal.NewFromInt(-1), true).AmountUSD)
	assert.Empty(t, BuildChannelCost(config, decimal.NewFromInt(1), false).AmountUSD)
}

func TestChannelCostConsumeLogStorage(t *testing.T) {
	for _, backend := range []string{"sqlite", "clickhouse"} {
		t.Run(backend, func(t *testing.T) {
			if backend == "clickhouse" {
				setupClickHouseLogTest(t)
			} else {
				truncateTables(t)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			ratio, base := 0.25, "0.123456789"
			c.Set(ChannelCostContextKey, ChannelCostSettings{Mode: "manual", Ratio: &ratio})
			RecordConsumeLog(c, 95123, RecordConsumeLogParams{CostBaseUSD: &base, Quota: 42, ChannelId: 1, ModelName: "cost-test"})
			var logs []*Log
			require.NoError(t, LOG_DB.Where("user_id = ?", 95123).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, 42, logs[0].Quota, "cost snapshots must not alter the customer's charge")
			var other struct {
				Cost LogCost `json:"root_cost"`
			}
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			assert.Equal(t, "0.03086419725", other.Cost.AmountUSD)
			formatUserLogs(logs, 0)
			assert.NotContains(t, logs[0].Other, "root_cost")
			c.Set(ChannelCostContextKey, ChannelCostSettings{Mode: "auto", Ratio: &ratio})
			RecordConsumeLog(c, 95124, RecordConsumeLogParams{CostBaseUSD: &base, Quota: 42, ChannelId: 1, ModelName: "cost-task"})
			require.NoError(t, LOG_DB.Where("user_id = ?", 95124).Find(&logs).Error)
			require.Len(t, logs, 1)
			other.Cost = LogCost{}
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			assert.Equal(t, "unknown", other.Cost.Status, "a token-scope declaration cannot price per-call tasks")
			assert.Empty(t, other.Cost.AmountUSD)
		})
	}
}

func TestChannelCostPeakAndCredentialChange(t *testing.T) {
	base, peak, enabled := 0.25, 1.5, true
	config := ChannelCostSettings{Mode: "auto", Ratio: &base, Upstream: &UpstreamCostDeclaration{
		PeakEnabled: &enabled, PeakStart: "09:00", PeakEnd: "18:00", PeakRate: &peak, Timezone: "Asia/Shanghai",
	}}
	for _, tt := range []struct {
		utc  string
		want float64
	}{
		{"2026-10-30T00:59:59Z", 0.25},
		{"2026-10-30T01:00:00Z", 0.375},
		{"2026-10-30T09:59:59Z", 0.375},
		{"2026-10-30T10:00:00Z", 0.25},
	} {
		at, err := time.Parse(time.RFC3339, tt.utc)
		require.NoError(t, err)
		snapshot := config.SnapshotAt(at)
		require.NotNil(t, snapshot.Ratio)
		assert.Equal(t, tt.want, *snapshot.Ratio)
		assert.Equal(t, 0.25, *config.Ratio, "peak snapshots must not overwrite baseline")
	}
	channel := Channel{Key: "first-key"}
	config.Identity = channel.CostIdentity()
	raw, err := common.Marshal(config)
	require.NoError(t, err)
	channel.CostSettings = string(raw)
	require.NotNil(t, channel.GetCostSettings().Ratio)
	channel.Key = "new-key"
	assert.Nil(t, channel.GetCostSettings().Ratio, "a different account must not inherit an automatic rate")
	assert.Nil(t, channel.GetCostSettings().Upstream)
}

func TestLogCostRedaction(t *testing.T) {
	logs := []*Log{{Other: `{"root_cost":{"amount_usd":"0.75"},"admin_info":{"use_channel":[1]},"model_ratio":2,"large":9007199254740993}`}}
	FilterLogCosts(logs, false)
	assert.Contains(t, logs[0].Other, "9007199254740993", "redaction must preserve unrelated numeric precision")
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	assert.NotContains(t, other, "root_cost")
	assert.Contains(t, other, "admin_info")
	logs[0].Other = `{"root_cost":{"amount_usd":"0.75"}}`
	FilterLogCosts(logs, true)
	assert.Contains(t, logs[0].Other, "0.75")
	formatUserLogs(logs, 0)
	assert.NotContains(t, logs[0].Other, "root_cost")
	logs[0].Other = `{"root_cost": broken`
	FilterLogCosts(logs, false)
	assert.Equal(t, "{}", logs[0].Other, "malformed private metadata must fail closed")
	logs[0].Other = `{"root_\u0063ost":{"amount_usd":"secret"}}`
	FilterLogCosts(logs, false)
	assert.NotContains(t, logs[0].Other, "secret")
}
