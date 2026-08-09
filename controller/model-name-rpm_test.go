package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/channel_limiter"
	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type t3ControllerModelNameRPMRule struct {
	GlobalRPM int `json:"global_rpm"`
}

func t3ControllerConfigureModelNameRPM(t *testing.T, modelName string, globalRPM int) {
	t.Helper()
	previous := setting.ModelNameRPMRateLimit2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous))
	})
	payload, err := common.Marshal(struct {
		Enabled bool                                    `json:"enabled"`
		Models  map[string]t3ControllerModelNameRPMRule `json:"models"`
	}{
		Enabled: true,
		Models: map[string]t3ControllerModelNameRPMRule{
			modelName: {GlobalRPM: globalRPM},
		},
	})
	require.NoError(t, err)
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(string(payload)))
	require.NoError(t, i18n.Init())
}

func t3ControllerSetupModelDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldMemoryCache := common.MemoryCacheEnabled
	dsn := "file:t3_controller_rpm?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}))
	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.MemoryCacheEnabled = oldMemoryCache
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func t3ControllerLimiterUsesMemory(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	oldRedisClient := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRedisClient
	})
}

func TestT3RelayTaskRemixGateUsesResolvedOriginModel(t *testing.T) {
	modelName := "t3-remix-controller-model"
	t3ControllerConfigureModelNameRPM(t, modelName, 1)
	t3ControllerLimiterUsesMemory(t)
	db := t3ControllerSetupModelDB(t)

	const userID = 731001
	const channelID = 731002
	require.NoError(t, db.Create(&model.Channel{
		Id:     channelID,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "origin-key",
		Name:   "t3-remix-channel",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: modelName,
	}).Error)
	require.NoError(t, db.Create(&model.Task{
		TaskID:     "origin-task",
		UserId:     userID,
		ChannelId:  channelID,
		Properties: model.Properties{OriginModelName: modelName},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/origin-task/remix", strings.NewReader(`{"prompt":"remix"}`))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)
	c.Params = gin.Params{{Key: "video_id", Value: "origin-task"}}
	c.Set("id", userID)
	c.Set("relay_mode", relayconstant.RelayModeVideoSubmit)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{Language: i18n.LangEn})
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	// Consume the only slot so the controller's E3 call must reject. The
	// request itself has no E1/E2 opportunity because remix is shouldSelect=false.
	key := "mdrl:v1:rpm:model:" + modelName
	first := model_name_limiter.Acquire(context.Background(), []string{key}, []int{1})
	require.True(t, first.Allowed)
	RelayTask(c)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	var response dto.TaskError
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, string(types.ErrorCodeModelNameRateLimited), response.Code)
	assert.NotContains(t, recorder.Body.String(), modelName)
	assert.Empty(t, c.GetStringSlice("use_channel"), "RPM rejection must happen before the retry loop")
}

func TestT3RelayTaskRetryLoopDoesNotReacquireModelRPM(t *testing.T) {
	modelName := "t3-retry-controller-model"
	t3ControllerConfigureModelNameRPM(t, modelName, 2)
	t3ControllerLimiterUsesMemory(t)

	c := newRelayRateLimitTestContext(t)
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{Language: i18n.LangEn})
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	require.True(t, middleware.EnforceModelNameRPMForTask(c, modelName, "default", c.Request.URL.Path))

	channel := taskRateLimitChannel(731003, "t3-retry-channel", channel_limiter.OnLimitReject)
	setTaskRateLimitChannelContext(c, channel)
	c.Set(common.KeyBodyStorage, newTrackingBodyStorage([]byte(`{"prompt":"retry"}`)))
	relayInfo := newTaskRateLimitRelayInfo()
	relayInfo.OriginModelName = modelName
	relayInfo.TokenGroup = "default"
	retryParam := newTaskRateLimitRetryParam(c)
	retryParam.ModelName = modelName
	submitCalls := 0
	oldErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = false
	t.Cleanup(func() { constant.ErrorLogEnabled = oldErrorLogEnabled })

	result, taskErr := relayTaskSubmitWithRetry(c, relayInfo, retryParam, 2, taskRelaySubmitDependencies{
		getChannel: func(*gin.Context, *relaycommon.RelayInfo, *service.RetryParam) (*model.Channel, *types.NewAPIError) {
			return channel, nil
		},
		acquireRateLimit: func(context.Context, int, *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
			return nil, channel_limiter.Decision{Allowed: true}
		},
		submit: func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
			submitCalls++
			return nil, &dto.TaskError{
				Code:       "upstream_error",
				Message:    "upstream failed",
				StatusCode: http.StatusInternalServerError,
				Error:      errors.New("upstream failed"),
			}
		},
	})

	assert.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Equal(t, 3, submitCalls)

	// The gate's one hit plus this probe fit under the limit. A second gate
	// invocation from the retry loop would have consumed the second slot and
	// made this probe reject.
	second := model_name_limiter.Acquire(context.Background(), []string{"mdrl:v1:rpm:model:" + modelName}, []int{2})
	assert.True(t, second.Allowed)
}
