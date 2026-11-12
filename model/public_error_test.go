package model

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserErrorLogsRedactHistoricalDetails(t *testing.T) {
	rows := []*Log{
		{Type: LogTypeError, Content: "status_code=503, group private-group unavailable", RequestId: "local-log-id", UpstreamRequestId: "private-id", Other: `{"admin_info":{"channel_name":"private-channel"},"error_code":{"provider":"private-provider"}}`},
		{Type: LogTypeError, Content: "status_code=400, group: Extra inputs are not permitted"},
		{Type: LogTypeError, Content: "account private-account disabled", Other: `{"status_code":403}`},
		{Type: LogTypeConsume, Content: "group untouched"},
	}
	formatUserLogs(rows, 0)
	assert.Contains(t, rows[0].Content, "上游服务暂不可用，请稍后重试")
	assert.Contains(t, rows[0].Content, "local-log-id")
	assert.Empty(t, rows[0].UpstreamRequestId)
	assert.NotContains(t, rows[0].Other, "private-")
	assert.Equal(t, "status_code=400, group: Extra inputs are not permitted", rows[1].Content)
	assert.Equal(t, "请求参数有误，请检查请求内容", rows[2].Content)
	assert.Equal(t, "group untouched", rows[3].Content)
}

func TestUserErrorExportKeepsStoredDiagnosticsAndCursor(t *testing.T) {
	const userID = 987654
	require.NoError(t, DB.Where("user_id = ?", userID).Delete(&Log{}).Error)
	t.Cleanup(func() { _ = DB.Where("user_id = ?", userID).Delete(&Log{}).Error })
	row := &Log{UserId: userID, Type: LogTypeError, Content: "group private-group unavailable", RequestId: "local-export-id", Other: `{"status_code":503,"admin_info":{"channel_name":"private-channel"}}`}
	require.NoError(t, DB.Create(row).Error)
	calls := 0
	err := ExportUserLogs(context.Background(), userID, LogTypeError, 0, 0, "", "", "", "", "", func(rows []*Log) error {
		calls++
		if calls > 1 {
			return errors.New("export cursor repeated the batch")
		}
		require.Len(t, rows, 1)
		assert.Contains(t, rows[0].Content, "local-export-id")
		assert.NotContains(t, rows[0].Content, "private-group")
		assert.NotContains(t, rows[0].Other, "private-channel")
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	var stored Log
	require.NoError(t, DB.First(&stored, row.Id).Error)
	assert.Equal(t, row.Content, stored.Content)
	assert.Equal(t, row.Other, stored.Other)
}
