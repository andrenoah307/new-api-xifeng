package model

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDiscountCodeTest(t *testing.T) {
	t.Helper()
	oldDB, oldLogDB, oldRedis := DB, LOG_DB, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "discount.db")), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB, common.RedisEnabled = db, db, false
	require.NoError(t, db.AutoMigrate(&DiscountCode{}, &DiscountCodeUsage{}, &TopUp{}, &User{}, &Log{}))
	t.Cleanup(func() {
		DB, LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
}

func TestDiscountCodeMaxAmount(t *testing.T) {
	old := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldQPU := common.QuotaPerUnit
	t.Cleanup(func() { operation_setting.GetGeneralSetting().QuotaDisplayType = old; common.QuotaPerUnit = oldQPU })
	common.QuotaPerUnit = 500000
	for _, tc := range []struct {
		name, mode  string
		max, amount int64
		want        string
	}{
		{"unlimited", "USD", 0, 1000000, ""},
		{"equal", "USD", 10, 10, ""},
		{"over", "USD", 10, 11, "该折扣码单笔最多充值 10"},
		{"tokens equal", "TOKENS", 10, 5000000, ""},
		{"one token over", "TOKENS", 10, 5000001, "该折扣码单笔最多充值 5000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.mode
			dc := DiscountCode{MaxAmount: tc.max}
			err := dc.CheckMaxAmount(decimal.NewFromInt(tc.amount))
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.want)
			}
		})
	}
}

func TestDiscountCodeUpdateZero(t *testing.T) {
	setupDiscountCodeTest(t)
	dc := DiscountCode{Code: "zero", DiscountRate: 90, MaxAmount: 10}
	require.NoError(t, dc.Insert())
	dc.MaxAmount = 0
	require.NoError(t, dc.Update())
	got, err := GetDiscountCodeById(dc.Id)
	require.NoError(t, err)
	assert.Zero(t, got.MaxAmount)
}

func TestTopUpStatusIndexMigrationSafety(t *testing.T) {
	setupDiscountCodeTest(t)
	stmt := &gorm.Statement{DB: DB}
	require.NoError(t, stmt.Parse(&TopUp{}))
	// Indexing this legacy MySQL LONGTEXT column would change its inferred type.
	for _, index := range stmt.Schema.ParseIndexes() {
		for _, field := range index.Fields {
			assert.NotEqual(t, "status", field.DBName, index.Name)
		}
	}
}

func TestDiscountCodePendingTTL(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
	}{
		{"", 1800}, {"0", 1800}, {"-1", 1800}, {"invalid", 1800}, {"60", 60},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("DISCOUNT_CODE_PENDING_TTL_SECONDS", tc.value)
			assert.Equal(t, tc.want, DiscountCodePendingTTLSeconds())
		})
	}
}

func TestDiscountCodeReservationRechecksAvailability(t *testing.T) {
	for _, state := range []string{"disabled", "future", "ended", "deleted", "insert failure"} {
		t.Run(state, func(t *testing.T) {
			setupDiscountCodeTest(t)
			dc := DiscountCode{Code: "recheck", DiscountRate: 90}
			require.NoError(t, dc.Insert())
			_, err := ValidateDiscountCode(dc.Code, 1)
			require.NoError(t, err)
			switch state {
			case "disabled":
				dc.Status = DiscountCodeStatusDisabled
			case "future":
				dc.StartTime = common.GetTimestamp() + 100
			case "ended":
				dc.EndTime = common.GetTimestamp() - 100
			case "deleted":
				require.NoError(t, dc.Delete())
			case "insert failure":
				require.NoError(t, DB.Exec("CREATE TRIGGER reject_order BEFORE INSERT ON top_ups BEGIN SELECT RAISE(ABORT, 'forced failure'); END").Error)
			}
			if state != "deleted" {
				require.NoError(t, dc.Update())
			}
			order := TopUp{TradeNo: "rejected", UserId: 1, DiscountCodeId: dc.Id, DiscountRate: 90, Status: "pending", CreateTime: common.GetTimestamp()}
			err = ReserveDiscountCodeTopUp(&order)
			require.Error(t, err)
			var businessErr DiscountCodeValidationError
			assert.Equal(t, state != "insert failure", errors.As(err, &businessErr))
			var count int64
			require.NoError(t, DB.Model(&TopUp{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestDiscountCodeOccupancyAndCleanup(t *testing.T) {
	setupDiscountCodeTest(t)
	t.Setenv("DISCOUNT_CODE_PENDING_TTL_SECONDS", "1800")
	now := int64(2000000000)
	dc := DiscountCode{Code: "counts", DiscountRate: 90, MaxUsesTotal: 1, MaxUsesPerUser: 1, UsedCount: 999}
	require.NoError(t, dc.Insert())
	for _, tc := range []struct {
		name, status, source string
		age                  int64
		occupied             bool
	}{
		{"recent", "pending", "", 1799, true},
		{"boundary", "pending", "", 1800, false},
		{"old", "pending", "", 1801, false},
		{"paid", "success", "", 9999, true},
		{"bonus", "success", "discount_bonus", 0, false},
		{"pending bonus", "pending", "discount_bonus", 9999, false},
		{"failed", "failed", "", 0, false},
		{"expired", "expired", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, DB.Where("1 = 1").Delete(&TopUp{}).Error)
			order := TopUp{UserId: 1, TradeNo: tc.name, DiscountCodeId: dc.Id, Status: tc.status, Source: tc.source, CreateTime: now - tc.age}
			require.NoError(t, DB.Create(&order).Error)
			count, err := GetDiscountCodeTotalCount(DB, dc.Id, now)
			require.NoError(t, err)
			assert.Equal(t, tc.occupied, count == 1)
			if tc.source == "" {
				require.NoError(t, DB.Model(&order).Update("source", nil).Error)
				count, err = GetDiscountCodeTotalCount(DB, dc.Id, now)
				require.NoError(t, err)
				assert.Equal(t, tc.occupied, count == 1)
			}
			count, err = GetDiscountCodeUserCount(DB, dc.Id, 1, now)
			require.NoError(t, err)
			assert.Equal(t, tc.occupied, count == 1)
			count, err = GetDiscountCodeUserCount(DB, dc.Id, 2, now)
			require.NoError(t, err)
			assert.Zero(t, count)
		})
	}
	require.NoError(t, DB.Where("1 = 1").Delete(&TopUp{}).Error)
	now = common.GetTimestamp()
	for _, order := range []TopUp{
		{TradeNo: "old", DiscountCodeId: dc.Id, Status: "pending", CreateTime: now - 1900},
		{TradeNo: "recent", DiscountCodeId: dc.Id, Status: "pending", CreateTime: now},
		{TradeNo: "bonus", DiscountCodeId: dc.Id, Status: "pending", Source: "discount_bonus", CreateTime: now - 1900},
		{TradeNo: "paid", DiscountCodeId: dc.Id, Status: "success", CreateTime: now - 1900},
		{TradeNo: "other", DiscountCodeId: dc.Id + 1, Status: "pending", CreateTime: now - 1900},
	} {
		require.NoError(t, DB.Create(&order).Error)
	}
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "old").Update("source", nil).Error)
	n, err := CleanupPendingOrdersByDiscountCode(dc.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	assert.Equal(t, "expired", GetTopUpByTradeNo("old").Status)
	assert.Equal(t, "pending", GetTopUpByTradeNo("recent").Status)
	assert.Equal(t, "pending", GetTopUpByTradeNo("bonus").Status)
	assert.Equal(t, "success", GetTopUpByTradeNo("paid").Status)
	assert.Equal(t, "pending", GetTopUpByTradeNo("other").Status)
}

func TestDiscountCodeReservation(t *testing.T) {
	setupDiscountCodeTest(t)
	dc := DiscountCode{Code: "reserve", DiscountRate: 90, MaxUsesTotal: 1, UsedCount: 999}
	require.NoError(t, dc.Insert())
	require.NoError(t, RecordDiscountCodeUsage(dc.Id, 1, 99))
	_, err := ValidateDiscountCode(dc.Code, 1)
	require.NoError(t, err)
	order := TopUp{TradeNo: "first", UserId: 1, DiscountCodeId: dc.Id, DiscountRate: 90, Status: "pending", CreateTime: common.GetTimestamp()}
	require.NoError(t, ReserveDiscountCodeTopUp(&order))
	second := order
	second.Id = 0
	second.TradeNo = "second"
	err = ReserveDiscountCodeTopUp(&second)
	require.EqualError(t, err, "该折扣码名额暂被待支付订单占用，请稍后再试")
	var businessErr DiscountCodeValidationError
	require.ErrorAs(t, err, &businessErr)
	assert.Nil(t, GetTopUpByTradeNo("second"))
	_, err = ValidateDiscountCode(dc.Code, 2)
	require.Error(t, err)
	dc.MaxUsesTotal = 0
	dc.MaxUsesPerUser = 1
	require.NoError(t, dc.Update())
	err = ReserveDiscountCodeTopUp(&second)
	require.EqualError(t, err, "您有使用该折扣码的待支付订单，请先完成支付；未支付的订单超过 30 分钟后自动释放名额")
	require.ErrorAs(t, err, &businessErr)
	second.UserId = 2
	require.NoError(t, ReserveDiscountCodeTopUp(&second))
	require.NoError(t, DB.Migrator().DropTable(&TopUp{}))
	err = ReserveDiscountCodeTopUp(&second)
	require.Error(t, err)
	assert.False(t, errors.As(err, &businessErr))
	_, err = ValidateDiscountCode(dc.Code, 1)
	require.EqualError(t, err, "查询使用记录失败")
	dc.MaxUsesTotal = 1
	require.NoError(t, dc.Update())
	_, err = ValidateDiscountCode(dc.Code, 1)
	require.EqualError(t, err, "查询使用记录失败")
}

func TestDiscountCodeBonusSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot int
		deleted  bool
		bonus    int64
	}{
		{"snapshot", 80, false, 20}, {"legacy", 0, false, 80},
		{"deleted snapshot", 80, true, 20}, {"deleted legacy", 0, true, 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupDiscountCodeTest(t)
			dc := DiscountCode{Code: "bonus", DiscountRate: 50}
			require.NoError(t, dc.Insert())
			if tc.deleted {
				require.NoError(t, dc.Delete())
			}
			user := User{Username: "bonus", Quota: 80}
			require.NoError(t, DB.Create(&user).Error)
			oldQPU := common.QuotaPerUnit
			common.QuotaPerUnit = 100
			t.Cleanup(func() { common.QuotaPerUnit = oldQPU })
			order := TopUp{UserId: user.Id, TradeNo: "paid", Status: "pending", PaymentProvider: PaymentProviderEpay, Money: 0.8, DiscountCodeId: dc.Id, DiscountRate: tc.snapshot}
			require.NoError(t, DB.Create(&order).Error)
			require.NoError(t, rechargeDiscountOrder(&order))
			require.NoError(t, rechargeDiscountOrder(&order))
			bonus := GetTopUpByTradeNo("paid_bonus")
			require.NotNil(t, bonus)
			assert.Equal(t, tc.bonus, bonus.QuotaGranted)
			require.NoError(t, DB.First(&user, user.Id).Error)
			assert.EqualValues(t, 160+tc.bonus, user.Quota)
		})
	}
}

func TestRechargeExpiredDiscountOrder(t *testing.T) {
	for _, provider := range []string{PaymentProviderEpay, PaymentProviderWaffo, PaymentProviderWaffoPancake} {
		t.Run(provider, func(t *testing.T) {
			setupDiscountCodeTest(t)
			user, order := insertEpayRechargeFixture(t, provider, provider, common.TopUpStatusExpired, 1, 1, 0)
			dc := DiscountCode{Code: "late", DiscountRate: 50}
			require.NoError(t, dc.Insert())
			order.DiscountCodeId, order.DiscountRate = dc.Id, 80
			require.NoError(t, order.Update())
			require.NoError(t, dc.Delete())
			var err error
			switch provider {
			case PaymentProviderEpay:
				_, _, err = RechargeEpay(order.TradeNo, "alipay")
			case PaymentProviderWaffo:
				err = RechargeWaffo(order.TradeNo, "")
			case PaymentProviderWaffoPancake:
				err = RechargeWaffoPancake(order.TradeNo)
			}
			require.NoError(t, err)
			assert.Equal(t, common.TopUpStatusSuccess, GetTopUpByTradeNo(order.TradeNo).Status)
			require.NoError(t, DB.First(user, user.Id).Error)
			assert.EqualValues(t, 100+common.QuotaPerUnit*1.25, user.Quota)
			bonus := GetTopUpByTradeNo(order.TradeNo + "_bonus")
			require.NotNil(t, bonus)
			assert.EqualValues(t, common.QuotaPerUnit*0.25, bonus.QuotaGranted)
		})
	}
}
