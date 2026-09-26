package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func rechargeDiscountOrder(order *TopUp) error {
	switch order.PaymentProvider {
	case PaymentProviderStripe:
		return Recharge(order.TradeNo, "customer", "")
	case PaymentProviderEpay:
		_, _, err := RechargeEpay(order.TradeNo, "alipay")
		return err
	case PaymentProviderWaffo:
		return RechargeWaffo(order.TradeNo, "")
	default:
		return RechargeWaffoPancake(order.TradeNo)
	}
}

func TestRechargeDiscountBonusAtomicReplay(t *testing.T) {
	for _, provider := range []string{PaymentProviderStripe, PaymentProviderEpay, PaymentProviderWaffo, PaymentProviderWaffoPancake} {
		for _, failure := range []string{"none", "bonus conflict", "quota update", "expired conflict"} {
			if provider == PaymentProviderStripe && failure == "expired conflict" {
				continue
			}
			t.Run(provider+"/"+failure, func(t *testing.T) {
				setupDiscountCodeTest(t)
				oldQPU, oldBatch := common.QuotaPerUnit, common.BatchUpdateEnabled
				common.QuotaPerUnit, common.BatchUpdateEnabled = 100, true
				t.Cleanup(func() { common.QuotaPerUnit, common.BatchUpdateEnabled = oldQPU, oldBatch })
				dc := DiscountCode{Code: "atomic", DiscountRate: 50}
				require.NoError(t, dc.Insert())
				status := common.TopUpStatusPending
				if failure == "expired conflict" {
					status = common.TopUpStatusExpired
				}
				user, order := insertEpayRechargeFixture(t, "atomic", provider, status, 1, 0.8, dc.Id)
				order.DiscountRate = 80
				require.NoError(t, order.Update())
				if failure == "bonus conflict" || failure == "expired conflict" {
					require.NoError(t, DB.Create(&TopUp{TradeNo: order.TradeNo + "_bonus"}).Error)
				}
				if failure == "quota update" {
					require.NoError(t, DB.Exec("CREATE TRIGGER reject_bonus_quota BEFORE UPDATE OF quota ON users BEGIN SELECT RAISE(ABORT, 'quota update failure'); END").Error)
				}
				if failure != "none" {
					require.Error(t, rechargeDiscountOrder(order))
					stored := GetTopUpByTradeNo(order.TradeNo)
					require.NotNil(t, stored)
					assert.Equal(t, status, stored.Status)
					assert.Zero(t, stored.QuotaGranted)
					require.NoError(t, DB.First(user, user.Id).Error)
					assert.Equal(t, 100, user.Quota)
					count, err := GetDiscountCodeUserUsageCount(dc.Id, user.Id)
					require.NoError(t, err)
					assert.Zero(t, count)
					require.NoError(t, DB.First(&dc, dc.Id).Error)
					assert.Zero(t, dc.UsedCount)
					if failure == "quota update" {
						assert.Nil(t, GetTopUpByTradeNo(order.TradeNo+"_bonus"))
						require.NoError(t, DB.Exec("DROP TRIGGER reject_bonus_quota").Error)
					} else {
						require.NoError(t, DB.Where("trade_no = ?", order.TradeNo+"_bonus").Delete(&TopUp{}).Error)
					}
				}
				require.NoError(t, rechargeDiscountOrder(order))
				err := rechargeDiscountOrder(order)
				if provider == PaymentProviderStripe {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				stored := GetTopUpByTradeNo(order.TradeNo)
				require.NotNil(t, stored)
				assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
				assert.EqualValues(t, 80, stored.QuotaGranted)
				bonus := GetTopUpByTradeNo(order.TradeNo + "_bonus")
				require.NotNil(t, bonus)
				assert.EqualValues(t, 20, bonus.QuotaGranted)
				assert.Equal(t, 80, bonus.DiscountRate)
				assert.Equal(t, dc.Id, bonus.DiscountCodeId)
				assert.Equal(t, "discount_bonus", bonus.Source)
				assert.Equal(t, "discount_bonus", bonus.PaymentMethod)
				assert.Equal(t, "discount_code", bonus.PaymentProvider)
				assert.Equal(t, common.TopUpStatusSuccess, bonus.Status)
				require.NoError(t, DB.First(user, user.Id).Error)
				assert.Equal(t, 200, user.Quota)
				count, err := GetDiscountCodeUserUsageCount(dc.Id, user.Id)
				require.NoError(t, err)
				assert.EqualValues(t, 1, count)
				require.NoError(t, DB.First(&dc, dc.Id).Error)
				assert.Equal(t, 1, dc.UsedCount)
				var logs int64
				require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND content LIKE ?", user.Id, "折扣码赠金 %").Count(&logs).Error)
				assert.EqualValues(t, 1, logs)
			})
		}
	}
}

func TestRechargeDiscountBonusMissingCode(t *testing.T) {
	setupDiscountCodeTest(t)
	user, order := insertEpayRechargeFixture(t, "missing-code", PaymentProviderEpay, common.TopUpStatusPending, 1, 1, 999)
	require.NoError(t, rechargeDiscountOrder(order))
	assert.Equal(t, common.TopUpStatusSuccess, GetTopUpByTradeNo(order.TradeNo).Status)
	assert.Nil(t, GetTopUpByTradeNo(order.TradeNo+"_bonus"))
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.EqualValues(t, 100+common.QuotaPerUnit, user.Quota)
}

func TestDiscountBonusEligibility(t *testing.T) {
	setupDiscountCodeTest(t)
	for _, tc := range []struct {
		name  string
		order TopUp
	}{
		{"no code", TopUp{QuotaGranted: 80, DiscountRate: 80}},
		{"no paid quota", TopUp{DiscountCodeId: 1, DiscountRate: 80}},
		{"invalid negative rate", TopUp{DiscountCodeId: 1, QuotaGranted: 80, DiscountRate: -1}},
		{"no discount", TopUp{DiscountCodeId: 1, QuotaGranted: 80, DiscountRate: 100}},
		{"fraction below one quota", TopUp{DiscountCodeId: 1, QuotaGranted: 1, DiscountRate: 99}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
				bonus, err := grantDiscountCodeBonusTx(tx, &tc.order)
				assert.Zero(t, bonus)
				return err
			}))
			var count int64
			require.NoError(t, DB.Model(&TopUp{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestRechargeDiscountBonusLookupFailureRollsBack(t *testing.T) {
	setupDiscountCodeTest(t)
	user, order := insertEpayRechargeFixture(t, "lookup-error", PaymentProviderEpay, common.TopUpStatusPending, 1, 1, 1)
	require.NoError(t, DB.Migrator().DropTable(&DiscountCode{}))
	require.Error(t, rechargeDiscountOrder(order))
	assert.Equal(t, common.TopUpStatusPending, GetTopUpByTradeNo(order.TradeNo).Status)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestRechargeDiscountBonusDisplayFailuresDoNotUndoCredit(t *testing.T) {
	setupDiscountCodeTest(t)
	dc := DiscountCode{Code: "display", DiscountRate: 80}
	require.NoError(t, dc.Insert())
	user, order := insertEpayRechargeFixture(t, "display-error", PaymentProviderEpay, common.TopUpStatusPending, 1, 1, dc.Id)
	order.DiscountRate = 80
	require.NoError(t, order.Update())
	require.NoError(t, DB.Exec("CREATE TRIGGER reject_bonus_usage BEFORE INSERT ON discount_code_usages BEGIN SELECT RAISE(ABORT, 'usage failure'); END").Error)
	require.NoError(t, DB.Exec("CREATE TRIGGER reject_bonus_count BEFORE UPDATE OF used_count ON discount_codes BEGIN SELECT RAISE(ABORT, 'used count failure'); END").Error)
	require.NoError(t, DB.Exec("CREATE TRIGGER reject_bonus_log BEFORE INSERT ON logs BEGIN SELECT RAISE(ABORT, 'log failure'); END").Error)
	require.NoError(t, rechargeDiscountOrder(order))
	assert.Equal(t, common.TopUpStatusSuccess, GetTopUpByTradeNo(order.TradeNo).Status)
	require.NotNil(t, GetTopUpByTradeNo(order.TradeNo+"_bonus"))
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.EqualValues(t, 100+common.QuotaPerUnit*1.25, user.Quota)
}
