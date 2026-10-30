package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostReportSnapshotPriorityAndConservativeFallback(t *testing.T) {
	old := common.LogDatabaseType()
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { common.SetLogDatabaseType(old) })
	checkCostReportSnapshotPriority(t)
}

func TestClickHouseCostReportSnapshotPriorityAndConservativeFallback(t *testing.T) {
	setupClickHouseLogTest(t)
	checkCostReportSnapshotPriority(t)
}

func checkCostReportSnapshotPriority(t *testing.T) {
	t.Helper()
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		t.Cleanup(func() { LOG_DB.Exec("DELETE FROM logs") })
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	}
	rows := []Log{
		{CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 200, Other: `{"root_cost":{"status":"estimated","amount_usd":"0"}}`},
		{CreatedAt: 101, Type: 2, ChannelId: 7, Quota: 200, Other: `{"root_cost":{"status":"stale","amount_usd":"0.5"}}`},
		{CreatedAt: 102, Type: 2, ChannelId: 7, Quota: 200, Other: `{"group_ratio":2}`},
		{CreatedAt: 103, Type: 2, ChannelId: 7, Quota: 200, Other: `{"group_ratio":0}`},
		{CreatedAt: 104, Type: 2, ChannelId: 7, Quota: 200, Other: `{"group_ratio":1,"web_search":true}`},
		{CreatedAt: 105, Type: 6, ChannelId: 7, Quota: 50, Other: `{"root_cost":{"status":"estimated","amount_usd":"1"}}`},
		{CreatedAt: 106, Type: 2, ChannelId: 7, Quota: 200, Other: `{"root_cost":{"status":"unknown","base_usd":"3"}}`},
		{CreatedAt: 107, Type: 2, ChannelId: 7, Quota: 200, Other: `{"group_ratio":1,"admin_info":{"quota_saturation":{"kind":"overflow"}}}`},
		{CreatedAt: 108, Type: 2, ChannelId: 7, Quota: 200, Other: `invalid`},
		{CreatedAt: 109, Type: 2, ChannelId: 7, Quota: 200, Other: `{"group_ratio":1,"billing_mode":"tiered_expr"}`},
		{CreatedAt: 110, Type: 2, ChannelId: 7, Quota: 1000},
	}
	require.NoError(t, LOG_DB.Create(&rows).Error)
	policy := CostReportPolicy{Start: 100, End: 110, QuotaPerUnit: "100", FallbackMode: "billed_basis", Rates: []CostReportRate{{ChannelID: 7, Ratio: "0.5"}}}
	got, err := AggregateChannelCostWindow(context.Background(), policy, 100, 110)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(9), got[0].ConsumeCount)
	assert.Equal(t, int64(2), got[0].OriginalCount)
	assert.Equal(t, int64(1), got[0].StaleCount)
	assert.Equal(t, int64(2), got[0].FallbackCount)
	assert.Equal(t, int64(5), got[0].MissingCount)
	assert.Equal(t, "18", got[0].ConsumeAmount)
	assert.Equal(t, "0.5", got[0].RefundAmount)
	assert.Equal(t, "0.5", got[0].OriginalCost)
	assert.Equal(t, "2", got[0].FallbackCost)
	assert.Equal(t, "8", got[0].KnownRevenue)
	assert.Equal(t, "10", got[0].UnknownRevenue)
	policy.QuotaPerUnit = "3"
	thirds, err := AggregateChannelCostWindow(context.Background(), policy, 100, 110)
	require.NoError(t, err)
	assert.Equal(t, "599.999999999999999994", thirds[0].ConsumeAmount)
	assert.Equal(t, "266.666666666666666664", thirds[0].KnownRevenue)
	policy.QuotaPerUnit = "100"
	policy.FallbackMode = "none"
	policy.Rates = nil
	got, err = AggregateChannelCostWindow(context.Background(), policy, 100, 110)
	require.NoError(t, err)
	assert.Equal(t, int64(7), got[0].MissingCount)
	_, err = AggregateChannelCostWindow(context.Background(), policy, 100, 4000)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = AggregateChannelCostWindow(ctx, policy, 100, 110)
	require.Error(t, err)

	ratio := 0.1234567890123456
	snapshot := BuildChannelCost(ChannelCostSettings{Mode: "manual", Ratio: &ratio}, decimal.RequireFromString("0.000734"), true)
	require.Equal(t, "0.0000906172831350616704", snapshot.AmountUSD)
	encoded, err := common.Marshal(map[string]any{"root_cost": snapshot})
	require.NoError(t, err)
	preciseRows := []Log{
		{CreatedAt: 120, Type: 2, ChannelId: 8, Quota: 100, Other: string(encoded)},
		{CreatedAt: 121, Type: 2, ChannelId: 8, Quota: 100, Other: `{"root_cost":{"status":"estimated","amount_usd":"0.000000000000000000000000000001"}}`},
		{CreatedAt: 122, Type: 2, ChannelId: 8, Quota: 100, Other: `{"root_cost":{"status":"estimated","amount_usd":"0"}}`},
		{CreatedAt: 123, Type: 2, ChannelId: 8, Quota: 100, Other: `{"root_cost":{"status":"unknown","base_usd":"0.123456789012345678901"}}`},
	}
	require.NoError(t, LOG_DB.Create(&preciseRows).Error)
	precisePolicy := CostReportPolicy{Start: 120, End: 124, QuotaPerUnit: "100", FallbackMode: "billed_basis", Rates: []CostReportRate{{ChannelID: 8, Ratio: "10"}}}
	precise, err := AggregateChannelCostWindow(context.Background(), precisePolicy, 120, 124)
	require.NoError(t, err)
	require.Len(t, precise, 1)
	assert.Equal(t, int64(3), precise[0].OriginalCount)
	assert.Equal(t, "0.000090617283135061", precise[0].OriginalCost)
	assert.Equal(t, int64(1), precise[0].FallbackCount)
	assert.Equal(t, "1.23456789012345678", precise[0].FallbackCost)
	assert.Equal(t, int64(0), precise[0].MissingCount)
}

func TestCostReportPolicyAndMerge(t *testing.T) {
	valid := CostReportPolicy{Start: 100, End: 110, QuotaPerUnit: "100", FallbackMode: "none"}
	require.NoError(t, ValidateCostReportPolicy(valid))
	for _, mutate := range []func(*CostReportPolicy){
		func(p *CostReportPolicy) { p.QuotaPerUnit = "0" },
		func(p *CostReportPolicy) { p.End = p.Start + 32*86400 },
		func(p *CostReportPolicy) { p.FallbackMode = "billed_basis" },
		func(p *CostReportPolicy) { p.Rates = []CostReportRate{{ChannelID: 1, Ratio: "1"}} },
		func(p *CostReportPolicy) {
			p.FallbackMode = "billed_basis"
			p.Rates = []CostReportRate{{ChannelID: 1, Ratio: "1"}, {ChannelID: 1, Ratio: "2"}}
		},
	} {
		p := valid
		mutate(&p)
		require.Error(t, ValidateCostReportPolicy(p))
	}
	merged, err := MergeChannelCostReportRows([]ChannelCostReportRow{{ChannelID: 1, ConsumeCount: 1, MissingCount: 1, ConsumeAmount: "9007199254740993.1", UnknownRevenue: "9007199254740993.1"}}, []ChannelCostReportRow{{ChannelID: 1, ConsumeCount: 2, MissingCount: 2, ConsumeAmount: "0.2", UnknownRevenue: "0.2"}})
	require.NoError(t, err)
	require.Len(t, merged, 1)
	assert.Equal(t, "9007199254740993.3", merged[0].ConsumeAmount)
	assert.Equal(t, int64(3), merged[0].ConsumeCount)
	_, err = MergeChannelCostReportRows([]ChannelCostReportRow{{ChannelID: 1, ConsumeCount: 1<<63 - 1, MissingCount: 1<<63 - 1}}, []ChannelCostReportRow{{ChannelID: 1, ConsumeCount: 1, MissingCount: 1}})
	require.Error(t, err)
}

func TestCostReportRejectsCorruptPersistedSummary(t *testing.T) {
	for _, row := range []ChannelCostReportRow{
		{ChannelID: -1},
		{ConsumeCount: 1},
		{ConsumeCount: 1, OriginalCount: 1, FallbackCount: 1},
		{ConsumeCount: 1, OriginalCount: 1, StaleCount: 2},
		{ConsumeCount: 1, MissingCount: 1, ConsumeAmount: "1", KnownRevenue: "1", UnknownRevenue: "1"},
		{ConsumeCount: 1, MissingCount: 1, ConsumeAmount: "-1"},
		{OriginalCost: "1e99999999"},
	} {
		_, err := MergeChannelCostReportRows(nil, []ChannelCostReportRow{row})
		require.Error(t, err)
	}
	_, err := MergeChannelCostReportRows(nil, []ChannelCostReportRow{{}})
	require.NoError(t, err)
}

func TestClickHouseCostReportDedupAndConflict(t *testing.T) {
	db := setupClickHouseLogTest(t)
	require.NoError(t, db.Exec(`INSERT INTO logs (event_id,created_at,type,channel_id,quota,other) VALUES
 ('same',100,2,7,200,'{"root_cost":{"status":"estimated","amount_usd":"0"}}'),
 ('same',100,2,7,200,'{"root_cost":{"status":"estimated","amount_usd":"0"}}'),
 ('fallback',101,2,7,200,'{"group_ratio":2}'),
 ('refund',102,6,7,50,'{}')`).Error)
	policy := CostReportPolicy{Start: 100, End: 110, QuotaPerUnit: "100", FallbackMode: "billed_basis", Rates: []CostReportRate{{ChannelID: 7, Ratio: "0.5"}}}
	got, err := AggregateChannelCostWindow(context.Background(), policy, 100, 110)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(2), got[0].ConsumeCount)
	assert.Equal(t, "4", got[0].ConsumeAmount)
	assert.Equal(t, "0.5", got[0].FallbackCost)
	assert.Equal(t, "0.5", got[0].RefundAmount)
	policy.ChannelID = 7
	filtered, err := AggregateChannelCostWindow(context.Background(), policy, 100, 110)
	require.NoError(t, err)
	assert.Equal(t, got, filtered)
	require.NoError(t, db.Exec(`INSERT INTO logs (event_id,created_at,type,channel_id,quota,other) VALUES ('same',100,2,8,201,'{}')`).Error)
	_, err = AggregateChannelCostWindow(context.Background(), policy, 100, 110)
	require.ErrorContains(t, err, "conflicting")
}

func TestClickHouseCostReportSourceIdentityTracksTableReplacement(t *testing.T) {
	db := setupClickHouseLogTest(t)
	ctx := context.Background()
	original, err := ChannelCostReportSourceIdentity(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, original)
	require.NoError(t, db.Exec(`INSERT INTO logs (event_id,created_at,type,channel_id,quota) VALUES ('identity-insert',100,2,7,200)`).Error)
	afterInsert, err := ChannelCostReportSourceIdentity(ctx)
	require.NoError(t, err)
	assert.Equal(t, original, afterInsert, "ordinary ingestion must not invalidate a resumable report")
	require.NoError(t, db.Exec("RENAME TABLE logs TO logs_before_replacement").Error)
	require.NoError(t, CreateClickHouseLogTable(db, 0))
	afterReplacement, err := ChannelCostReportSourceIdentity(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, original, afterReplacement, "same-name replacement must fence old report windows")
	stableReplacement, err := ChannelCostReportSourceIdentity(ctx)
	require.NoError(t, err)
	assert.Equal(t, afterReplacement, stableReplacement)
}
