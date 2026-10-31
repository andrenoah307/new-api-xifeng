package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrCostReportConfigChanged = errors.New("channel cost settings changed; review the channel list again")

type CostReportConfiguredChannel struct {
	CostReportRate
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	Version   string `json:"version"`
	UpdatedAt int64  `json:"updated_at"`
}

type CostReportConfigPreview struct {
	Channels     []CostReportConfiguredChannel `json:"channels"`
	Skipped      int                           `json:"skipped"`
	Fingerprint  string                        `json:"fingerprint"`
	QuotaPerUnit string                        `json:"quota_per_unit"`
}

// Load only cost/identity fields. Keys are used to validate automatic account
// identity internally and must never be included in the preview or job policy.
func PreviewChannelCostReportConfig(ctx context.Context, channelID int) (CostReportConfigPreview, error) {
	preview := CostReportConfigPreview{Channels: []CostReportConfiguredChannel{}, QuotaPerUnit: decimal.NewFromFloat(common.QuotaPerUnit).String()}
	if channelID < 0 {
		return preview, errors.New("invalid channel")
	}
	var channels []Channel
	query := DB.WithContext(ctx).Select("id", "name", "type", "key", "base_url", "setting", "channel_info", "cost_settings").Order("id ASC").Limit(5001)
	if channelID > 0 {
		query = query.Where("id = ?", channelID)
	}
	if err := query.Find(&channels).Error; err != nil {
		return preview, err
	}
	if len(channels) > 5000 {
		return preview, errors.New("cost report exceeds channel limit")
	}
	for _, channel := range channels {
		settings := channel.GetCostSettings()
		if (settings.Mode != "manual" && settings.Mode != "auto") || settings.Validate() != nil || settings.Ratio == nil {
			preview.Skipped++
			continue
		}
		rate := CostReportRate{ChannelID: channel.Id, Ratio: decimal.NewFromFloat(*settings.Ratio).String()}
		if settings.Mode == "auto" {
			if channel.ChannelInfo.IsMultiKey || settings.SnapshotAt(time.Now()).Ratio == nil {
				preview.Skipped++
				continue
			}
			if *settings.Upstream.PeakEnabled {
				rate.Peak = &CostReportPeak{Start: settings.Upstream.PeakStart, End: settings.Upstream.PeakEnd,
					Timezone: settings.Upstream.Timezone, Ratio: decimal.NewFromFloat(*settings.Upstream.PeakRate).String()}
			}
		}
		if ValidateCostReportPolicy(CostReportPolicy{Start: 1, End: 2, QuotaPerUnit: "500000", FallbackMode: "billed_basis", Rates: []CostReportRate{rate}}) != nil {
			preview.Skipped++
			continue
		}
		preview.Channels = append(preview.Channels, CostReportConfiguredChannel{CostReportRate: rate, Name: channel.Name, Mode: settings.Mode, Version: settings.Version, UpdatedAt: settings.UpdatedAt})
	}
	encoded, err := common.Marshal([]any{channelID, preview.QuotaPerUnit, preview.Channels, preview.Skipped})
	if err != nil {
		return preview, err
	}
	preview.Fingerprint = fmt.Sprintf("%x", sha256.Sum256(encoded))
	return preview, nil
}

func CreateConfiguredChannelCostReportJob(ctx context.Context, ownerID int, requestKey string, policy CostReportPolicy) (*ChannelCostReportJob, error) {
	if err := AuthorizeChannelCostReportOwner(ctx, ownerID); err != nil {
		return nil, err
	}
	if policy.FallbackMode != "billed_basis" || policy.CostFingerprint == "" || len(policy.Rates) != 0 {
		return nil, ErrCostReportConfigChanged
	}
	// Retry the confirmed snapshot even if the live configuration subsequently
	// changed. Reusing a key with a different request must still be rejected.
	var prior ChannelCostReportJob
	err := DB.WithContext(ctx).Where("request_key = ?", requestKey).First(&prior).Error
	if err == nil {
		var saved CostReportPolicy
		if common.UnmarshalJsonStr(prior.PolicyJSON, &saved) != nil || prior.OwnerID != ownerID || saved.CostFingerprint != policy.CostFingerprint || saved.Start != policy.Start || saved.End != policy.End || saved.ChannelID != policy.ChannelID || saved.FallbackMode != policy.FallbackMode {
			return nil, ErrChannelCostReportConflict
		}
		return &prior, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	preview, err := PreviewChannelCostReportConfig(ctx, policy.ChannelID)
	if err != nil {
		return nil, err
	}
	if preview.Fingerprint != policy.CostFingerprint || len(preview.Channels) == 0 {
		return nil, ErrCostReportConfigChanged
	}
	for _, channel := range preview.Channels {
		policy.Rates = append(policy.Rates, channel.CostReportRate)
	}
	policy.QuotaPerUnit = preview.QuotaPerUnit
	return CreateChannelCostReportJob(ctx, ownerID, requestKey, policy)
}
