package operation_setting

import (
	"github.com/QuantumNous/new-api/setting/config"
)

type PressureCoolingSetting struct {
	Enabled                      bool    `json:"enabled"`
	ObservationWindowSeconds     int     `json:"observation_window_seconds"`
	FRTThresholdMs               int     `json:"frt_threshold_ms"`
	TriggerPercent               int     `json:"trigger_percent"`
	UpstreamErrorTriggerPercent  int     `json:"upstream_error_trigger_percent"`
	UpstreamErrorMinSamples      int     `json:"upstream_error_min_samples"`
	UpstreamErrorTriggerCount    int     `json:"upstream_error_trigger_count"`
	ConditionMode                string  `json:"condition_mode"`
	CooldownSeconds              int     `json:"cooldown_seconds"`
	MaxConsecutiveCooldowns      int     `json:"max_consecutive_cooldowns"`
	CooldownBackoffMultiplier    float64 `json:"cooldown_backoff_multiplier"`
	MaxCooldownSeconds           int     `json:"max_cooldown_seconds"`
	GracePeriodSeconds           int     `json:"grace_period_seconds"`
	MinActiveChannelsPerGroup    int     `json:"min_active_channels_per_group"`
	RecoveryCheckIntervalSeconds int     `json:"recovery_check_interval_seconds"`
}

var pressureCoolingSetting = PressureCoolingSetting{
	Enabled:                      false,
	ObservationWindowSeconds:     60,
	FRTThresholdMs:               8000,
	TriggerPercent:               50,
	UpstreamErrorTriggerPercent:  0,
	UpstreamErrorMinSamples:      10,
	UpstreamErrorTriggerCount:    0,
	ConditionMode:                "any",
	CooldownSeconds:              300,
	MaxConsecutiveCooldowns:      5,
	CooldownBackoffMultiplier:    1.5,
	MaxCooldownSeconds:           3600,
	GracePeriodSeconds:           30,
	MinActiveChannelsPerGroup:    1,
	RecoveryCheckIntervalSeconds: 30,
}

func init() {
	config.GlobalConfig.Register("pressure_cooling", &pressureCoolingSetting)
}

func GetPressureCoolingSetting() PressureCoolingSetting {
	return pressureCoolingSetting
}
