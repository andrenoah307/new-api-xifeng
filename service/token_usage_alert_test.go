package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func tokenUsageAlertFixture(t *testing.T) (*miniredis.Miniredis, model.Token) {
	t.Helper()
	oldDB, oldRedis, oldEnabled := model.DB, common.RDB, common.RedisEnabled
	oldConfig, oldSend := setting.TokenUsageAlertsJSON(), sendTokenUsageAlertEmail
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "alerts.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}, &model.Option{}, &model.TokenUsageAlertDelivery{}, &model.TokenUsageAlertBudget{}))
	model.DB = db
	server := miniredis.RunT(t)
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	config := `{"enabled":true,"rules":[{"id":"half","percent":50,"cooldown_days":7,"scope":"any"},{"id":"high","percent":70,"cooldown_days":3,"scope":"any"}]}`
	require.NoError(t, setting.UpdateTokenUsageAlerts(config))
	require.NoError(t, db.Create(&model.Option{Key: setting.TokenUsageAlertOption, Value: config}).Error)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "<user>", Email: "test@example.test", Status: common.UserStatusEnabled}).Error)
	token := model.Token{Id: 2, UserId: 1, Name: "<img src=x>", Key: "private-key-never-in-email", Status: common.TokenStatusEnabled, UsedQuota: 80, RemainQuota: 20}
	require.NoError(t, db.Create(&token).Error)
	t.Cleanup(func() {
		_ = common.RDB.Close()
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
		model.DB, common.RDB, common.RedisEnabled = oldDB, oldRedis, oldEnabled
		sendTokenUsageAlertEmail = oldSend
		require.NoError(t, setting.UpdateTokenUsageAlerts(oldConfig))
	})
	return server, token
}

func TestTokenUsageAlertRedisLossAndRuleIndependence(t *testing.T) {
	server, token := tokenUsageAlertFixture(t)
	ctx := context.Background()
	sent := 0
	sendTokenUsageAlertEmail = func(_ context.Context, subject, to, body string) (string, error) {
		sent++
		assert.Equal(t, "test@example.test", to)
		assert.Contains(t, body, "&lt;img src=x&gt;")
		assert.NotContains(t, body, token.Key)
		return "accepted", nil
	}
	require.NoError(t, cacheAndQueueTokenUsageAlerts(ctx, []model.Token{token}, time.Now()))
	assert.Equal(t, int64(2), common.RDB.ZCard(ctx, tokenAlertPrefix+"queue").Val())
	processTokenUsageAlert("1:2:half")
	assert.Equal(t, 1, sent)
	server.FlushAll()
	processTokenUsageAlert("1:2:half")
	assert.Equal(t, 1, sent, "durable cooldown survives complete Redis loss")
	processTokenUsageAlert("1:2:high")
	assert.Equal(t, 2, sent, "different rule has an independent cooldown")
	var rows []model.TokenUsageAlertDelivery
	require.NoError(t, model.DB.Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Equal(t, "sent", row.Status)
	}
}

func TestTokenUsageAlertRevalidatesQueueAndUnknownDelivery(t *testing.T) {
	server, _ := tokenUsageAlertFixture(t)
	sent := 0
	sendTokenUsageAlertEmail = func(_ context.Context, _, _, _ string) (string, error) { sent++; return "unknown", nil }
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = 2").Updates(map[string]any{"used_quota": 0, "remain_quota": 100}).Error)
	processTokenUsageAlert("1:2:half")
	assert.Zero(t, sent, "quota restored after queueing")
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = 2").Updates(map[string]any{"used_quota": 80, "remain_quota": 20}).Error)
	require.NoError(t, model.DB.Model(&model.Option{}).Where(&model.Option{Key: setting.TokenUsageAlertOption}).Update("value", `{"enabled":false,"rules":[]}`).Error)
	processTokenUsageAlert("1:2:half")
	assert.Zero(t, sent, "persisted disable overrides stale process config")
	require.NoError(t, model.DB.Model(&model.Option{}).Where(&model.Option{Key: setting.TokenUsageAlertOption}).Update("value", setting.TokenUsageAlertsJSON()).Error)
	processTokenUsageAlert("1:2:half")
	assert.Equal(t, 1, sent)
	server.FlushAll()
	processTokenUsageAlert("1:2:half")
	assert.Equal(t, 1, sent)
	var row model.TokenUsageAlertDelivery
	require.NoError(t, model.DB.First(&row).Error)
	assert.Equal(t, "unknown", row.Status)
}

func TestTokenUsageAlertPacingAndBoundedQueue(t *testing.T) {
	server, token := tokenUsageAlertFixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	require.NoError(t, cacheAndQueueTokenUsageAlerts(ctx, []model.Token{token, token}, time.Now()))
	assert.Equal(t, int64(2), common.RDB.ZCard(ctx, tokenAlertPrefix+"queue").Val(), "duplicates coalesce")
	keys := []string{tokenAlertPrefix + "queue", tokenAlertPrefix + "send-slot"}
	first, err := popTokenUsageAlert.Run(ctx, common.RDB, keys, now).Text()
	require.NoError(t, err)
	assert.NotEmpty(t, first)
	second, err := popTokenUsageAlert.Run(ctx, common.RDB, keys, now).Text()
	require.NoError(t, err)
	assert.Empty(t, second)
	server.FastForward(6 * time.Second)
	second, err = popTokenUsageAlert.Run(ctx, common.RDB, keys, now).Text()
	require.NoError(t, err)
	assert.NotEmpty(t, second)
	members := make([]*redis.Z, 10000)
	for i := range members {
		members[i] = &redis.Z{Score: float64(i), Member: i}
	}
	require.NoError(t, common.RDB.ZAdd(ctx, tokenAlertPrefix+"queue", members...).Err())
	queued, err := enqueueTokenUsageAlert.Run(ctx, common.RDB, []string{tokenAlertPrefix + "queue", tokenAlertPrefix + "cooldown:new"}, "new", now).Int()
	require.NoError(t, err)
	assert.Equal(t, -1, queued)
	assert.Equal(t, int64(10000), common.RDB.ZCard(ctx, tokenAlertPrefix+"queue").Val())
}
