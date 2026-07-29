package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/channel_limiter"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRelayRateLimitTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{}`))
	t.Cleanup(func() {
		common.CleanupBodyStorage(c)
	})
	return c
}

func TestGetChannelRateLimitSettingFromFirstAttemptContext(t *testing.T) {
	tests := []struct {
		name       string
		setting    *dto.ChannelSettings
		isMultiKey bool
		autoBan    bool
	}{
		{
			name: "active setting and multi-key channel",
			setting: &dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{
				Enabled:     true,
				RPM:         50,
				Concurrency: 3,
				OnLimit:     channel_limiter.OnLimitSkip,
			}},
			isMultiKey: true,
			autoBan:    true,
		},
		{
			name: "disabled setting and single-key channel",
			setting: &dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{
				Enabled: false,
				RPM:     25,
				OnLimit: channel_limiter.OnLimitReject,
			}},
		},
		{
			name:       "explicit zero values remain present",
			setting:    &dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{}},
			isMultiKey: true,
		},
		{
			name: "missing setting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newRelayRateLimitTestContext(t)
			common.SetContextKey(c, constant.ContextKeyChannelId, 17)
			common.SetContextKey(c, constant.ContextKeyChannelType, 14)
			common.SetContextKey(c, constant.ContextKeyChannelName, "first-channel")
			common.SetContextKey(c, constant.ContextKeyChannelAutoBan, tt.autoBan)
			common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, tt.isMultiKey)
			if tt.setting != nil {
				common.SetContextKey(c, constant.ContextKeyChannelSetting, *tt.setting)
			}

			info := &relaycommon.RelayInfo{}
			retryParam := &service.RetryParam{Ctx: c, Retry: common.GetPointer(0)}
			channel, apiErr := getChannel(c, info, retryParam)

			require.Nil(t, apiErr)
			require.NotNil(t, channel)
			assert.Equal(t, 17, channel.Id)
			assert.Equal(t, 14, channel.Type)
			assert.Equal(t, "first-channel", channel.Name)
			assert.Equal(t, tt.isMultiKey, channel.ChannelInfo.IsMultiKey)
			require.NotNil(t, channel.AutoBan)
			assert.Equal(t, tt.autoBan, *channel.AutoBan == 1)

			if tt.setting == nil {
				assert.Nil(t, channel.Setting)
				assert.Equal(t, dto.ChannelSettings{}, channel.GetSetting())
				return
			}
			assert.Equal(t, *tt.setting, channel.GetSetting())
		})
	}
}

func TestRelayRateLimitConfigFallsBackOnlyWhenChannelSettingIsMissing(t *testing.T) {
	activeContextSetting := dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{
		Enabled:     true,
		Concurrency: 2,
		OnLimit:     channel_limiter.OnLimitSkip,
	}}

	tests := []struct {
		name           string
		channelSetting *dto.ChannelSettings
		contextSetting *dto.ChannelSettings
		expected       *dto.ChannelRateLimit
	}{
		{
			name:           "missing channel setting uses context",
			contextSetting: &activeContextSetting,
			expected:       activeContextSetting.RateLimit,
		},
		{
			name: "disabled channel setting is not replaced by active context",
			channelSetting: &dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{
				Enabled: false,
				RPM:     10,
				OnLimit: channel_limiter.OnLimitReject,
			}},
			contextSetting: &activeContextSetting,
			expected: &dto.ChannelRateLimit{
				Enabled: false,
				RPM:     10,
				OnLimit: channel_limiter.OnLimitReject,
			},
		},
		{
			name:           "explicit zero channel setting is not replaced",
			channelSetting: &dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{}},
			contextSetting: &activeContextSetting,
			expected:       &dto.ChannelRateLimit{},
		},
		{
			name: "missing channel and context settings stays nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newRelayRateLimitTestContext(t)
			if tt.contextSetting != nil {
				common.SetContextKey(c, constant.ContextKeyChannelSetting, *tt.contextSetting)
			}
			channel := &model.Channel{Id: 23}
			if tt.channelSetting != nil {
				channel.SetSetting(*tt.channelSetting)
			}

			actual := getChannelRateLimitConfig(c, channel)

			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestRelayRateLimitSpecificChannelSkipIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"test"}]}`),
	)
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)
	t.Cleanup(func() {
		common.CleanupBodyStorage(c)
	})

	originalGroupRatio := ratio_setting.GetGroupRatio("default")
	ratio_setting.GetGroupRatioSetting().GroupRatio.Set("default", 0)
	quotaSetting := operation_setting.GetQuotaSetting()
	originalFreeModelPreConsume := quotaSetting.EnableFreeModelPreConsume
	quotaSetting.EnableFreeModelPreConsume = false
	t.Cleanup(func() {
		ratio_setting.GetGroupRatioSetting().GroupRatio.Set("default", originalGroupRatio)
		quotaSetting.EnableFreeModelPreConsume = originalFreeModelPreConsume
	})

	const channelID = 980001
	rateLimitCfg := &dto.ChannelRateLimit{
		Enabled:     true,
		Concurrency: 1,
		OnLimit:     channel_limiter.OnLimitSkip,
	}
	heldToken, decision := channel_limiter.Acquire(c.Request.Context(), channelID, rateLimitCfg)
	require.True(t, decision.Allowed)
	require.NotNil(t, heldToken)
	t.Cleanup(heldToken.Release)

	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o-mini")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{AcceptUnsetRatioModel: true})
	common.SetContextKey(c, constant.ContextKeyChannelId, channelID)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelName, "specific-channel")
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://example.invalid")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{RateLimit: rateLimitCfg})
	common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, channelID)

	Relay(c, types.RelayFormatOpenAI)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Contains(t, recorder.Body.String(), string(types.ErrorCodeChannelRateLimited))
	assert.Equal(t, []string{"980001"}, c.GetStringSlice("use_channel"))
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyRateLimitSkipped))
}
