package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyPressureCoolingAttempt(t *testing.T) {
	tests := []struct {
		name      string
		err       *types.NewAPIError
		wantCount bool
		wantError bool
	}{
		{name: "success", err: nil, wantCount: true, wantError: false},
		{name: "skip retry 403", err: types.NewErrorWithStatusCode(errors.New("local"), types.ErrorCodeBadResponse, http.StatusForbidden, types.ErrOptionWithSkipRetry()), wantCount: false, wantError: false},
		{name: "skip retry 500", err: types.NewErrorWithStatusCode(errors.New("local"), types.ErrorCodeBadResponse, http.StatusInternalServerError, types.ErrOptionWithSkipRetry()), wantCount: false, wantError: false},
		{name: "403", err: types.NewErrorWithStatusCode(errors.New("upstream"), types.ErrorCodeBadResponse, http.StatusForbidden), wantCount: true, wantError: true},
		{name: "429", err: types.NewErrorWithStatusCode(errors.New("upstream"), types.ErrorCodeBadResponse, http.StatusTooManyRequests), wantCount: true, wantError: true},
		{name: "500", err: types.NewErrorWithStatusCode(errors.New("upstream"), types.ErrorCodeBadResponse, 500), wantCount: true, wantError: true},
		{name: "503", err: types.NewErrorWithStatusCode(errors.New("upstream"), types.ErrorCodeBadResponse, 503), wantCount: true, wantError: true},
		{name: "400", err: types.NewErrorWithStatusCode(errors.New("client"), types.ErrorCodeBadResponse, http.StatusBadRequest), wantCount: true, wantError: false},
		{name: "404", err: types.NewErrorWithStatusCode(errors.New("missing"), types.ErrorCodeBadResponse, http.StatusNotFound), wantCount: true, wantError: false},
		{name: "413", err: types.NewErrorWithStatusCode(errors.New("large"), types.ErrorCodeBadResponse, http.StatusRequestEntityTooLarge), wantCount: true, wantError: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			count, upstream := classifyPressureCoolingAttempt(test.err)
			assert.Equal(t, test.wantCount, count)
			assert.Equal(t, test.wantError, upstream)
		})
	}
}

func TestResolvePressureCoolingUpstreamErrorConfig(t *testing.T) {
	setting := config.GlobalConfig.Get("pressure_cooling").(*operation_setting.PressureCoolingSetting)
	original := *setting
	setting.Enabled = true
	setting.UpstreamErrorEnabled = true
	setting.UpstreamErrorTriggerPercent = 41
	setting.UpstreamErrorMinSamples = 12
	setting.ConditionMode = "ALL"
	t.Cleanup(func() { *setting = original })

	cfg := resolvePressureCoolingConfig(nil)
	assert.True(t, cfg.UpstreamErrorEnabled)
	assert.Equal(t, 41, cfg.UpstreamErrorTriggerPercent)
	assert.Equal(t, 12, cfg.UpstreamErrorMinSamples)
	assert.Equal(t, "all", cfg.ConditionMode)

	enabled := false
	percent := 75
	minSamples := 20
	cfg = resolvePressureCoolingConfig(&dto.PressureCoolingOverride{
		UpstreamErrorEnabled:        &enabled,
		UpstreamErrorTriggerPercent: &percent,
		UpstreamErrorMinSamples:     &minSamples,
		ConditionMode:               "ALL",
	})
	assert.False(t, cfg.UpstreamErrorEnabled)
	assert.Equal(t, 75, cfg.UpstreamErrorTriggerPercent)
	assert.Equal(t, 20, cfg.UpstreamErrorMinSamples)
	assert.Equal(t, "all", cfg.ConditionMode)
}

func TestPressureCoolingConditionModesAndGates(t *testing.T) {
	base := resolvedPressureCoolingConfig{UpstreamErrorEnabled: true, TriggerPercent: 50, UpstreamErrorMinSamples: 3, ConditionMode: "any"}
	assert.True(t, pressureCoolingConditionsMet(base, false, true))
	assert.True(t, pressureCoolingConditionsMet(base, true, false))
	assert.False(t, pressureCoolingConditionsMet(base, false, false))

	base.ConditionMode = "all"
	assert.False(t, pressureCoolingConditionsMet(base, false, true))
	assert.False(t, pressureCoolingConditionsMet(base, true, false))
	assert.True(t, pressureCoolingConditionsMet(base, true, true))

	base.UpstreamErrorEnabled = false
	assert.True(t, pressureCoolingConditionsMet(base, true, false), "disabled error condition preserves the FRT-only path")
	assert.False(t, pressureCoolingConditionsMet(base, false, true))
}

func TestPressureCoolingErrorGateRequiresSamplesAndThreshold(t *testing.T) {
	cfg := resolvedPressureCoolingConfig{UpstreamErrorEnabled: true, UpstreamErrorMinSamples: 10, UpstreamErrorTriggerPercent: 50}
	assert.False(t, pressureCoolingErrorConditionMet(cfg, 9, 9))
	assert.False(t, pressureCoolingErrorConditionMet(cfg, 10, 9))
	assert.True(t, pressureCoolingErrorConditionMet(cfg, 10, 10))
}

func TestPressureCoolingErrorStateGates(t *testing.T) {
	cfg := resolvedPressureCoolingConfig{Enabled: true, UpstreamErrorEnabled: true, ConditionMode: "any", ObservationWindowSeconds: 60, TriggerPercent: 50}
	now := time.Now().Unix()
	assert.False(t, pressureCoolingErrorStateEligible(&PressureCoolingState{State: "cool"}, cfg, now))
	assert.False(t, pressureCoolingErrorStateEligible(&PressureCoolingState{State: "susp"}, cfg, now))
	assert.False(t, pressureCoolingErrorStateEligible(&PressureCoolingState{State: "obs", GraceUntil: now + 1}, cfg, now))
	assert.True(t, pressureCoolingErrorStateEligible(&PressureCoolingState{State: "obs", WindowStart: now - 61}, cfg, now), "ANY mode does not require fresh FRT evidence")
	cfg.ConditionMode = "all"
	assert.False(t, pressureCoolingErrorStateEligible(&PressureCoolingState{State: "obs", WindowStart: now - 61}, cfg, now), "ALL mode must reject stale FRT evidence")
	assert.True(t, pressureCoolingErrorStateEligible(&PressureCoolingState{State: "obs", WindowStart: now - 60}, cfg, now))
	assert.False(t, pressureCoolingErrorStateEligible(&PressureCoolingState{State: "obs", WindowStart: 0}, cfg, now))

	require.Equal(t, "all", cfg.ConditionMode)
}

func preparePressureCoolingErrorTest(t *testing.T) (*model.Channel, resolvedPressureCoolingConfig, int64) {
	t.Helper()
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = true
		setting.FRTThresholdMs = 100
		setting.TriggerPercent = 50
		setting.UpstreamErrorEnabled = true
		setting.UpstreamErrorTriggerPercent = 50
		setting.UpstreamErrorMinSamples = 3
		setting.ObservationWindowSeconds = 60
		setting.MinActiveChannelsPerGroup = 0
		setting.CooldownSeconds = 30
		setting.MaxCooldownSeconds = 60
		setting.GracePeriodSeconds = 0
	})
	channel := pressureCoolingTestChannel(t, 8050, "upstream-errors", "pro", "error-model", dto.ChannelSettings{})
	model.InitChannelCache()
	now := time.Now().Unix()
	return channel, resolvePressureCoolingConfig(nil), now
}

func TestRecordPressureCoolingAttemptRequiresMinimumSamplesThenTriggers(t *testing.T) {
	channel, cfg, now := preparePressureCoolingErrorTest(t)
	savePressureCoolingState(channel.Id, &PressureCoolingState{State: "obs", WindowStart: now}, 600)

	recordPressureCoolingAttemptAt(channel, cfg, true, now)
	recordPressureCoolingAttemptAt(channel, cfg, true, now)
	assert.Equal(t, "obs", loadPressureCoolingState(channel.Id).State)
	recordPressureCoolingAttemptAt(channel, cfg, true, now)
	assert.Equal(t, "cool", loadPressureCoolingState(channel.Id).State)
}

func TestRecordPressureCoolingAttemptAnyAndAllModes(t *testing.T) {
	channel, cfg, now := preparePressureCoolingErrorTest(t)
	savePressureCoolingState(channel.Id, &PressureCoolingState{State: "obs", WindowStart: now, TotalRequests: 3}, 600)
	now = time.Now().Unix()
	for i := 0; i < 3; i++ {
		recordPressureCoolingAttemptAt(channel, cfg, true, now)
	}
	assert.Equal(t, "cool", loadPressureCoolingState(channel.Id).State, "ANY triggers on error condition alone")

	channel = pressureCoolingTestChannel(t, 8051, "upstream-errors-all", "pro", "error-model", dto.ChannelSettings{})
	model.InitChannelCache()
	now = time.Now().Unix()
	cfg.ConditionMode = "all"
	savePressureCoolingState(channel.Id, &PressureCoolingState{State: "obs", WindowStart: now, TotalRequests: 3, Violations: 0}, 600)
	for i := 0; i < 3; i++ {
		recordPressureCoolingAttemptAt(channel, cfg, true, now)
	}
	assert.Equal(t, "obs", loadPressureCoolingState(channel.Id).State, "ALL requires FRT too")
	savePressureCoolingState(channel.Id, &PressureCoolingState{State: "obs", WindowStart: now, TotalRequests: 3, Violations: 3}, 600)
	recordPressureCoolingAttemptAt(channel, cfg, true, now)
	assert.Equal(t, "cool", loadPressureCoolingState(channel.Id).State, "ALL triggers when both conditions are met")
}

func TestRecordPressureCoolingAttemptDoesNotMigrateIneligibleState(t *testing.T) {
	channel, cfg, now := preparePressureCoolingErrorTest(t)
	for _, state := range []*PressureCoolingState{
		{State: "cool", WindowStart: now},
		{State: "susp", WindowStart: now},
		{State: "obs", WindowStart: now, GraceUntil: now + 60},
	} {
		savePressureCoolingState(channel.Id, state, 600)
		recordPressureCoolingAttemptAt(channel, cfg, true, now)
		stored := loadPressureCoolingState(channel.Id)
		assert.Equal(t, state.State, stored.State)
		assert.Equal(t, state.GraceUntil, stored.GraceUntil)
	}

	cfg.ConditionMode = "all"
	stale := &PressureCoolingState{State: "obs", WindowStart: now - 61, TotalRequests: 3, Violations: 3}
	savePressureCoolingState(channel.Id, stale, 600)
	recordPressureCoolingAttemptAt(channel, cfg, true, now)
	assert.Equal(t, "obs", loadPressureCoolingState(channel.Id).State, "stale FRT evidence cannot satisfy ALL")
}

func TestRecordPressureCoolingAttemptAllRequiresFreshFRTWindow(t *testing.T) {
	tests := []struct {
		name        string
		windowStart func(now int64, observationWindow int) int64
		wantState   string
		wantStatus  int
	}{
		{
			name:        "stale FRT evidence does not cool channel",
			windowStart: func(now int64, observationWindow int) int64 { return now - int64(observationWindow) - 1 },
			wantState:   "obs",
			wantStatus:  common.ChannelStatusEnabled,
		},
		{
			name:        "fresh FRT evidence cools channel",
			windowStart: func(now int64, _ int) int64 { return now },
			wantState:   "cool",
			wantStatus:  common.ChannelStatusAutoDisabled,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel, cfg, now := preparePressureCoolingErrorTest(t)
			cfg.ConditionMode = "all"
			state := &PressureCoolingState{
				State: "obs", Scope: "channel", WindowStart: test.windowStart(now, cfg.ObservationWindowSeconds),
				TotalRequests: 3, Violations: 3,
			}
			savePressureCoolingState(channel.Id, state, 600)
			for i := 0; i < cfg.UpstreamErrorMinSamples-1; i++ {
				incrPressureCoolingErrorWindowAt(channel.Id, cfg.ObservationWindowSeconds, true, now)
			}

			recordPressureCoolingAttemptAt(channel, cfg, true, now)

			storedState := loadPressureCoolingState(channel.Id)
			assert.Equal(t, test.wantState, storedState.State)
			if test.wantState == "obs" {
				assert.Equal(t, state, storedState)
			}
			storedChannel, err := model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			assert.Equal(t, test.wantStatus, storedChannel.Status)
		})
	}
}

func TestPressureCoolingReasonPreservesFRTOnlyTextAndAddsErrorText(t *testing.T) {
	state := &PressureCoolingState{Violations: 3, TotalRequests: 4}
	cfg := resolvedPressureCoolingConfig{FRTThresholdMs: 8000}
	assert.Equal(t,
		"压力冷却：观察期内 3/4 请求 FRT 超 8000ms（75%），冷却 30s",
		formatPressureCoolingReason(state, cfg, 30, nil),
	)
	assert.Equal(t,
		"压力冷却：观察期内 5/10 次上游报错（50%），冷却 30s",
		formatPressureCoolingReason(state, cfg, 30, &pressureCoolingReason{errorMet: true, errorAttempts: 10, errorCount: 5}),
	)
	combined := formatPressureCoolingReason(state, cfg, 30, &pressureCoolingReason{
		frtMet: true, errorMet: true, errorAttempts: 10, errorCount: 5,
	})
	assert.Contains(t, combined, "FRT")
	assert.Contains(t, combined, "上游报错")
}

func TestCheckPressureCoolingUpstreamErrorAnyAllIntegration(t *testing.T) {
	channel, cfg, now := preparePressureCoolingErrorTest(t)
	cfg.ConditionMode = "any"
	cfg.FRTThresholdMs = 100
	for i := 0; i < 3; i++ {
		CheckPressureCooling(channel.Id, 200)
	}
	assert.Equal(t, "cool", loadPressureCoolingState(channel.Id).State, "ANY preserves FRT-only triggering")

	allChannel := pressureCoolingTestChannel(t, 8052, "upstream-errors-all-check", "pro", "error-model-all", dto.ChannelSettings{
		PressureCooling: &dto.PressureCoolingOverride{ConditionMode: "all"},
	})
	model.InitChannelCache()
	cfg.ConditionMode = "all"
	cfg.FRTThresholdMs = 100
	CheckPressureCooling(allChannel.Id, 1)
	assert.Equal(t, "obs", loadPressureCoolingState(allChannel.Id).State, "ALL does not read or trigger error condition before FRT is met")
	now = time.Now().Unix()
	for i := 0; i < 3; i++ {
		incrPressureCoolingErrorWindowAt(allChannel.Id, cfg.ObservationWindowSeconds, true, now)
	}
	for i := 0; i < 1; i++ {
		CheckPressureCooling(allChannel.Id, 200)
	}
	assert.Equal(t, "obs", loadPressureCoolingState(allChannel.Id).State)
	CheckPressureCooling(allChannel.Id, 200)
	assert.Equal(t, "cool", loadPressureCoolingState(allChannel.Id).State, "ALL triggers after both FRT and error evidence are met")
}

func TestRecordPressureCoolingAttemptFastGates(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	configurePressureCooling(t, func(setting *operation_setting.PressureCoolingSetting) {
		setting.Enabled = false
		setting.UpstreamErrorEnabled = true
	})
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	RecordPressureCoolingAttempt(8090, nil)
	common.MemoryCacheEnabled = true
	RecordPressureCoolingAttempt(8090, nil)

	setting := config.GlobalConfig.Get("pressure_cooling").(*operation_setting.PressureCoolingSetting)
	setting.Enabled = true
	RecordPressureCoolingAttempt(8090, nil)
	channel := pressureCoolingTestChannel(t, 8091, "fast-gates", "pro", "fast-gates-model", dto.ChannelSettings{})
	model.InitChannelCache()
	setting.UpstreamErrorEnabled = false
	RecordPressureCoolingAttempt(channel.Id, nil)
	setting.UpstreamErrorEnabled = true
	skipErr := types.NewErrorWithStatusCode(errors.New("local"), types.ErrorCodeBadResponse, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	RecordPressureCoolingAttempt(channel.Id, skipErr)
	common.MemoryCacheEnabled = oldMemoryCache
}

func TestPressureCoolingStateTTLUsesObservationWindowLowerBound(t *testing.T) {
	assert.Equal(t, 30, pressureCoolingStateTTL(resolvedPressureCoolingConfig{MaxCooldownSeconds: 1, ObservationWindowSeconds: 10}))
	assert.Equal(t, 60, pressureCoolingStateTTL(resolvedPressureCoolingConfig{MaxCooldownSeconds: 20, ObservationWindowSeconds: 10}))
}
