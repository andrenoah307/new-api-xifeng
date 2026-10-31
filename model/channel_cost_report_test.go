package model

import (
	"context"
	"errors"
	"testing"

	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestCostReportNextTimestamp(t *testing.T) {
	checkCostReportNextTimestamp(t)
}

func TestClickHouseCostReportNextTimestamp(t *testing.T) {
	setupClickHouseLogTest(t)
	checkCostReportNextTimestamp(t)
}

func TestClickHouseCostReportSeekAcrossHistoricalParts(t *testing.T) {
	db := setupClickHouseLogTest(t)
	// Imported batches overlap in time. LIMIT 1 still initializes reads across
	// these parts and can exhaust the row budget before producing its first row.
	const start = int64(1790784000)
	const end = start + 31*86400
	require.NoError(t, db.Exec(`INSERT INTO logs (created_at, type, migration_batch)
 SELECT ? + intDiv(number,280)*130, 2, toString(number%280)
 FROM numbers(5600000) SETTINGS max_partitions_per_insert_block=1000`, start).Error)
	var oldResult []int64
	err := db.Raw(`SELECT created_at FROM logs
 WHERE ifNull(created_at,0)>=? AND ifNull(created_at,0)<? AND type IN (2,6)
 ORDER BY ifNull(created_at,0) ASC LIMIT 1
 SETTINGS optimize_read_in_order=1,max_threads=2,max_memory_usage=536870912,
 max_execution_time=30,max_rows_to_read=5000000,read_overflow_mode='throw'`, start, end).Scan(&oldResult).Error
	var exception *clickhousedriver.Exception
	require.Error(t, err)
	if errors.As(err, &exception) {
		require.EqualValues(t, 158, exception.Code)
	} else {
		require.Contains(t, err.Error(), "Code: 158.")
		require.Contains(t, err.Error(), "(TOO_MANY_ROWS)")
	}
	p := CostReportPolicy{Start: start, End: end, QuotaPerUnit: "500000", FallbackMode: "none"}
	next, err := NextChannelCostReportTimestamp(context.Background(), p, start)
	require.NoError(t, err)
	assert.Equal(t, start, next)
	// A cursor between events must return the next event, not repeat the first.
	next, err = NextChannelCostReportTimestamp(context.Background(), p, start+1)
	require.NoError(t, err)
	assert.Equal(t, start+130, next)
}

func TestClickHouseCostReportSeekDoesNotSkipFailedRanges(t *testing.T) {
	for _, tc := range []struct {
		name        string
		err         error
		recoverable bool
		hourLimit   bool
	}{
		{"native-row-limit", &clickhousedriver.Exception{Code: 158}, true, false},
		{"http-row-limit", errors.New("Code: 158. Limit for rows exceeded (TOO_MANY_ROWS)"), true, false},
		{"timeout", context.DeadlineExceeded, false, false},
		{"hour-row-limit", &clickhousedriver.Exception{Code: 158}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupClickHouseLogTest(t)
			p := CostReportPolicy{Start: 100, End: 100 + 86400, QuotaPerUnit: "500000", FallbackMode: "none"}
			want := p.Start + 7200
			require.NoError(t, db.Create(&Log{CreatedAt: want, Type: 6, ChannelId: 7}).Error)
			require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:seek-limit", func(tx *gorm.DB) {
				if len(tx.Statement.Vars) == 2 {
					start, okStart := tx.Statement.Vars[0].(int64)
					end, okEnd := tx.Statement.Vars[1].(int64)
					if okStart && okEnd && (end-start > 3600 || tc.hourLimit) {
						tx.AddError(tc.err)
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Row().Remove("test:seek-limit")) })
			next, err := NextChannelCostReportTimestamp(context.Background(), p, p.Start)
			if !tc.recoverable {
				require.ErrorIs(t, err, tc.err)
				assert.Zero(t, next)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, want, next, "a failed wider probe must not skip the refund at its lower narrowed boundary")
		})
	}
}

func checkCostReportNextTimestamp(t *testing.T) {
	t.Helper()
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
		t.Cleanup(func() { LOG_DB.Exec("DELETE FROM logs") })
	}
	const recent = int64(100 + 3600*24*364)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{CreatedAt: 100, Type: 1, ChannelId: 7},
		{CreatedAt: recent, Type: 6, ChannelId: 7},
		{CreatedAt: recent + 10, Type: 2, ChannelId: 7},
		{CreatedAt: recent + 3600, Type: 2, ChannelId: 7},
	}).Error)
	p := CostReportPolicy{Start: 100, End: recent + 3600, QuotaPerUnit: "500000", FallbackMode: "none"}
	for _, tc := range []struct{ start, want int64 }{
		{100, recent}, {recent, recent}, {recent + 1, recent + 10}, {recent + 11, p.End},
	} {
		next, err := NextChannelCostReportTimestamp(context.Background(), p, tc.start)
		require.NoError(t, err)
		assert.Equal(t, tc.want, next)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NextChannelCostReportTimestamp(ctx, p, p.Start)
	require.Error(t, err)
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		p.ChannelID = 8
		next, err := NextChannelCostReportTimestamp(context.Background(), p, p.Start)
		require.NoError(t, err)
		assert.Equal(t, p.End, next)
	}
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
	for _, end := range []int64{valid.Start + 32*86400, valid.Start + 10*365*86400} {
		historical := valid
		historical.End = end
		require.NoError(t, ValidateCostReportPolicy(historical))
	}
	for _, mutate := range []func(*CostReportPolicy){
		func(p *CostReportPolicy) { p.QuotaPerUnit = "0" },
		func(p *CostReportPolicy) { p.End = 1<<63 - 1 },
		func(p *CostReportPolicy) { p.End = p.Start },
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
