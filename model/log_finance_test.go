package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelFinanceStatsPreservesHistoricalBasisAndUnresolvedRefunds(t *testing.T) {
	oldType := common.LogDatabaseType()
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { common.SetLogDatabaseType(oldType); DB.Exec("DELETE FROM logs") })
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	rows := []Log{
		{CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 200, Other: `{"group_ratio":2}`},
		{CreatedAt: 101, Type: 2, ChannelId: 7, Quota: 300, Other: `{"group_ratio":3}`},
		{CreatedAt: 102, Type: 2, ChannelId: 7, Quota: 10, Other: `{"group_ratio":0}`},
		{CreatedAt: 103, Type: 2, ChannelId: 7, Quota: 20, Other: `{"group_ratio":1,"admin_info":{"quota_saturation":{"reason":"fixture"}}}`},
		{CreatedAt: 104, Type: 6, ChannelId: 7, Quota: 40, Other: `{"group_ratio":1}`},
		{CreatedAt: 105, Type: 2, ChannelId: 8, Quota: 90, Other: `{}`},
		{CreatedAt: 106, Type: 2, ChannelId: 8, Quota: 10, Other: `invalid`},
		{CreatedAt: 110, Type: 2, ChannelId: 7, Quota: 999, Other: `{"group_ratio":1}`},
	}
	require.NoError(t, DB.Create(&rows).Error)
	result, err := GetChannelFinanceStats(context.Background(), 100, 110, 0)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "530", result[0].ConsumeQuota)
	assert.Equal(t, "40", result[0].RefundQuota)
	assert.Equal(t, "490", result[0].NetQuota)
	assert.Equal(t, "200", result[0].KnownModelQuota)
	assert.Equal(t, int64(2), result[0].MissingBasis)
	assert.Equal(t, int64(1), result[0].UnresolvedRefunds)
	assert.Equal(t, int64(2), result[1].MissingBasis)
	assert.Equal(t, "100", result[1].ConsumeQuota)
	filtered, err := GetChannelFinanceStats(context.Background(), 100, 102, 7)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "500", filtered[0].ConsumeQuota)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = GetChannelFinanceStats(ctx, 100, 110, 0)
	require.Error(t, err)
}
