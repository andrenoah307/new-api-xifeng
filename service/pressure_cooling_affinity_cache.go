package service

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	realtimemetrics "github.com/QuantumNous/new-api/pkg/realtime_metrics"
	"github.com/shopspring/decimal"
)

func checkChannelAffinityCacheCooling(now int64) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	channels, err := model.PressureCoolingChannels()
	if err != nil {
		common.SysError("affinity cache cooling: channel snapshot failed: " + err.Error())
		return
	}
	for _, ch := range channels {
		cfg := resolvePressureCoolingConfig(ch.GetSetting().PressureCooling)
		if !cfg.Enabled || cfg.AffinityCacheMinPercent <= 0 {
			continue
		}
		state, err := loadPressureCoolingStateResult(ch.Id)
		if err != nil || state == nil || state.State != "obs" || now < state.GraceUntil {
			continue
		}
		usage, err := realtimemetrics.ReadChannelAffinityCacheUsage(context.Background(), ch.Id, cfg.AffinityCacheWindowHours, state.GraceUntil, now)
		if err != nil {
			common.SysError("affinity cache cooling: window read failed: " + err.Error())
			continue
		}
		reason := affinityCacheCoolingReason(cfg, state, usage, now)
		if reason == nil {
			continue
		}
		if normalizePressureCoolingConditionMode(cfg.ConditionMode) == "all" && pressureCoolingErrorConditionConfigured(cfg) {
			reason.errorAttempts, reason.errorCount = loadPressureCoolingErrorWindow(ch.Id, cfg.ObservationWindowSeconds)
			reason.errorMet = pressureCoolingErrorConditionMet(cfg, reason.errorAttempts, reason.errorCount)
			if !reason.errorMet {
				continue
			}
		}
		executePressureCooling(ch, state, cfg, now, pressureCoolingStateTTL(cfg), reason)
	}
}

func affinityCacheCoolingReason(cfg resolvedPressureCoolingConfig, state *PressureCoolingState, usage realtimemetrics.ChannelAffinityCacheUsage, now int64) *pressureCoolingReason {
	if cfg.AffinityCacheMinPercent <= 0 || usage.PromptTokens <= 0 || usage.Samples < int64(cfg.AffinityCacheMinSamples) {
		return nil
	}
	// Compare exact token totals: floating-point rounding must not turn a rate
	// exactly at the configured threshold into a cooldown violation.
	if decimal.NewFromInt(usage.CachedTokens).Mul(decimal.NewFromInt(100)).GreaterThanOrEqual(
		decimal.NewFromInt(usage.PromptTokens).Mul(decimal.NewFromInt(int64(cfg.AffinityCacheMinPercent)))) {
		return nil
	}
	rate := float64(usage.CachedTokens) / float64(usage.PromptTokens) * 100
	frtMet := state.WindowStart > 0 && now-state.WindowStart <= int64(cfg.ObservationWindowSeconds) &&
		state.TotalRequests >= 3 && state.Violations*100/state.TotalRequests >= int64(cfg.TriggerPercent)
	if normalizePressureCoolingConditionMode(cfg.ConditionMode) == "all" && !frtMet {
		return nil
	}
	return &pressureCoolingReason{cacheMet: true, cachePercent: rate, cacheSamples: usage.Samples, frtMet: frtMet}
}
