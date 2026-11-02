package model

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func tokenAlertDatabase(t *testing.T) {
	t.Helper()
	prior := DB
	var dialect gorm.Dialector = sqlite.Open(filepath.Join(t.TempDir(), "alerts.db"))
	if dsn := os.Getenv("TOKEN_ALERT_TEST_MYSQL_DSN"); dsn != "" {
		dialect = mysql.Open(dsn)
	} else if dsn := os.Getenv("TOKEN_ALERT_TEST_POSTGRES_DSN"); dsn != "" {
		dialect = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialect, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	if dialect.Name() == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxOpenConns(8)
		require.NoError(t, db.Migrator().DropTable(&TokenUsageAlertDelivery{}, &TokenUsageAlertBudget{}))
	}
	require.NoError(t, db.AutoMigrate(&TokenUsageAlertDelivery{}, &TokenUsageAlertBudget{}))
	DB = db
	t.Cleanup(func() { DB = prior; _ = sqlDB.Close() })
}

func TestTokenUsageAlertQuotaScopesAndPeriodReset(t *testing.T) {
	now := time.Date(2026, 11, 2, 4, 0, 0, 0, time.FixedZone("UTC+8", 28800))
	token := Token{Id: 1, UserId: 2, Status: common.TokenStatusEnabled, UsedQuota: 70, RemainQuota: 30, PeriodType: "days", PeriodDays: 1, PeriodQuotaLimit: 100, PeriodUsedQuota: 80}
	start, _, ok := common.TokenPeriodBounds(token.PeriodType, 1, 0, now)
	require.True(t, ok)
	token.PeriodStartAt = start
	rule := setting.TokenUsageAlertRule{Percent: 70, Scope: "any"}
	usage, matched := MatchTokenUsageAlert(TokenUsageAlertUsages(token, now), rule)
	require.True(t, matched)
	assert.Equal(t, "period", usage.Scope)
	assert.Equal(t, int64(80), usage.Used)
	token.UnlimitedQuota = true
	usage, matched = MatchTokenUsageAlert(TokenUsageAlertUsages(token, now), rule)
	require.True(t, matched)
	assert.Equal(t, "period", usage.Scope)
	_, matched = MatchTokenUsageAlert(TokenUsageAlertUsages(token, now.AddDate(0, 0, 1)), rule)
	assert.False(t, matched, "old bucket must not alert after reset")
	token.PeriodType = ""
	assert.Empty(t, TokenUsageAlertUsages(token, now), "unlimited token without period limit")
	token.UnlimitedQuota = false
	token.Status = common.TokenStatusDisabled
	assert.Empty(t, TokenUsageAlertUsages(token, now))
	token.Status = common.TokenStatusExhausted
	token.RemainQuota = -5
	_, matched = MatchTokenUsageAlert(TokenUsageAlertUsages(token, now), rule)
	assert.True(t, matched, "exhaustion must not suppress warnings")
	_, matched = MatchTokenUsageAlert([]TokenUsageAlertUsage{{Scope: "period", Used: math.MaxInt64 - 1, Limit: math.MaxInt64}}, setting.TokenUsageAlertRule{Percent: 100, Scope: "period"})
	assert.False(t, matched)
	_, matched = MatchTokenUsageAlert([]TokenUsageAlertUsage{{Scope: "period", Used: math.MaxInt64, Limit: math.MaxInt64}}, setting.TokenUsageAlertRule{Percent: 100, Scope: "period"})
	assert.True(t, matched)
}

func TestTokenUsageAlertIndependentCooldownAndConcurrency(t *testing.T) {
	tokenAlertDatabase(t)
	ctx := context.Background()
	now := int64(1000000)
	rule := setting.TokenUsageAlertRule{ID: "half", Percent: 50, CooldownDays: 7, Scope: "any"}
	var wg sync.WaitGroup
	claims := make(chan bool, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, claimed, err := ClaimTokenUsageAlert(ctx, 1, 2, rule, now)
			claims <- claimed
			errs <- err
		}()
	}
	wg.Wait()
	close(claims)
	close(errs)
	count := 0
	for claimed := range claims {
		if claimed {
			count++
		}
	}
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, 1, count)
	_, claimed, err := ClaimTokenUsageAlert(ctx, 1, 2, rule, now+86400)
	require.NoError(t, err)
	assert.False(t, claimed, "period reset does not reset cooldown")
	critical := setting.TokenUsageAlertRule{ID: "critical", Percent: 70, CooldownDays: 3, Scope: "period"}
	_, claimed, err = ClaimTokenUsageAlert(ctx, 1, 2, critical, now+86400)
	require.NoError(t, err)
	assert.True(t, claimed)
	_, claimed, err = ClaimTokenUsageAlert(ctx, 1, 3, rule, now)
	require.NoError(t, err)
	assert.True(t, claimed, "another key is independent")
	_, claimed, err = ClaimTokenUsageAlert(ctx, 1, 2, rule, now+7*86400)
	require.NoError(t, err)
	assert.True(t, claimed)
	var budget TokenUsageAlertBudget
	require.NoError(t, DB.First(&budget, 1).Error)
	assert.Equal(t, 3, budget.Rows, "reusing a delivery must not grow the table")
}

func TestTokenUsageAlertDeliveryRecoveryAndBudget(t *testing.T) {
	tokenAlertDatabase(t)
	ctx := context.Background()
	now := int64(1000000)
	rule := setting.TokenUsageAlertRule{ID: "half", Percent: 50, CooldownDays: 7, Scope: "any"}
	delivery, claimed, err := ClaimTokenUsageAlert(ctx, 1, 2, rule, now)
	require.NoError(t, err)
	require.True(t, claimed)
	next, err := FinishTokenUsageAlert(ctx, delivery, "retryable", now+1)
	require.NoError(t, err)
	assert.Equal(t, now+61, next)
	_, claimed, err = ClaimTokenUsageAlert(ctx, 1, 2, rule, now+60)
	require.NoError(t, err)
	assert.False(t, claimed)
	delivery, claimed, err = ClaimTokenUsageAlert(ctx, 1, 2, rule, next)
	require.NoError(t, err)
	require.True(t, claimed)
	assert.Equal(t, 2, delivery.Attempts)
	_, err = FinishTokenUsageAlert(ctx, delivery, "unknown", next+1)
	require.NoError(t, err)
	_, claimed, err = ClaimTokenUsageAlert(ctx, 1, 2, rule, next+1000)
	require.NoError(t, err)
	assert.False(t, claimed, "unknown SMTP outcome must not be replayed")
	_, err = FinishTokenUsageAlert(ctx, delivery, "sent", next+2)
	require.Error(t, err, "stale worker must not overwrite result")
	_, _, err = ClaimTokenUsageAlert(ctx, 1, 3, rule, now)
	require.NoError(t, err)
	retries, err := MaintainTokenUsageAlerts(ctx, now+200)
	require.NoError(t, err)
	assert.Empty(t, retries)
	var interrupted TokenUsageAlertDelivery
	require.NoError(t, DB.First(&interrupted, "id = ?", TokenUsageAlertID(1, 3, "half")).Error)
	assert.Equal(t, "unknown", interrupted.Status)
	require.NoError(t, DB.Model(&TokenUsageAlertBudget{}).Where("id = 1").Update("entry_count", TokenUsageAlertCapacity).Error)
	_, claimed, err = ClaimTokenUsageAlert(ctx, 1, 4, rule, now)
	require.Error(t, err)
	assert.False(t, claimed)
	require.NoError(t, DB.Model(&TokenUsageAlertBudget{}).Where("id = 1").Update("entry_count", 2).Error)
	_, err = MaintainTokenUsageAlerts(ctx, next+7*86400+200)
	require.NoError(t, err)
	var budget TokenUsageAlertBudget
	require.NoError(t, DB.First(&budget, 1).Error)
	assert.Zero(t, budget.Rows)
}

func TestTokenUsageAlertRetryLimit(t *testing.T) {
	tokenAlertDatabase(t)
	ctx := context.Background()
	now := int64(1000000)
	rule := setting.TokenUsageAlertRule{ID: "half", Percent: 50, CooldownDays: 7, Scope: "any"}
	for attempt := 1; attempt <= 3; attempt++ {
		delivery, claimed, err := ClaimTokenUsageAlert(ctx, 1, 2, rule, now)
		require.NoError(t, err)
		require.True(t, claimed)
		assert.Equal(t, attempt, delivery.Attempts)
		next, err := FinishTokenUsageAlert(ctx, delivery, "retryable", now+1)
		require.NoError(t, err)
		if attempt < 3 {
			require.Greater(t, next, now)
			now = next
		} else {
			assert.Zero(t, next)
		}
	}
	_, claimed, err := ClaimTokenUsageAlert(ctx, 1, 2, rule, now+3600)
	require.NoError(t, err)
	assert.False(t, claimed)
	var delivery TokenUsageAlertDelivery
	require.NoError(t, DB.First(&delivery).Error)
	assert.Equal(t, "failed", delivery.Status)
}
