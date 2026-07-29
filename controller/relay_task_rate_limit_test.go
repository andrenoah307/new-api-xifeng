package controller

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/channel_limiter"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trackingBodyStorage struct {
	data      []byte
	reader    *bytes.Reader
	seekErr   error
	seekCalls int
	closed    bool
}

func newTrackingBodyStorage(data []byte) *trackingBodyStorage {
	return &trackingBodyStorage{data: data, reader: bytes.NewReader(data)}
}

func (s *trackingBodyStorage) Read(p []byte) (int, error) {
	if s.closed {
		return 0, common.ErrStorageClosed
	}
	return s.reader.Read(p)
}

func (s *trackingBodyStorage) Seek(offset int64, whence int) (int64, error) {
	s.seekCalls++
	if s.seekErr != nil {
		return 0, s.seekErr
	}
	if s.closed {
		return 0, common.ErrStorageClosed
	}
	return s.reader.Seek(offset, whence)
}

func (s *trackingBodyStorage) Close() error {
	s.closed = true
	return nil
}

func (s *trackingBodyStorage) Bytes() ([]byte, error) {
	if s.closed {
		return nil, common.ErrStorageClosed
	}
	data := make([]byte, len(s.data))
	copy(data, s.data)
	return data, nil
}

func (s *trackingBodyStorage) Size() int64 {
	return int64(s.reader.Size())
}

func (s *trackingBodyStorage) IsDisk() bool {
	return false
}

type countingRateLimitToken struct {
	releaseCalls int
}

func (t *countingRateLimitToken) Release() {
	t.releaseCalls++
}

func taskRateLimitChannel(id int, name string, onLimit string) *model.Channel {
	autoBan := 0
	channel := &model.Channel{
		Id:      id,
		Type:    1,
		Name:    name,
		Key:     "test-key",
		AutoBan: &autoBan,
	}
	channel.SetSetting(dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{
		Enabled:     true,
		Concurrency: 1,
		OnLimit:     onLimit,
	}})
	return channel
}

func setTaskRateLimitChannelContext(c *gin.Context, channel *model.Channel) {
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, channel.ChannelInfo.IsMultiKey)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, channel.GetSetting())
	common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://example.invalid")
}

func newTaskRateLimitRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: "task-model",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
}

func newTaskRateLimitRetryParam(c *gin.Context) *service.RetryParam {
	return &service.RetryParam{
		Ctx:         c,
		TokenGroup:  "default",
		ModelName:   "task-model",
		RequestPath: c.Request.URL.Path,
		Retry:       common.GetPointer(0),
	}
}

func TestRelayTaskRateLimitRejectsFirstAttempt(t *testing.T) {
	c := newRelayRateLimitTestContext(t)
	channel := taskRateLimitChannel(101, "reject-first", channel_limiter.OnLimitReject)
	setTaskRateLimitChannelContext(c, channel)
	bodyStorage := newTrackingBodyStorage([]byte(`{"prompt":"test"}`))
	c.Set(common.KeyBodyStorage, bodyStorage)
	relayInfo := newTaskRateLimitRelayInfo()
	submitCalls := 0

	result, taskErr := relayTaskSubmitWithRetry(c, relayInfo, newTaskRateLimitRetryParam(c), 3, taskRelaySubmitDependencies{
		getChannel: getChannel,
		acquireRateLimit: func(_ context.Context, channelID int, cfg *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
			assert.Equal(t, channel.Id, channelID)
			require.NotNil(t, cfg)
			assert.Equal(t, channel_limiter.OnLimitReject, cfg.OnLimit)
			return nil, channel_limiter.Decision{Allowed: false, Reason: channel_limiter.ReasonConcurrencyExceeded}
		},
		submit: func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
			submitCalls++
			return nil, nil
		},
	})

	assert.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusTooManyRequests, taskErr.StatusCode)
	assert.Equal(t, string(types.ErrorCodeChannelRateLimited), taskErr.Code)
	assert.True(t, taskErr.LocalError)
	assert.Equal(t, 0, submitCalls)
	assert.Equal(t, 0, bodyStorage.seekCalls)
	assert.Equal(t, []string{"101"}, c.GetStringSlice("use_channel"))
	require.NotNil(t, relayInfo.LastError)
	assert.Equal(t, http.StatusTooManyRequests, relayInfo.LastError.StatusCode)
}

func TestRelayTaskRateLimitSkipMovesToNextChannel(t *testing.T) {
	c := newRelayRateLimitTestContext(t)
	channelA := taskRateLimitChannel(201, "saturated", channel_limiter.OnLimitSkip)
	channelB := taskRateLimitChannel(202, "available", channel_limiter.OnLimitSkip)
	setTaskRateLimitChannelContext(c, channelA)
	bodyStorage := newTrackingBodyStorage([]byte(`{"prompt":"test"}`))
	c.Set(common.KeyBodyStorage, bodyStorage)
	relayInfo := newTaskRateLimitRelayInfo()
	getChannelCalls := 0
	acquireCalls := 0
	submittedChannelIDs := make([]int, 0, 1)
	token := &countingRateLimitToken{}

	result, taskErr := relayTaskSubmitWithRetry(c, relayInfo, newTaskRateLimitRetryParam(c), 1, taskRelaySubmitDependencies{
		getChannel: func(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
			getChannelCalls++
			if getChannelCalls == 1 {
				require.Nil(t, info.ChannelMeta)
				return getChannel(c, info, retryParam)
			}
			require.Equal(t, 2, getChannelCalls)
			require.NotNil(t, info.ChannelMeta)
			assert.Equal(t, channelA.Id, info.ChannelMeta.ChannelId)
			setTaskRateLimitChannelContext(c, channelB)
			return channelB, nil
		},
		acquireRateLimit: func(_ context.Context, channelID int, _ *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
			acquireCalls++
			if acquireCalls == 1 {
				assert.Equal(t, channelA.Id, channelID)
				return nil, channel_limiter.Decision{Allowed: false, Reason: channel_limiter.ReasonConcurrencyExceeded}
			}
			assert.Equal(t, channelB.Id, channelID)
			return token, channel_limiter.Decision{Allowed: true}
		},
		submit: func(c *gin.Context, _ *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
			submittedChannelIDs = append(submittedChannelIDs, c.GetInt("channel_id"))
			return &relay.TaskSubmitResult{UpstreamTaskID: "task-upstream"}, nil
		},
	})

	require.Nil(t, taskErr)
	require.NotNil(t, result)
	assert.Equal(t, "task-upstream", result.UpstreamTaskID)
	assert.Equal(t, 2, getChannelCalls)
	assert.Equal(t, 2, acquireCalls)
	assert.Equal(t, []int{channelB.Id}, submittedChannelIDs)
	assert.Equal(t, []string{"201", "202"}, c.GetStringSlice("use_channel"))
	assert.Equal(t, 1, bodyStorage.seekCalls)
	assert.Equal(t, 1, token.releaseCalls)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyRateLimitSkipped))
}

func TestRelayTaskRateLimitLockedSkipIsRejected(t *testing.T) {
	c := newRelayRateLimitTestContext(t)
	channel := taskRateLimitChannel(301, "locked", channel_limiter.OnLimitSkip)
	setTaskRateLimitChannelContext(c, channel)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{
		Enabled:     true,
		Concurrency: 1,
		OnLimit:     channel_limiter.OnLimitReject,
	}})
	bodyStorage := newTrackingBodyStorage([]byte(`{"prompt":"test"}`))
	c.Set(common.KeyBodyStorage, bodyStorage)
	relayInfo := newTaskRateLimitRelayInfo()
	relayInfo.LockedChannel = channel
	getChannelCalls := 0
	submitCalls := 0

	result, taskErr := relayTaskSubmitWithRetry(c, relayInfo, newTaskRateLimitRetryParam(c), 3, taskRelaySubmitDependencies{
		getChannel: func(*gin.Context, *relaycommon.RelayInfo, *service.RetryParam) (*model.Channel, *types.NewAPIError) {
			getChannelCalls++
			return nil, nil
		},
		acquireRateLimit: func(_ context.Context, _ int, cfg *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
			require.NotNil(t, cfg)
			assert.Equal(t, channel_limiter.OnLimitSkip, cfg.OnLimit)
			return nil, channel_limiter.Decision{Allowed: false, Reason: channel_limiter.ReasonConcurrencyExceeded}
		},
		submit: func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
			submitCalls++
			return nil, nil
		},
	})

	assert.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusTooManyRequests, taskErr.StatusCode)
	assert.Equal(t, 0, getChannelCalls)
	assert.Equal(t, 0, submitCalls)
	assert.Equal(t, 0, bodyStorage.seekCalls)
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyRateLimitSkipped))
}

func TestRelayTaskRateLimitQueueTimeoutReturns429WhenRetriesExhausted(t *testing.T) {
	c := newRelayRateLimitTestContext(t)
	channel := taskRateLimitChannel(401, "queued", channel_limiter.OnLimitQueue)
	setTaskRateLimitChannelContext(c, channel)
	bodyStorage := newTrackingBodyStorage([]byte(`{"prompt":"test"}`))
	c.Set(common.KeyBodyStorage, bodyStorage)
	relayInfo := newTaskRateLimitRelayInfo()
	submitCalls := 0

	result, taskErr := relayTaskSubmitWithRetry(c, relayInfo, newTaskRateLimitRetryParam(c), 0, taskRelaySubmitDependencies{
		getChannel: getChannel,
		acquireRateLimit: func(context.Context, int, *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
			return nil, channel_limiter.Decision{Allowed: false, Reason: channel_limiter.ReasonQueueTimeout}
		},
		submit: func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
			submitCalls++
			return nil, nil
		},
	})

	assert.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusTooManyRequests, taskErr.StatusCode)
	assert.Equal(t, string(types.ErrorCodeChannelRateLimited), taskErr.Code)
	assert.Equal(t, 0, submitCalls)
	assert.Equal(t, 0, bodyStorage.seekCalls)
	require.NotNil(t, relayInfo.ChannelMeta)
	assert.Equal(t, channel.Id, relayInfo.ChannelMeta.ChannelId)
}

func TestRelayTaskRateLimitReleasesTokenExactlyOnce(t *testing.T) {
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = false
	t.Cleanup(func() {
		constant.ErrorLogEnabled = originalErrorLogEnabled
	})

	tests := []struct {
		name            string
		bodyErr         error
		submitErr       *dto.TaskError
		wantSubmitCalls int
		wantStatus      int
	}{
		{
			name:            "success",
			wantSubmitCalls: 1,
		},
		{
			name:            "body error",
			bodyErr:         errors.New("body unavailable"),
			wantSubmitCalls: 0,
			wantStatus:      http.StatusBadRequest,
		},
		{
			name: "upstream error",
			submitErr: &dto.TaskError{
				Code:       "upstream_error",
				Message:    "upstream failed",
				StatusCode: http.StatusInternalServerError,
				Error:      errors.New("upstream failed"),
			},
			wantSubmitCalls: 1,
			wantStatus:      http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newRelayRateLimitTestContext(t)
			channel := taskRateLimitChannel(501, "release", channel_limiter.OnLimitReject)
			setTaskRateLimitChannelContext(c, channel)
			bodyStorage := newTrackingBodyStorage([]byte(`{"prompt":"test"}`))
			bodyStorage.seekErr = tt.bodyErr
			c.Set(common.KeyBodyStorage, bodyStorage)
			relayInfo := newTaskRateLimitRelayInfo()
			token := &countingRateLimitToken{}
			submitCalls := 0

			result, taskErr := relayTaskSubmitWithRetry(c, relayInfo, newTaskRateLimitRetryParam(c), 0, taskRelaySubmitDependencies{
				getChannel: func(*gin.Context, *relaycommon.RelayInfo, *service.RetryParam) (*model.Channel, *types.NewAPIError) {
					return channel, nil
				},
				acquireRateLimit: func(context.Context, int, *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
					return token, channel_limiter.Decision{Allowed: true}
				},
				submit: func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
					submitCalls++
					if tt.submitErr != nil {
						return nil, tt.submitErr
					}
					return &relay.TaskSubmitResult{UpstreamTaskID: "task-upstream"}, nil
				},
			})

			assert.Equal(t, 1, token.releaseCalls)
			assert.Equal(t, tt.wantSubmitCalls, submitCalls)
			if tt.wantStatus == 0 {
				require.Nil(t, taskErr)
				require.NotNil(t, result)
				return
			}
			assert.Nil(t, result)
			require.NotNil(t, taskErr)
			assert.Equal(t, tt.wantStatus, taskErr.StatusCode)
		})
	}
}

var _ common.BodyStorage = (*trackingBodyStorage)(nil)
