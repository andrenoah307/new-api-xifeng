package controller

import (
	"bytes"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDiscountControllerTest(t *testing.T) {
	t.Helper()
	oldDB, oldRedis := model.DB, common.RedisEnabled
	oldPath, oldMaster := common.SQLitePath, common.IsMasterNode
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldEnabled := operation_setting.GetDiscountCodeSetting().Enabled
	oldGeneral := *operation_setting.GetGeneralSetting()
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	common.SQLitePath, common.IsMasterNode = filepath.Join(t.TempDir(), "discount.db"), false
	require.NoError(t, model.InitDB())
	db := model.DB
	common.RedisEnabled = false
	operation_setting.GetDiscountCodeSetting().Enabled = true
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.DiscountCode{}, &model.TopUp{}))
	require.NoError(t, db.Create(&model.User{Id: 12345, Username: "discount", Group: "default"}).Error)
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = oldDB, oldRedis
		common.SQLitePath, common.IsMasterNode = oldPath, oldMaster
		common.SetDatabaseTypes(oldMainType, oldLogType)
		operation_setting.GetDiscountCodeSetting().Enabled = oldEnabled
		*operation_setting.GetGeneralSetting() = oldGeneral
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
}

func discountControllerRequest(t *testing.T, handler gin.HandlerFunc, payload any) map[string]any {
	t.Helper()
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", 12345)
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler(c)
	assert.Equal(t, 200, w.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	return response
}

func TestDiscountCodePreviewAmountLimits(t *testing.T) {
	setupDiscountControllerTest(t)
	dc := model.DiscountCode{Code: "limit", DiscountRate: 90, MaxAmount: 100}
	require.NoError(t, dc.Insert())
	for _, handler := range []struct {
		name string
		fn   gin.HandlerFunc
	}{
		{"epay", RequestAmount}, {"stripe", RequestStripeAmount},
		{"waffo", RequestWaffoAmount}, {"pancake", RequestWaffoPancakeAmount},
	} {
		t.Run(handler.name, func(t *testing.T) {
			for _, mode := range []string{"USD", "TOKENS"} {
				operation_setting.GetGeneralSetting().QuotaDisplayType = mode
				limit := dc.MaxAmount
				if mode == "TOKENS" {
					limit *= int64(common.QuotaPerUnit)
				}
				response := discountControllerRequest(t, handler.fn, gin.H{"amount": limit, "discount_code": dc.Code})
				assert.Equal(t, "success", response["message"], response)
				response = discountControllerRequest(t, handler.fn, gin.H{"amount": limit + 1, "discount_code": dc.Code})
				assert.Equal(t, "error", response["message"], response)
				assert.Contains(t, response["data"], "该折扣码单笔最多充值")
			}
		})
	}
}

func TestDiscountCodeEpayPreviewDisabled(t *testing.T) {
	setupDiscountControllerTest(t)
	operation_setting.GetDiscountCodeSetting().Enabled = false
	response := discountControllerRequest(t, RequestAmount, gin.H{"amount": 100, "discount_code": "any"})
	assert.Equal(t, "折扣码功能未启用", response["data"])
}

func TestDiscountCodeAdminAndValidate(t *testing.T) {
	setupDiscountControllerTest(t)
	for _, handler := range []gin.HandlerFunc{AddDiscountCode, UpdateDiscountCode} {
		response := discountControllerRequest(t, handler, gin.H{"max_amount": -1, "discount_rate": 90, "count": 1})
		assert.Equal(t, false, response["success"])
		assert.Equal(t, "单笔充值上限不能小于 0", response["message"])
	}
	response := discountControllerRequest(t, AddDiscountCode, gin.H{"code": "admin", "max_amount": 100, "discount_rate": 90, "count": 1})
	assert.Equal(t, true, response["success"])
	dc, err := model.GetDiscountCodeByCode("admin")
	require.NoError(t, err)
	assert.EqualValues(t, 100, dc.MaxAmount)
	for _, mode := range []string{"USD", "TOKENS"} {
		operation_setting.GetGeneralSetting().QuotaDisplayType = mode
		response = discountControllerRequest(t, ValidateUserDiscountCode, gin.H{"code": "admin"})
		require.Equal(t, true, response["success"])
		assert.Equal(t, dc.MaxAmountInInputUnits().InexactFloat64(), response["data"].(map[string]any)["max_amount"])
	}
	response = discountControllerRequest(t, UpdateDiscountCode, gin.H{"id": dc.Id, "max_amount": 0, "discount_rate": 90})
	assert.Equal(t, true, response["success"])
	dc, err = model.GetDiscountCodeById(dc.Id)
	require.NoError(t, err)
	assert.Zero(t, dc.MaxAmount)
}

func TestDiscountCodeOrderAmountLimits(t *testing.T) {
	setupDiscountControllerTest(t)
	oldWaffo := setting.WaffoEnabled
	oldMerchant, oldKey, oldProduct := setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID
	oldPayment := *operation_setting.GetPaymentSetting()
	setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID = "test", "test", "test"
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	setting.WaffoEnabled = true
	t.Cleanup(func() {
		setting.WaffoEnabled = oldWaffo
		setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID = oldMerchant, oldKey, oldProduct
		*operation_setting.GetPaymentSetting() = oldPayment
	})
	dc := model.DiscountCode{Code: "order", DiscountRate: 90, MaxAmount: 100}
	require.NoError(t, dc.Insert())
	for _, handler := range []struct {
		name   string
		fn     gin.HandlerFunc
		method string
	}{
		{"epay", RequestEpay, "alipay"}, {"stripe", RequestStripePay, "stripe"}, {"waffo", RequestWaffoPay, "waffo"},
		{"pancake", RequestWaffoPancakePay, "waffo_pancake"},
	} {
		t.Run(handler.name, func(t *testing.T) {
			response := discountControllerRequest(t, handler.fn, gin.H{"amount": 101, "discount_code": dc.Code, "payment_method": handler.method})
			assert.Equal(t, "该折扣码单笔最多充值 100", response["data"])
			var count int64
			require.NoError(t, model.DB.Model(&model.TopUp{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestDiscountCodeCleanup(t *testing.T) {
	setupDiscountControllerTest(t)
	t.Setenv("DISCOUNT_CODE_PENDING_TTL_SECONDS", "60")
	dc := model.DiscountCode{Code: "cleanup", DiscountRate: 90}
	require.NoError(t, dc.Insert())
	for _, order := range []model.TopUp{
		{TradeNo: "old", DiscountCodeId: dc.Id, Status: "pending", CreateTime: common.GetTimestamp() - 120},
		{TradeNo: "recent", DiscountCodeId: dc.Id, Status: "pending", CreateTime: common.GetTimestamp()},
	} {
		require.NoError(t, model.DB.Create(&order).Error)
	}
	handler := func(c *gin.Context) {
		c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(dc.Id)}}
		CleanupDiscountCodePendingOrders(c)
	}
	response := discountControllerRequest(t, handler, gin.H{})
	assert.Equal(t, true, response["success"])
	assert.EqualValues(t, 1, response["data"])
	assert.Equal(t, "expired", model.GetTopUpByTradeNo("old").Status)
	assert.Equal(t, "pending", model.GetTopUpByTradeNo("recent").Status)
}
