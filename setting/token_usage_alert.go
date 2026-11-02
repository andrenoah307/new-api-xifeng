package setting

import (
	"errors"
	"regexp"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const TokenUsageAlertOption = "TokenUsageAlertRules"

type TokenUsageAlertRule struct {
	ID           string `json:"id"`
	Percent      int    `json:"percent"`
	CooldownDays int    `json:"cooldown_days"`
	Scope        string `json:"scope"`
}

type TokenUsageAlertConfig struct {
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
	if len(raw) > 16384 || common.UnmarshalJsonStr(raw, &config) != nil {
		return config, errors.New("Invalid API Key usage alert settings")
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
