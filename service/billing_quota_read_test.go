package service

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var userQuotaCallbackSeq atomic.Int64

func countUserQuotaQueries(t *testing.T) *atomic.Int64 {
	t.Helper()
	var count atomic.Int64
	// Unique per call: one test may install several counters in sequence.
	callbackName := fmt.Sprintf("test:count_user_quota_read:%s:%d",
		strings.ReplaceAll(t.Name(), "/", "_"), userQuotaCallbackSeq.Add(1))
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			if _, ok := tx.Statement.Dest.(*int); ok {
				count.Add(1)
			}
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove(callbackName)) })
	return &count
}

func seedAuthoritativeQuotaFixture(t *testing.T, userID, dbQuota, cacheQuota int) {
	t.Helper()
	user := &model.User{Id: userID, Username: fmt.Sprintf("quota-read-%d", userID), Password: "hashed", Status: common.UserStatusEnabled, Quota: dbQuota, Group: "default", AffCode: fmt.Sprintf("quota-read-aff-%d", userID)}
	require.NoError(t, model.DB.Create(user).Error)
	t.Cleanup(func() { model.DB.Unscoped().Delete(&model.User{}, userID) })
	require.NoError(t, common.RedisHSetObj(fmt.Sprintf("user:%d", userID), &model.UserBase{Id: userID, Group: "default", Email: "", Quota: cacheQuota, Status: common.UserStatusEnabled, Username: user.Username, Setting: ""}, time.Minute))
}

func TestReadAuthoritativeUserQuotaUsesTrustBand(t *testing.T) {
	setupServiceQuotaRedis(t)

	trustQuota := common.GetTrustQuota()
	seedAuthoritativeQuotaFixture(t, 910001, 100, 0)
	count := countUserQuotaQueries(t)
	quota, fresh, err := readAuthoritativeUserQuota(910001)
	require.NoError(t, err)
	assert.Equal(t, 100, quota)
	assert.True(t, fresh)
	assert.Equal(t, int64(1), count.Load())

	seedAuthoritativeQuotaFixture(t, 910002, 1, trustQuota+trustQuota/2)
	count = countUserQuotaQueries(t)
	quota, fresh, err = readAuthoritativeUserQuota(910002)
	require.NoError(t, err)
	assert.Equal(t, 1, quota)
	assert.True(t, fresh)
	assert.Equal(t, int64(1), count.Load())

	seedAuthoritativeQuotaFixture(t, 910003, 1, trustQuota*5+1)
	count = countUserQuotaQueries(t)
	quota, fresh, err = readAuthoritativeUserQuota(910003)
	require.NoError(t, err)
	assert.Equal(t, trustQuota*5+1, quota)
	assert.False(t, fresh)
	assert.Equal(t, int64(0), count.Load())

	// Keep the boundary explicit: exactly 2x TrustQuota remains in the recheck band.
	seedAuthoritativeQuotaFixture(t, 910004, 2, trustQuota*2)
	count = countUserQuotaQueries(t)
	quota, fresh, err = readAuthoritativeUserQuota(910004)
	require.NoError(t, err)
	assert.Equal(t, 2, quota)
	assert.True(t, fresh)
	assert.Equal(t, int64(1), count.Load())

	// The exported wrapper used by the relay packages must not diverge.
	exportedQuota, exportedFresh, err := ReadAuthoritativeUserQuota(910004)
	require.NoError(t, err)
	assert.Equal(t, 2, exportedQuota)
	assert.True(t, exportedFresh)
}

func seedBillingWalletQuota(t *testing.T, userID, dbQuota, cacheQuota int) {
	t.Helper()
	seedAuthoritativeQuotaFixture(t, userID, dbQuota, cacheQuota)
}

func TestBillingWalletUsesAuthoritativeQuotaInsideTrustBand(t *testing.T) {
	setupServiceQuotaRedis(t)
	// Stale-low cache (0) inside the recheck band: the wallet is actually funded
	// well above TrustQuota, so the authoritative read must restore the bypass.
	seedBillingWalletQuota(t, 910011, common.GetTrustQuota()*3, 0)
	info := &relaycommon.RelayInfo{UserId: 910011, TokenUnlimited: true, IsPlayground: true, OriginModelName: "test-model"}
	session, apiErr := NewBillingSession(periodBillingContext(), info, 8, 1)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, common.GetTrustQuota()*3, info.UserQuota)
	assert.Equal(t, 0, session.GetPreConsumedQuota(), "the authoritative balance restores the trust bypass")
}

func TestBillingWalletReserveRejectsStaleHighWithoutNegativeBalance(t *testing.T) {
	setupServiceQuotaRedis(t)
	seedBillingWalletQuota(t, 910012, 3, 100)
	info := &relaycommon.RelayInfo{UserId: 910012, TokenUnlimited: true, IsPlayground: true, ForcePreConsume: true, OriginModelName: "test-model"}
	_, apiErr := NewBillingSession(periodBillingContext(), info, 200, 150)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	var user model.User
	require.NoError(t, model.DB.First(&user, 910012).Error)
	assert.Equal(t, 3, user.Quota, "a rejected pre-consume must not move the wallet")
}

func TestBillingWalletTrustBandClosesTrustGapButPreservesOutOfBandResidual(t *testing.T) {
	setupServiceQuotaRedis(t)
	seedBillingWalletQuota(t, 910013, 1, common.GetTrustQuota()+common.GetTrustQuota()/2)
	info := &relaycommon.RelayInfo{UserId: 910013, TokenUnlimited: true, IsPlayground: true, OriginModelName: "test-model"}
	_, apiErr := NewBillingSession(periodBillingContext(), info, 8, 2)
	require.NotNil(t, apiErr)
	assert.Equal(t, 1, info.UserQuota)

	// Known residual risk: a cache value above 2x TrustQuota intentionally skips
	// the DB recheck, so this stale-high value still enters the trust bypass.
	outOfBand := common.GetTrustQuota() * 3
	seedBillingWalletQuota(t, 910014, 1, outOfBand)
	info = &relaycommon.RelayInfo{UserId: 910014, TokenUnlimited: true, IsPlayground: true, OriginModelName: "test-model"}
	session, apiErr := NewBillingSession(periodBillingContext(), info, 8, 2)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, outOfBand, info.UserQuota)
	assert.Equal(t, 0, session.GetPreConsumedQuota())
}

func TestBillingWalletRejectionRechecksDatabaseAtMostOnce(t *testing.T) {
	setupServiceQuotaRedis(t)
	seedBillingWalletQuota(t, 910015, 3, 100)
	count := countUserQuotaQueries(t)
	info := &relaycommon.RelayInfo{UserId: 910015, TokenUnlimited: true, IsPlayground: true, ForcePreConsume: true, OriginModelName: "test-model"}
	_, apiErr := NewBillingSession(periodBillingContext(), info, 200, 150)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, int64(1), count.Load())
}

func setupServiceQuotaRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldClient, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB = client
	common.RedisEnabled = true
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		server.Close()
		common.RDB = oldClient
		common.RedisEnabled = oldEnabled
	})
	return server, client
}

// A database failure must surface as an error, never as a zero balance: a
// silent 0 would reject every request with "insufficient quota".
func TestReadAuthoritativeUserQuotaPropagatesDatabaseFailure(t *testing.T) {
	setupServiceQuotaRedis(t)
	oldDB := model.DB
	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = brokenDB
	t.Cleanup(func() {
		model.DB = oldDB
		if sqlDB, dbErr := brokenDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	quota, fresh, err := readAuthoritativeUserQuota(910021)
	require.Error(t, err)
	assert.Equal(t, 0, quota)
	assert.False(t, fresh)
}
