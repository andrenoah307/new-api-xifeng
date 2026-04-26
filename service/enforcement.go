package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
)

// updateUserStatusForEnforcement is a tiny helper to avoid pulling in the
// full UpdateUser flow (which expects an admin form payload). Lives in the
// service package because the model layer doesn't expose a status-only
// updater today and pulling User.Status back through the heavy admin path
// would be overkill for a single column flip.
func updateUserStatusForEnforcement(userID, status int) error {
	if userID <= 0 {
		return fmt.Errorf("invalid user id")
	}
	return model.DB.Exec(
		"UPDATE users SET status = ? WHERE id = ?", status, userID,
	).Error
}

// EnforcementHit is the unified entry point for the post-hit handling layer.
// Both the distribution-detection engine (service/risk_control.go) and the
// moderation engine (service/moderation_center.go) call this after they
// finalise their own decision, so the email + auto-ban policy lives in one
// place. The function MUST be non-blocking from the caller's perspective —
// it spawns a gopool goroutine internally and never touches the relay
// response.
func EnforcementHit(userID int, group, source, ruleHint string) {
	if userID <= 0 || group == "" || source == "" {
		return
	}
	cfg := operation_setting.GetEnforcementSetting()
	if !operation_setting.IsEnforcementSourceEnabled(cfg, source) {
		return
	}
	gopool.Go(func() {
		processEnforcementHit(userID, group, source, ruleHint, cfg)
	})
}

func processEnforcementHit(
	userID int, group, source, ruleHint string,
	cfg *operation_setting.EnforcementSetting,
) {
	snap, err := model.LoadEnforcementCounter(userID)
	if err != nil {
		common.SysError("enforcement load counter failed: " + err.Error())
		return
	}
	if snap.AutoBannedAt > 0 {
		// Already auto-banned; the user remains disabled until an admin
		// unbans them. Skip silently to avoid duplicate ban emails on the
		// next hit, but still record one "hit" audit row so admins can see
		// the user kept trying.
		_ = model.CreateEnforcementIncident(&model.EnforcementIncident{
			CreatedAt:       common.GetTimestamp(),
			UserID:          userID,
			Username:        snap.Username,
			Group:           group,
			Source:          source,
			Action:          model.EnforcementActionHit,
			HitCountAfter:   counterForSource(snap, source),
			Threshold:       operation_setting.EffectiveEnforcementBanThreshold(cfg, source),
			EmailDelivered:  false,
			EmailSkipReason: "already_banned",
			RuleHint:        ruleHint,
			Reason:          "user already auto-banned",
		})
		return
	}

	now := common.GetTimestamp()
	rolled := false
	windowStart := snap.WindowStartAt
	hitRisk := snap.HitCountRisk
	hitMod := snap.HitCountModeration
	if cfg.CountWindowHours > 0 {
		expiresAt := windowStart + int64(cfg.CountWindowHours)*3600
		if windowStart == 0 || now >= expiresAt {
			windowStart = now
			hitRisk = 0
			hitMod = 0
			rolled = true
		}
	} else if windowStart == 0 {
		windowStart = now
	}
	switch source {
	case operation_setting.EnforcementSourceRiskDistribution:
		hitRisk++
	case operation_setting.EnforcementSourceModeration:
		hitMod++
	default:
		return
	}

	if err := model.ApplyEnforcementHit(userID, hitRisk, hitMod, windowStart, now); err != nil {
		common.SysError("enforcement apply hit failed: " + err.Error())
		return
	}

	threshold := operation_setting.EffectiveEnforcementBanThreshold(cfg, source)
	currentCount := hitRisk
	if source == operation_setting.EnforcementSourceModeration {
		currentCount = hitMod
	}

	emailDelivered, skipReason := false, ""
	if cfg.EmailOnHit {
		emailDelivered, skipReason = sendEnforcementEmailWithRateLimit(
			snap, cfg, group, source, currentCount, threshold, false, now,
		)
	} else {
		skipReason = model.EnforcementEmailSkipReasonDisabled
	}

	reason := fmt.Sprintf("source=%s count=%d threshold=%d window_rolled=%v", source, currentCount, threshold, rolled)
	_ = model.CreateEnforcementIncident(&model.EnforcementIncident{
		CreatedAt:       now,
		UserID:          userID,
		Username:        snap.Username,
		Group:           group,
		Source:          source,
		Action:          model.EnforcementActionHit,
		HitCountAfter:   currentCount,
		Threshold:       threshold,
		EmailDelivered:  emailDelivered,
		EmailSkipReason: skipReason,
		RuleHint:        ruleHint,
		Reason:          reason,
	})

	if threshold > 0 && currentCount >= threshold {
		applyEnforcementAutoBan(snap, cfg, group, source, currentCount, threshold, now)
	}
}

func applyEnforcementAutoBan(
	snap *model.EnforcementCounterSnapshot,
	cfg *operation_setting.EnforcementSetting,
	group, source string, currentCount, threshold int, now int64,
) {
	if err := model.MarkEnforcementAutoBanned(snap.UserID, now); err != nil {
		common.SysError("enforcement auto-ban update failed: " + err.Error())
		return
	}
	emailDelivered, skipReason := false, ""
	if cfg.EmailOnAutoBan {
		// Refresh the snapshot for the email rate-limit window — the hit
		// path may have just sent an email and bumped the counter.
		fresh, err := model.LoadEnforcementCounter(snap.UserID)
		if err == nil && fresh != nil {
			snap = fresh
		}
		emailDelivered, skipReason = sendEnforcementEmailWithRateLimit(
			snap, cfg, group, source, currentCount, threshold, true, now,
		)
	} else {
		skipReason = model.EnforcementEmailSkipReasonDisabled
	}
	_ = model.CreateEnforcementIncident(&model.EnforcementIncident{
		CreatedAt:       now,
		UserID:          snap.UserID,
		Username:        snap.Username,
		Group:           group,
		Source:          source,
		Action:          model.EnforcementActionAutoBan,
		HitCountAfter:   currentCount,
		Threshold:       threshold,
		EmailDelivered:  emailDelivered,
		EmailSkipReason: skipReason,
		Reason:          fmt.Sprintf("auto-ban triggered by %s reaching %d/%d", source, currentCount, threshold),
	})
}

func counterForSource(snap *model.EnforcementCounterSnapshot, source string) int {
	if snap == nil {
		return 0
	}
	if source == operation_setting.EnforcementSourceModeration {
		return snap.HitCountModeration
	}
	return snap.HitCountRisk
}

// sendEnforcementEmailWithRateLimit returns (delivered, skip_reason). Skips
// silently when the per-user "max N emails per M minutes" budget is
// exhausted, when the user has no email address, or when the SMTP layer
// errors out. Skip reasons are recorded on the audit row so admins can
// understand why their threshold settings produced no email.
func sendEnforcementEmailWithRateLimit(
	snap *model.EnforcementCounterSnapshot,
	cfg *operation_setting.EnforcementSetting,
	group, source string, count, threshold int,
	isBan bool, now int64,
) (bool, string) {
	if snap == nil || strings.TrimSpace(snap.Email) == "" {
		return false, model.EnforcementEmailSkipReasonNoEmail
	}
	if cfg.EmailRateLimitMaxPerWindow > 0 && cfg.EmailRateLimitWindowMinutes > 0 {
		windowStart := snap.EmailWindowStartAt
		windowSec := int64(cfg.EmailRateLimitWindowMinutes) * 60
		newCount := snap.EmailCountInWindow
		if windowStart == 0 || now-windowStart >= windowSec {
			windowStart = now
			newCount = 0
		}
		if newCount >= cfg.EmailRateLimitMaxPerWindow {
			return false, model.EnforcementEmailSkipReasonRateLimit
		}
		newCount++
		if err := model.MarkEnforcementEmailSent(snap.UserID, windowStart, newCount); err != nil {
			common.SysError("enforcement email window update failed: " + err.Error())
		}
	}
	subject, body := renderEnforcementEmail(cfg, snap.Username, group, source, count, threshold, isBan, now)
	if err := common.SendEmail(subject, snap.Email, body); err != nil {
		common.SysError("enforcement send email failed: " + err.Error())
		return false, model.EnforcementEmailSkipReasonSendError
	}
	return true, ""
}

func renderEnforcementEmail(
	cfg *operation_setting.EnforcementSetting,
	username, group, source string,
	count, threshold int,
	isBan bool, now int64,
) (string, string) {
	subject := cfg.EmailHitSubject
	tmpl := cfg.EmailHitTemplate
	if isBan {
		subject = cfg.EmailBanSubject
		tmpl = cfg.EmailBanTemplate
	}
	timeStr := time.Unix(now, 0).Format("2006-01-02 15:04:05")
	sourceZh := map[string]string{
		operation_setting.EnforcementSourceRiskDistribution: "分发检测",
		operation_setting.EnforcementSourceModeration:       "内容审核",
	}[source]
	if sourceZh == "" {
		sourceZh = source
	}
	replacer := strings.NewReplacer(
		"{{username}}", username,
		"{{time}}", timeStr,
		"{{group}}", group,
		"{{source_zh}}", sourceZh,
		"{{source}}", source,
		"{{count}}", fmt.Sprintf("%d", count),
		"{{threshold}}", fmt.Sprintf("%d", threshold),
	)
	return replacer.Replace(subject), replacer.Replace(tmpl)
}

// SendEnforcementTestEmail powers the "发送测试邮件" admin button. The
// destination is fixed to the calling admin (decision point 7 == 前者) so
// nobody can use the endpoint as an arbitrary email relay.
func SendEnforcementTestEmail(adminUserID int) error {
	if adminUserID <= 0 {
		return fmt.Errorf("invalid admin user")
	}
	user, err := model.GetUserById(adminUserID, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(user.Email) == "" {
		return fmt.Errorf("admin %s has no email configured", user.Username)
	}
	cfg := operation_setting.GetEnforcementSetting()
	now := common.GetTimestamp()
	subject, body := renderEnforcementEmail(cfg, user.Username, "test_group", operation_setting.EnforcementSourceModeration, 1, 5, false, now)
	subject = "[测试] " + subject
	if err := common.SendEmail(subject, user.Email, body); err != nil {
		_ = model.CreateEnforcementIncident(&model.EnforcementIncident{
			CreatedAt:       now,
			UserID:          adminUserID,
			Username:        user.Username,
			Source:          "test",
			Action:          model.EnforcementActionTest,
			EmailDelivered:  false,
			EmailSkipReason: model.EnforcementEmailSkipReasonSendError,
			Reason:          err.Error(),
		})
		return err
	}
	_ = model.CreateEnforcementIncident(&model.EnforcementIncident{
		CreatedAt:      now,
		UserID:         adminUserID,
		Username:       user.Username,
		Source:         "test",
		Action:         model.EnforcementActionTest,
		EmailDelivered: true,
		Reason:         "admin test email",
	})
	return nil
}

// EnforcementOverview powers the admin overview card. 24h windows align
// with the operations cadence — anything older surfaces in the listing.
func EnforcementOverview() (map[string]any, error) {
	cfg := operation_setting.GetEnforcementSetting()
	since := time.Now().Add(-24 * time.Hour).Unix()
	hits24, err := model.CountEnforcementIncidentsBy(model.EnforcementActionHit, since)
	if err != nil {
		return nil, err
	}
	bans24, err := model.CountEnforcementIncidentsBy(model.EnforcementActionAutoBan, since)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"enabled":          cfg.Enabled,
		"hits_24h":         hits24,
		"auto_bans_24h":    bans24,
		"ban_threshold":    cfg.BanThreshold,
		"window_hours":     cfg.CountWindowHours,
		"enabled_sources":  cfg.EnabledSources,
		"email_on_hit":     cfg.EmailOnHit,
		"email_on_autoban": cfg.EmailOnAutoBan,
	}, nil
}

// ManualUnbanEnforcement is invoked by the admin "立即解封" button. Per
// decision point 6 this resets the hit counters as well, so the just-
// unbanned user starts a fresh window instead of immediately tripping
// auto-ban on the very first new hit.
func ManualUnbanEnforcement(adminUserID, targetUserID int) error {
	if targetUserID <= 0 {
		return fmt.Errorf("invalid user id")
	}
	user, err := model.GetUserById(targetUserID, false)
	if err != nil {
		return err
	}
	if err := model.ResetEnforcementCounter(targetUserID); err != nil {
		return err
	}
	if err := updateUserStatusForEnforcement(targetUserID, common.UserStatusEnabled); err != nil {
		return err
	}
	now := common.GetTimestamp()
	_ = model.CreateEnforcementIncident(&model.EnforcementIncident{
		CreatedAt:      now,
		UserID:         targetUserID,
		Username:       user.Username,
		Action:         model.EnforcementActionUnban,
		EmailDelivered: false,
		Reason:         fmt.Sprintf("admin %d unban + counter reset", adminUserID),
	})
	return nil
}

// ManualResetEnforcementCounter zeros counters without changing the user
// status — used when an admin wants to forgive the user without revealing
// they were close to ban.
func ManualResetEnforcementCounter(adminUserID, targetUserID int) error {
	if targetUserID <= 0 {
		return fmt.Errorf("invalid user id")
	}
	user, err := model.GetUserById(targetUserID, false)
	if err != nil {
		return err
	}
	if err := model.ResetEnforcementCounter(targetUserID); err != nil {
		return err
	}
	now := common.GetTimestamp()
	_ = model.CreateEnforcementIncident(&model.EnforcementIncident{
		CreatedAt: now,
		UserID:    targetUserID,
		Username:  user.Username,
		Action:    model.EnforcementActionReset,
		Reason:    fmt.Sprintf("admin %d counter reset", adminUserID),
	})
	return nil
}
