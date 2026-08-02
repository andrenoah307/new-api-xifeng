package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	commonRelay "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyStoresTokenPeriodAttribution(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	require.NoError(t, db.AutoMigrate(&Midjourney{}))

	task := &Midjourney{
		UserId:             9,
		MjId:               "mj-period-1",
		TokenId:            77,
		TokenPeriodStartAt: 1785657600,
		Quota:              123,
	}
	require.NoError(t, db.Create(task).Error)

	var loaded Midjourney
	require.NoError(t, db.First(&loaded, task.Id).Error)
	assert.Equal(t, 77, loaded.TokenId)
	assert.Equal(t, int64(1785657600), loaded.TokenPeriodStartAt)
}

func TestInitTaskCopiesTokenPeriodAttribution(t *testing.T) {
	info := &commonRelay.RelayInfo{
		TokenId:            11,
		TokenPeriodStartAt: 1785657600,
		UserId:             3,
		UsingGroup:         "default",
	}
	task := InitTask(constant.TaskPlatform("video"), info)
	require.NotNil(t, task)
	assert.Equal(t, 11, task.PrivateData.TokenId)
	assert.Equal(t, int64(1785657600), task.PrivateData.TokenPeriodStartAt)
}
