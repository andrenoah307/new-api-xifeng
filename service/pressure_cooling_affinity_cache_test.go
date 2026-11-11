package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	realtimemetrics "github.com/QuantumNous/new-api/pkg/realtime_metrics"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffinityCacheCoolingThresholdAndCombination(t *testing.T) {
	cfg := resolvedPressureCoolingConfig{AffinityCacheMinPercent: 30, AffinityCacheMinSamples: 100, ObservationWindowSeconds: 60, TriggerPercent: 50}
	state := &PressureCoolingState{WindowStart: 1000, TotalRequests: 4, Violations: 2}
	for _, tt := range []struct {
		name            string
		samples, cached int64
		mode            string
		now             int64
		met             bool
	}{
		{"insufficient", 99, 0, "any", 1010, false},
		{"equal", 100, 300, "any", 1010, false},
		{"below", 100, 299, "any", 1010, true},
		{"all fresh", 100, 100, "all", 1010, true},
		{"all stale FRT", 100, 100, "all", 1061, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg.ConditionMode = tt.mode
			r := affinityCacheCoolingReason(cfg, state, realtimemetrics.ChannelAffinityCacheUsage{PromptTokens: 1000, CachedTokens: tt.cached, Samples: tt.samples}, tt.now)
			assert.Equal(t, tt.met, r != nil)
		})
	}
	cfg.ConditionMode = "all"
	assert.False(t, pressureCoolingConditionsMet(cfg, true, true), "request path cannot bypass the cache condition")
	cfg.ConditionMode, cfg.AffinityCacheMinPercent = "any", 29
	assert.Nil(t, affinityCacheCoolingReason(cfg, state, realtimemetrics.ChannelAffinityCacheUsage{PromptTokens: 1000, CachedTokens: 290, Samples: 100}, 1010))
}

func TestAffinityCacheCoolingBackgroundAndRecoveryGates(t *testing.T) {
	ch, _, now := preparePressureCoolingErrorTest(t)
	percent, hours, samples := 30, 2, 100
	ch.SetSetting(dto.ChannelSettings{PressureCooling: &dto.PressureCoolingOverride{AffinityCacheMinPercent: &percent, AffinityCacheWindowHours: &hours, AffinityCacheMinSamples: &samples}})
	require.NoError(t, model.DB.Model(ch).Update("setting", ch.Setting).Error)
	model.InitChannelCache()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	old, enabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = old, enabled; _ = client.Close() })
	key := fmt.Sprintf("nak:rt:affcache:%d:%d", ch.Id, now/300*300)
	require.NoError(t, client.HSet(context.Background(), key, "p", 1000, "c", 100, "n", 99).Err())
	require.NoError(t, client.Expire(context.Background(), key, 25*time.Hour).Err())
	checkChannelAffinityCacheCooling(now)
	assert.Equal(t, "obs", loadPressureCoolingState(ch.Id).State)
	require.NoError(t, client.HSet(context.Background(), key, "n", 100).Err())
	require.True(t, savePressureCoolingState(ch.Id, &PressureCoolingState{State: "obs", GraceUntil: now + 30}, 3600))
	checkChannelAffinityCacheCooling(now)
	assert.Equal(t, "obs", loadPressureCoolingState(ch.Id).State)
	require.True(t, savePressureCoolingState(ch.Id, &PressureCoolingState{State: "obs"}, 3600))
	checkChannelAffinityCacheCooling(now)
	state := loadPressureCoolingState(ch.Id)
	assert.Equal(t, "cool", state.State)
	assert.EqualValues(t, 1, state.Consecutive)
	checkChannelAffinityCacheCooling(now + 1)
	assert.EqualValues(t, 1, loadPressureCoolingState(ch.Id).Consecutive)
	stored, err := model.GetChannelById(ch.Id, false)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	pressureCoolingRecoveryOnce(now + 31)
	assert.Equal(t, "obs", loadPressureCoolingState(ch.Id).State)
	checkChannelAffinityCacheCooling(now + 32)
	assert.Equal(t, "obs", loadPressureCoolingState(ch.Id).State, "old low-rate data must not immediately recool after recovery")
	server.FastForward(26 * time.Hour)
	checkChannelAffinityCacheCooling(now + 33)
	assert.Equal(t, "obs", loadPressureCoolingState(ch.Id).State)
}
