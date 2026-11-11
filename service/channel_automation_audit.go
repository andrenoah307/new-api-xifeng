package service

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var channelAuditCredential = regexp.MustCompile(`(?i)(?:bearer\s+\S+|sk-[a-z0-9_-]+|(?:api[_-]?key|authorization|token|secret|password)\s*[=:]\s*[^\s,;]+)`)
var channelAuditFailureLimiter common.InMemoryRateLimiter
var channelAuditLimiterOnce sync.Once

func channelAuditReason(reason, key string) string {
	if key != "" {
		reason = strings.ReplaceAll(reason, key, "[redacted]")
	}
	reason = channelAuditCredential.ReplaceAllString(reason, "[redacted]")
	reason = strings.Join(strings.Fields(reason), " ")
	runes := []rune(reason)
	if len(runes) > 800 {
		reason = string(runes[:800]) + "..."
	}
	return reason
}

func pressureCoolingAuditDetails(state *PressureCoolingState, cfg resolvedPressureCoolingConfig) map[string]any {
	return map[string]any{
		"scope": state.Scope, "state": state.State, "groups": append([]string(nil), state.CooledGroups...),
		"cooldown_until": state.CooldownUntil, "consecutive": state.Consecutive,
		"max_consecutive": cfg.MaxConsecutiveCooldowns, "grace_until": state.GraceUntil,
		"observation_window_seconds": cfg.ObservationWindowSeconds, "frt_threshold_ms": cfg.FRTThresholdMs,
		"trigger_percent": cfg.TriggerPercent, "condition_mode": cfg.ConditionMode,
		"affinity_cache_min_percent":     cfg.AffinityCacheMinPercent,
		"affinity_cache_window_hours":    cfg.AffinityCacheWindowHours,
		"affinity_cache_min_samples":     cfg.AffinityCacheMinSamples,
		"upstream_error_trigger_percent": cfg.UpstreamErrorTriggerPercent,
		"upstream_error_min_samples":     cfg.UpstreamErrorMinSamples, "upstream_error_trigger_count": cfg.UpstreamErrorTriggerCount,
	}
}

func recordPressureCoolingAudit(id int, event, reason string, state *PressureCoolingState, cfg resolvedPressureCoolingConfig) {
	if event == "recovery_failed" {
		channelAuditLimiterOnce.Do(func() { channelAuditFailureLimiter.Init(10 * time.Minute) })
		if !channelAuditFailureLimiter.Request(fmt.Sprintf("%d:%s", id, reason), 1, 300) {
			return
		}
	}
	model.RecordChannelAutomationLog(id, "channel.pressure_cooling."+event, reason, pressureCoolingAuditDetails(state, cfg))
}
