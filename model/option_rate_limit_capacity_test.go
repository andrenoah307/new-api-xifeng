package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRateLimitCapacityOptionDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := DB
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func TestRateLimitCapacityCardOptionPersistsAndUpdatesRuntime(t *testing.T) {
	db := setupRateLimitCapacityOptionDB(t)
	previous := setting.IsRateLimitCapacityCardEnabled()
	t.Cleanup(func() { setting.SetRateLimitCapacityCardEnabled(previous) })
	setting.SetRateLimitCapacityCardEnabled(false)

	require.NoError(t, UpdateOption("RateLimitCapacityCardEnabled", "true"))
	assert.True(t, setting.IsRateLimitCapacityCardEnabled())

	var stored Option
	require.NoError(t, db.First(&stored, "key = ?", "RateLimitCapacityCardEnabled").Error)
	assert.Equal(t, "true", stored.Value)

	require.NoError(t, UpdateOption("RateLimitCapacityCardEnabled", "false"))
	assert.False(t, setting.IsRateLimitCapacityCardEnabled())
}

func TestRateLimitCapacityCardOptionMissingRowReloadsAsDefaultFalse(t *testing.T) {
	setupRateLimitCapacityOptionDB(t)
	previous := setting.IsRateLimitCapacityCardEnabled()
	common.OptionMapRWMutex.RLock()
	previousOptionMap := make(map[string]string, len(common.OptionMap))
	for key, value := range common.OptionMap {
		previousOptionMap[key] = value
	}
	common.OptionMapRWMutex.RUnlock()
	t.Cleanup(func() {
		setting.SetRateLimitCapacityCardEnabled(previous)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	setting.SetRateLimitCapacityCardEnabled(false)
	InitOptionMap()

	assert.False(t, setting.IsRateLimitCapacityCardEnabled())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "false", common.OptionMap["RateLimitCapacityCardEnabled"])
	common.OptionMapRWMutex.RUnlock()
}
