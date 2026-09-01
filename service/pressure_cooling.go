package service

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
)

type resolvedPressureCoolingConfig struct {
	Enabled                     bool
	Scope                       string
	CooldownGroups              []string
	ObservationWindowSeconds    int
	FRTThresholdMs              int
	TriggerPercent              int
	UpstreamErrorEnabled        bool
	UpstreamErrorTriggerPercent int
	UpstreamErrorMinSamples     int
	ConditionMode               string
	CooldownSeconds             int
	MaxConsecutiveCooldowns     int
	CooldownBackoffMultiplier   float64
	MaxCooldownSeconds          int
	GracePeriodSeconds          int
	MinActiveChannelsPerGroup   int
}

type pressureCoolingReason struct {
	frtMet        bool
	errorMet      bool
	errorAttempts int64
	errorCount    int64
}

func resolvePressureCoolingConfig(override *dto.PressureCoolingOverride) resolvedPressureCoolingConfig {
	g := operation_setting.GetPressureCoolingSetting()
	r := resolvedPressureCoolingConfig{
		Enabled:                     g.Enabled,
		Scope:                       "channel",
		ObservationWindowSeconds:    g.ObservationWindowSeconds,
		FRTThresholdMs:              g.FRTThresholdMs,
		TriggerPercent:              g.TriggerPercent,
		UpstreamErrorEnabled:        g.UpstreamErrorEnabled,
		UpstreamErrorTriggerPercent: g.UpstreamErrorTriggerPercent,
		UpstreamErrorMinSamples:     g.UpstreamErrorMinSamples,
		ConditionMode:               normalizePressureCoolingConditionMode(g.ConditionMode),
		CooldownSeconds:             g.CooldownSeconds,
		MaxConsecutiveCooldowns:     g.MaxConsecutiveCooldowns,
		CooldownBackoffMultiplier:   g.CooldownBackoffMultiplier,
		MaxCooldownSeconds:          g.MaxCooldownSeconds,
		GracePeriodSeconds:          g.GracePeriodSeconds,
		MinActiveChannelsPerGroup:   g.MinActiveChannelsPerGroup,
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
	if override.UpstreamErrorEnabled != nil {
		r.UpstreamErrorEnabled = *override.UpstreamErrorEnabled
	}
	if override.UpstreamErrorTriggerPercent != nil {
		r.UpstreamErrorTriggerPercent = *override.UpstreamErrorTriggerPercent
	}
	if override.UpstreamErrorMinSamples != nil {
		r.UpstreamErrorMinSamples = *override.UpstreamErrorMinSamples
	}
	if override.ConditionMode != "" {
		r.ConditionMode = normalizePressureCoolingConditionMode(override.ConditionMode)
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

func normalizePressureCoolingConditionMode(mode string) string {
	if strings.ToLower(mode) == "all" {
		return "all"
	}
	return "any"
}

func classifyPressureCoolingAttempt(err *types.NewAPIError) (shouldCount, isUpstreamError bool) {
	if err == nil {
		return true, false
	}
	if err.IsSkipRetry() {
		return false, false
	}
	statusCode := err.StatusCode
	return true, statusCode == 403 || statusCode == 429 || (statusCode >= 500 && statusCode <= 599)
}

func pressureCoolingErrorConditionMet(cfg resolvedPressureCoolingConfig, attempts, errors int64) bool {
	if !cfg.UpstreamErrorEnabled || attempts <= 0 || errors < int64(cfg.UpstreamErrorMinSamples) {
		return false
	}
	return float64(errors)*100/float64(attempts) >= float64(cfg.UpstreamErrorTriggerPercent)
}

func pressureCoolingConditionsMet(cfg resolvedPressureCoolingConfig, frtMet, errorMet bool) bool {
	if !cfg.UpstreamErrorEnabled {
		return frtMet
	}
	if normalizePressureCoolingConditionMode(cfg.ConditionMode) == "all" {
		return frtMet && errorMet
	}
	return frtMet || errorMet
}

func pressureCoolingErrorStateEligible(state *PressureCoolingState, cfg resolvedPressureCoolingConfig, now int64) bool {
	if state == nil || state.State != "obs" || now < state.GraceUntil {
		return false
	}
	if cfg.UpstreamErrorEnabled && normalizePressureCoolingConditionMode(cfg.ConditionMode) == "all" {
		if state.WindowStart == 0 || now-state.WindowStart > int64(cfg.ObservationWindowSeconds) {
			return false
		}
	}
	return true
}

func pressureCoolingStateTTL(cfg resolvedPressureCoolingConfig) int {
	ttl := cfg.MaxCooldownSeconds * 3
	if ttl < cfg.ObservationWindowSeconds*3 {
		ttl = cfg.ObservationWindowSeconds * 3
	}
	if ttl < cfg.GracePeriodSeconds*3 {
		ttl = cfg.GracePeriodSeconds * 3
	}
	return ttl
}

// This mutex closes the check/action window within a process; cross-node residuals come from stale channel caches.
var pressureCoolingExecutionMu sync.Mutex

type pressureCoolingNotification struct {
	key            string
	subject        string
	content        string
	refreshOverlay bool
}

func RecordPressureCoolingAttempt(channelId int, err *types.NewAPIError) {
	shouldCount, isUpstreamError := classifyPressureCoolingAttempt(err)
	if !shouldCount {
		return
	}

	globalCfg := operation_setting.GetPressureCoolingSetting()
	var ch *model.Channel
	var override *dto.PressureCoolingOverride
	if !globalCfg.Enabled {
		// The cache is the only allowed lookup in the disabled global fast path.
		// Without it, an override cannot be discovered without a database query.
		if !common.MemoryCacheEnabled {
			return
		}
		ch, _ = model.CacheGetChannel(channelId)
		if ch == nil || ch.Setting == nil || *ch.Setting == "" ||
			!strings.Contains(*ch.Setting, "pressure_cooling") {
			return
		}
		override = ch.GetSetting().PressureCooling
	} else {
		ch, _ = model.CacheGetChannel(channelId)
		if ch == nil {
			return
		}
		override = ch.GetSetting().PressureCooling
	}
	cfg := resolvePressureCoolingConfig(override)
	if !cfg.Enabled || !cfg.UpstreamErrorEnabled {
		return
	}
	gopool.Go(func() {
		recordPressureCoolingAttemptAt(ch, cfg, isUpstreamError, time.Now().Unix())
	})
}

func recordPressureCoolingAttemptAt(ch *model.Channel, cfg resolvedPressureCoolingConfig, isUpstreamError bool, now int64) {
	if ch == nil {
		return
	}
	attempts, errors := incrPressureCoolingErrorWindowAt(ch.Id, cfg.ObservationWindowSeconds, isUpstreamError, now)
	if !isUpstreamError || !pressureCoolingErrorConditionMet(cfg, attempts, errors) {
		return
	}
	state, stateErr := loadPressureCoolingStateResult(ch.Id)
	if stateErr != nil || !pressureCoolingErrorStateEligible(state, cfg, now) {
		return
	}
	frtMet := state.TotalRequests >= 3 && state.Violations*100/state.TotalRequests >= int64(cfg.TriggerPercent)
	if state.WindowStart == 0 || now-state.WindowStart > int64(cfg.ObservationWindowSeconds) {
		frtMet = false
	}
	if normalizePressureCoolingConditionMode(cfg.ConditionMode) == "all" && !frtMet {
		return
	}
	executePressureCooling(ch, state, cfg, now, pressureCoolingStateTTL(cfg), &pressureCoolingReason{
		frtMet: frtMet, errorMet: true, errorAttempts: attempts, errorCount: errors,
	})
}

func formatPressureCoolingReason(state *PressureCoolingState, cfg resolvedPressureCoolingConfig, cooldownSec int64, info *pressureCoolingReason) string {
	if info == nil || !info.errorMet {
		pct := int64(0)
		if state.TotalRequests > 0 {
			pct = state.Violations * 100 / state.TotalRequests
		}
		return fmt.Sprintf("压力冷却：观察期内 %d/%d 请求 FRT 超 %dms（%d%%），冷却 %ds",
			state.Violations, state.TotalRequests, cfg.FRTThresholdMs, pct, cooldownSec)
	}
	errorPct := int64(0)
	if info.errorAttempts > 0 {
		errorPct = info.errorCount * 100 / info.errorAttempts
	}
	if info.frtMet {
		frtPct := int64(0)
		if state.TotalRequests > 0 {
			frtPct = state.Violations * 100 / state.TotalRequests
		}
		return fmt.Sprintf("压力冷却：观察期内 %d/%d 请求 FRT 超 %dms（%d%%），%d/%d 次上游报错（%d%%），冷却 %ds",
			state.Violations, state.TotalRequests, cfg.FRTThresholdMs, frtPct,
			info.errorCount, info.errorAttempts, errorPct, cooldownSec)
	}
	return fmt.Sprintf("压力冷却：观察期内 %d/%d 次上游报错（%d%%），冷却 %ds",
		info.errorCount, info.errorAttempts, errorPct, cooldownSec)
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

	state, stateErr := loadPressureCoolingStateResult(channelId)
	if stateErr != nil || state == nil {
		return
	}
	now := time.Now().Unix()
	stateTTL := pressureCoolingStateTTL(cfg)

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
	frtMet := state.TotalRequests >= 3 && violationPct >= int64(cfg.TriggerPercent)
	errorMet := false
	var reason *pressureCoolingReason
	if cfg.UpstreamErrorEnabled {
		mode := normalizePressureCoolingConditionMode(cfg.ConditionMode)
		if mode == "all" && !frtMet {
			savePressureCoolingState(channelId, state, stateTTL)
			return
		}
		if !frtMet || mode == "all" {
			a, e := loadPressureCoolingErrorWindow(channelId, cfg.ObservationWindowSeconds)
			errorMet = pressureCoolingErrorConditionMet(cfg, a, e)
			if errorMet {
				reason = &pressureCoolingReason{frtMet: frtMet, errorMet: true, errorAttempts: a, errorCount: e}
			}
		}
	}
	if pressureCoolingConditionsMet(cfg, frtMet, errorMet) {
		executePressureCooling(ch, state, cfg, now, stateTTL, reason)
	} else {
		savePressureCoolingState(channelId, state, stateTTL)
	}
}

func executePressureCooling(ch *model.Channel, state *PressureCoolingState, cfg resolvedPressureCoolingConfig, now int64, stateTTL int, reason *pressureCoolingReason) {
	notification := executePressureCoolingDecision(ch, state, cfg, now, stateTTL, reason)
	if notification == nil {
		return
	}
	if notification.refreshOverlay {
		refreshPressureCoolingOverlay()
	}
	NotifyRootUser(notification.key, notification.subject, notification.content)
}

func executePressureCoolingDecision(ch *model.Channel, state *PressureCoolingState, cfg resolvedPressureCoolingConfig, now int64, stateTTL int, reason *pressureCoolingReason) *pressureCoolingNotification {
	pressureCoolingExecutionMu.Lock()
	defer pressureCoolingExecutionMu.Unlock()

	if cfg.Scope == "groups" {
		targetGroups := pressureCoolingTargetGroups(ch, cfg.CooldownGroups)
		if len(targetGroups) == 0 {
			common.SysLog(fmt.Sprintf("pressure cooling: skip channel #%d (%s) — no configured cooling groups belong to channel", ch.Id, ch.Name))
			state.Violations = 0
			state.WindowStart = now
			savePressureCoolingState(ch.Id, state, stateTTL)
			return nil
		}
		if !canCoolChannelGroups(ch, targetGroups, cfg.MinActiveChannelsPerGroup) {
			common.SysLog(fmt.Sprintf("pressure cooling: skip channel #%d (%s) — would leave target group/model below minimum active", ch.Id, ch.Name))
			state.Violations = 0
			state.WindowStart = now
			savePressureCoolingState(ch.Id, state, stateTTL)
			return nil
		}

		effectiveCooldown := float64(cfg.CooldownSeconds)
		for i := int64(0); i < state.Consecutive; i++ {
			effectiveCooldown *= cfg.CooldownBackoffMultiplier
		}
		if effectiveCooldown > float64(cfg.MaxCooldownSeconds) {
			effectiveCooldown = float64(cfg.MaxCooldownSeconds)
		}
		cooldownSec := int64(math.Ceil(effectiveCooldown))
		reasonText := formatPressureCoolingReason(state, cfg, cooldownSec, reason)
		reasonText += fmt.Sprintf("，摘除分组 %s", strings.Join(targetGroups, ", "))
		state.Scope = "groups"
		state.CooledGroups = targetGroups
		state.State = "cool"
		state.CooldownUntil = now + cooldownSec
		state.Consecutive++
		state.Violations = 0
		savePressureCoolingState(ch.Id, state, stateTTL)

		subject := fmt.Sprintf("渠道「%s」(#%d) 因高延迟已自动冷却分组", ch.Name, ch.Id)
		content := fmt.Sprintf("渠道「%s」(#%d) 已从分组 %s 摘除，其余分组不受影响。%s\n冷却将于 %s 后自动恢复（第 %d 次连续冷却）",
			ch.Name, ch.Id, strings.Join(targetGroups, ", "), reasonText, formatCooldownDuration(cooldownSec), state.Consecutive)
		return &pressureCoolingNotification{
			key:            fmt.Sprintf("pressure_cooling_%d", ch.Id),
			subject:        subject,
			content:        content,
			refreshOverlay: true,
		}
	}

	state.Scope = "channel"
	state.CooledGroups = nil
	if ch.Status != common.ChannelStatusEnabled {
		common.SysLog(fmt.Sprintf("压力冷却：渠道当前状态非启用，压力冷却放弃冷却（渠道 #%d，%s）", ch.Id, ch.Name))
		state.Violations = 0
		state.WindowStart = now
		savePressureCoolingState(ch.Id, state, stateTTL)
		return nil
	}
	if !canCoolChannel(ch.Id, cfg.MinActiveChannelsPerGroup) {
		common.SysLog(fmt.Sprintf("pressure cooling: skip channel #%d (%s) — would leave (group, model) below minimum active", ch.Id, ch.Name))
		state.Violations = 0
		state.WindowStart = now
		savePressureCoolingState(ch.Id, state, stateTTL)
		return nil
	}

	effectiveCooldown := float64(cfg.CooldownSeconds)
	for i := int64(0); i < state.Consecutive; i++ {
		effectiveCooldown *= cfg.CooldownBackoffMultiplier
	}
	if effectiveCooldown > float64(cfg.MaxCooldownSeconds) {
		effectiveCooldown = float64(cfg.MaxCooldownSeconds)
	}
	cooldownSec := int64(math.Ceil(effectiveCooldown))

	reasonText := formatPressureCoolingReason(state, cfg, cooldownSec, reason)
	model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusAutoDisabled, reasonText)

	state.State = "cool"
	state.CooldownUntil = now + cooldownSec
	state.Consecutive++
	state.Violations = 0
	savePressureCoolingState(ch.Id, state, stateTTL)

	subject := fmt.Sprintf("渠道「%s」(#%d) 因高延迟已自动冷却", ch.Name, ch.Id)
	content := fmt.Sprintf("渠道「%s」(#%d) %s\n冷却将于 %s 后自动恢复（第 %d 次连续冷却）",
		ch.Name, ch.Id, reasonText, formatCooldownDuration(cooldownSec), state.Consecutive)
	return &pressureCoolingNotification{
		key:     fmt.Sprintf("pressure_cooling_%d", ch.Id),
		subject: subject,
		content: content,
	}
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
	states, stateErr := listCoolingChannelStatesResult()
	if stateErr != nil {
		return
	}
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
			stateTTL := pressureCoolingStateTTL(cfg)
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
		stateTTL := pressureCoolingStateTTL(cfg)
		if !cfg.Enabled {
			if ch.Status == common.ChannelStatusAutoDisabled {
				model.UpdateChannelStatus(channelId, "", common.ChannelStatusEnabled, "压力冷却已禁用，自动恢复")
			} else {
				common.SysLog(fmt.Sprintf("压力冷却：渠道当前状态非自动禁用，压力冷却放弃恢复（渠道 #%d，%s）", channelId, ch.Name))
			}
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
			if ch.Status != common.ChannelStatusManuallyDisabled {
				model.UpdateChannelStatus(channelId, "", common.ChannelStatusAutoDisabled, reason)
			}
			NotifyRootUser(fmt.Sprintf("pressure_cooling_susp_%d", channelId),
				fmt.Sprintf("渠道「%s」(#%d) 压力冷却已挂起", ch.Name, channelId),
				fmt.Sprintf("渠道「%s」(#%d) 连续 %d 次冷却达上限，需管理员手动恢复", ch.Name, channelId, state.Consecutive))
			continue
		}
		if ch.Status == common.ChannelStatusAutoDisabled {
			model.UpdateChannelStatus(channelId, "", common.ChannelStatusEnabled, "压力冷却恢复")
		} else {
			common.SysLog(fmt.Sprintf("压力冷却：渠道当前状态非自动禁用，压力冷却放弃恢复（渠道 #%d，%s）", channelId, ch.Name))
		}
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
	cleanupPressureCoolingErrorWindows(time.Now().Unix())
	refreshPressureCoolingOverlay()
	for {
		interval := operation_setting.GetPressureCoolingSetting().RecoveryCheckIntervalSeconds
		if interval <= 0 {
			interval = 30
		}
		time.Sleep(time.Duration(interval) * time.Second)
		cleanupPressureCoolingErrorWindows(time.Now().Unix())
		refreshPressureCoolingOverlay()
	}
}

func refreshPressureCoolingOverlay() {
	states, err := listCoolingChannelStatesResult()
	if err != nil {
		return
	}
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
