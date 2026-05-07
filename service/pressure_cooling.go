package service

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type resolvedPressureCoolingConfig struct {
	Enabled                  bool
	ObservationWindowSeconds int
	FRTThresholdMs           int
	TriggerPercent           int
	CooldownSeconds          int
	MaxConsecutiveCooldowns  int
	CooldownBackoffMultiplier float64
	MaxCooldownSeconds       int
	GracePeriodSeconds       int
	MinActiveChannelsPerGroup int
}

func resolvePressureCoolingConfig(override *dto.PressureCoolingOverride) resolvedPressureCoolingConfig {
	g := operation_setting.GetPressureCoolingSetting()
	r := resolvedPressureCoolingConfig{
		Enabled:                   g.Enabled,
		ObservationWindowSeconds:  g.ObservationWindowSeconds,
		FRTThresholdMs:            g.FRTThresholdMs,
		TriggerPercent:            g.TriggerPercent,
		CooldownSeconds:           g.CooldownSeconds,
		MaxConsecutiveCooldowns:   g.MaxConsecutiveCooldowns,
		CooldownBackoffMultiplier: g.CooldownBackoffMultiplier,
		MaxCooldownSeconds:        g.MaxCooldownSeconds,
		GracePeriodSeconds:        g.GracePeriodSeconds,
		MinActiveChannelsPerGroup: g.MinActiveChannelsPerGroup,
	}
	if override == nil {
		return r
	}
	if override.Enabled != nil {
		r.Enabled = *override.Enabled
	}
	if override.FRTThresholdMs != nil {
		r.FRTThresholdMs = *override.FRTThresholdMs
	}
	if override.TriggerPercent != nil {
		r.TriggerPercent = *override.TriggerPercent
	}
	if override.CooldownSeconds != nil {
		r.CooldownSeconds = *override.CooldownSeconds
	}
	if override.ObservationWindowSeconds != nil {
		r.ObservationWindowSeconds = *override.ObservationWindowSeconds
	}
	return r
}

func CheckPressureCooling(channelId int, frtMs int64) {
	ch, err := model.CacheGetChannel(channelId)
	if err != nil || ch == nil || frtMs <= 0 {
		return
	}
	setting := ch.GetSetting()
	cfg := resolvePressureCoolingConfig(setting.PressureCooling)
	if !cfg.Enabled {
		return
	}

	state := loadPressureCoolingState(channelId)
	now := time.Now().Unix()
	stateTTL := cfg.MaxCooldownSeconds * 3
	if stateTTL < cfg.ObservationWindowSeconds*3 {
		stateTTL = cfg.ObservationWindowSeconds * 3
	}

	switch state.State {
	case "cool":
		if ch.Status == common.ChannelStatusEnabled {
			state.State = "obs"
			state.GraceUntil = now + int64(cfg.GracePeriodSeconds)
			state.Violations = 0
			state.TotalRequests = 0
			state.WindowStart = now
			savePressureCoolingState(channelId, state, stateTTL)
		}
		return
	case "susp":
		return
	}

	if state.WindowStart == 0 {
		state.WindowStart = now
	}

	if now < state.GraceUntil {
		return
	}

	if now-state.WindowStart > int64(cfg.ObservationWindowSeconds) {
		state.Violations = 0
		state.TotalRequests = 0
		state.WindowStart = now
	}

	state.TotalRequests++
	if frtMs >= int64(cfg.FRTThresholdMs) {
		state.Violations++
	}

	violationPct := state.Violations * 100 / state.TotalRequests
	if violationPct >= int64(cfg.TriggerPercent) && state.TotalRequests >= 3 {
		executePressureCooling(ch, state, cfg, now, stateTTL)
	} else {
		savePressureCoolingState(channelId, state, stateTTL)
	}
}

func executePressureCooling(ch *model.Channel, state *PressureCoolingState, cfg resolvedPressureCoolingConfig, now int64, stateTTL int) {
	if !canCoolChannel(ch.Id, cfg.MinActiveChannelsPerGroup) {
		common.SysLog(fmt.Sprintf("pressure cooling: skip channel #%d (%s) — would leave (group, model) below minimum active", ch.Id, ch.Name))
		state.Violations = 0
		state.WindowStart = now
		savePressureCoolingState(ch.Id, state, stateTTL)
		return
	}

	effectiveCooldown := float64(cfg.CooldownSeconds)
	for i := int64(0); i < state.Consecutive; i++ {
		effectiveCooldown *= cfg.CooldownBackoffMultiplier
	}
	if effectiveCooldown > float64(cfg.MaxCooldownSeconds) {
		effectiveCooldown = float64(cfg.MaxCooldownSeconds)
	}
	cooldownSec := int64(math.Ceil(effectiveCooldown))

	pct := int64(0)
	if state.TotalRequests > 0 {
		pct = state.Violations * 100 / state.TotalRequests
	}
	reason := fmt.Sprintf("压力冷却：观察期内 %d/%d 请求 FRT 超 %dms（%d%%），冷却 %ds",
		state.Violations, state.TotalRequests, cfg.FRTThresholdMs, pct, cooldownSec)
	model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusAutoDisabled, reason)

	state.State = "cool"
	state.CooldownUntil = now + cooldownSec
	state.Consecutive++
	state.Violations = 0
	savePressureCoolingState(ch.Id, state, stateTTL)

	subject := fmt.Sprintf("渠道「%s」(#%d) 因高延迟已自动冷却", ch.Name, ch.Id)
	content := fmt.Sprintf("渠道「%s」(#%d) %s\n冷却将于 %s 后自动恢复（第 %d 次连续冷却）",
		ch.Name, ch.Id, reason, formatCooldownDuration(cooldownSec), state.Consecutive)
	NotifyRootUser(fmt.Sprintf("pressure_cooling_%d", ch.Id), subject, content)
}

func canCoolChannel(channelId int, minActive int) bool {
	ch, err := model.CacheGetChannel(channelId)
	if err != nil || ch == nil {
		return false
	}
	groups := ch.GetGroups()
	models := ch.GetModels()
	for _, group := range groups {
		for _, modelName := range models {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			if model.CountEnabledChannelsForGroupModel(group, modelName) <= minActive {
				return false
			}
		}
	}
	return true
}

func ResetPressureCoolingState(channelId int) {
	deletePressureCoolingState(channelId)
}

func StartPressureCoolingRecovery() {
	if !common.IsMasterNode {
		return
	}
	go pressureCoolingRecoveryLoop()
}

func pressureCoolingRecoveryLoop() {
	for {
		globalCfg := operation_setting.GetPressureCoolingSetting()
		interval := globalCfg.RecoveryCheckIntervalSeconds
		if interval <= 0 {
			interval = 30
		}
		time.Sleep(time.Duration(interval) * time.Second)

		states := listCoolingChannelStates()
		now := time.Now().Unix()

		for channelId, state := range states {
			if state.State != "cool" {
				continue
			}

			ch, err := model.CacheGetChannel(channelId)
			if err != nil || ch == nil {
				deletePressureCoolingState(channelId)
				continue
			}

			chSetting := ch.GetSetting()
			cfg := resolvePressureCoolingConfig(chSetting.PressureCooling)
			stateTTL := cfg.MaxCooldownSeconds * 3
			if stateTTL < cfg.ObservationWindowSeconds*3 {
				stateTTL = cfg.ObservationWindowSeconds * 3
			}

			if !cfg.Enabled {
				model.UpdateChannelStatus(channelId, "", common.ChannelStatusEnabled, "压力冷却已禁用，自动恢复")
				deletePressureCoolingState(channelId)
				continue
			}

			if ch.Status == common.ChannelStatusEnabled {
				state.State = "obs"
				state.GraceUntil = now + int64(cfg.GracePeriodSeconds)
				state.Violations = 0
				state.WindowStart = now
				savePressureCoolingState(channelId, state, stateTTL)
				continue
			}

			if now < state.CooldownUntil {
				continue
			}

			if state.Consecutive >= int64(cfg.MaxConsecutiveCooldowns) {
				state.State = "susp"
				savePressureCoolingState(channelId, state, stateTTL)
				reason := fmt.Sprintf("压力冷却挂起：连续 %d 次冷却达上限，需手动恢复", state.Consecutive)
				model.UpdateChannelStatus(channelId, "", common.ChannelStatusAutoDisabled, reason)
				NotifyRootUser(fmt.Sprintf("pressure_cooling_susp_%d", channelId),
					fmt.Sprintf("渠道「%s」(#%d) 压力冷却已挂起", ch.Name, channelId),
					fmt.Sprintf("渠道「%s」(#%d) 连续 %d 次冷却达上限，需管理员手动恢复", ch.Name, channelId, state.Consecutive))
				continue
			}

			model.UpdateChannelStatus(channelId, "", common.ChannelStatusEnabled, "压力冷却恢复")
			state.State = "obs"
			state.GraceUntil = now + int64(cfg.GracePeriodSeconds)
			state.Violations = 0
			state.TotalRequests = 0
			state.WindowStart = now
			savePressureCoolingState(channelId, state, stateTTL)

			NotifyRootUser(fmt.Sprintf("pressure_cooling_recover_%d", channelId),
				fmt.Sprintf("渠道「%s」(#%d) 已从压力冷却中恢复", ch.Name, channelId),
				fmt.Sprintf("渠道「%s」(#%d) 冷却期满，已自动恢复（累计 %d 次连续冷却）", ch.Name, channelId, state.Consecutive))
		}
	}
}

func formatCooldownDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm%ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh%dm", seconds/3600, (seconds%3600)/60)
}
