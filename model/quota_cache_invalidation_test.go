package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedQuotaCacheUser(t *testing.T, username string, quota int) *User {
	t.Helper()
	user := &User{Username: username, Password: "hashed", Status: common.UserStatusEnabled, Quota: quota, Group: "default", Email: username + "@example.com", AffCode: username + "-aff"}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&User{}, user.Id) })
	require.NoError(t, populateUserCache(*user))
	return user
}

func assertUserCacheAbsent(t *testing.T, userID int) {
	t.Helper()
	exists, err := common.RDB.Exists(context.Background(), getUserCacheKey(userID)).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
}

func assertUserCachePresent(t *testing.T, userID int) {
	t.Helper()
	exists, err := common.RDB.Exists(context.Background(), getUserCacheKey(userID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)
}

func assertUserCacheInvalidated(t *testing.T, userID int) {
	t.Helper()
	key := getUserCacheKey(userID)
	exists, err := common.RDB.Exists(context.Background(), key).Result()
	require.NoError(t, err)
	if exists == 0 {
		return
	}
	var user User
	require.NoError(t, DB.First(&user, userID).Error)
	quota, err := common.RDB.HGet(context.Background(), key, "Quota").Int()
	require.NoError(t, err)
	assert.Equal(t, user.Quota, quota)
}

func newPendingTopUp(t *testing.T, userID int, tradeNo, provider string) *TopUp {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          1,
		Money:           1,
		TradeNo:         tradeNo,
		PaymentMethod:   provider,
		PaymentProvider: provider,
		Status:          common.TopUpStatusPending,
		CreateTime:      1,
	}
	require.NoError(t, DB.Create(topUp).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&TopUp{}, topUp.Id) })
	return topUp
}

func TestQuotaCreditPathsInvalidateCacheAfterCommit(t *testing.T) {
	_, _ = setupUserCacheRedis(t)

	tests := []struct {
		name string
		run  func(*User, *TopUp) error
	}{
		{name: "stripe", run: func(user *User, topUp *TopUp) error {
			return Recharge(topUp.TradeNo, "stripe-customer", "127.0.0.1")
		}},
		{name: "manual", run: func(user *User, topUp *TopUp) error {
			return ManualCompleteTopUp(topUp.TradeNo, "127.0.0.1")
		}},
		{name: "creem", run: func(user *User, topUp *TopUp) error {
			return RechargeCreem(topUp.TradeNo, "", "", "127.0.0.1")
		}},
		{name: "waffo", run: func(user *User, topUp *TopUp) error {
			return RechargeWaffo(topUp.TradeNo, "127.0.0.1")
		}},
		{name: "waffo-pancake", run: func(user *User, topUp *TopUp) error {
			return RechargeWaffoPancake(topUp.TradeNo)
		}},
	}

	providers := []string{PaymentProviderStripe, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake}
	for index, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			user := seedQuotaCacheUser(t, fmt.Sprintf("cache-credit-%d", index), 100)
			topUp := newPendingTopUp(t, user.Id, fmt.Sprintf("cache-credit-%d", index), providers[index])
			require.NoError(t, testCase.run(user, topUp))
			assertUserCacheInvalidated(t, user.Id)
		})
	}
}

func TestQuotaCreditRollbacksKeepCache(t *testing.T) {
	_, _ = setupUserCacheRedis(t)
	tests := []struct {
		name string
		call func(*TopUp) error
	}{
		{name: "stripe", call: func(topUp *TopUp) error {
			return Recharge(topUp.TradeNo, "stripe-customer", "127.0.0.1")
		}},
		{name: "manual", call: func(topUp *TopUp) error {
			topUp.Amount = 0
			require.NoError(t, DB.Save(topUp).Error)
			return ManualCompleteTopUp(topUp.TradeNo, "127.0.0.1")
		}},
		{name: "creem", call: func(topUp *TopUp) error {
			return RechargeCreem(topUp.TradeNo, "", "", "127.0.0.1")
		}},
		{name: "waffo", call: func(topUp *TopUp) error {
			return RechargeWaffo(topUp.TradeNo, "127.0.0.1")
		}},
		{name: "waffo-pancake", call: func(topUp *TopUp) error {
			return RechargeWaffoPancake(topUp.TradeNo)
		}},
	}
	for index, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			user := seedQuotaCacheUser(t, fmt.Sprintf("cache-credit-rollback-%d", index), 100)
			topUp := newPendingTopUp(t, user.Id, fmt.Sprintf("cache-credit-rollback-%d", index), "epay")
			err := testCase.call(topUp)
			require.Error(t, err)
			assertUserCachePresent(t, user.Id)
		})
	}
}

func TestRedeemInvalidatesCacheAfterCommitAndKeepsItOnRollback(t *testing.T) {
	_, _ = setupUserCacheRedis(t)
	require.NoError(t, DB.AutoMigrate(&Redemption{}))
	user := seedQuotaCacheUser(t, "cache-redeem", 100)
	redemption := &Redemption{Key: "cache-redeem-key", Status: common.RedemptionCodeStatusEnabled, Quota: 20, CreatedTime: 1}
	require.NoError(t, DB.Create(redemption).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&Redemption{}, redemption.Id) })
	quota, err := Redeem(redemption.Key, user.Id)
	require.NoError(t, err)
	assert.Equal(t, 20, quota)
	assertUserCacheInvalidated(t, user.Id)

	rollbackUser := seedQuotaCacheUser(t, "cache-redeem-rollback", 100)
	used := &Redemption{Key: "cache-redeem-used", Status: common.RedemptionCodeStatusUsed, Quota: 20, CreatedTime: 1}
	require.NoError(t, DB.Create(used).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&Redemption{}, used.Id) })
	_, err = Redeem(used.Key, rollbackUser.Id)
	require.Error(t, err)
	assertUserCachePresent(t, rollbackUser.Id)

}

func TestTransferAndBatchQuotaWritesInvalidateCache(t *testing.T) {
	_, _ = setupUserCacheRedis(t)
	transferAmount := int(common.QuotaPerUnit)
	user := seedQuotaCacheUser(t, "cache-transfer", 100)
	user.AffQuota = transferAmount
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{"aff_quota": transferAmount}).Error)
	require.NoError(t, user.TransferAffQuotaToQuota(transferAmount))
	assertUserCacheInvalidated(t, user.Id)

	batchUser := seedQuotaCacheUser(t, "cache-batch", 100)
	updateUserQuotaUsedQuotaAndRequestCount(batchUser.Id, 5, 3, 1)
	assertUserCacheInvalidated(t, batchUser.Id)
}
