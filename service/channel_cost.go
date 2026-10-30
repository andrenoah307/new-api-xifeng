package service

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// modelCostUSD uses the same normalized usage and price snapshot as settlement,
// before customer group discounts, rounding, and quota saturation.
func modelCostUSD(info *relaycommon.RelayInfo, base decimal.Decimal, tiered *billingexpr.TieredResult, known bool) *string {
	unit := common.QuotaPerUnit
	if !known || info.ZeroChargeGuardTriggered || info.QuotaClamp != nil || common.QuotaPerUnit <= 0 {
		return nil
	}
	if info.TieredBillingSnapshot != nil {
		if tiered == nil || math.IsNaN(tiered.ActualQuotaBeforeGroup) || math.IsInf(tiered.ActualQuotaBeforeGroup, 0) {
			return nil
		}
		base = decimal.NewFromFloat(tiered.ActualQuotaBeforeGroup)
		unit = info.TieredBillingSnapshot.QuotaPerUnit
	}
	if base.IsNegative() || unit <= 0 || math.IsNaN(unit) || math.IsInf(unit, 0) {
		return nil
	}
	amount := base.Div(decimal.NewFromFloat(unit)).String()
	return &amount
}

func audioModelCostQuota(info QuotaInfo) decimal.Decimal {
	if info.UsePrice {
		return decimal.NewFromFloat(info.ModelPrice).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	completion := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
	audio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
	audioCompletion := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))
	return decimal.NewFromInt(int64(info.InputDetails.TextTokens)).
		Add(decimal.NewFromInt(int64(info.OutputDetails.TextTokens)).Mul(completion)).
		Add(decimal.NewFromInt(int64(info.InputDetails.AudioTokens)).Mul(audio)).
		Add(decimal.NewFromInt(int64(info.OutputDetails.AudioTokens)).Mul(audio).Mul(audioCompletion)).
		Mul(decimal.NewFromFloat(info.ModelRatio))
}

// FetchChannelCostRatio implements Sub2API's key-scoped billing declaration.
func FetchChannelCostRatio(ctx context.Context, channel *model.Channel, settings model.ChannelCostSettings) (*model.UpstreamCostDeclaration, error) {
	if settings.Mode != "auto" || settings.Validate() != nil || channel.ChannelInfo.IsMultiKey || strings.ContainsAny(channel.Key, "\r\n") || strings.TrimSpace(channel.Key) == "" {
		return nil, errors.New("cost sync unavailable")
	}
	base, err := url.Parse(channel.GetBaseURL())
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("invalid cost upstream")
	}
	// Endpoints are relative to the configured upstream installation, including
	// reverse-proxy subpaths. Never follow redirects with an upstream API key.
	base.Path = strings.TrimRight(base.Path, "/")
	if !strings.HasSuffix(base.Path, "/sub2api/billing") {
		if !strings.HasSuffix(base.Path, "/v1") {
			base.Path += "/v1"
		}
		base.Path += "/sub2api/billing"
	}
	base.RawPath = ""
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, errors.New("cost sync unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+channel.Key)
	req.Header.Set("Accept", "application/json")
	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil || client == nil {
		return nil, errors.New("cost sync unavailable")
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := copyClient.Do(req)
	if err != nil {
		return nil, errors.New("cost sync unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("cost sync unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 {
		return nil, errors.New("invalid cost response")
	}
	var declaration model.UpstreamCostDeclaration
	if common.Unmarshal(body, &declaration) != nil {
		return nil, errors.New("invalid cost response")
	}
	if declaration.Object != "sub2api.key_billing" || declaration.SchemaVersion != 1 || declaration.BillingScope != "token" || declaration.ObservedAt.IsZero() {
		return nil, errors.New("unsupported cost declaration")
	}
	for _, value := range []*float64{declaration.GroupRate, declaration.ResolvedRate, declaration.EffectiveRate} {
		if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 10000 {
			return nil, errors.New("invalid cost ratio")
		}
	}
	if *declaration.ResolvedRate <= 0 || *declaration.ResolvedRate > 100 {
		return nil, errors.New("invalid automatic cost ratio")
	}
	expected := *declaration.GroupRate
	if declaration.UserRate != nil {
		expected = *declaration.UserRate
	}
	if math.IsNaN(expected) || math.IsInf(expected, 0) || math.Abs(expected-*declaration.ResolvedRate) > 1e-8 {
		return nil, errors.New("inconsistent base cost ratio")
	}
	peak, ok := declaration.PeakAt(declaration.ObservedAt)
	if !ok || math.Abs(*declaration.EffectiveRate-*declaration.ResolvedRate*peak) > 1e-8 {
		return nil, errors.New("inconsistent peak cost ratio")
	}
	if *declaration.PeakEnabled && declaration.AppliedPeak == nil {
		return nil, errors.New("missing peak cost ratio")
	}
	if declaration.AppliedPeak != nil && (math.IsNaN(*declaration.AppliedPeak) || math.Abs(*declaration.AppliedPeak-peak) > 1e-8) {
		return nil, errors.New("inconsistent peak cost ratio")
	}
	if time.Since(declaration.ObservedAt) > time.Hour || time.Until(declaration.ObservedAt) > 5*time.Minute {
		return nil, errors.New("stale upstream declaration")
	}
	return &declaration, nil
}

// Compare the original configuration and credential so a slow upstream reply
// cannot overwrite a newer manual choice or a different account's settings.
func SaveChannelCost(ctx context.Context, channel *model.Channel, settings model.ChannelCostSettings) error {
	if settings.Mode == "auto" {
		settings.Identity = channel.CostIdentity()
	} else {
		settings.Identity = ""
	}
	settings.Version = uuid.NewString()
	raw, err := common.Marshal(settings)
	if err != nil {
		return err
	}
	// key is reserved in MySQL; let GORM quote map keys.
	query := model.DB.WithContext(ctx).Model(&model.Channel{}).Where(map[string]any{"id": channel.Id, "key": channel.Key, "type": channel.Type, "base_url": channel.BaseURL, "setting": channel.Setting})
	if channel.CostSettings == "" {
		query = query.Where("cost_settings IS NULL OR cost_settings = ?", "")
	} else {
		query = query.Where("cost_settings = ?", channel.CostSettings)
	}
	result := query.Update("cost_settings", string(raw))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("cost configuration changed")
	}
	return nil
}

func StartChannelCostSync() {
	go func() {
		for range time.NewTicker(time.Minute).C {
			if !common.IsMasterNode {
				continue
			}
			var channels []*model.Channel
			if model.DB.Where("cost_settings IS NOT NULL AND cost_settings <> ?", "").Find(&channels).Error != nil {
				continue
			}
			sort.Slice(channels, func(i, j int) bool {
				return channels[i].GetCostSettings().LastAttemptAt < channels[j].GetCostSettings().LastAttemptAt
			})
			attempts := 0
			changed := false
			for _, channel := range channels {
				settings := channel.GetCostSettings()
				if attempts >= 20 {
					break
				}
				if settings.Mode != "auto" || time.Now().Unix()-settings.LastAttemptAt < 1800 {
					continue
				}
				attempts++
				declaration, err := FetchChannelCostRatio(context.Background(), channel, settings)
				settings.LastAttemptAt = time.Now().Unix()
				settings.SyncStatus = "failed"
				if err == nil {
					settings.Ratio, settings.UpdatedAt = declaration.ResolvedRate, time.Now().Unix()
					settings.Upstream, settings.SyncStatus = declaration, "ok"
				}
				if SaveChannelCost(context.Background(), channel, settings) == nil {
					changed = true
				}
			}
			if changed {
				model.InitChannelCache()
			}
		}
	}()
}
