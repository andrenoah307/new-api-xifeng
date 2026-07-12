package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestComputeAlphaSearchQuota(t *testing.T) {
	tests := []struct {
		name       string
		price      float64
		groupRatio float64
	}{
		{name: "standard group", price: 10, groupRatio: 1},
		{name: "discounted group", price: 10, groupRatio: 0.5},
		{name: "premium group", price: 10, groupRatio: 2},
		{name: "saturates oversized price", price: 1e20, groupRatio: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expected := common.QuotaFromDecimal(decimal.NewFromFloat(test.price).
				Div(decimal.NewFromInt(1000)).
				Mul(decimal.NewFromFloat(test.groupRatio)).
				Mul(decimal.NewFromFloat(common.QuotaPerUnit)))

			quota, _ := computeAlphaSearchQuota(test.price, test.groupRatio)

			assert.Equal(t, expected, quota)
			assert.GreaterOrEqual(t, quota, 0)
		})
	}
}

func TestAlphaSearchRequestURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{name: "base without version", baseURL: "https://api.openai.com", expected: "https://api.openai.com/v1/alpha/search"},
		{name: "base with version", baseURL: "https://api.openai.com/v1", expected: "https://api.openai.com/v1/alpha/search"},
		{name: "trailing slash", baseURL: "https://api.openai.com/v1/", expected: "https://api.openai.com/v1/alpha/search"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, alphaSearchRequestURL(test.baseURL, constant.ChannelTypeOpenAI))
		})
	}
}

func TestRelayAlphaSearch_ForwardsUpstreamErrorUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	originalRatio := ratio_setting.GetGroupRatio("default")
	ratio_setting.GetGroupRatioSetting().GroupRatio.Set("default", 0)
	t.Cleanup(func() {
		ratio_setting.GetGroupRatioSetting().GroupRatio.Set("default", originalRatio)
	})

	requestBody := `{"id":"search_1","model":"gpt-5-codex","settings":{"mode":"live"}}`
	upstreamBody := `{"error":{"message":"upstream rejected search"}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		assert.Equal(t, requestBody, string(body))
		assert.Equal(t, "Bearer upstream-key", request.Header.Get("Authorization"))
		assert.Equal(t, "/v1/alpha/search", request.URL.Path)
		writer.Header().Set("X-Upstream-Test", "preserved")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, err = writer.Write([]byte(upstreamBody))
		require.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(requestBody))
	context.Request.Header.Set("Content-Type", gin.MIMEJSON)
	common.SetContextKey(context, constant.ContextKeyOriginalModel, "gpt-5-codex")
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(context, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(context, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(context, constant.ContextKeyChannelKey, "upstream-key")
	common.SetContextKey(context, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)

	RelayAlphaSearch(context)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Equal(t, upstreamBody, recorder.Body.String())
	assert.Equal(t, "preserved", recorder.Header().Get("X-Upstream-Test"))
}

func TestRelayAlphaSearch_RequiresModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(`{"input":"latest news"}`))
	context.Request.Header.Set("Content-Type", gin.MIMEJSON)

	RelayAlphaSearch(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "model is required")
}

func TestRelayAlphaSearch_ChargesSuccessfulCallOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	initModelListColumnNames(t)
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalLogConsumeEnabled := common.LogConsumeEnabled

	database, err := gorm.Open(sqlite.Open("file:alpha_search_billing?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = database
	model.LOG_DB = database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.LogConsumeEnabled = originalLogConsumeEnabled
		sqlDB, dbErr := database.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	user := model.User{Id: 1, Username: "alpha-user", Quota: 20_000, Group: "default"}
	token := model.Token{Id: 1, UserId: user.Id, Key: "downstream-key", RemainQuota: 20_000}
	channel := model.Channel{Id: 1, Name: "alpha-channel", Type: constant.ChannelTypeOpenAI}
	require.NoError(t, database.Create(&user).Error)
	require.NoError(t, database.Create(&token).Error)
	require.NoError(t, database.Create(&channel).Error)

	requestBody := `{"id":"search_2","model":"gpt-5-codex","input":"latest news"}`
	upstreamBody := `{"encrypted_output":"ciphertext","output":"result"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", gin.MIMEJSON)
		_, writeErr := writer.Write([]byte(upstreamBody))
		require.NoError(t, writeErr)
	}))
	t.Cleanup(upstream.Close)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(requestBody))
	context.Request.Header.Set("Content-Type", gin.MIMEJSON)
	common.SetContextKey(context, constant.ContextKeyOriginalModel, "gpt-5-codex")
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(context, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(context, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(context, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(context, constant.ContextKeyTokenKey, token.Key)
	common.SetContextKey(context, constant.ContextKeyTokenUnlimited, false)
	common.SetContextKey(context, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
	common.SetContextKey(context, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(context, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(context, constant.ContextKeyChannelKey, "upstream-key")
	common.SetContextKey(context, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	context.Set("token_quota", token.RemainQuota)

	RelayAlphaSearch(context)

	expectedQuota := common.QuotaFromDecimal(decimal.NewFromFloat(10).
		Div(decimal.NewFromInt(1000)).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, upstreamBody, recorder.Body.String())

	var updatedUser model.User
	require.NoError(t, database.First(&updatedUser, user.Id).Error)
	assert.Equal(t, user.Quota-expectedQuota, updatedUser.Quota)
	assert.Equal(t, expectedQuota, updatedUser.UsedQuota)
	assert.Equal(t, 1, updatedUser.RequestCount)

	var updatedToken model.Token
	require.NoError(t, database.First(&updatedToken, token.Id).Error)
	assert.Equal(t, token.RemainQuota-expectedQuota, updatedToken.RemainQuota)
	assert.Equal(t, expectedQuota, updatedToken.UsedQuota)

	var consumeLog model.Log
	require.NoError(t, database.Where("type = ?", model.LogTypeConsume).First(&consumeLog).Error)
	assert.Equal(t, "gpt-5-codex", consumeLog.ModelName)
	assert.Equal(t, expectedQuota, consumeLog.Quota)
	assert.Zero(t, consumeLog.PromptTokens)
	assert.Zero(t, consumeLog.CompletionTokens)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(consumeLog.Other, &other))
	assert.Equal(t, float64(1), other["alpha_search"])
}
