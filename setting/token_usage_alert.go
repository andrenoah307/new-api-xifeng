package setting

import (
	"errors"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const TokenUsageAlertOption = "TokenUsageAlertRules"

const TokenUsageAlertSubject = "{{system_name}} · API Key 用量告警"
const TokenUsageAlertBody = `<h2>API Key 用量告警</h2><p>{{username}}，您好：</p><p>您的 API Key「{{key_name}}」（ID：{{key_id}}）{{scope}}使用比例已达到 {{usage_percent}}%，触发 {{threshold_percent}}% 告警规则。</p><table width="100%" cellpadding="8" style="border-collapse:collapse;overflow-wrap:anywhere"><tr><td>已用额度</td><td>{{used_quota}}</td></tr><tr><td>额度限额</td><td>{{quota_limit}}</td></tr><tr><td>计价单位</td><td>{{quota_unit}}</td></tr><tr><td>邮件间隔</td><td>{{cooldown_days}} 天</td></tr><tr><td>周期重置时间</td><td>{{reset_at}}</td></tr></table><p>请及时查看用量并调整额度。</p><p style="font-size:12px;color:#666">统计时间：{{generated_at}}（北京时间）。用量沿用 Key 账面额度，可能包含在途预扣；周期重置不重置本规则邮件间隔。</p>`

var TokenUsageAlertPlaceholders = []string{"system_name", "username", "key_name", "key_id", "scope", "used_quota", "quota_limit", "quota_unit", "usage_percent", "threshold_percent", "cooldown_days", "reset_at", "generated_at"}

type TokenUsageAlertRule struct {
	ID           string `json:"id"`
	Percent      int    `json:"percent"`
	CooldownDays int    `json:"cooldown_days"`
	Scope        string `json:"scope"`
}

type TokenUsageAlertConfig struct {
	Subject string                `json:"subject"`
	Body    string                `json:"body"`
	Enabled bool                  `json:"enabled"`
	Rules   []TokenUsageAlertRule `json:"rules"`
}

var tokenUsageAlerts atomic.Pointer[TokenUsageAlertConfig]
var tokenUsageAlertID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,48}$`)

func init() {
	tokenUsageAlerts.Store(&TokenUsageAlertConfig{Rules: []TokenUsageAlertRule{
		{ID: "warning", Percent: 50, CooldownDays: 7, Scope: "any"},
		{ID: "critical", Percent: 70, CooldownDays: 3, Scope: "any"},
	}})
}

func ParseTokenUsageAlerts(raw string) (TokenUsageAlertConfig, error) {
	var config TokenUsageAlertConfig
	if len(raw) > 65536 || common.UnmarshalJsonStr(raw, &config) != nil {
		return config, errors.New("Invalid API Key usage alert settings")
	}
	if strings.TrimSpace(config.Subject) == "" {
		config.Subject = TokenUsageAlertSubject
	}
	if strings.TrimSpace(config.Body) == "" {
		config.Body = TokenUsageAlertBody
	}
	if len(config.Subject) > 800 || len(config.Body) > 32768 || strings.ContainsAny(config.Subject, "\r\n") {
		return config, errors.New("Invalid API Key usage alert settings")
	}
	for _, template := range []string{config.Subject, config.Body} {
		rest := template
		for {
			start := strings.Index(rest, "{{")
			if start < 0 {
				if strings.Contains(rest, "}}") {
					return config, errors.New("Invalid API Key usage alert settings")
				}
				break
			}
			if strings.Contains(rest[:start], "}}") {
				return config, errors.New("Invalid API Key usage alert settings")
			}
			end := strings.Index(rest[start+2:], "}}")
			if end < 0 {
				return config, errors.New("Invalid API Key usage alert settings")
			}
			key := strings.TrimSpace(rest[start+2 : start+2+end])
			valid := false
			for _, allowed := range TokenUsageAlertPlaceholders {
				if key == allowed {
					valid = true
					break
				}
			}
			if !valid {
				return config, errors.New("Invalid API Key usage alert settings")
			}
			rest = rest[start+end+4:]
		}
	}
	if len(config.Rules) > 20 || (config.Enabled && len(config.Rules) == 0) {
		return config, errors.New("API Key usage alerts require 1 to 20 rules")
	}
	ids := map[string]bool{}
	for i, r := range config.Rules {
		if !tokenUsageAlertID.MatchString(r.ID) || ids[r.ID] || r.Percent < 1 || r.Percent > 100 || r.CooldownDays < 1 || r.CooldownDays > 365 || (r.Scope != "any" && r.Scope != "total" && r.Scope != "period") {
			return config, errors.New("Invalid API Key usage alert rule")
		}
		ids[r.ID] = true
		for _, prior := range config.Rules[:i] {
			if prior.Percent == r.Percent && (prior.Scope == r.Scope || prior.Scope == "any" || r.Scope == "any") {
				return config, errors.New("Duplicate API Key usage alert threshold")
			}
		}
	}
	return config, nil
}

func GetTokenUsageAlerts() TokenUsageAlertConfig {
	config := *tokenUsageAlerts.Load()
	if config.Subject == "" {
		config.Subject = TokenUsageAlertSubject
	}
	if config.Body == "" {
		config.Body = TokenUsageAlertBody
	}
	config.Rules = append([]TokenUsageAlertRule(nil), config.Rules...)
	return config
}

func TokenUsageAlertsEnabled() bool { return tokenUsageAlerts.Load().Enabled }

func TokenUsageAlertsJSON() string {
	data, _ := common.Marshal(GetTokenUsageAlerts())
	return string(data)
}

func UpdateTokenUsageAlerts(raw string) error {
	config, err := ParseTokenUsageAlerts(raw)
	if err != nil {
		return err
	}
	tokenUsageAlerts.Store(&config)
	return nil
}
