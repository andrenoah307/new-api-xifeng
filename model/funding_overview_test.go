package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFundingOverviewAccounting(t *testing.T) {
	oldDB, oldLog := DB, LOG_DB
	oldEnabled, oldUnit, oldRate := common.DataExportEnabled, common.QuotaPerUnit, operation_setting.USDExchangeRate
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLog
		common.DataExportEnabled, common.QuotaPerUnit, operation_setting.USDExchangeRate = oldEnabled, oldUnit, oldRate
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	common.DataExportEnabled, common.QuotaPerUnit, operation_setting.USDExchangeRate = true, 500000, 1
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&QuotaData{}, &TopUp{}, &Log{}))
	start, end := int64(3600), int64(7200)
	require.NoError(t, db.Create(&[]QuotaData{{CreatedAt: start, Quota: 25000000}, {CreatedAt: end, Quota: 99999999}}).Error)
	orders := []TopUp{
		{TradeNo: "online", Status: common.TopUpStatusSuccess, CompleteTime: start, Money: 100, QuotaGranted: 60000000, PaymentProvider: "epay"},
		{TradeNo: "legacy", Status: common.TopUpStatusSuccess, CompleteTime: start, Money: 25, PaymentMethod: "wxpay"},
		{TradeNo: "manual", Status: common.TopUpStatusSuccess, CompleteTime: start, Money: 21, QuotaGranted: 10000000, PaymentProvider: "manual", Source: "manual", CreatedBy: 1, CheckoutVersion: 1},
		{TradeNo: "bonus", Status: common.TopUpStatusSuccess, CompleteTime: start, Money: 90, Source: "discount_bonus"},
		{TradeNo: "pending", Status: common.TopUpStatusPending, CompleteTime: start, Money: 99999, PaymentProvider: "epay"},
		{TradeNo: "outside", Status: common.TopUpStatusSuccess, CompleteTime: end, Money: 99999, PaymentProvider: "epay"},
	}
	require.NoError(t, db.Create(&orders).Error)
	for _, entry := range []string{
		`{"op":{"action":"user.quota_add","params":{"quota":5000000}}}`,
		`{"op":{"action":"user.quota_override","params":{"from":1000000,"to":3500000}}}`,
		`{"op":{"action":"user.quota_override","params":{"from":9000000,"to":0}}}`,
		`{"op":{"action":"user.quota_subtract","params":{"quota":1000000}}}`,
		`{"op":{"action":"user.manual_topup","params":{"quota":10000000}}}`,
		`{"op":{"action":"user.topup_complete","params":{"quota":60000000}}}`,
		`{"op":{"action":"option.update","params":{"from":"unrelated"}}}`,
	} {
		require.NoError(t, db.Create(&Log{CreatedAt: start, Type: LogTypeManage, Other: entry}).Error)
	}
	require.NoError(t, db.Create(&Log{CreatedAt: end, Type: LogTypeManage, Other: `{"op":{"action":"user.quota_add","params":{"quota":999999}}}`}).Error)
	result, err := QueryFundingOverview(context.Background(), start, end)
	require.NoError(t, err)
	assert.Equal(t, "50.000000", result.Consumption)
	assert.Equal(t, "125.000000", result.OnlineTopUps)
	assert.Equal(t, "35.000000", result.ManualCredits)
	assert.Equal(t, "160.000000", result.TotalFunding)
	require.NotNil(t, result.ConsumptionRatio)
	assert.Equal(t, 0.3125, *result.ConsumptionRatio)
	empty, err := QueryFundingOverview(context.Background(), 10800, 14400)
	require.NoError(t, err)
	assert.Nil(t, empty.ConsumptionRatio)
	require.NoError(t, db.Create(&Log{CreatedAt: 10800, Type: LogTypeManage, Other: `{"op":{"action":"user.quota_add","params":{}}}`}).Error)
	_, err = QueryFundingOverview(context.Background(), 10800, 14400)
	assert.ErrorIs(t, err, ErrFundingOverviewUnavailable)
	common.DataExportEnabled = false
	_, err = QueryFundingOverview(context.Background(), start, end)
	assert.ErrorIs(t, err, ErrFundingOverviewUnavailable)
	common.DataExportEnabled = true
	require.NoError(t, db.Create(&TopUp{TradeNo: "usd", Status: common.TopUpStatusSuccess, CompleteTime: start, Money: 10, PaymentProvider: "stripe"}).Error)
	_, err = QueryFundingOverview(context.Background(), start, end)
	assert.ErrorIs(t, err, ErrFundingOverviewUnavailable)
}

func TestFundingOverviewRange(t *testing.T) {
	for _, tc := range []struct {
		start, end, now int64
		valid           bool
	}{
		{3600, 7200, 7200, true}, {3600, 3600 + 31*86400, 3600 + 31*86400, true},
		{0, 7200, 7200, false}, {3601, 7200, 7200, false}, {3600, 7201, 8000, false},
		{7200, 3600, 8000, false}, {3600, 3600, 8000, false}, {3600, 7200, 7199, false},
		{3600, 7200 + 31*86400, 7200 + 31*86400, false},
	} {
		assert.Equal(t, tc.valid, ValidFundingOverviewRange(tc.start, tc.end, tc.now), "%+v", tc)
	}
}
