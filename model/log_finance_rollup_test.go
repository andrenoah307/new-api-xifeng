package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClickHouseFinanceDailyPublicationAndInvalidation(t *testing.T) {
	db := setupClickHouseLogTest(t)
	ctx := context.Background()
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id,created_at,type,channel_id,quota,other,event_id) VALUES
 (7,?,2,18,200,?,'consume'), (8,?,2,18,300,?,'other'),
 (7,?,6,18,40,'{}','refund'), (7,?,2,19,50,'{}','missing')`, start, `{"group_ratio":2}`, start+1, `{"group_ratio":3}`, start+2, start+86400).Error)
	require.NoError(t, CreateClickHouseFinanceTable(db))
	require.NoError(t, RebuildClickHouseFinanceDay(ctx, db, start))
	require.NoError(t, RebuildClickHouseFinanceDay(ctx, db, start+86400))
	require.NoError(t, RebuildClickHouseFinanceDay(ctx, db, start+2*86400))
	// Replacing the same partition is idempotent, including empty days.
	require.NoError(t, RebuildClickHouseFinanceDay(ctx, db, start))
	result, err := GetFinanceWindow(ctx, start, start+3*86400, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "clickhouse_daily", result.Storage)
	assert.Equal(t, 3, result.SummaryDays)
	require.Len(t, result.Channels, 2)
	assert.Equal(t, "500", result.Channels[0].ConsumeQuota)
	assert.Equal(t, "200", result.Channels[0].KnownModelQuota)
	assert.Equal(t, "460", result.Channels[0].NetQuota)
	assert.EqualValues(t, 1, result.Channels[0].UnresolvedRefunds)
	personal, err := GetFinanceWindow(ctx, start, start+86400, 0, 7)
	require.NoError(t, err)
	require.Len(t, personal.Channels, 1)
	assert.Equal(t, "200", personal.Channels[0].ConsumeQuota)
	assert.Equal(t, "40", personal.Channels[0].RefundQuota)
	// Late historical writes must invalidate snapshots without waiting for a TTL.
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id,created_at,type,channel_id,quota,other,event_id) VALUES (7,?,2,18,100,?,'late')`, start+3, `{"group_ratio":2}`).Error)
	result, err = GetFinanceWindow(ctx, start, start+3*86400, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "raw", result.Storage)
	assert.Equal(t, start+86400, result.End)
	assert.Equal(t, "600", result.Channels[0].ConsumeQuota)
	require.NoError(t, RebuildClickHouseFinanceDay(ctx, db, start))
	result, err = GetFinanceWindow(ctx, start, start+86400, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "clickhouse_daily", result.Storage)
	assert.Equal(t, "600", result.Channels[0].ConsumeQuota)
	// Exact partial-day queries must never substitute the whole day's snapshot.
	result, err = GetFinanceWindow(ctx, start+1, start+3, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "raw", result.Storage)
	assert.Equal(t, "300", result.Channels[0].ConsumeQuota)
	require.Error(t, RebuildClickHouseFinanceDay(ctx, db, start+1))
	require.Error(t, RebuildClickHouseFinanceDay(ctx, db, time.Now().UTC().Truncate(24*time.Hour).Unix()))
	// Another owner must not steal a gate or replace an already published day.
	gate := fmt.Sprintf("logs_finance_lock_%d", start)
	require.NoError(t, db.Exec("CREATE TABLE "+gate+" (id UInt8) ENGINE=Memory").Error)
	require.Error(t, RebuildClickHouseFinanceDay(ctx, db, start))
	require.NoError(t, db.Exec("DROP TABLE "+gate+" SYNC").Error)
	// A new log table is a different source, even if part names are reused.
	require.NoError(t, db.Exec("RENAME TABLE logs TO old_logs").Error)
	require.NoError(t, CreateClickHouseLogTable(db, 0))
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id,created_at,type,channel_id,quota,other,event_id) VALUES (7,?,2,18,700,?,'replacement')`, start, `{"group_ratio":2}`).Error)
	result, err = GetFinanceWindow(ctx, start, start+86400, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "raw", result.Storage)
	assert.Equal(t, "700", result.Channels[0].ConsumeQuota)
}

func TestClickHouseFinanceLargeAmountsPersonalScopeAndPartitionReplacement(t *testing.T) {
	db := setupClickHouseLogTest(t)
	ctx := context.Background()
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id,created_at,type,channel_id,quota,other,event_id) VALUES
	(7,?,2,18,9000000000000000000,?,'a'),(7,?,2,19,9000000000000000000,?,'b'),
	(8,?,2,18,10,?,'other-user'),(7,?,2,18,9000000000000000000,?,'next-day')`,
		start, `{"group_ratio":2}`, start+1, `{"group_ratio":3}`, start+2, `{"group_ratio":1}`, start+86400, `{"group_ratio":2}`).Error)
	raw, err := GetFinanceWindow(ctx, start, start+86400, 0, 7)
	require.NoError(t, err)
	require.Len(t, raw.Channels, 1)
	assert.Equal(t, 0, raw.Channels[0].ChannelID)
	assert.Equal(t, "18000000000000000000", raw.Channels[0].ConsumeQuota)
	assert.EqualValues(t, 2, raw.Channels[0].ConsumeCount)
	require.NoError(t, CreateClickHouseFinanceTable(db))
	require.NoError(t, RebuildClickHouseFinanceDay(ctx, db, start))
	require.NoError(t, RebuildClickHouseFinanceDay(ctx, db, start+86400))
	personal, err := GetFinanceWindow(ctx, start, start+86400, 0, 7)
	require.NoError(t, err)
	assert.Equal(t, raw.Channels, personal.Channels)
	monthly, err := GetFinanceWindow(ctx, start, start+2*86400, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "clickhouse_daily", monthly.Storage)
	require.Len(t, monthly.Channels, 2)
	assert.Equal(t, "18000000000000000010", monthly.Channels[0].ConsumeQuota)
	assert.Equal(t, "9000000000000000010", monthly.Channels[0].KnownModelQuota)
	// Import publication bypasses INSERT triggers on logs. Its new active part
	// must invalidate every affected daily snapshot, including empty-day markers.
	require.NoError(t, db.Exec("CREATE TABLE replacement_stage AS logs").Error)
	require.NoError(t, db.Exec(`INSERT INTO replacement_stage (user_id,created_at,type,channel_id,quota,other,event_id) VALUES (7,?,2,18,700,?,'replacement')`, start, `{"group_ratio":2}`).Error)
	require.NoError(t, db.Exec("ALTER TABLE logs REPLACE PARTITION (202501,'') FROM replacement_stage").Error)
	after, err := GetFinanceWindow(ctx, start, start+2*86400, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "raw", after.Storage)
	assert.Equal(t, start+86400, after.End)
	require.Len(t, after.Channels, 1)
	assert.Equal(t, "700", after.Channels[0].ConsumeQuota)
	require.NoError(t, RebuildClickHouseFinanceDay(ctx, db, start))
	after, err = GetFinanceWindow(ctx, start, start+86400, 0, 7)
	require.NoError(t, err)
	assert.Equal(t, "clickhouse_daily", after.Storage)
	assert.Equal(t, "700", after.Channels[0].ConsumeQuota)
}
