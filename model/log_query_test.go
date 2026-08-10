package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManageLogsAreVisibleBySubjectUsernameAndSelfQuery(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	t.Cleanup(func() {
		_ = DB.Exec("DELETE FROM logs").Error
		_ = DB.Exec("DELETE FROM users").Error
	})

	target := &User{
		Username: "log-query-target",
		Password: "password",
		AffCode:  "log-query-target-code",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	admin := &User{
		Username: "log-query-admin",
		Password: "password",
		AffCode:  "log-query-admin-code",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(target).Error)
	require.NoError(t, DB.Create(admin).Error)

	logs := []*Log{
		{
			UserId:    target.Id,
			Username:  target.Username,
			CreatedAt: 100,
			Type:      LogTypeManage,
			Content:   "target quota changed",
		},
		{
			UserId:    admin.Id,
			Username:  admin.Username,
			CreatedAt: 101,
			Type:      LogTypeManage,
			Content:   "administrator resource event",
		},
		{
			UserId:    target.Id,
			Username:  target.Username,
			CreatedAt: 102,
			Type:      LogTypeConsume,
			Content:   "target consumption",
		},
	}
	require.NoError(t, DB.Create(&logs).Error)

	allLogs, allTotal, err := GetAllLogs(
		LogTypeManage, 0, 0, "", target.Username, "", 0, 10, 0, "", "", "", 0,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(1), allTotal)
	require.Len(t, allLogs, 1)
	assert.Equal(t, target.Id, allLogs[0].UserId)
	assert.Equal(t, target.Username, allLogs[0].Username)

	userLogs, userTotal, err := GetUserLogs(
		target.Id, LogTypeManage, 0, 0, "", "", 0, 10, "", "", "", 0,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(1), userTotal)
	require.Len(t, userLogs, 1)
	assert.Equal(t, "target quota changed", userLogs[0].Content)
}
