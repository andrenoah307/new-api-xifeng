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

func TestGetAllLogsOrderingMatchesFilterIndexes(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		_ = DB.Exec("DELETE FROM logs").Error
	})

	logs := []*Log{
		{Id: 10001, CreatedAt: 300, ChannelId: 901, Group: "log-order-group", Type: LogTypeConsume},
		{Id: 10002, CreatedAt: 200, ChannelId: 901, Group: "log-order-group", Type: LogTypeConsume},
		{Id: 10003, CreatedAt: 100, ChannelId: 901, Group: "log-order-group", Type: LogTypeConsume},
	}
	require.NoError(t, DB.Create(&logs).Error)

	tests := []struct {
		name    string
		channel int
		group   string
		wantIds []int
	}{
		{name: "channel", channel: 901, wantIds: []int{10001, 10002, 10003}},
		{name: "group", group: "log-order-group", wantIds: []int{10001, 10002, 10003}},
		{name: "channel and group", channel: 901, group: "log-order-group", wantIds: []int{10001, 10002, 10003}},
		{name: "default", wantIds: []int{10003, 10002, 10001}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, total, err := GetAllLogs(
				LogTypeUnknown, 0, 0, "", "", "", 0, 10, tt.channel, tt.group, "", "", 0,
			)
			require.NoError(t, err)
			assert.Equal(t, int64(len(logs)), total)
			require.Len(t, got, len(tt.wantIds))

			gotIds := make([]int, len(got))
			for i := range got {
				gotIds[i] = got[i].Id
			}
			assert.Equal(t, tt.wantIds, gotIds)
		})
	}
}
