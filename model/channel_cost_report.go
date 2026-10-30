package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

type CostReportRate struct {
	ChannelID int    `json:"channel_id"`
	Ratio     string `json:"ratio"`
}
type CostReportPolicy struct {
	Start        int64            `json:"start"`
	End          int64            `json:"end"`
	ChannelID    int              `json:"channel_id"`
	QuotaPerUnit string           `json:"quota_per_unit"`
	Rates        []CostReportRate `json:"rates"`
	FallbackMode string           `json:"fallback_mode"`
}
type ChannelCostReportRow struct {
	ChannelID      int    `json:"channel_id"`
	ConsumeCount   int64  `json:"consume_count"`
	RefundCount    int64  `json:"refund_count"`
	OriginalCount  int64  `json:"original_count"`
	FallbackCount  int64  `json:"fallback_count"`
	MissingCount   int64  `json:"missing_count"`
	StaleCount     int64  `json:"stale_count"`
	ConsumeAmount  string `json:"consume_amount"`
	RefundAmount   string `json:"refund_amount"`
	OriginalCost   string `json:"original_cost"`
	FallbackCost   string `json:"fallback_cost"`
	KnownRevenue   string `json:"known_revenue"`
	UnknownRevenue string `json:"unknown_revenue"`
}

// Fixed decimal bounds also protect ClickHouse Decimal256 arithmetic from overflow.
const costReportDecimalPattern = `^[0-9]{1,18}([.][0-9]{1,18}){0,1}$`
const costReportSourceDecimalPattern = `^[0-9]{1,18}([.][0-9]{1,54}){0,1}$`

var costReportDecimalRE = regexp.MustCompile(costReportDecimalPattern)
var costReportSourceDecimalRE = regexp.MustCompile(costReportSourceDecimalPattern)
var costReportAggregateDecimalRE = regexp.MustCompile(`^[0-9]{1,60}([.][0-9]{1,18}){0,1}$`)
var costReportUnsupportedRE = regexp.MustCompile(`(?i)(tool|search|audio|image|video|tiered|expression|extra_ratio)`)

func costReportDecimal(value string) (decimal.Decimal, bool) {
	if !costReportDecimalRE.MatchString(value) {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(value)
	return d, err == nil
}

// Snapshot multiplication can exceed report precision. Preserve its provenance,
// even when truncation to the report's 18 decimal places produces zero.
func costReportSourceDecimal(value string) (decimal.Decimal, bool) {
	if !costReportSourceDecimalRE.MatchString(value) {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(value)
	return d.Truncate(18), err == nil
}

func ValidateCostReportPolicy(p CostReportPolicy) error {
	q, ok := costReportDecimal(p.QuotaPerUnit)
	if p.Start < 1 || p.End <= p.Start || p.End > time.Now().Unix() || p.ChannelID < 0 || !ok || q.LessThan(decimal.NewFromInt(1)) || q.GreaterThan(decimal.NewFromInt(1000000000000)) || (p.FallbackMode != "none" && p.FallbackMode != "billed_basis") || len(p.Rates) > 5000 {
		return errors.New("invalid cost report policy")
	}
	if (p.FallbackMode == "none" && len(p.Rates) != 0) || (p.FallbackMode == "billed_basis" && len(p.Rates) == 0) {
		return errors.New("cost report rates do not match fallback mode")
	}
	seen := map[int]bool{}
	for _, r := range p.Rates {
		d, ok := costReportDecimal(r.Ratio)
		if r.ChannelID <= 0 || (p.ChannelID > 0 && r.ChannelID != p.ChannelID) || seen[r.ChannelID] || !ok || d.GreaterThan(decimal.NewFromInt(10000)) {
			return errors.New("invalid or duplicate cost report rate")
		}
		seen[r.ChannelID] = true
	}
	return nil
}

// Locate the next relevant timestamp using the source time index, not OFFSET or
// a JSON scan. An empty result proves the remaining interval empty at read time.
func NextChannelCostReportTimestamp(ctx context.Context, p CostReportPolicy, start int64) (int64, error) {
	if err := ValidateCostReportPolicy(p); err != nil {
		return 0, err
	}
	if start < p.Start || start >= p.End {
		return 0, errors.New("invalid cost report cursor")
	}
	var timestamps []int64
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// Match the MergeTree ordering expression. Keep all channels here because
		// the aggregation also checks conflicting event identities across channels.
		err := LOG_DB.WithContext(ctx).Raw(`SELECT created_at FROM logs
 WHERE ifNull(created_at,0) >= ? AND ifNull(created_at,0) < ? AND type IN (2,6)
 ORDER BY ifNull(created_at,0) ASC LIMIT 1
 SETTINGS optimize_read_in_order=1,max_threads=2,max_memory_usage=536870912,max_execution_time=30,
 timeout_overflow_mode='throw',max_rows_to_read=5000000,read_overflow_mode='throw'`, start, p.End).Scan(&timestamps).Error
		if err != nil {
			return 0, err
		}
	} else {
		query := LOG_DB.WithContext(ctx).Model(&Log{}).
			Where("created_at >= ? AND created_at < ? AND type IN ?", start, p.End, []int{2, 6})
		if p.ChannelID > 0 {
			query = query.Where("channel_id = ?", p.ChannelID)
		}
		if err := query.Order("created_at ASC").Limit(1).Pluck("created_at", &timestamps).Error; err != nil {
			return 0, err
		}
	}
	if len(timestamps) == 0 {
		return p.End, nil
	}
	if timestamps[0] < start || timestamps[0] >= p.End {
		return 0, errors.New("invalid next cost report timestamp")
	}
	return timestamps[0], nil
}

// AggregateChannelCostWindow reads one bounded immutable report window. It never
// writes source logs. Late arrivals require a new report generation.
func AggregateChannelCostWindow(ctx context.Context, p CostReportPolicy, start, end int64) ([]ChannelCostReportRow, error) {
	if err := ValidateCostReportPolicy(p); err != nil {
		return nil, err
	}
	if start < p.Start || end > p.End || end <= start || end-start > 3600 {
		return nil, errors.New("invalid cost report window")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return aggregateClickHouseCostWindow(ctx, p, start, end)
	}
	query := LOG_DB.WithContext(ctx).Model(&Log{}).Select("id, channel_id, type, quota, other").Where("created_at >= ? AND created_at < ? AND type IN ?", start, end, []int{2, 6})
	if p.ChannelID > 0 {
		query = query.Where("channel_id = ?", p.ChannelID)
	}
	rows, err := query.Limit(5000001).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	q, _ := costReportDecimal(p.QuotaPerUnit)
	rates := map[int]decimal.Decimal{}
	for _, rate := range p.Rates {
		rates[rate.ChannelID], _ = costReportDecimal(rate.Ratio)
	}
	result := map[int]ChannelCostReportRow{}
	for count := 0; rows.Next(); count++ {
		if count >= 5000000 {
			return nil, errors.New("cost report window exceeds row limit")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var log Log
		if err := LOG_DB.ScanRows(rows, &log); err != nil {
			return nil, err
		}
		if log.Quota < 0 {
			return nil, errors.New("negative quota in cost report")
		}
		row := ChannelCostReportRow{ChannelID: log.ChannelId}
		revenue, _ := decimal.NewFromInt(int64(log.Quota)).QuoRem(q, 18)
		if log.Type == 6 {
			row.RefundCount = 1
			row.RefundAmount = revenue.String()
		} else {
			row.ConsumeCount = 1
			row.ConsumeAmount = revenue.String()
			var other map[string]json.RawMessage
			valid := common.UnmarshalJsonStr(log.Other, &other) == nil
			var root LogCost
			_ = common.Unmarshal(other["root_cost"], &root)
			amount, ok := costReportSourceDecimal(root.AmountUSD)
			if ok && (root.Status == "estimated" || root.Status == "stale") {
				row.OriginalCount = 1
				row.OriginalCost = amount.String()
				row.KnownRevenue = revenue.String()
				if root.Status == "stale" {
					row.StaleCount = 1
				}
			} else {
				base, baseOK := costReportSourceDecimal(root.BaseUSD)
				safe := valid
				var admin map[string]json.RawMessage
				_ = common.Unmarshal(other["admin_info"], &admin)
				if string(admin["quota_saturation"]) != "" && string(admin["quota_saturation"]) != "null" {
					safe = false
				}
				if string(other["quota_saturated"]) == "true" {
					safe = false
				}
				for key := range other {
					if costReportUnsupportedRE.MatchString(key) {
						safe = false
					}
				}
				if raw := string(other["billing_mode"]); raw != "" && raw != `""` && raw != `"token"` {
					safe = false
				}
				if !baseOK {
					ratioText := string(other["group_ratio"])
					ratio, ratioOK := costReportDecimal(ratioText)
					baseOK = safe && ratioOK && ratio.GreaterThanOrEqual(decimal.RequireFromString("0.000001")) && ratio.LessThanOrEqual(decimal.NewFromInt(10000))
					if baseOK {
						base, _ = decimal.NewFromInt(int64(log.Quota)).QuoRem(q.Mul(ratio), 18)
					}
				}
				rate, hasRate := rates[log.ChannelId]
				if p.FallbackMode == "billed_basis" && baseOK && safe && hasRate {
					row.FallbackCount = 1
					row.FallbackCost = base.Mul(rate).Truncate(18).String()
					row.KnownRevenue = revenue.String()
				} else {
					row.MissingCount = 1
					row.UnknownRevenue = revenue.String()
				}
			}
		}
		previous := result[log.ChannelId]
		previous.ChannelID = log.ChannelId
		merged, err := MergeChannelCostReportRows([]ChannelCostReportRow{previous}, []ChannelCostReportRow{row})
		if err != nil {
			return nil, err
		}
		result[log.ChannelId] = merged[len(merged)-1]
		if len(result) > 5000 {
			return nil, errors.New("cost report exceeds channel limit")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]ChannelCostReportRow, 0, len(result))
	for _, row := range result {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ChannelID < out[j].ChannelID })
	return out, nil
}

func MergeChannelCostReportRows(existing, incoming []ChannelCostReportRow) ([]ChannelCostReportRow, error) {
	result := map[int]ChannelCostReportRow{}
	for _, list := range [][]ChannelCostReportRow{existing, incoming} {
		for _, r := range list {
			if r.ChannelID < 0 {
				return nil, errors.New("invalid cost report channel")
			}
			old := result[r.ChannelID]
			old.ChannelID = r.ChannelID
			counts := []*int64{&old.ConsumeCount, &old.RefundCount, &old.OriginalCount, &old.FallbackCount, &old.MissingCount, &old.StaleCount}
			for i, v := range []int64{r.ConsumeCount, r.RefundCount, r.OriginalCount, r.FallbackCount, r.MissingCount, r.StaleCount} {
				if v < 0 || *counts[i] > (1<<63-1)-v {
					return nil, errors.New("cost report count overflow")
				}
				*counts[i] += v
			}
			if r.OriginalCount > r.ConsumeCount || r.FallbackCount > r.ConsumeCount-r.OriginalCount || r.MissingCount != r.ConsumeCount-r.OriginalCount-r.FallbackCount || r.StaleCount > r.OriginalCount {
				return nil, errors.New("inconsistent cost report coverage counts")
			}
			targets := []*string{&old.ConsumeAmount, &old.RefundAmount, &old.OriginalCost, &old.FallbackCost, &old.KnownRevenue, &old.UnknownRevenue}
			amounts := make([]decimal.Decimal, len(targets))
			for i, v := range []string{r.ConsumeAmount, r.RefundAmount, r.OriginalCost, r.FallbackCost, r.KnownRevenue, r.UnknownRevenue} {
				if v == "" {
					v = "0"
				}
				if *targets[i] == "" {
					*targets[i] = "0"
				}
				if !costReportAggregateDecimalRE.MatchString(v) {
					return nil, errors.New("invalid cost report amount")
				}
				a, err := decimal.NewFromString(*targets[i])
				if err != nil {
					return nil, err
				}
				b, err := decimal.NewFromString(v)
				if err != nil || b.IsNegative() {
					return nil, errors.New("invalid cost report amount")
				}
				amounts[i] = b
				*targets[i] = a.Add(b).String()
			}
			if !amounts[4].Add(amounts[5]).Equal(amounts[0]) {
				return nil, errors.New("inconsistent cost report revenue coverage")
			}
			result[r.ChannelID] = old
		}
	}
	if len(result) > 5000 {
		return nil, errors.New("cost report exceeds channel limit")
	}
	out := make([]ChannelCostReportRow, 0, len(result))
	for _, row := range result {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ChannelID < out[j].ChannelID })
	return out, nil
}

func aggregateClickHouseCostWindow(ctx context.Context, p CostReportPolicy, start, end int64) ([]ChannelCostReportRow, error) {
	// Dedup and conflicts are checked within the same source snapshot/query. No
	// per-event data leaves ClickHouse, and neither FINAL nor a full-table join is used.
	rateExpr := "CAST(NULL AS Nullable(Decimal256(18)))"
	args := []any{start, end}
	filter := ""
	if p.ChannelID > 0 {
		filter = " WHERE channel_id = ? OR variants > 1"
		args = append(args, p.ChannelID)
	}
	if p.FallbackMode == "billed_basis" && len(p.Rates) > 0 {
		parts := []string{}
		for _, r := range p.Rates {
			parts = append(parts, fmt.Sprintf("channel_id = %d, toDecimal256('%s',18)", r.ChannelID, r.Ratio))
		}
		rateExpr = "multiIf(" + strings.Join(parts, ",") + ", CAST(NULL AS Nullable(Decimal256(18))))"
	}
	sql := `WITH dedup AS (
 SELECT event_id, any(ifNull(logs.channel_id,0)) channel_id, any(ifNull(type,0)) kind,
 any(ifNull(quota,0)) units, any(ifNull(other,'')) doc,
 uniqExact(tuple(logs.channel_id,type,quota,other,created_at)) variants
 FROM logs WHERE created_at >= ? AND created_at < ? AND type IN (2,6) GROUP BY event_id
 ), parsed AS (
 SELECT *, JSONExtractString(doc,'root_cost','status') status,
 JSONExtractString(doc,'root_cost','amount_usd') amount_text,
 JSONExtractString(doc,'root_cost','base_usd') base_text,
 JSONExtractRaw(doc,'group_ratio') group_text,
 ` + rateExpr + ` rate,
 isValidJSON(doc) AND JSONExtractRaw(doc,'admin_info','quota_saturation') IN ('','null')
 AND NOT JSONExtractBool(doc,'quota_saturated')
 AND NOT arrayExists(k -> match(lower(k),'(tool|search|audio|image|video|tiered|expression|extra_ratio)'),JSONExtractKeys(doc))
 AND JSONExtractString(doc,'billing_mode') IN ('','token') safe
 FROM dedup` + filter + `
 ), amounts AS (
 SELECT *, if(match(amount_text,'` + costReportSourceDecimalPattern + `'),toDecimal256OrNull(amount_text,18),NULL) snapshot,
 if(match(base_text,'` + costReportSourceDecimalPattern + `'),toDecimal256OrNull(base_text,18),NULL) base,
 if(match(group_text,'` + costReportDecimalPattern + `'),toDecimal256OrNull(group_text,18),NULL) group_rate,
 divideDecimal(toDecimal256(units,18),toDecimal256('` + p.QuotaPerUnit + `',18),18) revenue
 FROM parsed
 ), classified AS (
 SELECT *, kind=2 AND snapshot IS NOT NULL AND status IN ('estimated','stale') original,
 if(base IS NOT NULL,base,if(group_rate >= toDecimal256('0.000001',18) AND group_rate <= toDecimal256('10000',18),divideDecimal(toDecimal256(units,18),toDecimal256('` + p.QuotaPerUnit + `',18)*ifNull(nullIf(group_rate,0),toDecimal256(1,18)),18),NULL)) basis
 FROM amounts
 ), resolved AS (
 SELECT *, kind=2 AND NOT original AND safe AND rate IS NOT NULL AND basis IS NOT NULL fallback
 FROM classified
 ) SELECT channel_id,
 countIf(kind=2) consume_count,countIf(kind=6) refund_count,
 countIf(original) original_count,countIf(fallback) fallback_count,
 countIf(kind=2 AND NOT original AND NOT fallback) missing_count,
 countIf(original AND status='stale') stale_count,
 toString(sumIf(revenue,kind=2)) consume_amount,toString(sumIf(revenue,kind=6)) refund_amount,
 toString(sumIf(ifNull(snapshot,toDecimal256(0,18)),original)) original_cost,
 toString(sumIf(multiplyDecimal(ifNull(basis,toDecimal256(0,18)),ifNull(rate,toDecimal256(0,18)),18),fallback)) fallback_cost,
 toString(sumIf(revenue,original OR fallback)) known_revenue,
 toString(sumIf(revenue,kind=2 AND NOT original AND NOT fallback)) unknown_revenue,
 countIf(variants>1 OR event_id='' OR units<0) invalid_count
 FROM resolved GROUP BY channel_id ORDER BY channel_id LIMIT 5001
 SETTINGS max_threads=2,max_memory_usage=536870912,max_execution_time=30,timeout_overflow_mode='throw',max_rows_to_read=5000000,read_overflow_mode='throw',max_bytes_before_external_group_by=134217728`
	var rows []struct {
		ChannelCostReportRow
		InvalidCount int64
	}
	if err := LOG_DB.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 5000 {
		return nil, errors.New("cost report exceeds channel limit")
	}
	out := make([]ChannelCostReportRow, 0, len(rows))
	for _, r := range rows {
		if r.InvalidCount > 0 {
			return nil, errors.New("conflicting event identity or invalid quota in cost report")
		}
		out = append(out, r.ChannelCostReportRow)
	}
	return MergeChannelCostReportRows(nil, out)
}
