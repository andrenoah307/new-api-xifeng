package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEpayRechargeTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	common.RedisEnabled = false
	require.NoError(t, DB.AutoMigrate(&User{}, &TopUp{}))
}

func insertEpayRechargeFixture(t *testing.T, tradeNo string, provider string, status string, amount int64, money float64, discountCodeID int) (*User, *TopUp) {
	t.Helper()
	user := &User{Username: "epay-user-" + tradeNo, Password: "hashed", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, DB.Create(user).Error)
	topUp := &TopUp{
		UserId:          user.Id,
		Amount:          amount,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: provider,
		Status:          status,
		DiscountCodeId:  discountCodeID,
		CreateTime:      common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(topUp).Error)
	return user, topUp
}

func TestRechargeEpayCreditsPendingOrderAtomically(t *testing.T) {
	setupEpayRechargeTest(t)

	user, topUp := insertEpayRechargeFixture(t, "epay-pending", PaymentProviderEpay, common.TopUpStatusPending, 4, 1.25, 0)
	completed, quotaToAdd, err := RechargeEpay(topUp.TradeNo, "wxpay")
	require.NoError(t, err)
	require.NotNil(t, completed)

	assert.Equal(t, 4*int(common.QuotaPerUnit), quotaToAdd)
	assert.Equal(t, common.TopUpStatusSuccess, completed.Status)
	assert.Equal(t, "wxpay", completed.PaymentMethod)
	assert.Equal(t, int64(quotaToAdd), completed.QuotaGranted)
	assert.Equal(t, common.GetTimestamp(), completed.CompleteTime)

	var gotUser User
	require.NoError(t, DB.First(&gotUser, user.Id).Error)
	assert.Equal(t, 100+quotaToAdd, gotUser.Quota)
}

func TestRechargeEpaySucceedsWithRedisDisabled(t *testing.T) {
	setupEpayRechargeTest(t)

	_, topUp := insertEpayRechargeFixture(t, "epay-cache-disabled", PaymentProviderEpay, common.TopUpStatusPending, 1, 1, 0)
	_, quotaToAdd, err := RechargeEpay(topUp.TradeNo, "alipay")
	require.NoError(t, err)
	assert.Positive(t, quotaToAdd)
}

func TestRechargeEpayIsIdempotent(t *testing.T) {
	setupEpayRechargeTest(t)

	user, topUp := insertEpayRechargeFixture(t, "epay-idempotent", PaymentProviderEpay, common.TopUpStatusPending, 2, 1, 0)
	_, firstQuota, err := RechargeEpay(topUp.TradeNo, "alipay")
	require.NoError(t, err)
	_, secondQuota, err := RechargeEpay(topUp.TradeNo, "alipay")
	require.NoError(t, err)

	assert.Positive(t, firstQuota)
	assert.Zero(t, secondQuota)

	var gotUser User
	require.NoError(t, DB.First(&gotUser, user.Id).Error)
	assert.Equal(t, 100+firstQuota, gotUser.Quota)
}

func TestRechargeEpayRejectsMismatchedProvider(t *testing.T) {
	setupEpayRechargeTest(t)

	user, topUp := insertEpayRechargeFixture(t, "epay-provider-mismatch", PaymentProviderStripe, common.TopUpStatusPending, 3, 1, 0)
	completed, quotaToAdd, err := RechargeEpay(topUp.TradeNo, "alipay")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)
	assert.NotNil(t, completed)
	assert.Zero(t, quotaToAdd)

	var gotTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", topUp.TradeNo).First(&gotTopUp).Error)
	assert.Equal(t, common.TopUpStatusPending, gotTopUp.Status)
	assert.Zero(t, gotTopUp.QuotaGranted)

	var gotUser User
	require.NoError(t, DB.First(&gotUser, user.Id).Error)
	assert.Equal(t, 100, gotUser.Quota)
}

func TestRechargeEpayUsesDiscountAndAmountQuotaBases(t *testing.T) {
	tests := []struct {
		name           string
		amount         int64
		money          float64
		discountCodeID int
		wantQuota      int
	}{
		{name: "amount without discount", amount: 3, money: 99, discountCodeID: 0, wantQuota: 300},
		{name: "money with discount", amount: 3, money: 2.5, discountCodeID: 11, wantQuota: 250},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupEpayRechargeTest(t)
			oldQuotaPerUnit := common.QuotaPerUnit
			common.QuotaPerUnit = 100
			t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

			_, topUp := insertEpayRechargeFixture(t, "epay-basis-"+tt.name, PaymentProviderEpay, common.TopUpStatusPending, tt.amount, tt.money, tt.discountCodeID)
			_, gotQuota, err := RechargeEpay(topUp.TradeNo, "alipay")
			require.NoError(t, err)
			assert.Equal(t, tt.wantQuota, gotQuota)
		})
	}
}

func TestRechargeEpayRejectsMissingOrInvalidOrders(t *testing.T) {
	setupEpayRechargeTest(t)

	_, _, err := RechargeEpay("", "alipay")
	require.Error(t, err)

	_, _, err = RechargeEpay("missing-epay-order", "alipay")
	require.ErrorIs(t, err, ErrTopUpNotFound)

	_, topUp := insertEpayRechargeFixture(t, "epay-invalid-status", PaymentProviderEpay, common.TopUpStatusFailed, 1, 1, 0)
	_, _, err = RechargeEpay(topUp.TradeNo, "alipay")
	require.ErrorIs(t, err, ErrTopUpStatusInvalid)
}

func TestRechargeEpayRollsBackWhenUserQuotaUpdateFails(t *testing.T) {
	setupEpayRechargeTest(t)

	topUp := &TopUp{
		UserId:          987654,
		Amount:          1,
		Money:           1,
		TradeNo:         "epay-missing-user",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(topUp).Error)

	_, _, err := RechargeEpay(topUp.TradeNo, "alipay")
	require.Error(t, err)

	var got TopUp
	require.NoError(t, DB.First(&got, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, got.Status)
	assert.Zero(t, got.QuotaGranted)
}

func TestRechargeEpayRejectsNonPositiveQuota(t *testing.T) {
	setupEpayRechargeTest(t)

	_, topUp := insertEpayRechargeFixture(t, "epay-zero-quota", PaymentProviderEpay, common.TopUpStatusPending, 0, 0, 0)
	_, _, err := RechargeEpay(topUp.TradeNo, "alipay")
	require.Error(t, err)

	var got TopUp
	require.NoError(t, DB.First(&got, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, got.Status)
}

func TestRechargeEpayReturnsTopUpSaveError(t *testing.T) {
	setupEpayRechargeTest(t)
	user, topUp := insertEpayRechargeFixture(t, "epay-save-error", PaymentProviderEpay, common.TopUpStatusPending, 1, 1, 0)
	require.NoError(t, DB.Exec("CREATE TRIGGER epay_save_error BEFORE UPDATE ON top_ups BEGIN SELECT RAISE(ABORT, 'forced save error'); END").Error)
	t.Cleanup(func() { _ = DB.Exec("DROP TRIGGER epay_save_error").Error })

	_, _, err := RechargeEpay(topUp.TradeNo, "alipay")
	require.Error(t, err)
	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, 100, got.Quota)
}

func TestRechargeEpayReturnsQuotaUpdateError(t *testing.T) {
	setupEpayRechargeTest(t)
	_, topUp := insertEpayRechargeFixture(t, "epay-quota-error", PaymentProviderEpay, common.TopUpStatusPending, 1, 1, 0)
	require.NoError(t, DB.Exec("CREATE TRIGGER epay_quota_error BEFORE UPDATE OF quota ON users BEGIN SELECT RAISE(ABORT, 'forced quota error'); END").Error)
	t.Cleanup(func() { _ = DB.Exec("DROP TRIGGER epay_quota_error").Error })

	_, _, err := RechargeEpay(topUp.TradeNo, "alipay")
	require.Error(t, err)
	var got TopUp
	require.NoError(t, DB.First(&got, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, got.Status)
}
