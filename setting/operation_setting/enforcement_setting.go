package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	EnforcementSourceRiskDistribution = "risk_distribution"
	EnforcementSourceModeration       = "moderation"
)

// EnforcementSetting drives the unified post-hit handling layer that lives
// on top of both the distribution-detection engine and the moderation
// engine. It owns the email + auto-ban policy so neither upstream engine
// has to know about email plumbing or per-user counters.
//
// Defaults are deliberately permissive (Enabled=false, BanThreshold=0,
// EmailOn... =false) so upgrading does not silently start banning users
// before an operator opts in.
type EnforcementSetting struct {
	Enabled        bool `json:"enabled"`
	EmailOnHit     bool `json:"email_on_hit"`
	EmailOnAutoBan bool `json:"email_on_auto_ban"`

	// CountWindowHours: fixed window for the per-user hit counter. When
	// (now - window_start) crosses this, both source counters reset to 0
	// and window_start advances to now (decision point 2 == fixed window).
	// 0 disables decay (lifetime tally — useful for hard-line policies).
	CountWindowHours int `json:"count_window_hours"`

	// BanThreshold: hits within the window before auto-ban fires. 0 turns
	// the auto-ban off entirely (counters and emails still run).
	BanThreshold int `json:"ban_threshold"`

	// BanThresholdPerSource: optional per-source override. When the source
	// is not present, BanThreshold is used. Lets ops weight moderation
	// hits more aggressively than distribution hits, or vice versa.
	BanThresholdPerSource map[string]int `json:"ban_threshold_per_source"`

	// EnabledSources: which engines participate. Defaults to both. Sources
	// that have been removed at runtime stop receiving hits silently —
	// matches the "未启用 = 零侵入" pattern used everywhere else.
	EnabledSources []string `json:"enabled_sources"`

	EmailHitTemplate string `json:"email_hit_template"`
	EmailBanTemplate string `json:"email_ban_template"`
	EmailHitSubject  string `json:"email_hit_subject"`
	EmailBanSubject  string `json:"email_ban_subject"`

	// EmailRateLimitWindowMinutes / EmailRateLimitMaxPerWindow implement
	// "no more than N emails per user in M minutes" (decision point 8).
	// Defaults: 10 / 3 — three emails per ten minutes.
	EmailRateLimitWindowMinutes int `json:"email_rate_limit_window_minutes"`
	EmailRateLimitMaxPerWindow  int `json:"email_rate_limit_max_per_window"`
}

var enforcementSetting = EnforcementSetting{
	Enabled:                     false,
	EmailOnHit:                  false,
	EmailOnAutoBan:              false,
	CountWindowHours:            24,
	BanThreshold:                0,
	BanThresholdPerSource:       map[string]int{},
	EnabledSources:              []string{EnforcementSourceRiskDistribution, EnforcementSourceModeration},
	EmailHitTemplate:            DefaultEnforcementHitTemplate,
	EmailBanTemplate:            DefaultEnforcementBanTemplate,
	EmailHitSubject:             "您的账户触发了平台风控策略",
	EmailBanSubject:             "您的账户已被自动封禁",
	EmailRateLimitWindowMinutes: 10,
	EmailRateLimitMaxPerWindow:  3,
}

// DefaultEnforcementHitTemplate is intentionally vague — see DEV_GUIDE
// "防绕过" red line. Variables the engine substitutes:
//
//	{{username}} {{time}} {{group}} {{source_zh}} {{count}} {{threshold}}
//
// Specifically: never {{rule_name}} or any rule-specific identifier.
const DefaultEnforcementHitTemplate = `<p>尊敬的用户 {{username}}：</p>
<p>您的账户在 <b>{{time}}</b> 因 <b>{{group}}</b> 分组的请求触发了 <b>{{source_zh}}</b> 风控策略。</p>
<p>这是当前计数周期内第 {{count}} / {{threshold}} 次命中。如多次触发，账户将被自动限制使用。</p>
<p>如有疑问，请联系平台管理员。</p>`

const DefaultEnforcementBanTemplate = `<p>尊敬的用户 {{username}}：</p>
<p>您的账户已于 <b>{{time}}</b> 被自动封禁，原因是计数周期内多次触发 <b>{{source_zh}}</b> 风控策略（共 {{count}} 次）。</p>
<p>如需解除限制，请联系平台管理员。</p>`

func isKnownEnforcementSource(s string) bool {
	switch strings.TrimSpace(s) {
	case EnforcementSourceRiskDistribution, EnforcementSourceModeration:
		return true
	default:
		return false
	}
}

func normalizeEnforcementSources(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if !isKnownEnforcementSource(s) {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// NormalizeEnforcementSetting clamps numeric fields and drops invalid sources
// from the per-source threshold map so a partial UI submission cannot leave
// the engine in a broken state.
func NormalizeEnforcementSetting(setting *EnforcementSetting) {
	if setting == nil {
		return
	}
	if setting.CountWindowHours < 0 {
		setting.CountWindowHours = 24
	}
	if setting.BanThreshold < 0 {
		setting.BanThreshold = 0
	}
	if setting.EmailRateLimitWindowMinutes <= 0 {
		setting.EmailRateLimitWindowMinutes = 10
	}
	if setting.EmailRateLimitMaxPerWindow <= 0 {
		setting.EmailRateLimitMaxPerWindow = 3
	}
	cleanedPerSource := map[string]int{}
	for k, v := range setting.BanThresholdPerSource {
		k = strings.TrimSpace(k)
		if !isKnownEnforcementSource(k) {
			continue
		}
		if v < 0 {
			v = 0
		}
		cleanedPerSource[k] = v
	}
	setting.BanThresholdPerSource = cleanedPerSource
	setting.EnabledSources = normalizeEnforcementSources(setting.EnabledSources)
	setting.EmailHitSubject = strings.TrimSpace(setting.EmailHitSubject)
	if setting.EmailHitSubject == "" {
		setting.EmailHitSubject = "您的账户触发了平台风控策略"
	}
	setting.EmailBanSubject = strings.TrimSpace(setting.EmailBanSubject)
	if setting.EmailBanSubject == "" {
		setting.EmailBanSubject = "您的账户已被自动封禁"
	}
	if strings.TrimSpace(setting.EmailHitTemplate) == "" {
		setting.EmailHitTemplate = DefaultEnforcementHitTemplate
	}
	if strings.TrimSpace(setting.EmailBanTemplate) == "" {
		setting.EmailBanTemplate = DefaultEnforcementBanTemplate
	}
}

func init() {
	config.GlobalConfig.Register("enforcement", &enforcementSetting)
}

func GetEnforcementSetting() *EnforcementSetting {
	NormalizeEnforcementSetting(&enforcementSetting)
	return &enforcementSetting
}

// IsEnforcementSourceEnabled centralises the gate every engine consults
// before forwarding a hit. Mirrors IsRiskControlEnabledForGroup in spirit:
// global switch AND source whitelist must both pass.
func IsEnforcementSourceEnabled(setting *EnforcementSetting, source string) bool {
	if setting == nil || !setting.Enabled {
		return false
	}
	for _, s := range setting.EnabledSources {
		if s == source {
			return true
		}
	}
	return false
}

// EffectiveEnforcementBanThreshold resolves the threshold for a source,
// falling back to the global default when no per-source override exists.
// 0 still means "auto-ban disabled".
func EffectiveEnforcementBanThreshold(setting *EnforcementSetting, source string) int {
	if setting == nil {
		return 0
	}
	if v, ok := setting.BanThresholdPerSource[source]; ok && v > 0 {
		return v
	}
	return setting.BanThreshold
}
