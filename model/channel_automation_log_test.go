package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelAutomationAuditVisibility(t *testing.T) {
	for _, backend := range []string{"sqlite", "clickhouse"} {
		t.Run(backend, func(t *testing.T) {
			if backend == "clickhouse" {
				setupClickHouseLogTest(t)
			} else {
				require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
			}
			RecordChannelAutomationLog(12801, "channel.pressure_cooling.recovered", "Cooldown expired", map[string]any{"consecutive": 2})
			ctx := context.Background()
			logs, total, err := GetAllLogs(ctx, LogTypeManage, 0, 0, "", "", "", 0, 10, 12801, "", "", "", 0)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, logs, 1)
			assert.Equal(t, 0, logs[0].UserId)
			assert.Equal(t, 0, logs[0].TokenId)
			assert.Contains(t, logs[0].Content, "Cooldown expired")
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			assert.Contains(t, other, "admin_info")
			for _, kind := range []int{LogTypeUnknown, LogTypeManage} {
				userLogs, count, err := GetUserLogs(ctx, 12802, kind, 0, 0, "", "", 0, 10, "", "", "", 0)
				require.NoError(t, err)
				assert.Empty(t, userLogs)
				assert.Zero(t, count)
				var exported []*Log
				require.NoError(t, ExportUserLogs(ctx, 12802, kind, 0, 0, "", "", "", "", "", func(batch []*Log) error { exported = append(exported, batch...); return nil }))
				assert.Empty(t, exported, "self and offline exports must not contain system events")
			}
			tokenLogs, err := GetLogByTokenId(12803)
			require.NoError(t, err)
			assert.Empty(t, tokenLogs)
			var exported []*Log
			require.NoError(t, ExportAllLogs(ctx, LogTypeManage, 0, 0, "", "", "", 12801, "", "", "", func(batch []*Log) error { exported = append(exported, batch...); return nil }))
			require.Len(t, exported, 1)
			assert.Contains(t, exported[0].Content, "Cooldown expired")
		})
	}
}
