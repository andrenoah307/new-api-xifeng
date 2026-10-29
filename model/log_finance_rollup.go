package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const financeDailyTable = "logs_finance_daily_v1"

// Explicit initialization belongs to the offline builder, never to a report GET.
func CreateClickHouseFinanceTable(db *gorm.DB) error {
	return db.Exec(`CREATE TABLE IF NOT EXISTS logs_finance_daily_v1 (
 day Int64, marker UInt8, user_id Int64, channel_id Int64, type Int64,
 ratio_value String, saturated UInt8, records Int64, quota_sum Int128, min_quota Int64,
 source_fingerprint String, generated_at Int64
 ) ENGINE = MergeTree PARTITION BY day ORDER BY (day,marker,user_id,channel_id,type,ratio_value,saturated)`).Error
}

// Parts are cheap metadata, not a scan of hundreds of millions of log rows.
// Monthly fingerprints deliberately over-invalidate after merges or late writes.
func financeSourceFingerprint(ctx context.Context, db *gorm.DB, day int64) (string, error) {
	var tableUUID string
	if err := db.WithContext(ctx).Raw("SELECT toString(uuid) FROM system.tables WHERE database=currentDatabase() AND name='logs'").Scan(&tableUUID).Error; err != nil {
		return "", err
	}
	if tableUUID == "" {
		return "", errors.New("source log table missing")
	}
	var parts []struct {
		Name   string
		Digest string
	}
	month := time.Unix(day, 0).UTC().Format("200601")
	err := db.WithContext(ctx).Raw(`SELECT name, toString(hash_of_all_files) AS digest FROM system.parts
 WHERE database=currentDatabase() AND table='logs' AND active AND partition LIKE ? ORDER BY name`, "("+month+",%").Scan(&parts).Error
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	fmt.Fprintln(hash, tableUUID)
	for _, part := range parts {
		fmt.Fprintln(hash, part.Name, part.Digest)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// RebuildClickHouseFinanceDay atomically replaces one complete UTC day. A crash
// leaves the ownership gate for operator inspection; it is never time-stolen.
func RebuildClickHouseFinanceDay(ctx context.Context, db *gorm.DB, day int64) (err error) {
	if day <= 0 || day%86400 != 0 || day+86400 > time.Now().UTC().Truncate(24*time.Hour).Unix() {
		return errors.New("finance rebuild requires one completed UTC day")
	}
	ctx = chdriver.Context(ctx, chdriver.WithSettings(chdriver.Settings{"async_insert": 0, "wait_for_async_insert": 1, "max_threads": 2, "max_memory_usage": 2147483648, "max_bytes_before_external_group_by": 268435456}))
	if err = validateClickHouseLogSchema(db.WithContext(ctx)); err != nil {
		return err
	}
	gate := fmt.Sprintf("logs_finance_lock_%d", day)
	if err = db.WithContext(ctx).Exec("CREATE TABLE " + gate + " (id UInt8) ENGINE=Memory").Error; err != nil {
		return fmt.Errorf("finance day is locked: %w", err)
	}
	stage := "logs_finance_stage_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	defer func() {
		if ctx.Err() != nil {
			err = errors.Join(err, fmt.Errorf("canceled builder: inspect server queries, %s and %s before removing the gate", gate, stage))
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if e := db.WithContext(cleanup).Exec("DROP TABLE IF EXISTS " + stage + " SYNC").Error; e != nil {
			err = errors.Join(err, e)
			return
		}
		err = errors.Join(err, db.WithContext(cleanup).Exec("DROP TABLE "+gate+" SYNC").Error)
	}()
	fingerprint, err := financeSourceFingerprint(ctx, db, day)
	if err != nil {
		return err
	}
	if err = db.WithContext(ctx).Exec("CREATE TABLE " + stage + " AS " + financeDailyTable).Error; err != nil {
		return err
	}
	generated := time.Now().Unix()
	// Preserve exact grouped integer quota and historical ratio; divide only when
	// producing the report, matching the SQL fallback's decimal arithmetic.
	if err = db.WithContext(ctx).Exec(`INSERT INTO `+stage+`
 SELECT ?,0,ifNull(user_id,0),ifNull(channel_id,0),ifNull(type,0),
 JSONExtractRaw(if(isValidJSON(other),other,'{}'),'group_ratio') AS ratio,
 if(JSONExtractRaw(if(isValidJSON(other),other,'{}'),'admin_info','quota_saturation') NOT IN ('','null') OR JSONExtractBool(if(isValidJSON(other),other,'{}'),'quota_saturated'),1,0) AS sat,
 toInt64(count()),sum(toInt128(ifNull(quota,0))),min(ifNull(quota,0)),?,?
 FROM logs WHERE created_at>=? AND created_at<? AND type IN (2,6)
 GROUP BY user_id,channel_id,type,ratio,sat`, day, fingerprint, generated, day, day+86400).Error; err != nil {
		return err
	}
	// The marker makes a successfully built empty day distinguishable from missing data.
	if err = db.WithContext(ctx).Exec("INSERT INTO "+stage+" VALUES (?,1,0,0,0,'',0,0,0,0,?,?)", day, fingerprint, generated).Error; err != nil {
		return err
	}
	after, err := financeSourceFingerprint(ctx, db, day)
	if err != nil {
		return err
	}
	if after != fingerprint {
		return errors.New("source parts changed during finance rebuild; retry this day")
	}
	return db.WithContext(ctx).Exec("ALTER TABLE "+financeDailyTable+" REPLACE PARTITION ? FROM "+stage, day).Error
}

type FinanceWindow struct {
	End                int64
	Channels           []ChannelFinanceStat
	Storage            string
	SummaryDays        int
	RawDays            int
	SummaryGeneratedAt int64
}

// GetFinanceWindow uses published daily snapshots and at most one raw day per
// call. Thus a missing snapshot cannot turn a monthly request into a full scan.
func GetFinanceWindow(ctx context.Context, start, end int64, channel, userID int) (FinanceWindow, error) {
	result := FinanceWindow{End: start, Storage: "raw", Channels: []ChannelFinanceStat{}}
	if start < 1 || end <= start || end-start > 32*86400 || channel < 0 || userID < 0 {
		return result, errors.New("invalid finance window")
	}
	isCH := common.UsingLogDatabase(common.DatabaseTypeClickHouse)
	hasSummary := false
	if isCH {
		ctx = chdriver.Context(ctx, chdriver.WithSettings(chdriver.Settings{"max_threads": 2, "max_memory_usage": 2147483648, "max_bytes_before_external_group_by": 268435456}))
		var count uint64
		if err := LOG_DB.WithContext(ctx).Raw("SELECT count() FROM system.tables WHERE database=currentDatabase() AND name=?", financeDailyTable).Scan(&count).Error; err != nil {
			return result, err
		}
		hasSummary = count == 1
	}
	totals := map[int]*ChannelFinanceStat{}
	usedSources := map[int64]string{}
	for cursor := start; cursor < end; {
		next := (cursor/86400 + 1) * 86400
		if next > end {
			next = end
		}
		var rows []ChannelFinanceStat
		used := false
		if hasSummary && cursor%86400 == 0 && next-cursor == 86400 && next <= time.Now().UTC().Truncate(24*time.Hour).Unix() {
			fingerprint, err := financeSourceFingerprint(ctx, LOG_DB, cursor)
			if err != nil {
				return result, err
			}
			var markers []struct {
				SourceFingerprint string
				GeneratedAt       int64
			}
			if err := LOG_DB.WithContext(ctx).Table(financeDailyTable).Select("source_fingerprint, generated_at").Where("day=? AND marker=1", cursor).Limit(2).Scan(&markers).Error; err != nil {
				return result, err
			}
			if len(markers) == 1 && markers[0].SourceFingerprint == fingerprint {
				query := LOG_DB.WithContext(ctx).Table(financeDailyTable).Where("day=? AND marker=0 AND source_fingerprint=?", cursor, fingerprint)
				channelCol, ratio, sat := "channel_id", "ratio_value", "saturated"
				if userID > 0 {
					query = query.Where("user_id=?", userID)
					channelCol, ratio, sat = "0", "'1'", "0"
				} else if channel > 0 {
					query = query.Where("channel_id=?", channel)
				}
				var groups []financeGroup
				if err := query.Select(fmt.Sprintf("%s AS channel_id,type,%s AS ratio_value,%s AS saturated,sum(records) AS records,toString(sum(quota_sum)) AS quota_sum,min(min_quota) AS min_quota", channelCol, ratio, sat)).Group("channel_id,type,ratio_value,saturated").Limit(10001).Scan(&groups).Error; err != nil {
					return result, err
				}
				if len(groups) > 10000 {
					return result, errors.New("finance aggregation has too many groups")
				}
				after, err := financeSourceFingerprint(ctx, LOG_DB, cursor)
				if err != nil {
					return result, err
				}
				// Validate the marker again too: a concurrent publisher may replace an empty partition.
				var verified int64
				if err := LOG_DB.WithContext(ctx).Table(financeDailyTable).Where("day=? AND marker=1 AND source_fingerprint=?", cursor, fingerprint).Count(&verified).Error; err != nil {
					return result, err
				}
				if after == fingerprint && verified == 1 {
					rows, err = reduceFinanceGroups(groups)
					if err != nil {
						return result, err
					}
					used = true
					day := time.Unix(cursor, 0).UTC()
					month := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
					if previous, ok := usedSources[month]; ok && previous != fingerprint {
						return result, errors.New("source changed during finance query; retry the window")
					}
					usedSources[month] = fingerprint
					if result.SummaryGeneratedAt == 0 || markers[0].GeneratedAt < result.SummaryGeneratedAt {
						result.SummaryGeneratedAt = markers[0].GeneratedAt
					}
				}
			}
		}
		if !used {
			var err error
			rows, err = getFinanceStats(ctx, cursor, next, channel, userID)
			if err != nil {
				return result, err
			}
			result.RawDays++
		} else {
			result.SummaryDays++
		}
		for _, row := range rows {
			previous := totals[row.ChannelID]
			if previous == nil {
				copy := row
				totals[row.ChannelID] = &copy
				continue
			}
			for _, pair := range [][2]*string{{&previous.ConsumeQuota, &row.ConsumeQuota}, {&previous.RefundQuota, &row.RefundQuota}, {&previous.NetQuota, &row.NetQuota}, {&previous.KnownModelQuota, &row.KnownModelQuota}} {
				a, _ := decimal.NewFromString(*pair[0])
				b, _ := decimal.NewFromString(*pair[1])
				*pair[0] = a.Add(b).String()
			}
			previous.ConsumeCount += row.ConsumeCount
			previous.RefundCount += row.RefundCount
			previous.MissingBasis += row.MissingBasis
			previous.UnresolvedRefunds += row.UnresolvedRefunds
		}
		cursor = next
		result.End = next
		if !used {
			break
		}
	}
	// A late write while reading a later day must not leave an earlier day's
	// stale summary in an otherwise successful monthly response.
	for day, fingerprint := range usedSources {
		current, err := financeSourceFingerprint(ctx, LOG_DB, day)
		if err != nil {
			return result, err
		}
		if current != fingerprint {
			return result, errors.New("source changed during finance query; retry the window")
		}
	}
	for _, row := range totals {
		result.Channels = append(result.Channels, *row)
	}
	sort.Slice(result.Channels, func(i, j int) bool { return result.Channels[i].ChannelID < result.Channels[j].ChannelID })
	if result.SummaryDays > 0 {
		result.Storage = "clickhouse_daily"
		if result.RawDays > 0 {
			result.Storage = "mixed"
		}
	}
	return result, nil
}
