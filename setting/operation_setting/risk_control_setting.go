package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	RiskControlModeOff         = "off"
	RiskControlModeObserveOnly = "observe_only"
	RiskControlModeEnforce     = "enforce"
)

type RiskControlSetting struct {
	Enabled                 bool   `json:"enabled"`
	Mode                    string `json:"mode"`
	TrustedIPHeaderEnabled  bool   `json:"trusted_ip_header_enabled"`
	TrustedIPHeader         string `json:"trusted_ip_header"`
	EventQueueSize          int    `json:"event_queue_size"`
	WorkerCount             int    `json:"worker_count"`
	LocalCacheSeconds       int    `json:"local_cache_seconds"`
	RedisTimeoutMS          int    `json:"redis_timeout_ms"`
	DefaultStatusCode       int    `json:"default_status_code"`
	DefaultResponseMessage  string `json:"default_response_message"`
	DefaultRecoverMode      string `json:"default_recover_mode"`
	DefaultRecoverAfterSecs int    `json:"default_recover_after_secs"`
	SnapshotRetentionHours  int    `json:"snapshot_retention_hours"`
}

var riskControlSetting = RiskControlSetting{
	Enabled:                 false,
	Mode:                    RiskControlModeObserveOnly,
	TrustedIPHeaderEnabled:  false,
	TrustedIPHeader:         "X-Real-IP",
	EventQueueSize:          8192,
	WorkerCount:             2,
	LocalCacheSeconds:       2,
	RedisTimeoutMS:          30,
	DefaultStatusCode:       429,
	DefaultResponseMessage:  "当前请求触发风控，请稍后再试",
	DefaultRecoverMode:      "ttl",
	DefaultRecoverAfterSecs: 900,
	SnapshotRetentionHours:  72,
}

func NormalizeRiskControlSetting(setting *RiskControlSetting) {
	if setting == nil {
		return
	}
	if setting.EventQueueSize <= 0 {
		setting.EventQueueSize = 8192
	}
	if setting.WorkerCount <= 0 {
		setting.WorkerCount = 2
	}
	if setting.LocalCacheSeconds <= 0 {
		setting.LocalCacheSeconds = 2
	}
	if setting.RedisTimeoutMS <= 0 {
		setting.RedisTimeoutMS = 30
	}
	if setting.DefaultStatusCode <= 0 {
		setting.DefaultStatusCode = 429
	}
	if setting.DefaultRecoverAfterSecs <= 0 {
		setting.DefaultRecoverAfterSecs = 900
	}
	if setting.DefaultRecoverMode == "" {
		setting.DefaultRecoverMode = "ttl"
	}
	if setting.DefaultResponseMessage == "" {
		setting.DefaultResponseMessage = "当前请求触发风控，请稍后再试"
	}
	setting.TrustedIPHeader = strings.TrimSpace(setting.TrustedIPHeader)
	switch setting.Mode {
	case RiskControlModeOff, RiskControlModeObserveOnly, RiskControlModeEnforce:
	default:
		setting.Mode = RiskControlModeObserveOnly
	}
	if setting.SnapshotRetentionHours <= 0 {
		setting.SnapshotRetentionHours = 72
	}
}

func init() {
	config.GlobalConfig.Register("risk_control", &riskControlSetting)
}

func GetRiskControlSetting() *RiskControlSetting {
	NormalizeRiskControlSetting(&riskControlSetting)
	return &riskControlSetting
}
