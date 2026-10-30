package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

const ChannelCostContextKey = "channel_cost_settings"

// ChannelCostSettings is deliberately excluded from ordinary channel JSON.
// Only the dedicated root endpoint can read or update this configuration.
type ChannelCostSettings struct {
	Mode          string                   `json:"mode"`
	Ratio         *float64                 `json:"ratio"`
	Upstream      *UpstreamCostDeclaration `json:"upstream,omitempty"`
	LastAttemptAt int64                    `json:"last_attempt_at,omitempty"`
	SyncStatus    string                   `json:"sync_status,omitempty"`
	AppliedPeak   float64                  `json:"-"`
	Identity      string                   `json:"identity,omitempty"`
	UpdatedAt     int64                    `json:"updated_at"`
	Version       string                   `json:"version"`
}

func (s ChannelCostSettings) Validate() error {
	if s.Mode != "" && s.Mode != "disabled" && s.Mode != "manual" && s.Mode != "auto" {
		return errors.New("invalid cost mode")
	}
	if s.Mode == "manual" && s.Ratio == nil {
		return errors.New("missing cost ratio")
	}
	if s.Ratio != nil && (math.IsNaN(*s.Ratio) || math.IsInf(*s.Ratio, 0) || *s.Ratio < 0 || *s.Ratio > 10000) {
		return errors.New("invalid cost ratio")
	}
	return nil
}

type UpstreamCostDeclaration struct {
	Object        string    `json:"object"`
	SchemaVersion int       `json:"schema_version"`
	BillingScope  string    `json:"billing_scope"`
	GroupRate     *float64  `json:"group_rate_multiplier"`
	UserRate      *float64  `json:"user_rate_multiplier,omitempty"`
	ResolvedRate  *float64  `json:"resolved_rate_multiplier"`
	PeakEnabled   *bool     `json:"peak_rate_enabled"`
	PeakStart     string    `json:"peak_start,omitempty"`
	PeakEnd       string    `json:"peak_end,omitempty"`
	PeakRate      *float64  `json:"peak_rate_multiplier,omitempty"`
	AppliedPeak   *float64  `json:"applied_peak_multiplier,omitempty"`
	EffectiveRate *float64  `json:"effective_rate_multiplier"`
	Timezone      string    `json:"timezone,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
}

func (d *UpstreamCostDeclaration) PeakAt(at time.Time) (float64, bool) {
	if d == nil || d.PeakEnabled == nil {
		return 0, false
	}
	if !*d.PeakEnabled {
		return 1, true
	}
	start, err := time.Parse("15:04", d.PeakStart)
	if err != nil {
		return 0, false
	}
	end, err := time.Parse("15:04", d.PeakEnd)
	if err != nil || !start.Before(end) || d.PeakRate == nil || *d.PeakRate <= 0 || *d.PeakRate > 100 || math.IsNaN(*d.PeakRate) {
		return 0, false
	}
	location, err := time.LoadLocation(d.Timezone)
	if err != nil || d.Timezone == "" {
		return 0, false
	}
	local := at.In(location)
	minute := local.Hour()*60 + local.Minute()
	if minute >= start.Hour()*60+start.Minute() && minute < end.Hour()*60+end.Minute() {
		return *d.PeakRate, true
	}
	return 1, true
}

func (s ChannelCostSettings) SnapshotAt(at time.Time) ChannelCostSettings {
	if s.Mode != "auto" {
		return s
	}
	peak, ok := s.Upstream.PeakAt(at)
	if !ok || s.Ratio == nil {
		s.Ratio = nil
		return s
	}
	ratio := *s.Ratio * peak
	s.Ratio, s.AppliedPeak = &ratio, peak
	return s
}

func (channel *Channel) GetCostSettings() ChannelCostSettings {
	var settings ChannelCostSettings
	if channel.CostSettings != "" {
		if common.UnmarshalJsonStr(channel.CostSettings, &settings) != nil {
			return ChannelCostSettings{}
		}
	}
	if settings.Mode == "auto" && settings.Identity != channel.CostIdentity() {
		settings.Ratio, settings.Upstream = nil, nil
		settings.SyncStatus, settings.LastAttemptAt = "changed", 0
	}
	return settings
}

func (channel *Channel) CostIdentity() string {
	data, _ := common.Marshal([]any{channel.Type, channel.Key, channel.GetBaseURL(), channel.GetSetting().Proxy, channel.ChannelInfo.IsMultiKey})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Cost is an estimate in pricing USD, independent of customer group discounts.
// Strings preserve decimal precision; missing amounts mean unknown, never zero.
type LogCost struct {
	Version        int      `json:"version"`
	Status         string   `json:"status"`
	BaseUSD        string   `json:"base_usd,omitempty"`
	AmountUSD      string   `json:"amount_usd,omitempty"`
	Ratio          *float64 `json:"ratio,omitempty"`
	Source         string   `json:"source"`
	ConfigVersion  string   `json:"config_version,omitempty"`
	RatioUpdatedAt int64    `json:"ratio_updated_at,omitempty"`
	PeakMultiplier float64  `json:"peak_multiplier,omitempty"`
	Basis          string   `json:"basis"`
}

func BuildChannelCost(settings ChannelCostSettings, baseUSD decimal.Decimal, known bool) *LogCost {
	if settings.Mode == "" || settings.Mode == "disabled" {
		return nil
	}
	cost := &LogCost{Version: 1, Status: "unknown", Source: settings.Mode, ConfigVersion: settings.Version, RatioUpdatedAt: settings.UpdatedAt, Basis: "model_price_excluding_tools"}
	if settings.Validate() != nil || settings.Ratio == nil || !known || baseUSD.IsNegative() {
		return cost
	}
	ratio := *settings.Ratio
	cost.Ratio = &ratio
	cost.PeakMultiplier = settings.AppliedPeak
	cost.BaseUSD = baseUSD.String()
	cost.AmountUSD = baseUSD.Mul(decimal.NewFromFloat(ratio)).String()
	cost.Status = "estimated"
	if settings.Mode == "auto" && time.Now().Unix()-settings.UpdatedAt > 3600 {
		cost.Status = "stale"
	}
	return cost
}

func FilterLogCosts(logs []*Log, root bool) {
	if root {
		return
	}
	for _, log := range logs {
		if log == nil || log.Other == "" {
			continue
		}
		var other map[string]json.RawMessage
		if common.UnmarshalJsonStr(log.Other, &other) != nil || other == nil {
			log.Other = "{}"
			continue
		}
		delete(other, "root_cost")
		raw, err := common.Marshal(other)
		if err != nil {
			log.Other = "{}"
			continue
		}
		log.Other = string(raw)
	}
}
