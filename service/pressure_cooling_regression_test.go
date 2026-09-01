package service

import (
	"context"
	"runtime"
	"sync"
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

func TestPressureCoolingRecoveryManualDisabledChannelKeepsStatusAndReleasesState(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.GracePeriodSeconds = 17
	})
	channel := pressureCoolingTestChannel(t, 10201, "manual-recovery", "pro", "manual-recovery-model", dto.ChannelSettings{})
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "admin"))
	now := time.Now().Unix()
	savePressureCoolingState(channel.Id, &PressureCoolingState{
		State: "cool", CooldownUntil: now - 1, Consecutive: 2,
		Violations: 4, TotalRequests: 6, WindowStart: now - 10,
	}, 900)

	pressureCoolingRecoveryOnce(now)

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	state := loadPressureCoolingState(channel.Id)
	assert.Equal(t, "obs", state.State)
	assert.Equal(t, int64(0), state.Violations)
	assert.Equal(t, int64(0), state.TotalRequests)
	assert.Equal(t, now, state.WindowStart)
	assert.Equal(t, now+17, state.GraceUntil)
}

func TestPressureCoolingRecoveryDisabledConfigDoesNotEnableManualDisabledChannel(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = false
	})
	channel := pressureCoolingTestChannel(t, 10202, "manual-disabled-config", "pro", "manual-disabled-config-model", dto.ChannelSettings{})
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "admin"))
	savePressureCoolingState(channel.Id, &PressureCoolingState{State: "cool", CooldownUntil: time.Now().Unix() - 1}, 900)

	pressureCoolingRecoveryOnce(time.Now().Unix())

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	_, exists := pressureCoolingMemStore.Load(channel.Id)
	assert.False(t, exists)
	assert.Equal(t, "obs", loadPressureCoolingState(channel.Id).State)
}

func TestPressureCoolingRecoverySuspendedManualDisabledChannelKeepsStatus(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.MaxConsecutiveCooldowns = 1
	})
	channel := pressureCoolingTestChannel(t, 10203, "manual-suspend", "pro", "manual-suspend-model", dto.ChannelSettings{})
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "admin"))
	now := time.Now().Unix()
	savePressureCoolingState(channel.Id, &PressureCoolingState{
		State: "cool", CooldownUntil: now - 1, Consecutive: 1,
	}, 900)

	pressureCoolingRecoveryOnce(now)

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "susp", loadPressureCoolingState(channel.Id).State)
}

func TestExecutePressureCoolingConcurrentMinimumActiveGuard(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.MinActiveChannelsPerGroup = 1
		setting.CooldownSeconds = 30
		setting.MaxCooldownSeconds = 60
	})
	first := pressureCoolingTestChannel(t, 10211, "concurrent-first", "pro", "concurrent-model", dto.ChannelSettings{})
	second := pressureCoolingTestChannel(t, 10212, "concurrent-second", "pro", "concurrent-model", dto.ChannelSettings{})
	model.InitChannelCache()
	cfg := resolvePressureCoolingConfig(nil)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, channel := range []*model.Channel{first, second} {
		wg.Add(1)
		go func(ch *model.Channel) {
			defer wg.Done()
			<-start
			executePressureCooling(ch, &PressureCoolingState{State: "obs", Violations: 3, TotalRequests: 3}, cfg, time.Now().Unix(), 900, nil)
		}(channel)
	}
	previousProcs := runtime.GOMAXPROCS(2)
	close(start)
	wg.Wait()
	runtime.GOMAXPROCS(previousProcs)

	firstStored, err := model.GetChannelById(first.Id, true)
	require.NoError(t, err)
	secondStored, err := model.GetChannelById(second.Id, true)
	require.NoError(t, err)
	disabled := 0
	for _, channel := range []*model.Channel{firstStored, secondStored} {
		if channel.Status == common.ChannelStatusAutoDisabled {
			disabled++
		}
	}
	assert.Equal(t, 1, disabled)
}

func TestExecutePressureCoolingSkipsManuallyDisabledChannelButCoolsEnabledChannel(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		wantStatus      int
		wantState       string
		wantConsecutive int64
	}{
		{
			name:            "manual disabled channel is left untouched",
			status:          common.ChannelStatusManuallyDisabled,
			wantStatus:      common.ChannelStatusManuallyDisabled,
			wantState:       "obs",
			wantConsecutive: 2,
		},
		{
			name:            "enabled channel still cools",
			status:          common.ChannelStatusEnabled,
			wantStatus:      common.ChannelStatusAutoDisabled,
			wantState:       "cool",
			wantConsecutive: 3,
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preparePressureCoolingGroupTest(t)
			configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
				setting.Enabled = true
				setting.MinActiveChannelsPerGroup = 0
				setting.CooldownSeconds = 30
				setting.MaxCooldownSeconds = 60
			})
			channel := pressureCoolingTestChannel(t, 10230+index*2, "status-guard", "pro", "status-guard-model", dto.ChannelSettings{})
			pressureCoolingTestChannel(t, 10231+index*2, "status-guard-peer", "pro", "status-guard-model", dto.ChannelSettings{})
			model.InitChannelCache()
			if test.status != common.ChannelStatusEnabled {
				require.True(t, model.UpdateChannelStatus(channel.Id, "", test.status, "admin"))
			}
			channel, err := model.CacheGetChannel(channel.Id)
			require.NoError(t, err)
			now := time.Now().Unix()
			state := &PressureCoolingState{
				State: "obs", Scope: "channel", Violations: 3, TotalRequests: 3,
				WindowStart: now - 1, Consecutive: 2,
			}

			executePressureCooling(channel, state, resolvePressureCoolingConfig(nil), now, 900, nil)

			stored, err := model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			assert.Equal(t, test.wantStatus, stored.Status)
			storedState := loadPressureCoolingState(channel.Id)
			assert.Equal(t, test.wantState, storedState.State)
			assert.Equal(t, test.wantConsecutive, storedState.Consecutive)
			assert.Zero(t, storedState.Violations)
			if test.status == common.ChannelStatusManuallyDisabled {
				assert.Equal(t, now, storedState.WindowStart)
			} else {
				assert.Equal(t, now-1, storedState.WindowStart)
			}
		})
	}
}

func TestLoadPressureCoolingStateRedisRejectsCorruptNumericField(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedis, oldClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedis, oldClient
	})
	require.NoError(t, client.HSet(context.Background(), pressureCoolingRedisKey(10221), map[string]interface{}{
		"st": "cool", "vc": "0", "tr": "0", "ws": "0", "cu": "broken", "cc": "0", "gu": "0",
	}).Err())

	state, err := loadPressureCoolingStateRedis(10221)
	require.Nil(t, state)
	require.Error(t, err)
	assert.ErrorIs(t, err, errPressureCoolingStateCorrupt)
}

func TestLoadPressureCoolingStateRedisTreatsMissingNumericFieldsAsZero(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedis, oldClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedis, oldClient
	})
	require.NoError(t, client.HSet(context.Background(), pressureCoolingRedisKey(10224), "st", "cool").Err())

	state, err := loadPressureCoolingStateRedis(10224)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, int64(0), state.Violations)
	assert.Equal(t, int64(0), state.TotalRequests)
	assert.Equal(t, int64(0), state.WindowStart)
	assert.Equal(t, int64(0), state.CooldownUntil)
	assert.Equal(t, int64(0), state.Consecutive)
	assert.Equal(t, int64(0), state.GraceUntil)
}

func TestLoadPressureCoolingStateRedisRejectsPresentInvalidNumericField(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "non-numeric", value: "abc"},
		{name: "empty", value: ""},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			oldRedis, oldClient := common.RedisEnabled, common.RDB
			common.RedisEnabled, common.RDB = true, client
			t.Cleanup(func() {
				common.RedisEnabled, common.RDB = oldRedis, oldClient
			})
			channelID := 10225 + index
			require.NoError(t, client.HSet(context.Background(), pressureCoolingRedisKey(channelID), map[string]interface{}{
				"st": "cool", "cu": test.value,
			}).Err())

			state, err := loadPressureCoolingStateRedis(channelID)
			require.Nil(t, state)
			require.Error(t, err)
			assert.ErrorIs(t, err, errPressureCoolingStateCorrupt)
			assert.Contains(t, err.Error(), "field cu")
		})
	}
}

func TestListCoolingChannelStatesSkipsCorruptKeyAndKeepsTransportErrorsFatal(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedis, oldClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedis, oldClient
	})
	require.NoError(t, client.HSet(context.Background(), pressureCoolingRedisKey(10222), map[string]interface{}{
		"st": "cool", "vc": "bad", "tr": "0", "ws": "0", "cu": "0", "cc": "0", "gu": "0",
	}).Err())
	require.NoError(t, client.HSet(context.Background(), pressureCoolingRedisKey(10223), map[string]interface{}{
		"st": "cool", "vc": "1", "tr": "2", "ws": "3", "cu": "4", "cc": "5", "gu": "6",
	}).Err())

	listed, err := listCoolingChannelStatesResult()
	require.NoError(t, err)
	assert.NotContains(t, listed, 10222)
	assert.Contains(t, listed, 10223)

	server.Close()
	listed, err = listCoolingChannelStatesResult()
	assert.Nil(t, listed)
	assert.Error(t, err)
}
