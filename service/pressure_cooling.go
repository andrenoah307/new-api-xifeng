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
	Enabled                   bool
	Scope                     string
	CooldownGroups            []string
	ObservationWindowSeconds  int
	FRTThresholdMs            int
	TriggerPercent            int
	CooldownSeconds           int
	MaxConsecutiveCooldowns   int
	CooldownBackoffMultiplier float64
	MaxCooldownSeconds        int
	GracePeriodSeconds        int
	MinActiveChannelsPerGroup int
}

func resolvePressureCoolingConfig(override *dto.PressureCoolingOverride) resolvedPressureCoolingConfig {
	g := operation_setting.GetPressureCoolingSetting()
	r := resolvedPressureCoolingConfig{
		Enabled:                   g.Enabled,
		Scope:                     "channel",
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
	if override.Scope == "groups" {
		r.Scope = "groups"
		r.CooldownGroups = append([]string(nil), override.CooldownGroups...)
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
		if state.Scope == "groups" {
			return
		}
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
	if cfg.Scope == "groups" {
		targetGroups := pressureCoolingTargetGroups(ch, cfg.CooldownGroups)
		if len(targetGroups) == 0 {
			common.SysLog(fmt.Sprintf("pressure cooling: skip channel #%d (%s) — no configured cooling groups belong to channel", ch.Id, ch.Name))
			state.Violations = 0
			state.WindowStart = now
			savePressureCoolingState(ch.Id, state, stateTTL)
			return
		}
		if !canCoolChannelGroups(ch, targetGroups, cfg.MinActiveChannelsPerGroup) {
			common.SysLog(fmt.Sprintf("pressure cooling: skip channel #%d (%s) — would leave target group/model below minimum active", ch.Id, ch.Name))
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
		reason := fmt.Sprintf("压力冷却：观察期内 %d/%d 请求 FRT 超 %dms（%d%%），冷却 %ds，摘除分组 %s",
			state.Violations, state.TotalRequests, cfg.FRTThresholdMs, pct, cooldownSec, strings.Join(targetGroups, ", "))
		state.Scope = "groups"
		state.CooledGroups = targetGroups
		state.State = "cool"
		state.CooldownUntil = now + cooldownSec
		state.Consecutive++
		state.Violations = 0
		savePressureCoolingState(ch.Id, state, stateTTL)
		refreshPressureCoolingOverlay()

		subject := fmt.Sprintf("渠道「%s」(#%d) 因高延迟已自动冷却分组", ch.Name, ch.Id)
		content := fmt.Sprintf("渠道「%s」(#%d) 已从分组 %s 摘除，其余分组不受影响。%s\n冷却将于 %s 后自动恢复（第 %d 次连续冷却）",
			ch.Name, ch.Id, strings.Join(targetGroups, ", "), reason, formatCooldownDuration(cooldownSec), state.Consecutive)
		NotifyRootUser(fmt.Sprintf("pressure_cooling_%d", ch.Id), subject, content)
		return
	}

	state.Scope = "channel"
	state.CooledGroups = nil
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

func pressureCoolingTargetGroups(ch *model.Channel, configured []string) []string {
	if ch == nil || len(configured) == 0 {
		return nil
	}
	channelGroups := make(map[string]struct{}, len(ch.GetGroups()))
	for _, group := range ch.GetGroups() {
		if group = strings.TrimSpace(group); group != "" {
			channelGroups[group] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(configured))
	target := make([]string, 0, len(configured))
	for _, group := range configured {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, belongs := channelGroups[group]; !belongs {
			continue
		}
		if _, duplicate := seen[group]; duplicate {
			continue
		}
		seen[group] = struct{}{}
		target = append(target, group)
	}
	return target
}

func canCoolChannelGroups(ch *model.Channel, targetGroups []string, minActive int) bool {
	if ch == nil {
		return false
	}
	for _, group := range targetGroups {
		for _, modelName := range ch.GetModels() {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			remaining := model.CountEnabledChannelsForGroupModel(group, modelName)
			if model.IsChannelEnabledForGroupModel(group, modelName, ch.Id) {
				remaining--
			}
			if remaining < minActive {
				return false
			}
		}
	}
	return true
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
	refreshPressureCoolingOverlay()
}

func StartPressureCoolingRecovery() {
	go pressureCoolingOverlayRefreshLoop()
	if common.IsMasterNode {
		go pressureCoolingRecoveryLoop()
	}
}

func pressureCoolingRecoveryLoop() {
	for {
		globalCfg := operation_setting.GetPressureCoolingSetting()
		interval := globalCfg.RecoveryCheckIntervalSeconds
		if interval <= 0 {
			interval = 30
		}
		time.Sleep(time.Duration(interval) * time.Second)

		pressureCoolingRecoveryOnce(time.Now().Unix())
	}
}

func pressureCoolingRecoveryOnce(now int64) {
	states := listCoolingChannelStates()
	overlayDirty := false
	for channelId, state := range states {
		if state.Scope == "groups" {
			ch, err := model.CacheGetChannel(channelId)
			if err != nil || ch == nil {
				deletePressureCoolingState(channelId)
				overlayDirty = true
				continue
			}
			cfg := resolvePressureCoolingConfig(ch.GetSetting().PressureCooling)
			stateTTL := cfg.MaxCooldownSeconds * 3
			if stateTTL < cfg.ObservationWindowSeconds*3 {
				stateTTL = cfg.ObservationWindowSeconds * 3
			}
			if !cfg.Enabled {
				deletePressureCoolingState(channelId)
				overlayDirty = true
				continue
			}
			if state.State != "cool" {
				continue
			}
			if now < state.CooldownUntil {
				continue
			}
			if state.Consecutive >= int64(cfg.MaxConsecutiveCooldowns) {
				state.State = "susp"
				savePressureCoolingState(channelId, state, stateTTL)
				overlayDirty = true
				reason := fmt.Sprintf("压力冷却挂起：连续 %d 次冷却达上限，需手动恢复", state.Consecutive)
				NotifyRootUser(fmt.Sprintf("pressure_cooling_susp_%d", channelId),
					fmt.Sprintf("渠道「%s」(#%d) 压力冷却分组已挂起", ch.Name, channelId),
					fmt.Sprintf("渠道「%s」(#%d) 连续 %d 次冷却达上限，分组 %s 仍被摘除，需管理员手动恢复：%s", ch.Name, channelId, state.Consecutive, strings.Join(state.CooledGroups, ", "), reason))
				continue
			}
			state.State = "obs"
			state.CooledGroups = nil
			state.GraceUntil = now + int64(cfg.GracePeriodSeconds)
			state.Violations = 0
			state.TotalRequests = 0
			state.WindowStart = now
			savePressureCoolingState(channelId, state, stateTTL)
			overlayDirty = true
			NotifyRootUser(fmt.Sprintf("pressure_cooling_recover_%d", channelId),
				fmt.Sprintf("渠道「%s」(#%d) 分组压力冷却已恢复", ch.Name, channelId),
				fmt.Sprintf("渠道「%s」(#%d) 冷却期满，已恢复全部分组调度（累计 %d 次连续冷却）", ch.Name, channelId, state.Consecutive))
			continue
		}

		if state.State != "cool" {
			continue
		}
		ch, err := model.CacheGetChannel(channelId)
		if err != nil || ch == nil {
			deletePressureCoolingState(channelId)
			continue
		}
		cfg := resolvePressureCoolingConfig(ch.GetSetting().PressureCooling)
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
	if overlayDirty {
		refreshPressureCoolingOverlay()
	}
}

func pressureCoolingOverlayRefreshLoop() {
	refreshPressureCoolingOverlay()
	for {
		interval := operation_setting.GetPressureCoolingSetting().RecoveryCheckIntervalSeconds
		if interval <= 0 {
			interval = 30
		}
		time.Sleep(time.Duration(interval) * time.Second)
		refreshPressureCoolingOverlay()
	}
}

func refreshPressureCoolingOverlay() {
	states := listCoolingChannelStates()
	overlay := make(map[int]map[string]struct{})
	for channelId, state := range states {
		if state.Scope != "groups" || (state.State != "cool" && state.State != "susp") || len(state.CooledGroups) == 0 {
			continue
		}
		groups := make(map[string]struct{}, len(state.CooledGroups))
		for _, group := range state.CooledGroups {
			if group = strings.TrimSpace(group); group != "" {
				groups[group] = struct{}{}
			}
		}
		if len(groups) > 0 {
			overlay[channelId] = groups
		}
	}
	model.SetChannelGroupCooling(overlay)
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
