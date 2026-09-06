package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type quotaWarningRedisErrorHook struct {
	commandName string
}

func (h quotaWarningRedisErrorHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == h.commandName {
		return ctx, errors.New("injected Redis command error")
	}
	return ctx, nil
}

func (quotaWarningRedisErrorHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (quotaWarningRedisErrorHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (quotaWarningRedisErrorHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func setupQuotaWarningRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousEnabled := common.RedisEnabled
	previousClient := common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = previousEnabled
		common.RDB = previousClient
		require.NoError(t, client.Close())
	})
	return server
}

func TestBuildQuotaWarningNotify(t *testing.T) {
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedisEnabled })

	prompt := "额度预警"
	remaining := 123456
	topUpLink := "https://example.test/topup"
	quota := logger.FormatQuota(remaining)

	tests := []struct {
		name       string
		notifyType string
		content    string
		values     []interface{}
		valueCount int
	}{
		{
			name:       "bark",
			notifyType: dto.NotifyTypeBark,
			content:    "{{value}}，剩余额度：{{value}}，请及时充值",
			values:     []interface{}{prompt, quota},
			valueCount: 2,
		},
		{
			name:       "gotify",
			notifyType: dto.NotifyTypeGotify,
			content:    "{{value}}，当前剩余额度为 {{value}}，请及时充值。",
			values:     []interface{}{prompt, quota},
			valueCount: 2,
		},
		{
			name:       "webhook",
			notifyType: dto.NotifyTypeWebhook,
			content:    "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>",
			values:     []interface{}{prompt, quota, topUpLink, topUpLink},
			valueCount: 4,
		},
		{
			name:       "email",
			notifyType: dto.NotifyTypeEmail,
			content:    "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>",
			values:     []interface{}{prompt, quota, topUpLink, topUpLink},
			valueCount: 4,
		},
		{
			name:       "empty defaults to email",
			notifyType: "",
			content:    "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>",
			values:     []interface{}{prompt, quota, topUpLink, topUpLink},
			valueCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, values := buildQuotaWarningNotify(tt.notifyType, prompt, remaining, topUpLink)
			require.Equal(t, tt.content, content)
			require.Len(t, values, tt.valueCount)
			assert.Equal(t, tt.values, values)
			assert.Equal(t, quota, values[1])
		})
	}
}

func TestBuildQuotaWarningNotifyNegativeRemaining(t *testing.T) {
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedisEnabled })

	remaining := -42
	content, values := buildQuotaWarningNotify(dto.NotifyTypeEmail, "额度预警", remaining, "https://example.test/topup")

	require.Equal(t, "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>", content)
	require.Len(t, values, 4)
	assert.Equal(t, logger.FormatQuota(remaining), values[1])
}

func TestShouldSendQuotaWarningEdge(t *testing.T) {
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedisEnabled })

	tests := []struct {
		name  string
		key   string
		steps []struct {
			below bool
			want  bool
		}
	}{
		{
			name: "first below only sends once",
			key:  "quota_warn_state:test-build-first",
			steps: []struct {
				below bool
				want  bool
			}{{below: true, want: true}, {below: true, want: false}},
		},
		{
			name: "clear then below sends again",
			key:  "quota_warn_state:test-build-clear",
			steps: []struct {
				below bool
				want  bool
			}{{below: true, want: true}, {below: false, want: false}, {below: true, want: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quotaWarnStateStore.Delete(tt.key)
			t.Cleanup(func() { quotaWarnStateStore.Delete(tt.key) })
			for _, step := range tt.steps {
				assert.Equal(t, step.want, shouldSendQuotaWarningEdge(tt.key, step.below))
			}
		})
	}
}

func TestShouldSendQuotaWarningEdgeRedis(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(t *testing.T, server *miniredis.Miniredis, key string)
		below      bool
		want       bool
		wantExists bool
		wantTTL    time.Duration
		withHook   bool
	}{
		{
			name:       "below acquires missing latch",
			below:      true,
			want:       true,
			wantExists: true,
			wantTTL:    quotaWarnStateTTL,
		},
		{
			name: "below does not reacquire held latch",
			prepare: func(t *testing.T, server *miniredis.Miniredis, key string) {
				require.NoError(t, server.Set(key, "1"))
			},
			below:      true,
			want:       false,
			wantExists: true,
		},
		{
			name: "recovery clears existing latch",
			prepare: func(t *testing.T, server *miniredis.Miniredis, key string) {
				require.NoError(t, server.Set(key, "1"))
				require.True(t, server.Exists(key))
			},
			below:      false,
			want:       false,
			wantExists: false,
		},
		{
			name:       "setnx error fails closed",
			below:      true,
			want:       false,
			wantExists: false,
			withHook:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupQuotaWarningRedis(t)
			key := "quota_warn_state:redis:" + tt.name
			if tt.prepare != nil {
				tt.prepare(t, server, key)
			}
			if tt.withHook {
				common.RDB.AddHook(quotaWarningRedisErrorHook{commandName: "set"})
			}

			assert.Equal(t, tt.want, shouldSendQuotaWarningEdge(key, tt.below))
			assert.Equal(t, tt.wantExists, server.Exists(key))
			if tt.wantTTL != 0 {
				require.Equal(t, tt.wantTTL, server.TTL(key))
			}
		})
	}
}

func TestReleaseQuotaWarningLatch(t *testing.T) {
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = originalRedisEnabled })

	stateKey := "quota_warn_state:test-release"
	quotaWarnStateStore.Delete(stateKey)
	t.Cleanup(func() { quotaWarnStateStore.Delete(stateKey) })

	require.True(t, shouldSendQuotaWarningEdge(stateKey, true))
	releaseQuotaWarningLatch(stateKey)
	assert.True(t, shouldSendQuotaWarningEdge(stateKey, true))
	assert.False(t, shouldSendQuotaWarningEdge(stateKey, true))
}

func TestReleaseQuotaWarningLatchRedis(t *testing.T) {
	server := setupQuotaWarningRedis(t)
	stateKey := "quota_warn_state:redis-release"
	require.NoError(t, server.Set(stateKey, "1"))
	server.SetTTL(stateKey, quotaWarnStateTTL)
	require.True(t, server.Exists(stateKey))
	require.Equal(t, quotaWarnStateTTL, server.TTL(stateKey))

	releaseQuotaWarningLatch(stateKey)

	assert.True(t, server.Exists(stateKey))
	assert.Equal(t, quotaWarnFailureRetryTTL, server.TTL(stateKey))
}
