package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/channel_limiter"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelayTaskRateLimitRedactsChannelDetailsForAllDecisionReasons(t *testing.T) {
	require.NoError(t, i18n.Init())
	reasons := []string{
		channel_limiter.ReasonRPMExceeded,
		channel_limiter.ReasonConcurrencyExceeded,
		channel_limiter.ReasonQueueTimeout,
	}
	policies := []struct {
		name       string
		onLimit    string
		maxRetries int
	}{
		{name: "reject", onLimit: channel_limiter.OnLimitReject, maxRetries: 3},
		{name: "retry exhausted", onLimit: channel_limiter.OnLimitSkip, maxRetries: 0},
	}

	for _, policy := range policies {
		for _, reason := range reasons {
			t.Run(policy.name+"/"+reason, func(t *testing.T) {
				c := newRelayRateLimitTestContext(t)
				channelName := "provider-secret-channel"
				channelID := 741003
				channel := taskRateLimitChannel(channelID, channelName, policy.onLimit)
				setTaskRateLimitChannelContext(c, channel)
				bodyStorage := newTrackingBodyStorage([]byte(`{"prompt":"test"}`))
				c.Set(common.KeyBodyStorage, bodyStorage)
				relayInfo := newTaskRateLimitRelayInfo()

				_, taskErr := relayTaskSubmitWithRetry(c, relayInfo, newTaskRateLimitRetryParam(c), policy.maxRetries, taskRelaySubmitDependencies{
					getChannel: getChannel,
					acquireRateLimit: func(context.Context, int, *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
						return nil, channel_limiter.Decision{Allowed: false, Reason: reason}
					},
					submit: func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
						return nil, nil
					},
				})

				require.NotNil(t, taskErr)
				assert.Equal(t, http.StatusTooManyRequests, taskErr.StatusCode)
				assert.Equal(t, string(types.ErrorCodeChannelRateLimited), taskErr.Code)
				assert.True(t, taskErr.LocalError)
				require.NotNil(t, taskErr.Error)
				require.NotNil(t, relayInfo.LastError)
				for _, message := range []string{taskErr.Message, taskErr.Error.Error(), relayInfo.LastError.Error()} {
					assert.NotContains(t, message, channelName)
					assert.NotContains(t, message, reason)
					assert.NotContains(t, message, "741003")
				}

				recorder := httptest.NewRecorder()
				responseContext, _ := gin.CreateTestContext(recorder)
				responseContext.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
				respondTaskError(responseContext, taskErr)
				assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
				assert.NotContains(t, recorder.Body.String(), channelName)
				assert.NotContains(t, recorder.Body.String(), reason)
				assert.NotContains(t, recorder.Body.String(), "741003")
			})
		}
	}
}

func TestRespondTaskErrorPreservesGenericUpstream429Message(t *testing.T) {
	require.NoError(t, i18n.Init())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	taskErr := &dto.TaskError{
		Code:       "upstream_error",
		Message:    "upstream-specific-429-message",
		Error:      errors.New("upstream-specific-429-message"),
		StatusCode: http.StatusTooManyRequests,
	}

	respondTaskError(c, taskErr)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "当前分组上游负载已饱和，请稍后再试")
	assert.NotContains(t, recorder.Body.String(), "upstream-specific-429-message")
}

func TestGetChannelRedactsSelectionDetailsOnFailureAndAbsence(t *testing.T) {
	require.NoError(t, i18n.Init())
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMemoryCache := common.MemoryCacheEnabled
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.MemoryCacheEnabled = originalMemoryCache
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false

	tests := []struct {
		name       string
		prepareDB  func(*testing.T, *gorm.DB)
		wantStatus int
	}{
		{
			name: "query failure",
			prepareDB: func(*testing.T, *gorm.DB) {
				// Deliberately leave the database without the abilities table.
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "no channel",
			prepareDB: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "get-channel-redaction.db")), &gorm.Config{})
			require.NoError(t, err)
			t.Cleanup(func() {
				if sqlDB, dbErr := db.DB(); dbErr == nil {
					_ = sqlDB.Close()
				}
			})
			model.DB = db
			require.NoError(t, model.InitLogDB())
			tt.prepareDB(t, db)
			info := &relaycommon.RelayInfo{
				OriginModelName: "internal-model-secret",
				ChannelMeta:     &relaycommon.ChannelMeta{},
			}
			ctx := newRelayRateLimitTestContext(t)
			retryParam := &service.RetryParam{Ctx: ctx, TokenGroup: "internal-group-secret", ModelName: info.OriginModelName, Retry: common.GetPointer(0)}

			channel, apiErr := getChannel(ctx, info, retryParam)

			require.Nil(t, channel)
			require.NotNil(t, apiErr)
			assert.Equal(t, tt.wantStatus, apiErr.StatusCode)
			assert.Equal(t, types.ErrorCodeGetChannelFailed, apiErr.GetErrorCode())
			assert.True(t, apiErr.IsSkipRetry())
			message := apiErr.ToOpenAIError().Message
			assert.NotContains(t, message, "internal-group-secret")
			assert.NotContains(t, message, "internal-model-secret")
			assert.NotContains(t, message, "no such table")
		})
	}
}

func TestRelayErrorFormatsStaticRateLimitMessageForOpenAIAndClaude(t *testing.T) {
	require.NoError(t, i18n.Init())
	channelName := "secret-channel"
	reasons := []string{
		channel_limiter.ReasonRPMExceeded,
		channel_limiter.ReasonConcurrencyExceeded,
		channel_limiter.ReasonQueueTimeout,
	}
	for _, skipRetry := range []bool{false, true} {
		for _, reason := range reasons {
			for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
				t.Run(string(format)+"/"+reason+"/"+fmt.Sprint(skipRetry), func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
					apiErr := newChannelRateLimitError(c, skipRetry)
					assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
					assert.Equal(t, types.ErrorCodeChannelRateLimited, apiErr.GetErrorCode())
					assert.Equal(t, skipRetry, apiErr.IsSkipRetry())
					var message string
					if format == types.RelayFormatClaude {
						message = apiErr.ToClaudeError().Message
					} else {
						message = apiErr.ToOpenAIError().Message
					}
					assert.NotContains(t, message, channelName)
					assert.NotContains(t, message, reason)
					assert.NotContains(t, message, "741003")
				})
			}
		}
	}
}

func TestGenericRelayRateLimitRedactsAcrossFormatsAndPolicies(t *testing.T) {
	require.NoError(t, i18n.Init())
	formats := []struct {
		name types.RelayFormat
		body string
	}{
		{name: types.RelayFormatOpenAI, body: `{"model":"redaction-model","messages":[{"role":"user","content":"hello"}]}`},
		{name: types.RelayFormatClaude, body: `{"model":"redaction-model","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`},
	}
	policies := []struct {
		name    string
		onLimit string
	}{
		{name: "reject", onLimit: channel_limiter.OnLimitReject},
		{name: "retry exhausted", onLimit: channel_limiter.OnLimitSkip},
	}

	for _, format := range formats {
		for _, policy := range policies {
			t.Run(string(format.name)+"/"+policy.name, func(t *testing.T) {
				const channelID = 761003
				channelName := "generic-secret-channel"
				recorder := newGenericRelayRateLimitRequest(t, format.name, format.body, channelID, channelName, policy.onLimit)

				assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
				if format.name == types.RelayFormatOpenAI {
					assert.Contains(t, recorder.Body.String(), string(types.ErrorCodeChannelRateLimited))
				}
				assert.NotContains(t, recorder.Body.String(), channelName)
				assert.NotContains(t, recorder.Body.String(), "761003")
				if policy.onLimit == channel_limiter.OnLimitReject {
					assert.NotContains(t, recorder.Body.String(), "已达限流")
				}
			})
		}
	}
}

func TestProcessChannelErrorStoresChannelDetailsUnderAdminInfo(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalErrorLogEnabled := constant.ErrorLogEnabled
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		common.RedisEnabled = originalRedisEnabled
		constant.ErrorLogEnabled = originalErrorLogEnabled
	})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	constant.ErrorLogEnabled = true
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "relay-error-redaction.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", 11)
	c.Set("token_id", 12)
	c.Set("token_name", "token")
	c.Set("original_model", "model")
	c.Set("group", "group")
	c.Set("channel_id", 713)
	c.Set("channel_name", "channel-name-sentinel")
	c.Set("channel_type", 987654321)
	processChannelError(c, types.ChannelError{ChannelId: 713}, types.NewErrorWithStatusCode(errors.New("upstream"), types.ErrorCodeBadResponse, http.StatusBadGateway))

	var stored model.Log
	require.NoError(t, db.Order("id desc").First(&stored).Error)
	other, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	assert.NotContains(t, other, "channel_name")
	assert.NotContains(t, other, "channel_type")
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "channel-name-sentinel", adminInfo["channel_name"])
	assert.Equal(t, float64(987654321), adminInfo["channel_type"])

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", 11)
	c.Set("token_id", 12)
	c.Set("token_name", "token")
	c.Set("original_model", "model")
	c.Set("group", "group")
	c.Set("channel_id", 714)
	c.Set("channel_name", "second-channel")
	c.Set("channel_type", 987654322)
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 2)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Unix(1, 0))
	common.SetContextKey(c, constant.ContextKeyIsStream, true)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{StripRequestId: true})
	c.Set("risk_audit", &types.RiskAudit{TokenDecision: &types.RiskDecision{Group: "risk-group"}})
	processChannelError(c, types.ChannelError{ChannelId: 714}, types.NewErrorWithStatusCode(errors.New("second upstream"), types.ErrorCodeBadResponse, http.StatusBadGateway))
	var secondStored model.Log
	require.NoError(t, db.Order("id desc").First(&secondStored).Error)
	secondOther, err := common.StrToMap(secondStored.Other)
	require.NoError(t, err)
	secondAdminInfo, ok := secondOther["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, secondAdminInfo["is_multi_key"])
	assert.Equal(t, float64(2), secondAdminInfo["multi_key_index"])
	assert.Contains(t, secondOther, "risk_control")
}

func newGenericRelayRateLimitRequest(t *testing.T, format types.RelayFormat, body string, channelID int, channelName, onLimit string) *httptest.ResponseRecorder {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMemoryCache := common.MemoryCacheEnabled
	originalRedisEnabled := common.RedisEnabled
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	originalErrorLogEnabled := constant.ErrorLogEnabled
	originalGroupRatio := ratio_setting.GetGroupRatio("default")
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.MemoryCacheEnabled = originalMemoryCache
		common.RedisEnabled = originalRedisEnabled
		common.SetDatabaseTypes(originalMainType, originalLogType)
		constant.ErrorLogEnabled = originalErrorLogEnabled
		ratio_setting.GetGroupRatioSetting().GroupRatio.Set("default", originalGroupRatio)
	})

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	constant.ErrorLogEnabled = false
	ratio_setting.GetGroupRatioSetting().GroupRatio.Set("default", 0)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "generic-relay-redaction.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSubscription{}, &model.Channel{}, &model.Ability{}))
	model.DB = db
	require.NoError(t, model.InitLogDB())
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "relay-redaction-user", Group: "default", Status: common.UserStatusEnabled, Quota: 1_000_000}).Error)

	priority := int64(10)
	weight := uint(1)
	autoBan := 0
	channel := &model.Channel{
		Id:       channelID,
		Name:     channelName,
		Key:      "generic-test-key",
		Status:   common.ChannelStatusEnabled,
		Group:    "default",
		Models:   "redaction-model",
		Priority: &priority,
		Weight:   &weight,
		AutoBan:  &autoBan,
		Type:     constant.ChannelTypeOpenAI,
	}
	channel.SetSetting(dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{Enabled: true, Concurrency: 1, OnLimit: onLimit}})
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "redaction-model", ChannelId: channelID, Enabled: true, Priority: &priority, Weight: weight}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "redaction-model")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{AcceptUnsetRatioModel: true, BillingPreference: "wallet_only"})
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyUserQuota, 1_000_000)
	common.SetContextKey(c, constant.ContextKeyUserStatus, common.UserStatusEnabled)
	common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
	common.SetContextKey(c, constant.ContextKeyChannelId, channelID)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelName, channelName)
	common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://example.invalid")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, channel.GetSetting())

	heldToken, decision := channel_limiter.Acquire(c.Request.Context(), channelID, channel.GetSetting().RateLimit)
	require.True(t, decision.Allowed)
	require.NotNil(t, heldToken)
	t.Cleanup(heldToken.Release)
	Relay(c, format)
	return recorder
}
