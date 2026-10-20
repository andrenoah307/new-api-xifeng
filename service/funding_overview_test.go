package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFundingOverviewRedisCache(t *testing.T) {
	oldDB, oldLog, oldRedis, oldEnabled, oldExport := model.DB, model.LOG_DB, common.RDB, common.RedisEnabled, common.DataExportEnabled
	oldCache := fundingOverviewCache
	fundingOverviewCacheOnce = sync.Once{}
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RDB, common.RedisEnabled, common.DataExportEnabled = oldDB, oldLog, oldRedis, oldEnabled, oldExport
		fundingOverviewCache = oldCache
		fundingOverviewCacheOnce = sync.Once{}
	})
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	common.RDB, common.RedisEnabled, common.DataExportEnabled = client, true, true
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.QuotaData{}, &model.Log{}))
	first, err := GetFundingOverview(context.Background(), 3600, 7200)
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&model.QuotaData{}))
	// Rebuild the local cache facade to emulate a second node reading Redis.
	fundingOverviewCacheOnce = sync.Once{}
	cached, err := GetFundingOverview(context.Background(), 3600, 7200)
	require.NoError(t, err)
	assert.Equal(t, first, cached)
	server.FastForward(6 * time.Minute)
	_, err = GetFundingOverview(context.Background(), 3600, 7200)
	assert.Error(t, err)
	server.Close()
	_, err = GetFundingOverview(context.Background(), 3600, 7200)
	assert.Error(t, err)
}
