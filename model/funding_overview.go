package model

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

var ErrFundingOverviewUnavailable = errors.New("funding overview unavailable")

// FundingOverview compares gross receipts and audited credits, not wallet balances.
type FundingOverview struct {
	StartTimestamp   int64    `json:"start_timestamp"`
	EndTimestamp     int64    `json:"end_timestamp"`
	CalculatedAt     int64    `json:"calculated_at"`
	Consumption      string   `json:"consumption"`
	OnlineTopUps     string   `json:"online_topups"`
	ManualCredits    string   `json:"manual_credits"`
	TotalFunding     string   `json:"total_funding"`
	ConsumptionRatio *float64 `json:"consumption_ratio"`
}

func ValidFundingOverviewRange(start, end, now int64) bool {
	return start > 0 && end > start && end <= now && end-start <= 31*86400 && start%3600 == 0 && end%3600 == 0
}

func QueryFundingOverview(ctx context.Context, start, end int64) (FundingOverview, error) {
	result := FundingOverview{StartTimestamp: start, EndTimestamp: end}
	if !ValidFundingOverviewRange(start, end, time.Now().Unix()) || !common.DataExportEnabled {
		return result, ErrFundingOverviewUnavailable
	}
	unit, exchangeRate := common.QuotaPerUnit, operation_setting.USDExchangeRate
	for _, value := range []float64{unit, exchangeRate} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return result, ErrFundingOverviewUnavailable
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var usage struct{ Quota decimal.Decimal }
	if err := DB.WithContext(ctx).Model(&QuotaData{}).Select("COALESCE(SUM(quota), 0) AS quota").
		Where("created_at >= ? AND created_at < ?", start, end).Scan(&usage).Error; err != nil {
		return result, err
	}
	// Currency is not snapshotted for every provider. Never silently add USD or
	// unknown currencies to CNY receipts. Only project-confirmed CNY orders qualify.
	var orders []struct {
		Source          string
		PaymentProvider string
		PaymentMethod   string
		CheckoutVersion int
		CreatedBy       int
		Money           decimal.Decimal
		Quota           decimal.Decimal
	}
	if err := DB.WithContext(ctx).Model(&TopUp{}).
		Select("source, payment_provider, payment_method, checkout_version, created_by, SUM(CAST(money AS DECIMAL(30,6))) AS money, SUM(quota_granted) AS quota").
		Where("status = ? AND complete_time >= ? AND complete_time < ? AND (source IS NULL OR source <> ?)", common.TopUpStatusSuccess, start, end, "discount_bonus").
		Group("source, payment_provider, payment_method, checkout_version, created_by").Scan(&orders).Error; err != nil {
		return result, err
	}
	rate := decimal.NewFromFloat(exchangeRate).Div(decimal.NewFromFloat(unit))
	online, manual := decimal.Zero, decimal.Zero
	for _, order := range orders {
		if !IsCNYInvoiceTopUp(&TopUp{PaymentProvider: order.PaymentProvider, PaymentMethod: order.PaymentMethod, CheckoutVersion: order.CheckoutVersion, CreatedBy: order.CreatedBy}) {
			return result, ErrFundingOverviewUnavailable
		}
		if order.Source == "manual" || order.PaymentProvider == "manual" {
			manual = manual.Add(order.Quota.Mul(rate))
		} else {
			online = online.Add(order.Money)
		}
	}
	// Manage logs are sparse and indexed by (type, created_at). No consume-log scan
	// and no provider-specific JSON SQL; the same parser works on all databases.
	var audit []struct{ Other string }
	if err := LOG_DB.WithContext(ctx).Model(&Log{}).Select("other").
		Where("type = ? AND created_at >= ? AND created_at < ?", LogTypeManage, start, end).
		Limit(20001).Find(&audit).Error; err != nil {
		return result, err
	}
	if len(audit) > 20000 {
		return result, ErrFundingOverviewUnavailable
	}
	for _, row := range audit {
		if row.Other == "" {
			continue
		}
		var entry struct {
			Op struct {
				Action string          `json:"action"`
				Params json.RawMessage `json:"params"`
			} `json:"op"`
		}
		if err := common.UnmarshalJsonStr(row.Other, &entry); err != nil {
			return result, err
		}
		if entry.Op.Action != "user.quota_add" && entry.Op.Action != "user.quota_override" {
			continue
		}
		var params struct {
			Quota *decimal.Decimal `json:"quota"`
			From  *decimal.Decimal `json:"from"`
			To    *decimal.Decimal `json:"to"`
		}
		if err := common.Unmarshal(entry.Op.Params, &params); err != nil {
			return result, err
		}
		credit := decimal.Zero
		switch entry.Op.Action {
		case "user.quota_add":
			if params.Quota == nil {
				return result, ErrFundingOverviewUnavailable
			}
			credit = *params.Quota
		case "user.quota_override":
			if params.From == nil || params.To == nil {
				return result, ErrFundingOverviewUnavailable
			}
			credit = params.To.Sub(*params.From)
		}
		if credit.IsPositive() {
			manual = manual.Add(credit.Mul(rate))
		}
	}
	consumption := usage.Quota.Mul(rate)
	total := online.Add(manual)
	result.Consumption = consumption.StringFixed(6)
	result.OnlineTopUps = online.StringFixed(6)
	result.ManualCredits = manual.StringFixed(6)
	result.TotalFunding = total.StringFixed(6)
	if total.IsPositive() {
		ratio, _ := consumption.Div(total).Float64()
		result.ConsumptionRatio = &ratio
	}
	result.CalculatedAt = time.Now().Unix()
	return result, nil
}
