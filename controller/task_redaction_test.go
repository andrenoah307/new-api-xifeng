package controller

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTasksToDtoUsesAdminProjectionOnlyForAdminRequests(t *testing.T) {
	originalDB := model.DB
	originalRedisEnabled := common.RedisEnabled
	t.Cleanup(func() {
		model.DB = originalDB
		common.RedisEnabled = originalRedisEnabled
	})
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "task-projection.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	require.NoError(t, db.Create(&model.User{Id: 902, Username: "task-user", Group: "default", Status: common.UserStatusEnabled}).Error)
	task := &model.Task{
		TaskID:    "task-controller-sentinel",
		Platform:  constant.TaskPlatformSuno,
		UserId:    902,
		Group:     "internal-task-group-sentinel",
		ChannelId: 734,
	}

	userDtos := tasksToDto([]*model.Task{task}, false)
	adminDtos := tasksToDto([]*model.Task{task}, true)
	require.Len(t, userDtos, 1)
	require.Len(t, adminDtos, 1)
	assert.Equal(t, "", userDtos[0].Group)
	assert.Equal(t, "internal-task-group-sentinel", adminDtos[0].Group)
	assert.Equal(t, 734, userDtos[0].ChannelId)
	assert.Equal(t, 734, adminDtos[0].ChannelId)
	assert.Equal(t, "task-user", adminDtos[0].Username)
}
