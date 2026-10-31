package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostReportFinalChargeBasis(t *testing.T) {
	checkCostReportFinalChargeBasis(t)
}

func TestClickHouseCostReportFinalChargeBasis(t *testing.T) {
	setupClickHouseLogTest(t)
	checkCostReportFinalChargeBasis(t)
}

func checkCostReportFinalChargeBasis(t *testing.T) {
	t.Helper()
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
		t.Cleanup(func() { LOG_DB.Exec("DELETE FROM logs") })
	}
	rows := []Log{
		{EventID: "snapshot", CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 200, Other: `{"root_cost":{"status":"estimated","amount_usd":"0.7"},"group_ratio":2}`},
		{EventID: "tiered", CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 200, Other: `{"billing_mode":"tiered_expr","group_ratio":2}`},
		{EventID: "search", CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 300, Other: `{"alpha_search":1,"group_ratio":1.5}`},
		{EventID: "no-group", CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 400, Other: `{"root_cost":{"base_usd":"99"}}`},
		{EventID: "zero-group", CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 100, Other: `{"group_ratio":0}`},
		{EventID: "saturated", CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 100, Other: `{"group_ratio":1,"admin_info":{"quota_saturation":{}}}`},
		{EventID: "no-rate", CreatedAt: 100, Type: 2, ChannelId: 8, Quota: 100, Other: `{"group_ratio":1}`},
		{EventID: "final-over-base", CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 300, Other: `{"group_ratio":1.5,"root_cost":{"base_usd":"99"}}`},
		{EventID: "free", CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 0, Other: `{"group_ratio":1}`},
		{EventID: "invalid", CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 100, Other: `invalid`},
	}
	require.NoError(t, LOG_DB.Create(&rows).Error)
	policy := CostReportPolicy{CostBasisVersion: 1, Start: 100, End: 110, QuotaPerUnit: "100", FallbackMode: "billed_basis", Rates: []CostReportRate{{ChannelID: 7, Ratio: "0.5"}}}
	got, err := AggregateChannelCostWindow(context.Background(), policy, 100, 110)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.EqualValues(t, 1, got[0].OriginalCount)
	assert.EqualValues(t, 4, got[0].FallbackCount)
	assert.EqualValues(t, 4, got[0].MissingCount)
	assert.Equal(t, "0.7", got[0].OriginalCost)
	assert.Equal(t, "2.5", got[0].FallbackCost)
	assert.Equal(t, "10", got[0].KnownRevenue)
	assert.Equal(t, "7", got[0].UnknownRevenue)
	assert.EqualValues(t, 1, got[1].MissingCount)
	policy.CostBasisVersion = 0
	legacy, err := AggregateChannelCostWindow(context.Background(), policy, 100, 110)
	require.NoError(t, err)
	assert.Equal(t, "99", legacy[0].FallbackCost, "resuming old jobs retains their original basis")
	assert.EqualValues(t, 3, legacy[0].FallbackCount)
}
