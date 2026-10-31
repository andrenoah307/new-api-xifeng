package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostReportHistoricalPeak(t *testing.T) { checkCostReportHistoricalPeak(t) }
func TestClickHouseCostReportHistoricalPeak(t *testing.T) {
	setupClickHouseLogTest(t)
	checkCostReportHistoricalPeak(t)
}
func checkCostReportHistoricalPeak(t *testing.T) {
	t.Helper()
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
		t.Cleanup(func() { LOG_DB.Exec("DELETE FROM logs") })
	}
	at := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC).Unix()
	require.NoError(t, LOG_DB.Create(&[]Log{
		{CreatedAt: at - 1, Type: 2, ChannelId: 7, Quota: 100, Other: `{"group_ratio":1}`},
		{CreatedAt: at, Type: 2, ChannelId: 7, Quota: 100, Other: `{"group_ratio":1}`},
		{CreatedAt: at + 60, Type: 2, ChannelId: 7, Quota: 100, Other: `{"group_ratio":1}`},
		{CreatedAt: at, Type: 2, ChannelId: 7, Quota: 100, Other: `{"root_cost":{"status":"estimated","amount_usd":"0.1"}}`},
		{CreatedAt: at, Type: 2, ChannelId: 8, Quota: 100, Other: `{"group_ratio":1}`},
	}).Error)
	policy := CostReportPolicy{Start: at - 1, End: at + 61, QuotaPerUnit: "100", FallbackMode: "billed_basis", Rates: []CostReportRate{{ChannelID: 7, Ratio: "0.5", Peak: &CostReportPeak{Start: "09:00", End: "09:01", Timezone: "Asia/Shanghai", Ratio: "2"}}}}
	rows, err := AggregateChannelCostWindow(context.Background(), policy, policy.Start, policy.End)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "2", rows[0].FallbackCost, "start is included, end excluded, in historical local time")
	assert.Equal(t, "0.1", rows[0].OriginalCost, "saved cost wins over the new estimate")
	assert.Equal(t, int64(1), rows[1].MissingCount, "unconfigured channels are never recalculated")
	for _, zone := range []string{"Local", "UTC');DROP TABLE logs;--"} {
		policy.Rates[0].Peak.Timezone = zone
		require.Error(t, ValidateCostReportPolicy(policy))
	}
	policy.Rates[0].Peak.Timezone = "UTC"
	policy.Rates[0].Peak.Start = "9:00"
	require.Error(t, ValidateCostReportPolicy(policy))
}
