package service

import (
	"fmt"
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

func pressureCoolingTestChannel(t *testing.T, id int, name, groups, models string, setting dto.ChannelSettings) *model.Channel {
	t.Helper()
	priority := int64(10)
	weight := uint(1)
	channel := &model.Channel{
		Id:       id,
		Name:     name,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Group:    groups,
		Models:   models,
		Priority: &priority,
		Weight:   &weight,
	}
	channel.SetSetting(setting)
	require.NoError(t, model.DB.Create(channel).Error)
	for _, group := range channel.GetGroups() {
		for _, modelName := range channel.GetModels() {
			require.NoError(t, model.DB.Create(&model.Ability{
				Group: group, Model: modelName, ChannelId: id, Enabled: true,
				Priority: &priority, Weight: weight,
			}).Error)
		}
	}
	return channel
}

func configurePressureCooling(t *testing.T, mutate func(*operation_setting.PressureCoolingSetting)) {
	t.Helper()
	setting := config.GlobalConfig.Get("pressure_cooling").(*operation_setting.PressureCoolingSetting)
	original := *setting
	mutate(setting)
	t.Cleanup(func() { *setting = original })
}

func preparePressureCoolingGroupTest(t *testing.T) {
	t.Helper()
	truncate(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 9900, Username: "pressure-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	originalMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	model.SetChannelGroupCooling(nil)
	model.InitChannelCache()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCache
		model.SetChannelGroupCooling(nil)
		pressureCoolingMemStore.Range(func(key, _ interface{}) bool {
			pressureCoolingMemStore.Delete(key)
			return true
		})
		if originalMemoryCache {
			model.InitChannelCache()
		}
	})
}

func TestExecutePressureCoolingDefaultScopeKeepsChannelBehavior(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.MinActiveChannelsPerGroup = 1
		setting.CooldownSeconds = 100
		setting.MaxCooldownSeconds = 120
		setting.CooldownBackoffMultiplier = 2
	})
	channel := pressureCoolingTestChannel(t, 9901, "channel-scope", "pro", "scope-model", dto.ChannelSettings{})
	pressureCoolingTestChannel(t, 9902, "channel-peer", "pro", "scope-model", dto.ChannelSettings{})
	model.InitChannelCache()

	state := &PressureCoolingState{State: "obs", Violations: 3, TotalRequests: 3, Consecutive: 1}
	cfg := resolvePressureCoolingConfig(nil)
	executePressureCooling(channel, state, cfg, time.Now().Unix(), 300)

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status, "default scope must use the existing channel status path")
	assert.Equal(t, "channel", loadPressureCoolingState(channel.Id).Scope)
	assert.False(t, model.IsChannelGroupCooled(channel.Id, "pro"))
}

func TestExecutePressureCoolingGroupsOnlyCoolsSelectedGroups(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.MinActiveChannelsPerGroup = 1
		setting.CooldownSeconds = 100
		setting.MaxCooldownSeconds = 120
		setting.CooldownBackoffMultiplier = 2
	})
	channel := pressureCoolingTestChannel(t, 9911, "group-scope", "pro,cheap", "scope-model", dto.ChannelSettings{})
	pressureCoolingTestChannel(t, 9912, "pro-peer", "pro", "scope-model", dto.ChannelSettings{})
	model.InitChannelCache()
	cfg := resolvePressureCoolingConfig(&dto.PressureCoolingOverride{Scope: "groups", CooldownGroups: []string{"pro"}})
	state := &PressureCoolingState{State: "obs", Violations: 3, TotalRequests: 3, Consecutive: 1}

	executePressureCooling(channel, state, cfg, time.Now().Unix(), 300)

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status, "group scope must not update channels.status")
	assert.True(t, model.IsChannelGroupCooled(channel.Id, "pro"))
	assert.False(t, model.IsChannelGroupCooled(channel.Id, "cheap"), "unspecified groups must remain available")
	state = loadPressureCoolingState(channel.Id)
	assert.Equal(t, "groups", state.Scope)
	assert.ElementsMatch(t, []string{"pro"}, state.CooledGroups)
}

func TestExecutePressureCoolingGroupsMinimumActiveOnlyTargetsSelectedGroups(t *testing.T) {
	tests := []struct {
		name       string
		proPeer    bool
		wantCooled bool
	}{
		{name: "target group has insufficient capacity", proPeer: false, wantCooled: false},
		{name: "unselected group capacity does not block", proPeer: true, wantCooled: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preparePressureCoolingGroupTest(t)
			configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
				setting.Enabled = true
				setting.MinActiveChannelsPerGroup = 1
			})
			channel := pressureCoolingTestChannel(t, 9921, "capacity", "pro,cheap", "capacity-model", dto.ChannelSettings{})
			if test.proPeer {
				pressureCoolingTestChannel(t, 9922, "pro-peer", "pro", "capacity-model", dto.ChannelSettings{})
			}
			model.InitChannelCache()
			cfg := resolvePressureCoolingConfig(&dto.PressureCoolingOverride{Scope: "groups", CooldownGroups: []string{"pro"}})
			state := &PressureCoolingState{State: "obs", Violations: 3, TotalRequests: 3}

			executePressureCooling(channel, state, cfg, time.Now().Unix(), 300)

			assert.Equal(t, test.wantCooled, model.IsChannelGroupCooled(channel.Id, "pro"))
			stored, err := model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
		})
	}
}

func TestCheckPressureCoolingGroupCoolDoesNotResetEnabledChannel(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.GracePeriodSeconds = 0
	})
	channel := pressureCoolingTestChannel(t, 9931, "check-cool", "pro", "check-model", dto.ChannelSettings{
		PressureCooling: &dto.PressureCoolingOverride{Scope: "groups", CooldownGroups: []string{"pro"}},
	})
	model.InitChannelCache()
	savePressureCoolingState(channel.Id, &PressureCoolingState{
		State: "cool", Scope: "groups", CooledGroups: []string{"pro"},
		Violations: 7, TotalRequests: 9, CooldownUntil: time.Now().Unix() + 300,
	}, 900)
	model.SetChannelGroupCooling(map[int]map[string]struct{}{channel.Id: {"pro": {}}})

	CheckPressureCooling(channel.Id, 10000)

	state := loadPressureCoolingState(channel.Id)
	assert.Equal(t, "cool", state.State)
	assert.Equal(t, "groups", state.Scope)
	assert.Equal(t, int64(7), state.Violations)
	assert.True(t, model.IsChannelGroupCooled(channel.Id, "pro"))
}

func TestPressureCoolingRecoveryGroupsRestoresOverlayWithoutChangingChannelStatus(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.GracePeriodSeconds = 0
	})
	channel := pressureCoolingTestChannel(t, 9941, "recover-groups", "pro,cheap", "recover-model", dto.ChannelSettings{
		PressureCooling: &dto.PressureCoolingOverride{Scope: "groups", CooldownGroups: []string{"pro"}},
	})
	model.InitChannelCache()
	savePressureCoolingState(channel.Id, &PressureCoolingState{
		State: "cool", Scope: "groups", CooledGroups: []string{"pro"},
		CooldownUntil: time.Now().Unix() - 1, Consecutive: 1,
	}, 900)
	model.SetChannelGroupCooling(map[int]map[string]struct{}{channel.Id: {"pro": {}}})

	pressureCoolingRecoveryOnce(time.Now().Unix())

	state := loadPressureCoolingState(channel.Id)
	assert.Equal(t, "obs", state.State)
	assert.Empty(t, state.CooledGroups)
	assert.False(t, model.IsChannelGroupCooled(channel.Id, "pro"))
	assert.False(t, model.IsChannelGroupCooled(channel.Id, "cheap"))
	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	candidates, err := model.GetSatisfiedChannelCandidates("pro", "recover-model", 0, "")
	require.NoError(t, err)
	assert.Len(t, candidates, 1)
}

func TestPressureCoolingRecoveryDisabledGroupConfigDoesNotTouchChannelStatus(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) { setting.Enabled = true })
	channel := pressureCoolingTestChannel(t, 9951, "disabled-config", "pro", "disabled-model", dto.ChannelSettings{
		PressureCooling: &dto.PressureCoolingOverride{Enabled: common.GetPointer(false), Scope: "groups", CooldownGroups: []string{"pro"}},
	})
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "test"))
	savePressureCoolingState(channel.Id, &PressureCoolingState{State: "cool", Scope: "groups", CooledGroups: []string{"pro"}, CooldownUntil: time.Now().Unix() - 1}, 900)
	model.SetChannelGroupCooling(map[int]map[string]struct{}{channel.Id: {"pro": {}}})

	pressureCoolingRecoveryOnce(time.Now().Unix())

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "obs", loadPressureCoolingState(channel.Id).State)
	assert.False(t, model.IsChannelGroupCooled(channel.Id, "pro"))
}

func TestPressureCoolingRecoveryGroupsHonorsCooldownAndSuspendsAtLimit(t *testing.T) {
	tests := []struct {
		name          string
		cooldownUntil int64
		consecutive   int64
		wantState     string
		wantOverlay   bool
	}{
		{name: "cooldown still active", cooldownUntil: time.Now().Unix() + 300, consecutive: 1, wantState: "cool", wantOverlay: true},
		{name: "limit reached after expiry", cooldownUntil: time.Now().Unix() - 1, consecutive: 1, wantState: "susp", wantOverlay: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preparePressureCoolingGroupTest(t)
			configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
				setting.Enabled = true
				setting.MaxConsecutiveCooldowns = 1
			})
			channel := pressureCoolingTestChannel(t, 9961, "recovery-branch", "pro", "recovery-branch-model", dto.ChannelSettings{
				PressureCooling: &dto.PressureCoolingOverride{Scope: "groups", CooldownGroups: []string{"pro"}},
			})
			model.InitChannelCache()
			savePressureCoolingState(channel.Id, &PressureCoolingState{
				State: "cool", Scope: "groups", CooledGroups: []string{"pro"},
				CooldownUntil: test.cooldownUntil, Consecutive: test.consecutive,
			}, 900)
			model.SetChannelGroupCooling(map[int]map[string]struct{}{channel.Id: {"pro": {}}})

			pressureCoolingRecoveryOnce(time.Now().Unix())

			assert.Equal(t, test.wantState, loadPressureCoolingState(channel.Id).State)
			assert.Equal(t, test.wantOverlay, model.IsChannelGroupCooled(channel.Id, "pro"))
			stored, err := model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
		})
	}
}

func TestExecutePressureCoolingGroupsWithNoMatchingConfiguredGroupSkips(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) { setting.Enabled = true })
	channel := pressureCoolingTestChannel(t, 9981, "no-target", "pro", "no-target-model", dto.ChannelSettings{})
	model.InitChannelCache()
	state := &PressureCoolingState{State: "obs", Violations: 3, TotalRequests: 3}
	cfg := resolvePressureCoolingConfig(&dto.PressureCoolingOverride{Scope: "groups", CooldownGroups: []string{"missing"}})

	executePressureCooling(channel, state, cfg, time.Now().Unix(), 900)

	assert.Equal(t, "obs", loadPressureCoolingState(channel.Id).State)
	assert.False(t, model.IsChannelGroupCooled(channel.Id, "pro"))
}

func TestResetPressureCoolingStateRefreshesGroupOverlay(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	savePressureCoolingState(9991, &PressureCoolingState{State: "cool", Scope: "groups", CooledGroups: []string{"pro"}}, 900)
	model.SetChannelGroupCooling(map[int]map[string]struct{}{9991: {"pro": {}}})

	ResetPressureCoolingState(9991)

	assert.False(t, model.IsChannelGroupCooled(9991, "pro"))
	assert.Equal(t, "channel", loadPressureCoolingState(9991).Scope)
}

func TestPressureCoolingRecoveryChannelScopeKeepsExistingStatusPath(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.GracePeriodSeconds = 0
	})
	channel := pressureCoolingTestChannel(t, 9992, "channel-recovery", "pro", "channel-recovery-model", dto.ChannelSettings{})
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "test"))
	savePressureCoolingState(channel.Id, &PressureCoolingState{
		State: "cool", CooldownUntil: time.Now().Unix() - 1, Consecutive: 1,
	}, 900)

	pressureCoolingRecoveryOnce(time.Now().Unix())

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, "obs", loadPressureCoolingState(channel.Id).State)
}

func TestCheckPressureCoolingChannelCoolResetsObservation(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.GracePeriodSeconds = 0
	})
	channel := pressureCoolingTestChannel(t, 9993, "channel-check", "pro", "channel-check-model", dto.ChannelSettings{})
	model.InitChannelCache()
	savePressureCoolingState(channel.Id, &PressureCoolingState{
		State: "cool", Violations: 4, TotalRequests: 5, CooldownUntil: time.Now().Unix() + 300,
	}, 900)

	CheckPressureCooling(channel.Id, 10000)

	state := loadPressureCoolingState(channel.Id)
	assert.Equal(t, "obs", state.State)
	assert.Zero(t, state.Violations)
	assert.Zero(t, state.TotalRequests)
}

func TestCheckPressureCoolingGroupsTriggersAfterObservationWindow(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.FRTThresholdMs = 100
		setting.TriggerPercent = 50
		setting.ObservationWindowSeconds = 60
		setting.CooldownSeconds = 30
		setting.MaxCooldownSeconds = 60
		setting.MinActiveChannelsPerGroup = 1
		setting.GracePeriodSeconds = 0
	})
	channel := pressureCoolingTestChannel(t, 9994, "check-trigger", "pro,cheap", "check-trigger-model", dto.ChannelSettings{
		PressureCooling: &dto.PressureCoolingOverride{Scope: "groups", CooldownGroups: []string{"pro"}},
	})
	pressureCoolingTestChannel(t, 9995, "check-trigger-peer", "pro", "check-trigger-model", dto.ChannelSettings{})
	model.InitChannelCache()

	for i := 0; i < 3; i++ {
		CheckPressureCooling(channel.Id, 200)
	}

	assert.True(t, model.IsChannelGroupCooled(channel.Id, "pro"))
	assert.False(t, model.IsChannelGroupCooled(channel.Id, "cheap"))
	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}

func TestPressureCoolingRecoveryDropsStateForMissingChannel(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) { setting.Enabled = true })
	savePressureCoolingState(9996, &PressureCoolingState{State: "cool", Scope: "groups", CooledGroups: []string{"pro"}}, 900)
	model.SetChannelGroupCooling(map[int]map[string]struct{}{9996: {"pro": {}}})

	pressureCoolingRecoveryOnce(time.Now().Unix())

	assert.False(t, model.IsChannelGroupCooled(9996, "pro"))
	assert.Equal(t, "obs", loadPressureCoolingState(9996).State)
}

func TestFormatCooldownDuration(t *testing.T) {
	tests := []struct {
		seconds int64
		want    string
	}{
		{seconds: 5, want: "5s"},
		{seconds: 65, want: "1m5s"},
		{seconds: 3660, want: "1h1m"},
	}
	for _, test := range tests {
		assert.Equal(t, test.want, formatCooldownDuration(test.seconds))
	}
}

func TestResolvePressureCoolingConfigAppliesScopeAndScalarOverrides(t *testing.T) {
	enabled := false
	frt, trigger, cooldown, window := 123, 77, 19, 11
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.FRTThresholdMs = 800
		setting.TriggerPercent = 50
		setting.CooldownSeconds = 300
		setting.ObservationWindowSeconds = 60
	})
	override := &dto.PressureCoolingOverride{
		Enabled: &enabled, FRTThresholdMs: &frt, TriggerPercent: &trigger,
		CooldownSeconds: &cooldown, ObservationWindowSeconds: &window,
		Scope: "groups", CooldownGroups: []string{"pro"},
	}

	resolved := resolvePressureCoolingConfig(override)

	assert.False(t, resolved.Enabled)
	assert.Equal(t, "groups", resolved.Scope)
	assert.Equal(t, []string{"pro"}, resolved.CooldownGroups)
	assert.Equal(t, frt, resolved.FRTThresholdMs)
	assert.Equal(t, trigger, resolved.TriggerPercent)
	assert.Equal(t, cooldown, resolved.CooldownSeconds)
	assert.Equal(t, window, resolved.ObservationWindowSeconds)
}

func TestPressureCoolingTargetGroupsNormalizesAndDeduplicates(t *testing.T) {
	channel := &model.Channel{Group: "pro,cheap"}
	assert.Nil(t, pressureCoolingTargetGroups(nil, []string{"pro"}))
	assert.Nil(t, pressureCoolingTargetGroups(channel, nil))
	assert.Equal(t, []string{"pro", "cheap"}, pressureCoolingTargetGroups(channel, []string{" pro ", "pro", "missing", "", "cheap"}))
}

func TestCanCoolChannelGroupsRejectsNilAndAllowsEmptyModelSet(t *testing.T) {
	assert.False(t, canCoolChannelGroups(nil, []string{"pro"}, 1))
	channel := &model.Channel{Id: 9997, Group: "pro", Models: ""}
	assert.True(t, canCoolChannelGroups(channel, []string{"pro"}, 1))
}

func TestCanCoolChannelRejectsMissingChannel(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	assert.False(t, canCoolChannel(9998, 1))
}

func TestExecutePressureCoolingChannelMinimumActiveGuard(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.MinActiveChannelsPerGroup = 1
		setting.CooldownSeconds = 100
		setting.MaxCooldownSeconds = 120
		setting.CooldownBackoffMultiplier = 2
	})
	channel := pressureCoolingTestChannel(t, 9999, "channel-minimum", "pro", "channel-minimum-model", dto.ChannelSettings{})
	model.InitChannelCache()
	state := &PressureCoolingState{State: "obs", Violations: 3, TotalRequests: 3, Consecutive: 1}

	executePressureCooling(channel, state, resolvePressureCoolingConfig(nil), time.Now().Unix(), 900)

	assert.Equal(t, "obs", loadPressureCoolingState(channel.Id).State)
	assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
}

func TestCheckPressureCoolingGuardsInvalidDisabledSuspendedAndGracefulStates(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, channel *model.Channel)
		frt   int64
		check func(t *testing.T, channelID int)
	}{
		{
			name: "non-positive frt is ignored",
			frt:  0,
			check: func(t *testing.T, channelID int) {
				assert.Equal(t, "obs", loadPressureCoolingState(channelID).State)
			},
		},
		{
			name: "disabled config is ignored",
			setup: func(t *testing.T, _ *model.Channel) {
				configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) { setting.Enabled = false })
			},
			frt: 100,
			check: func(t *testing.T, channelID int) {
				assert.Equal(t, "obs", loadPressureCoolingState(channelID).State)
			},
		},
		{
			name: "suspended state is sticky",
			setup: func(t *testing.T, channel *model.Channel) {
				savePressureCoolingState(channel.Id, &PressureCoolingState{State: "susp"}, 900)
			},
			frt: 100,
			check: func(t *testing.T, channelID int) {
				assert.Equal(t, "susp", loadPressureCoolingState(channelID).State)
			},
		},
		{
			name: "grace period blocks observation",
			setup: func(t *testing.T, channel *model.Channel) {
				savePressureCoolingState(channel.Id, &PressureCoolingState{State: "obs", GraceUntil: time.Now().Unix() + 300}, 900)
			},
			frt: 100,
			check: func(t *testing.T, channelID int) {
				assert.Equal(t, int64(0), loadPressureCoolingState(channelID).TotalRequests)
			},
		},
		{
			name: "expired observation window resets counters",
			setup: func(t *testing.T, channel *model.Channel) {
				savePressureCoolingState(channel.Id, &PressureCoolingState{State: "obs", WindowStart: time.Now().Unix() - 300, Violations: 4, TotalRequests: 5}, 900)
			},
			frt: 1,
			check: func(t *testing.T, channelID int) {
				state := loadPressureCoolingState(channelID)
				assert.Equal(t, int64(1), state.TotalRequests)
				assert.Zero(t, state.Violations)
			},
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preparePressureCoolingGroupTest(t)
			configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
				setting.Enabled = true
				setting.ObservationWindowSeconds = 60
				setting.FRTThresholdMs = 100
			})
			channel := pressureCoolingTestChannel(t, 10000+index, "check-guard", "pro", "check-guard-model", dto.ChannelSettings{})
			model.InitChannelCache()
			if test.setup != nil {
				test.setup(t, channel)
			}
			CheckPressureCooling(channel.Id, test.frt)
			test.check(t, channel.Id)
		})
	}
}

func TestPressureCoolingRecoveryChannelBranchesRemainUnchanged(t *testing.T) {
	tests := []struct {
		name        string
		globalOn    bool
		status      int
		untilOffset int64
		consecutive int64
		wantState   string
		wantStatus  int
	}{
		{
			name:        "cooldown not expired",
			globalOn:    true,
			status:      common.ChannelStatusAutoDisabled,
			untilOffset: 300,
			consecutive: 1,
			wantState:   "cool",
			wantStatus:  common.ChannelStatusAutoDisabled,
		},
		{
			name:        "enabled channel returns to observation",
			globalOn:    true,
			status:      common.ChannelStatusEnabled,
			untilOffset: -1,
			consecutive: 1,
			wantState:   "obs",
			wantStatus:  common.ChannelStatusEnabled,
		},
		{
			name:        "expired channel recovers",
			globalOn:    true,
			status:      common.ChannelStatusAutoDisabled,
			untilOffset: -1,
			consecutive: 1,
			wantState:   "obs",
			wantStatus:  common.ChannelStatusEnabled,
		},
		{
			name:        "expired channel suspends at limit",
			globalOn:    true,
			status:      common.ChannelStatusAutoDisabled,
			untilOffset: -1,
			consecutive: 5,
			wantState:   "susp",
			wantStatus:  common.ChannelStatusAutoDisabled,
		},
		{
			name:        "disabled global config recovers channel",
			globalOn:    false,
			status:      common.ChannelStatusAutoDisabled,
			untilOffset: -1,
			consecutive: 1,
			wantState:   "obs",
			wantStatus:  common.ChannelStatusEnabled,
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preparePressureCoolingGroupTest(t)
			configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
				setting.Enabled = test.globalOn
				setting.MaxConsecutiveCooldowns = 5
				setting.GracePeriodSeconds = 0
			})
			channel := pressureCoolingTestChannel(t, 10100+index, "channel-branch", "pro", "channel-branch-model", dto.ChannelSettings{})
			model.InitChannelCache()
			if test.status != common.ChannelStatusEnabled {
				require.True(t, model.UpdateChannelStatus(channel.Id, "", test.status, "test"))
			}
			savePressureCoolingState(channel.Id, &PressureCoolingState{
				State: "cool", CooldownUntil: time.Now().Unix() + test.untilOffset, Consecutive: test.consecutive,
				TotalRequests: 4,
			}, 900)

			pressureCoolingRecoveryOnce(time.Now().Unix())

			assert.Equal(t, test.wantState, loadPressureCoolingState(channel.Id).State)
			stored, err := model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			assert.Equal(t, test.wantStatus, stored.Status)
		})
	}
}

func TestPressureCoolingStateScopeFieldsRoundTripAndLegacyDefault(t *testing.T) {
	state := &PressureCoolingState{State: "cool", Scope: "groups", CooledGroups: []string{"pro", "cheap"}}
	fields := pressureCoolingStateFields(state)
	assert.Equal(t, "g", fields["sc"])
	assert.Equal(t, "pro,cheap", fields["cg"])
	decoded := pressureCoolingStateFromFields(fields)
	assert.Equal(t, "groups", decoded.Scope)
	assert.Equal(t, []string{"pro", "cheap"}, decoded.CooledGroups)

	legacy := pressureCoolingStateFromFields(map[string]string{"st": "cool"})
	assert.Equal(t, "channel", legacy.Scope)
	assert.Empty(t, legacy.CooledGroups)
}

func TestPressureCoolingStateRedisRoundTrip(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousRedisEnabled, previousRedis := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedis
		pressureCoolingMemStore.Delete(9971)
	})

	savePressureCoolingState(9971, &PressureCoolingState{
		State: "cool", Scope: "groups", CooledGroups: []string{"pro", "cheap"},
		Violations: 2, TotalRequests: 4, WindowStart: 11, CooldownUntil: 22,
		Consecutive: 3, GraceUntil: 33,
	}, 60)

	loaded := loadPressureCoolingState(9971)
	require.Equal(t, "groups", loaded.Scope)
	require.Equal(t, []string{"pro", "cheap"}, loaded.CooledGroups)
	assert.Equal(t, int64(2), loaded.Violations)
	assert.Equal(t, int64(4), loaded.TotalRequests)
	assert.Equal(t, int64(22), loaded.CooldownUntil)
	listed := listCoolingChannelStates()
	require.Contains(t, listed, 9971)
	assert.Equal(t, []string{"pro", "cheap"}, listed[9971].CooledGroups)

	server.FastForward(time.Minute)
	assert.Empty(t, listCoolingChannelStates())
}
