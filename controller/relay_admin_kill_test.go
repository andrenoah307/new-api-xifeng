/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/channel_limiter"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The admin kill error must be invisible to every mechanism that punishes a
// channel, and must stop the retry loop. The "channel:" prefix is the trap here:
// IsChannelError is a prefix match that both shouldRetry and ShouldDisableChannel
// consult *before* SkipRetry, so a channel-prefixed code would silently restore
// retries and arm auto-ban on a channel that did nothing wrong.
func TestAdminClearedConnectionErrorNeitherRetriesNorBlamesTheChannel(t *testing.T) {
	c := newRelayRateLimitTestContext(t)

	apiErr := newAdminClearedConnectionError(c)
	require.NotNil(t, apiErr)

	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode, "downstream retry logic keys off 503")
	assert.True(t, apiErr.IsSkipRetry())
	assert.False(t, types.IsChannelError(apiErr), "a channel: prefix would force a retry and arm auto-ban")
	assert.False(t, shouldRecordChannelError(apiErr), "an administrator's action is not a channel fault")
	assert.False(t, shouldRetry(c, apiErr, 3), "retrying would only re-enter the same queue")

	autoDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = autoDisable })
	assert.False(t, service.ShouldDisableChannel(apiErr), "clearing a connection must never disable the channel")
}

// The message crosses the API boundary to the caller, so it may not name any
// channel, group, or upstream address (pitfalls #195, #266-#272).
func TestAdminClearedConnectionErrorMessageIsDesensitised(t *testing.T) {
	c := newRelayRateLimitTestContext(t)
	channel := taskRateLimitChannel(401, "internal-vendor-channel", channel_limiter.OnLimitReject)
	setTaskRateLimitChannelContext(c, channel)
	c.Set("group", "vip-secret-group")

	message := newAdminClearedConnectionError(c).ToOpenAIError().Message

	assert.Equal(t, i18n.T(c, i18n.MsgChannelConnectionCleared), message)
	assert.NotContains(t, message, channel.Name)
	assert.NotContains(t, message, "vip-secret-group")
	assert.NotContains(t, message, "example.invalid")
}

func TestRelayTaskAdminKillStopsRetryWithoutBlamingTheChannel(t *testing.T) {
	c := newRelayRateLimitTestContext(t)
	channel := taskRateLimitChannel(402, "killed-mid-flight", channel_limiter.OnLimitReject)
	setTaskRateLimitChannelContext(c, channel)
	c.Set(common.KeyBodyStorage, newTrackingBodyStorage([]byte(`{"prompt":"test"}`)))
	relayInfo := newTaskRateLimitRelayInfo()
	submitCalls := 0

	result, taskErr := relayTaskSubmitWithRetry(c, relayInfo, newTaskRateLimitRetryParam(c), 3, taskRelaySubmitDependencies{
		getChannel: getChannel,
		acquireRateLimit: func(context.Context, int, *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
			return nil, channel_limiter.Decision{Allowed: true}
		},
		submit: func(_ *gin.Context, info *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
			submitCalls++
			info.MarkAdminKilled()
			return nil, &dto.TaskError{Code: "upstream_error", StatusCode: http.StatusBadGateway, Error: errors.New("upstream gave up")}
		},
	})

	assert.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Equal(t, 1, submitCalls, "an administrator-cleared attempt must not be retried")
	assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
	assert.Equal(t, string(types.ErrorCodeConnectionCleared), taskErr.Code)
	assert.True(t, taskErr.LocalError, "LocalError keeps the attempt out of the channel error counter")
	assert.Equal(t, []string{"402"}, c.GetStringSlice("use_channel"))
	assert.False(t, relayInfo.SwapAdminKilled(), "the marker describes one attempt and must be consumed")
}

// A kill that lands after the upstream already answered must not turn a success
// into an error: the response is on the wire and the marker is simply stale.
func TestRelayTaskAdminKillRacingASuccessLeavesTheResultIntact(t *testing.T) {
	c := newRelayRateLimitTestContext(t)
	channel := taskRateLimitChannel(403, "killed-too-late", channel_limiter.OnLimitReject)
	setTaskRateLimitChannelContext(c, channel)
	c.Set(common.KeyBodyStorage, newTrackingBodyStorage([]byte(`{"prompt":"test"}`)))
	relayInfo := newTaskRateLimitRelayInfo()

	result, taskErr := relayTaskSubmitWithRetry(c, relayInfo, newTaskRateLimitRetryParam(c), 3, taskRelaySubmitDependencies{
		getChannel: getChannel,
		acquireRateLimit: func(context.Context, int, *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
			return nil, channel_limiter.Decision{Allowed: true}
		},
		submit: func(_ *gin.Context, info *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
			info.MarkAdminKilled()
			return &relay.TaskSubmitResult{UpstreamTaskID: "task-upstream"}, nil
		},
	})

	require.Nil(t, taskErr)
	require.NotNil(t, result)
	assert.Equal(t, "task-upstream", result.UpstreamTaskID)
	assert.False(t, relayInfo.SwapAdminKilled(), "the stale marker must still be consumed, not left for the next attempt")
}

// The gauge release moved into a defer so a panicking adaptor cannot leave the
// channel pinned as busy on the dashboard. The closure that carries the defer
// must not become a recover point: task relay has no panic handling of its own
// and swallowing one here would hide a crash behind an empty result.
func TestRelayTaskSubmitPanicStillPropagates(t *testing.T) {
	c := newRelayRateLimitTestContext(t)
	channel := taskRateLimitChannel(404, "panicking-adaptor", channel_limiter.OnLimitReject)
	setTaskRateLimitChannelContext(c, channel)
	c.Set(common.KeyBodyStorage, newTrackingBodyStorage([]byte(`{"prompt":"test"}`)))

	assert.PanicsWithValue(t, "adaptor exploded", func() {
		relayTaskSubmitWithRetry(c, newTaskRateLimitRelayInfo(), newTaskRateLimitRetryParam(c), 3, taskRelaySubmitDependencies{
			getChannel: getChannel,
			acquireRateLimit: func(context.Context, int, *dto.ChannelRateLimit) (rateLimitReleaser, channel_limiter.Decision) {
				return nil, channel_limiter.Decision{Allowed: true}
			},
			submit: func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
				panic("adaptor exploded")
			},
		})
	})
}
