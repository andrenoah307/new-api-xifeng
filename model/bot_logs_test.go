package model

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBotLogsScopeFiltersCursorAndProjection(t *testing.T) {
	oldDB, oldLogDB := DB, LOG_DB
	mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLogDB; common.SetDatabaseTypes(mainType, logType) })
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	var err error
	DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
	require.NoError(t, err)
	for _, db := range []*gorm.DB{DB, LOG_DB} {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
	for id, role := range []int{1, 2, 3, 5, 10, 100, 0, 1} {
		name := "user" + string(rune('a'+id))
		require.NoError(t, DB.Create(&User{Id: id + 1, Username: name, AffCode: name, Role: role, Status: common.UserStatusDisabled}).Error)
		if role == common.RoleGuestUser {
			require.NoError(t, DB.Model(&User{}).Where("id = ?", id+1).Update("role", role).Error)
		}
	}
	require.NoError(t, DB.Delete(&User{}, 8).Error)
	for id := 1; id <= 9; id++ {
		require.NoError(t, LOG_DB.Create(&Log{UserId: id, Username: "sample", CreatedAt: 100, Type: LogTypeConsume, Group: "billed", ModelName: "model", RequestId: "request"}).Error)
	}
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "sample", CreatedAt: 101, Type: LogTypeError, Group: "billed", ModelName: "model", Content: "admin_info secret-channel secret-group", Other: `{"admin_info":{"secret":"never-public"},"nested":{"admin_info":"never-public"},"cache_tokens":20}`, Ip: "secret-ip"}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "sample", CreatedAt: 102, Type: LogTypeManage, Group: "billed"}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "sample", CreatedAt: 103, Type: LogTypeConsume, Group: "different"}).Error)
	filter := BotLogFilter{Start: 1, End: 200, Group: "billed", Username: "sample", ModelName: "model", PageSize: 1}
	page, err := GetBotLogs(context.Background(), 20, filter, "")
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, LogTypeError, page.Items[0].Type)
	assert.True(t, page.HasMore)
	require.NotEmpty(t, page.NextCursor)
	body, err := common.Marshal(page)
	require.NoError(t, err)
	for _, forbidden := range []string{"admin_info", "never-public", "secret-channel", "secret-group", "secret-ip", "billed", `"other"`, `"group"`, `"channel"`} {
		assert.NotContains(t, string(body), forbidden)
	}
	next, err := GetBotLogs(context.Background(), 20, filter, page.NextCursor)
	require.NoError(t, err)
	require.Len(t, next.Items, 1)
	assert.Equal(t, 1, next.Items[0].UserId)
	assert.Equal(t, LogTypeConsume, next.Items[0].Type)
	assert.False(t, next.HasMore)
	_, err = GetBotLogs(context.Background(), 21, filter, page.NextCursor)
	assert.ErrorIs(t, err, ErrInvalidLogCursor)
	changed := filter
	changed.Group = "different"
	_, err = GetBotLogs(context.Background(), 20, changed, page.NextCursor)
	assert.ErrorIs(t, err, ErrInvalidLogCursor)
	_, err = GetBotLogs(context.Background(), 20, filter, page.NextCursor+"tampered")
	assert.ErrorIs(t, err, ErrInvalidLogCursor)
	changed = filter
	changed.Username = "%%"
	_, err = GetBotLogs(context.Background(), 20, changed, "")
	assert.ErrorIs(t, err, ErrInvalidLogCursor)
	changed = filter
	changed.ModelName = "%"
	_, err = GetBotLogs(context.Background(), 20, changed, "")
	assert.ErrorIs(t, err, ErrInvalidLogCursor)
	changed = filter
	changed.Type = LogTypeManage
	_, err = GetBotLogs(context.Background(), 20, changed, "")
	assert.ErrorIs(t, err, ErrInvalidLogCursor)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 1).Update("role", common.RoleAdminUser).Error)
	next, err = GetBotLogs(context.Background(), 20, filter, page.NextCursor)
	require.NoError(t, err)
	assert.Empty(t, next.Items, "role changes must take effect on the next page")

	t.Run("pagination skips private records across database batches", func(t *testing.T) {
		require.NoError(t, DB.Model(&User{}).Where("id = ?", 1).Update("role", common.RoleCommonUser).Error)
		rows := make([]Log, 502)
		for i := range rows {
			rows[i] = Log{UserId: 10, CreatedAt: 100, Type: LogTypeConsume, Group: "batch"}
		}
		rows[0].UserId, rows[501].UserId = 1, 1
		rows[0].RequestId, rows[501].RequestId = "oldest", "newest"
		require.NoError(t, LOG_DB.CreateInBatches(rows, 100).Error)
		filter := BotLogFilter{Start: 1, End: 200, Group: "batch", PageSize: 1}
		first, err := GetBotLogs(context.Background(), 20, filter, "")
		require.NoError(t, err)
		require.Len(t, first.Items, 1)
		assert.Equal(t, "newest", first.Items[0].RequestId)
		require.True(t, first.HasMore)
		last, err := GetBotLogs(context.Background(), 20, filter, first.NextCursor)
		require.NoError(t, err)
		require.Len(t, last.Items, 1)
		assert.Equal(t, "oldest", last.Items[0].RequestId)
		assert.False(t, last.HasMore)
		assert.Empty(t, last.NextCursor)
	})
}
