package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// Cumulative checkpoints are immutable and committed with the fenced job cursor.
// Empty spans can be clipped; occupied windows may only be used in their entirety.
type ChannelCostReportCheckpoint struct {
	JobID       string `gorm:"type:varchar(36);primaryKey"`
	End         int64  `gorm:"column:end_timestamp;type:bigint;primaryKey;index:idx_cost_checkpoint_eviction"`
	Start       int64  `gorm:"column:start_timestamp;type:bigint"`
	Empty       bool   `gorm:"column:is_empty"`
	Data        []byte `json:"-"`
	StoredBytes int64
}

type CostQueryRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type CostQuerySource struct {
	CostQueryRange
	JobID     string `json:"job_id"`
	UpdatedAt int64  `json:"updated_at"`
}

type ChannelCostQueryResult struct {
	Complete bool                   `json:"complete"`
	Missing  []CostQueryRange       `json:"missing"`
	Sources  []CostQuerySource      `json:"sources"`
	Policy   *CostReportPolicy      `json:"policy"`
	Rows     []ChannelCostReportRow `json:"-"`
}

var ErrCostQueryTooLarge = errors.New("too many report checkpoints; narrow the query range")

type costQueryPiece struct {
	CostQueryRange
	job    *ChannelCostReportJob
	before int64
	after  int64
	whole  bool
	empty  bool
}

// Maximum-weight interval scheduling chooses exact, non-overlapping windows.
// Prefer fewer pieces on equal coverage, then the newest report (sorted first).
func planCostQuery(pieces []costQueryPiece) ([]costQueryPiece, int64) {
	sort.SliceStable(pieces, func(i, j int) bool {
		if pieces[i].End != pieces[j].End {
			return pieces[i].End < pieces[j].End
		}
		if pieces[i].Start != pieces[j].Start {
			return pieces[i].Start < pieces[j].Start
		}
		if pieces[i].job.CreatedAt != pieces[j].job.CreatedAt {
			return pieces[i].job.CreatedAt > pieces[j].job.CreatedAt
		}
		return pieces[i].job.ID > pieces[j].job.ID
	})
	weights := make([]int64, len(pieces)+1)
	counts := make([]int, len(pieces)+1)
	previous := make([]int, len(pieces)+1)
	take := make([]bool, len(pieces)+1)
	for i, piece := range pieces {
		p := sort.Search(i, func(k int) bool { return pieces[k].End > piece.Start })
		weight := weights[p] + piece.End - piece.Start
		weights[i+1], counts[i+1], previous[i+1] = weights[i], counts[i], i
		if weight > weights[i] || (weight == weights[i] && counts[p]+1 < counts[i]) {
			weights[i+1], counts[i+1], previous[i+1], take[i+1] = weight, counts[p]+1, p, true
		}
	}
	selected := make([]costQueryPiece, 0)
	for i := len(pieces); i > 0; i = previous[i] {
		if take[i] {
			selected = append(selected, pieces[i-1])
		}
	}
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	return selected, weights[len(pieces)]
}

// Reads only task metadata and published checkpoints, never the source log rows.
// Different source identities, cost algorithms, units and frozen rates cannot mix.
func QueryChannelCostReports(ctx context.Context, start, end int64, channelID int) (*ChannelCostQueryResult, error) {
	if start < 1 || end <= start || end > time.Now().Unix() || channelID < 0 {
		return nil, errors.New("invalid cost query range")
	}
	identity, err := ChannelCostReportSourceIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []ChannelCostReportJob
	var metadataBytes int64
	if err := DB.WithContext(ctx).Model(&ChannelCostReportJob{}).Where("source_identity = ? AND cursor_timestamp > ?", identity, start).Select("COALESCE(SUM(LENGTH(policy_json)),0)").Scan(&metadataBytes).Error; err != nil {
		return nil, err
	}
	if metadataBytes > 32*1024*1024 {
		return nil, ErrCostQueryTooLarge
	}
	if err := DB.WithContext(ctx).Omit("rows_json").Where("source_identity = ? AND cursor_timestamp > ?", identity, start).Order("created_at DESC, id DESC").Limit(1001).Find(&jobs).Error; err != nil {
		return nil, err
	}
	if len(jobs) > 1000 {
		return nil, ErrCostQueryTooLarge
	}
	groups := map[string][]costQueryPiece{}
	policies := map[string]CostReportPolicy{}
	keys := make([]string, 0)
	jobKeys := map[string]string{}
	jobMap := map[string]*ChannelCostReportJob{}
	ids := make([]string, 0)
	for i := range jobs {
		job := &jobs[i]
		var policy CostReportPolicy
		if err := common.UnmarshalJsonStr(job.PolicyJSON, &policy); err != nil {
			return nil, err
		}
		if policy.Start >= end || job.Cursor <= policy.Start || job.Cursor > policy.End || (policy.ChannelID != 0 && policy.ChannelID != channelID) {
			continue
		}
		if err := ValidateCostReportPolicy(policy); err != nil {
			return nil, err
		}
		canonical := policy
		canonical.Start, canonical.End, canonical.ChannelID, canonical.CostFingerprint = 0, 0, channelID, ""
		canonical.Rates = append([]CostReportRate(nil), policy.Rates...)
		if channelID > 0 {
			canonical.Rates = nil
			for _, rate := range policy.Rates {
				if rate.ChannelID == channelID {
					canonical.Rates = append(canonical.Rates, rate)
				}
			}
		}
		sort.Slice(canonical.Rates, func(i, j int) bool { return canonical.Rates[i].ChannelID < canonical.Rates[j].ChannelID })
		encoded, err := common.Marshal(canonical)
		if err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%x", sha256.Sum256(encoded))
		if _, ok := policies[key]; !ok {
			policies[key] = policy
			keys = append(keys, key)
		}
		jobKeys[job.ID], jobMap[job.ID] = key, job
		ids = append(ids, job.ID)
		if policy.Start >= start && job.Cursor <= end {
			groups[key] = append(groups[key], costQueryPiece{CostQueryRange: CostQueryRange{policy.Start, job.Cursor}, job: job, whole: true})
		}
	}
	if len(ids) > 0 {
		var checkpoints []ChannelCostReportCheckpoint
		if err := DB.WithContext(ctx).Omit("data").Where("job_id IN ? AND end_timestamp > ? AND start_timestamp < ? AND start_timestamp < end_timestamp", ids, start, end).
			Where("is_empty = ? OR EXISTS (SELECT 1 FROM channel_cost_report_checkpoints baseline WHERE baseline.job_id = channel_cost_report_checkpoints.job_id AND baseline.end_timestamp = channel_cost_report_checkpoints.start_timestamp)", true).
			Order("job_id, end_timestamp").Limit(int(CostCheckpointMaxRows) + 1).Find(&checkpoints).Error; err != nil {
			return nil, err
		}
		if int64(len(checkpoints)) > CostCheckpointMaxRows {
			return nil, ErrCostQueryTooLarge
		}
		for _, point := range checkpoints {
			job := jobMap[point.JobID]
			if point.End > job.Cursor {
				continue
			}
			a, b := point.Start, point.End
			if point.Empty {
				a, b = max(start, a), min(end, b)
			}
			if a < start || b > end || a >= b {
				continue
			}
			key := jobKeys[point.JobID]
			groups[key] = append(groups[key], costQueryPiece{CostQueryRange: CostQueryRange{a, b}, job: job, before: point.Start, after: point.End, empty: point.Empty})
		}
	}
	result := &ChannelCostQueryResult{Missing: make([]CostQueryRange, 0), Sources: make([]CostQuerySource, 0)}
	var best []costQueryPiece
	var covered int64
	for _, key := range keys {
		plan, weight := planCostQuery(groups[key])
		if weight > covered {
			best, covered = plan, weight
			policy := policies[key]
			result.Policy = &policy
		}
	}
	cursor := start
	for _, piece := range best {
		if piece.Start > cursor {
			result.Missing = append(result.Missing, CostQueryRange{cursor, piece.Start})
		}
		cursor = piece.End
	}
	if cursor < end {
		result.Missing = append(result.Missing, CostQueryRange{cursor, end})
	}
	result.Complete = len(result.Missing) == 0
	// Missing coverage is not zero revenue. Never present a partial total as complete.
	if !result.Complete {
		return result, nil
	}
	// Adjacent windows from one job need only two cumulative reads, even for a year.
	var runs []costQueryPiece
	for _, piece := range best {
		n := len(runs)
		if n > 0 && !piece.whole && !runs[n-1].whole && !runs[n-1].empty && runs[n-1].job.ID == piece.job.ID && runs[n-1].End == piece.Start && runs[n-1].after == piece.before {
			runs[n-1].End, runs[n-1].after = piece.End, piece.after
		} else {
			runs = append(runs, piece)
		}
	}
	wholeIDs := make([]string, 0)
	for _, run := range runs {
		if run.whole {
			wholeIDs = append(wholeIDs, run.job.ID)
		}
	}
	wholeResults := map[string]ChannelCostReportJob{}
	if len(wholeIDs) > 0 {
		stored, err := DB.WithContext(ctx).Model(&ChannelCostReportJob{}).Select("id, cursor_timestamp, rows_json").Where("id IN ? AND LENGTH(rows_json) <= ?", wholeIDs, costCheckpointDecodedLimit).Rows()
		if err != nil {
			return nil, err
		}
		defer stored.Close()
		var bytesRead int
		for stored.Next() {
			var job ChannelCostReportJob
			if err := DB.ScanRows(stored, &job); err != nil {
				return nil, err
			}
			bytesRead += len(job.RowsJSON)
			if bytesRead > 32*1024*1024 {
				return nil, ErrCostQueryTooLarge
			}
			wholeResults[job.ID] = job
		}
		if err := stored.Err(); err != nil {
			return nil, err
		}
		if err := stored.Close(); err != nil {
			return nil, err
		}
	}
	for _, run := range runs {
		var rows []ChannelCostReportRow
		if run.whole {
			stored, ok := wholeResults[run.job.ID]
			if !ok || stored.Cursor != run.End {
				return nil, errors.New("report changed; retry query")
			}
			if err := common.UnmarshalJsonStr(stored.RowsJSON, &rows); err != nil {
				return nil, err
			}
		} else if !run.empty {
			var points []ChannelCostReportCheckpoint
			if err := DB.WithContext(ctx).Where("job_id = ? AND end_timestamp IN ?", run.job.ID, []int64{run.before, run.after}).Order("end_timestamp").Find(&points).Error; err != nil {
				return nil, err
			}
			if len(points) != 2 {
				return nil, errors.New("report checkpoints changed; retry query")
			}
			var before, after []ChannelCostReportRow
			if err := decodeCostCheckpoint(points[0].Data, &before); err != nil {
				return nil, err
			}
			if err := decodeCostCheckpoint(points[1].Data, &after); err != nil {
				return nil, err
			}
			rows, err = subtractCostReportRows(after, before)
			if err != nil {
				return nil, err
			}
		}
		if channelID > 0 {
			filtered := make([]ChannelCostReportRow, 0, 1)
			for _, row := range rows {
				if row.ChannelID == channelID {
					filtered = append(filtered, row)
				}
			}
			rows = filtered
		}
		result.Rows, err = MergeChannelCostReportRows(result.Rows, rows)
		if err != nil {
			return nil, err
		}
		result.Sources = append(result.Sources, CostQuerySource{CostQueryRange: run.CostQueryRange, JobID: run.job.ID, UpdatedAt: run.job.UpdatedAt})
	}
	return result, nil
}

func subtractCostReportRows(after, before []ChannelCostReportRow) ([]ChannelCostReportRow, error) {
	a, err := MergeChannelCostReportRows(nil, after)
	if err != nil {
		return nil, err
	}
	b, err := MergeChannelCostReportRows(nil, before)
	if err != nil {
		return nil, err
	}
	prior := map[int]ChannelCostReportRow{}
	for _, row := range b {
		prior[row.ChannelID] = row
	}
	for i := range a {
		old, ok := prior[a[i].ChannelID]
		if !ok {
			continue
		}
		delete(prior, a[i].ChannelID)
		current := &a[i]
		counts := []*int64{&current.ConsumeCount, &current.RefundCount, &current.OriginalCount, &current.FallbackCount, &current.MissingCount, &current.StaleCount}
		values := []int64{old.ConsumeCount, old.RefundCount, old.OriginalCount, old.FallbackCount, old.MissingCount, old.StaleCount}
		for j, p := range counts {
			if *p < values[j] {
				return nil, errors.New("non-monotonic report checkpoint")
			}
			*p -= values[j]
		}
		amounts := []*string{&current.ConsumeAmount, &current.RefundAmount, &current.OriginalCost, &current.FallbackCost, &current.KnownRevenue, &current.UnknownRevenue}
		texts := []string{old.ConsumeAmount, old.RefundAmount, old.OriginalCost, old.FallbackCost, old.KnownRevenue, old.UnknownRevenue}
		for j, p := range amounts {
			x, e := decimal.NewFromString(*p)
			if e != nil {
				return nil, e
			}
			y, e := decimal.NewFromString(texts[j])
			if e != nil {
				return nil, e
			}
			if x.LessThan(y) {
				return nil, errors.New("non-monotonic report checkpoint")
			}
			*p = x.Sub(y).String()
		}
	}
	if len(prior) > 0 {
		return nil, errors.New("missing channel in report checkpoint")
	}
	return a, nil
}
