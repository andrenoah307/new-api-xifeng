package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configurePressureCoolingRuntimeTest(t *testing.T, mutate func(*operation_setting.PressureCoolingSetting)) {
	t.Helper()
	setting := config.GlobalConfig.Get("pressure_cooling").(*operation_setting.PressureCoolingSetting)
	original := *setting
	mutate(setting)
	t.Cleanup(func() { *setting = original })
}

func pressureCoolingRuntimeTestChannel(id int, setting dto.ChannelSettings) *model.Channel {
	channel := &model.Channel{Id: id}
	channel.SetSetting(setting)
	return channel
}

func usePressureCoolingRuntimeRedis(t *testing.T, server *miniredis.Miniredis) *redis.Client {
	t.Helper()
	oldEnabled, oldClient := common.RedisEnabled, common.RDB
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled, common.RDB = oldEnabled, oldClient
	})
	return client
}

type pressureCoolingPipelineHook struct {
	pipelineCalls int
	commandCalls  int
}

func (h *pressureCoolingPipelineHook) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	h.commandCalls++
	return ctx, nil
}

func (h *pressureCoolingPipelineHook) AfterProcess(context.Context, redis.Cmder) error { return nil }

func (h *pressureCoolingPipelineHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	h.pipelineCalls++
	return ctx, nil
}

func (h *pressureCoolingPipelineHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestBatchPressureCoolingRuntimeRedisPipelineValues(t *testing.T) {
	server := miniredis.RunT(t)
	client := usePressureCoolingRuntimeRedis(t, server)
	configurePressureCoolingRuntimeTest(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.UpstreamErrorTriggerPercent = 50
		setting.UpstreamErrorMinSamples = 10
		setting.UpstreamErrorTriggerCount = 0
		setting.ObservationWindowSeconds = 60
	})
	first := pressureCoolingRuntimeTestChannel(12001, dto.ChannelSettings{})
	secondWindow := 15
	secondCount := 5
	second := pressureCoolingRuntimeTestChannel(12002, dto.ChannelSettings{PressureCooling: &dto.PressureCoolingOverride{
		ObservationWindowSeconds:    &secondWindow,
		UpstreamErrorTriggerPercent: common.GetPointer(0),
		UpstreamErrorMinSamples:     common.GetPointer(0),
		UpstreamErrorTriggerCount:   &secondCount,
	}})
	now := time.Now().Unix()
	ctx := context.Background()
	firstKey := pressureCoolingErrorWindowKey(first.Id, 60, now)
	secondKey := pressureCoolingErrorWindowKey(second.Id, secondWindow, now)
	require.NoError(t, client.HSet(ctx, firstKey, "a", "12", "e", "6").Err())
	require.NoError(t, client.HSet(ctx, secondKey, "a", "8", "e", "5").Err())
	require.NoError(t, client.HSet(ctx, pressureCoolingRedisKey(first.Id), "st", "cool", "cu", "12345").Err())
	require.NoError(t, client.HSet(ctx, pressureCoolingRedisKey(second.Id), "st", "susp", "cu", "67890").Err())
	hook := &pressureCoolingPipelineHook{}
	client.AddHook(hook)

	runtimes := BatchPressureCoolingRuntime([]*model.Channel{first, second})
	assert.Equal(t, 1, hook.pipelineCalls)
	assert.Equal(t, 0, hook.commandCalls)
	assert.Equal(t, PressureCoolingRuntime{
		Configured: true, Enabled: true, Attempts: 12, Errors: 6,
		TriggerPercent: 50, MinSamples: 10, WindowSeconds: 60,
		State: "cool", CooldownUntil: 12345,
	}, runtimes[first.Id])
	assert.Equal(t, PressureCoolingRuntime{
		Configured: true, Enabled: true, Attempts: 8, Errors: 5,
		TriggerCount: 5, WindowSeconds: 15,
		State: "susp", CooldownUntil: 67890,
	}, runtimes[second.Id])
}

func TestBatchPressureCoolingRuntimeMemoryReadsExistingBuckets(t *testing.T) {
	oldEnabled, oldClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldEnabled, oldClient })
	configurePressureCoolingRuntimeTest(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.UpstreamErrorTriggerPercent = 0
		setting.UpstreamErrorTriggerCount = 3
		setting.ObservationWindowSeconds = 30
	})
	channel := pressureCoolingRuntimeTestChannel(12003, dto.ChannelSettings{})
	now := time.Now().Unix()
	for i := 0; i < 4; i++ {
		incrPressureCoolingErrorWindowAt(channel.Id, 30, i > 0, now)
	}
	savePressureCoolingState(channel.Id, &PressureCoolingState{State: "cool", CooldownUntil: 99}, 60)

	runtime := BatchPressureCoolingRuntime([]*model.Channel{channel})[channel.Id]
	assert.Equal(t, int64(4), runtime.Attempts)
	assert.Equal(t, int64(3), runtime.Errors)
	assert.Equal(t, 3, runtime.TriggerCount)
	assert.Equal(t, 30, runtime.WindowSeconds)
	assert.Equal(t, "cool", runtime.State)
	assert.Equal(t, int64(99), runtime.CooldownUntil)
}

func TestBatchPressureCoolingRuntimeRedisFailureReturnsZeroValues(t *testing.T) {
	server := miniredis.RunT(t)
	client := usePressureCoolingRuntimeRedis(t, server)
	configurePressureCoolingRuntimeTest(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.UpstreamErrorTriggerPercent = 50
	})
	channels := []*model.Channel{
		pressureCoolingRuntimeTestChannel(12004, dto.ChannelSettings{}),
		pressureCoolingRuntimeTestChannel(12005, dto.ChannelSettings{}),
	}
	server.Close()

	assert.Equal(t, map[int]PressureCoolingRuntime{
		12004: {},
		12005: {},
	}, BatchPressureCoolingRuntime(channels))
	assert.NoError(t, client.Close())
}

func TestBatchPressureCoolingRuntimeHandlesEmptyInputAndNilRedis(t *testing.T) {
	assert.Empty(t, BatchPressureCoolingRuntime(nil))

	oldEnabled, oldClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldEnabled, oldClient })
	channel := pressureCoolingRuntimeTestChannel(12006, dto.ChannelSettings{})
	assert.Equal(t, map[int]PressureCoolingRuntime{channel.Id: {}}, BatchPressureCoolingRuntime([]*model.Channel{channel, nil}))
}

func TestBatchPressureCoolingRuntimeIgnoresCorruptRedisState(t *testing.T) {
	server := miniredis.RunT(t)
	client := usePressureCoolingRuntimeRedis(t, server)
	configurePressureCoolingRuntimeTest(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.UpstreamErrorTriggerCount = 2
	})
	channel := pressureCoolingRuntimeTestChannel(12007, dto.ChannelSettings{})
	require.NoError(t, client.HSet(context.Background(), pressureCoolingRedisKey(channel.Id), "st", "cool", "cu", "invalid").Err())

	runtime := BatchPressureCoolingRuntime([]*model.Channel{channel})[channel.Id]
	assert.True(t, runtime.Configured)
	assert.Equal(t, int64(0), runtime.CooldownUntil)
	assert.Empty(t, runtime.State)
}
