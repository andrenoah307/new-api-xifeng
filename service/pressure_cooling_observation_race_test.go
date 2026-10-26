package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Interleave another request after the observer has read its state snapshot.
type coolingObservationHook struct {
	afterRead func()
}

func (h *coolingObservationHook) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (h *coolingObservationHook) AfterProcess(_ context.Context, cmd redis.Cmder) error {
	if cmd.Name() == "hgetall" && h.afterRead != nil {
		callback := h.afterRead
		h.afterRead = nil
		callback()
	}
	return nil
}
func (h *coolingObservationHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (h *coolingObservationHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestPressureCoolingLateObservationCannotEraseRecovery(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	server := miniredis.RunT(t)
	client := usePressureCoolingRuntimeRedis(t, server)
	configurePressureCooling(t, func(cfg *operation_setting.PressureCoolingSetting) {
		cfg.Enabled = true
		cfg.MinActiveChannelsPerGroup = 1
		cfg.CooldownSeconds = 60
		cfg.MaxCooldownSeconds = 120
	})
	channel := pressureCoolingTestChannel(t, 12501, "observation-race", "pro", "race-model", dto.ChannelSettings{})
	pressureCoolingTestChannel(t, 12502, "observation-peer", "pro", "race-model", dto.ChannelSettings{})
	model.InitChannelCache()
	now := time.Now().Unix()
	client.AddHook(&coolingObservationHook{afterRead: func() {
		executePressureCooling(channel, &PressureCoolingState{State: "obs", TotalRequests: 3, Violations: 3},
			resolvePressureCoolingConfig(nil), now, 360, nil)
	}})

	CheckPressureCooling(channel.Id, 1)

	assert.Equal(t, "cool", loadPressureCoolingState(channel.Id).State, "a late observer must not remove the recovery marker")
	pressureCoolingRecoveryOnce(now + 61)
	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status, "the disabled channel must still recover at its deadline")
}

func TestPressureCoolingObservationRejectsStaleSnapshots(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			if backend == "redis" {
				usePressureCoolingRuntimeRedis(t, miniredis.RunT(t))
			} else {
				previous := common.RedisEnabled
				common.RedisEnabled = false
				t.Cleanup(func() { common.RedisEnabled = previous })
			}
			const channelID = 12511
			t.Cleanup(func() { pressureCoolingMemStore.Delete(channelID) })
			for _, next := range []PressureCoolingState{
				{State: "cool", Scope: "channel", CooldownUntil: 600, Consecutive: 1},
				{State: "susp", Scope: "groups", CooledGroups: []string{"pro"}},
				{State: "obs", GraceUntil: 900, Consecutive: 2},
			} {
				savePressureCoolingState(channelID, &PressureCoolingState{State: "obs"}, 300)
				stale := loadPressureCoolingState(channelID)
				stale.TotalRequests++
				savePressureCoolingState(channelID, &next, 300)
				require.False(t, savePressureCoolingObservation(channelID, stale, 1))
				stored := loadPressureCoolingState(channelID)
				assert.Equal(t, next.State, stored.State)
				assert.Equal(t, next.GraceUntil, stored.GraceUntil)
				assert.Equal(t, next.CooldownUntil, stored.CooldownUntil)
				assert.Equal(t, next.CooledGroups, stored.CooledGroups)
			}
			current := loadPressureCoolingState(channelID)
			current.TotalRequests = 3
			require.True(t, savePressureCoolingObservation(channelID, current, 300))
			assert.Equal(t, int64(3), loadPressureCoolingState(channelID).TotalRequests)
			stale := loadPressureCoolingState(channelID)
			deletePressureCoolingState(channelID)
			require.False(t, savePressureCoolingObservation(channelID, stale, 300), "reset must invalidate outstanding observations")
		})
	}
}

func TestPressureCoolingObservationMigratesLegacyRedisState(t *testing.T) {
	server := miniredis.RunT(t)
	client := usePressureCoolingRuntimeRedis(t, server)
	const channelID = 12512
	require.NoError(t, client.HSet(context.Background(), pressureCoolingRedisKey(channelID), "st", "obs", "tr", "2").Err())
	state := loadPressureCoolingState(channelID)
	state.TotalRequests++
	require.True(t, savePressureCoolingObservation(channelID, state, 300))
	assert.Equal(t, int64(3), loadPressureCoolingState(channelID).TotalRequests)
	assert.Equal(t, 300*time.Second, server.TTL(pressureCoolingRedisKey(channelID)))
}
