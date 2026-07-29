package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/channel_limiter"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferredAffinityChannelLimitPreFilter(t *testing.T) {
	tests := []struct {
		name          string
		onLimit       string
		hardAffinity  bool
		decision      channel_limiter.Decision
		wantAvailable bool
		wantChecks    int
	}{
		{
			name:          "soft skip saturated falls back",
			onLimit:       channel_limiter.OnLimitSkip,
			decision:      channel_limiter.Decision{Allowed: false, Reason: channel_limiter.ReasonRPMExceeded},
			wantAvailable: false,
			wantChecks:    1,
		},
		{
			name:          "soft default skip saturated falls back",
			onLimit:       "",
			decision:      channel_limiter.Decision{Allowed: false, Reason: channel_limiter.ReasonConcurrencyExceeded},
			wantAvailable: false,
			wantChecks:    1,
		},
		{
			name:          "soft skip available keeps affinity",
			onLimit:       channel_limiter.OnLimitSkip,
			decision:      channel_limiter.Decision{Allowed: true},
			wantAvailable: true,
			wantChecks:    1,
		},
		{
			name:          "hard affinity bypasses prefilter",
			onLimit:       channel_limiter.OnLimitSkip,
			hardAffinity:  true,
			decision:      channel_limiter.Decision{Allowed: false},
			wantAvailable: true,
			wantChecks:    0,
		},
		{
			name:          "queue bypasses prefilter",
			onLimit:       channel_limiter.OnLimitQueue,
			decision:      channel_limiter.Decision{Allowed: false},
			wantAvailable: true,
			wantChecks:    0,
		},
		{
			name:          "reject bypasses prefilter",
			onLimit:       channel_limiter.OnLimitReject,
			decision:      channel_limiter.Decision{Allowed: false},
			wantAvailable: true,
			wantChecks:    0,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			channel := &model.Channel{Id: 9600 + index, Name: fmt.Sprintf("affinity-%d", index)}
			channel.SetSetting(dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{
				Enabled:     true,
				RPM:         1,
				Concurrency: 1,
				OnLimit:     test.onLimit,
			}})

			checks := 0
			available := preferredAffinityHasCapacity(
				c,
				channel,
				test.hardAffinity,
				func(_ context.Context, channelID int, cfg *dto.ChannelRateLimit) channel_limiter.Decision {
					checks++
					require.Equal(t, channel.Id, channelID)
					require.Equal(t, test.onLimit, cfg.OnLimit)
					return test.decision
				},
			)

			assert.Equal(t, test.wantAvailable, available)
			assert.Equal(t, test.wantChecks, checks)
			assert.Equal(t, !test.wantAvailable, common.GetContextKeyBool(c, constant.ContextKeyRateLimitSkipped))
		})
	}
}
