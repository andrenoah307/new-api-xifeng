package model

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// ChannelFinanceStat keeps quota amounts as strings, including partial known basis.
// Refund cost requires original-event attribution and is deliberately unresolved.
type ChannelFinanceStat struct {
	ChannelID         int    `json:"channel_id"`
	ConsumeQuota      string `json:"consume_quota"`
	RefundQuota       string `json:"refund_quota"`
	NetQuota          string `json:"net_quota"`
	KnownModelQuota   string `json:"known_model_quota"`
	ConsumeCount      int64  `json:"consume_count"`
	RefundCount       int64  `json:"refund_count"`
	MissingBasis      int64  `json:"missing_basis"`
	UnresolvedRefunds int64  `json:"unresolved_refunds"`
}

func GetChannelFinanceStats(ctx context.Context, start, end int64, channel int) ([]ChannelFinanceStat, error) {
	if start < 1 || end <= start || channel < 0 {
		return nil, errors.New("invalid finance range")
	}
	// Extract only pricing provenance, never group on or return the full other blob.
	var ratio, saturated string
	switch common.LogDatabaseType() {
	case common.DatabaseTypeMySQL:
		doc := "IF(JSON_VALID(other), other, '{}')"
		ratio = "COALESCE(CAST(JSON_EXTRACT(" + doc + ", '$.group_ratio') AS CHAR), '')"
		saturated = "CASE WHEN COALESCE(JSON_TYPE(JSON_EXTRACT(" + doc + ", '$.admin_info.quota_saturation')), 'NULL') <> 'NULL' OR JSON_EXTRACT(" + doc + ", '$.quota_saturated') = true THEN 1 ELSE 0 END"
	case common.DatabaseTypePostgreSQL:
		// PostgreSQL 9.6 has no safe text-to-JSON cast. Malformed JSON fails the
		// whole query rather than silently reporting a partial financial result.
		doc := "CAST(COALESCE(NULLIF(other, ''), '{}') AS jsonb)"
		ratio = "COALESCE(CAST(" + doc + " -> 'group_ratio' AS text), '')"
		saturated = "CASE WHEN COALESCE(" + doc + " #> '{admin_info,quota_saturation}', 'null'::jsonb) <> 'null'::jsonb OR " + doc + " ->> 'quota_saturated' = 'true' THEN 1 ELSE 0 END"
	case common.DatabaseTypeClickHouse:
		doc := "if(isValidJSON(other), other, '{}')"
		ratio = "JSONExtractRaw(" + doc + ", 'group_ratio')"
		saturated = "if(JSONExtractRaw(" + doc + ", 'admin_info', 'quota_saturation') NOT IN ('', 'null') OR JSONExtractBool(" + doc + ", 'quota_saturated'), 1, 0)"
	default:
		doc := "CASE WHEN json_valid(other) THEN other ELSE '{}' END"
		ratio = "CASE WHEN json_type(" + doc + ", '$.group_ratio') IN ('integer', 'real', 'text') THEN CAST(json_extract(" + doc + ", '$.group_ratio') AS TEXT) ELSE '' END"
		saturated = "CASE WHEN COALESCE(json_type(" + doc + ", '$.admin_info.quota_saturation'), 'null') <> 'null' OR json_extract(" + doc + ", '$.quota_saturated') = 1 THEN 1 ELSE 0 END"
	}
	query := LOG_DB.WithContext(ctx).Table("logs")
	// An existing covering type/time index avoids MySQL choosing the entire
	// channel index merely to satisfy GROUP BY. Never create indexes here.
	if channel == 0 && common.UsingLogDatabase(common.DatabaseTypeMySQL) && LOG_DB.WithContext(ctx).Migrator().HasIndex(&Log{}, "idx_logs_type_created_at_quota") {
		query = query.Table("logs FORCE INDEX (idx_logs_type_created_at_quota)")
	}
	if channel > 0 {
		query = query.Where("channel_id = ?", channel)
	}
	query = query.Where("type IN ? AND created_at >= ? AND created_at < ?", []int{LogTypeConsume, LogTypeRefund}, start, end).
		Select(fmt.Sprintf("channel_id, type, %s AS ratio_value, %s AS saturated, COUNT(*) AS records, SUM(quota) AS quota_sum, MIN(quota) AS min_quota", ratio, saturated)).
		Group("channel_id, type, ratio_value, saturated").Limit(10001)
	var groups []struct {
		ChannelID  int
		Type       int
		RatioValue string
		Saturated  int
		Records    int64
		QuotaSum   string
		MinQuota   int64
	}
	if err := query.Scan(&groups).Error; err != nil {
		return nil, err
	}
	if len(groups) > 10000 {
		return nil, errors.New("finance aggregation has too many groups")
	}
	totals := map[int]*ChannelFinanceStat{}
	for _, group := range groups {
		if group.MinQuota < 0 {
			return nil, errors.New("negative source quota")
		}
		quota, err := decimal.NewFromString(group.QuotaSum)
		if err != nil {
			return nil, err
		}
		row := totals[group.ChannelID]
		if row == nil {
			row = &ChannelFinanceStat{ChannelID: group.ChannelID, ConsumeQuota: "0", RefundQuota: "0", NetQuota: "0", KnownModelQuota: "0"}
			totals[group.ChannelID] = row
		}
		if group.Type == LogTypeRefund {
			previous, _ := decimal.NewFromString(row.RefundQuota)
			row.RefundQuota = previous.Add(quota).String()
			row.RefundCount += group.Records
			row.UnresolvedRefunds += group.Records
			continue
		}
		previous, _ := decimal.NewFromString(row.ConsumeQuota)
		row.ConsumeQuota = previous.Add(quota).String()
		row.ConsumeCount += group.Records
		value := group.RatioValue
		if len(value) > 128 {
			row.MissingBasis += group.Records
			continue
		}
		if len(value) > 0 && value[0] == '"' {
			var decoded string
			if common.UnmarshalJsonStr(value, &decoded) == nil {
				value = decoded
			}
		}
		// Bound the decimal exponent before division; malformed metadata is unresolved.
		multiplier, err := decimal.NewFromString(value)
		if err != nil || multiplier.Sign() <= 0 || multiplier.Exponent() < -18 || multiplier.Exponent() > 18 || group.Saturated != 0 {
			row.MissingBasis += group.Records
			continue
		}
		known, _ := decimal.NewFromString(row.KnownModelQuota)
		row.KnownModelQuota = known.Add(quota.DivRound(multiplier, 18)).String()
	}
	result := make([]ChannelFinanceStat, 0, len(totals))
	for _, row := range totals {
		consumed, _ := decimal.NewFromString(row.ConsumeQuota)
		refunded, _ := decimal.NewFromString(row.RefundQuota)
		row.NetQuota = consumed.Sub(refunded).String()
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ChannelID < result[j].ChannelID })
	return result, nil
}
