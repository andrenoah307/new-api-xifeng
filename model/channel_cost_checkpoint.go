package model

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const CostCheckpointMaxRows int64 = 10000
const CostCheckpointMaxBytes int64 = 128 * 1024 * 1024
const costCheckpointMaxPayload = 256 * 1024
const costCheckpointDecodedLimit = 10 * 1024 * 1024

// One shared row serializes reservations and eviction across all application nodes.
// The byte budget includes compressed payload and a fixed per-row overhead reserve.
type ChannelCostCheckpointBudget struct {
	ID          int   `gorm:"primaryKey;autoIncrement:false" json:"-"`
	StoredBytes int64 `json:"stored_bytes"`
	StoredRows  int64 `json:"stored_rows"`
	EvictedRows int64 `json:"evicted_rows"`
}

func GetCostCheckpointBudget(ctx context.Context) (ChannelCostCheckpointBudget, error) {
	var budget ChannelCostCheckpointBudget
	err := DB.WithContext(ctx).Where("id = ?", 1).First(&budget).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return budget, nil
	}
	return budget, err
}

func lockCostCheckpointBudget(tx *gorm.DB) (*ChannelCostCheckpointBudget, error) {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ChannelCostCheckpointBudget{ID: 1}).Error; err != nil {
		return nil, err
	}
	var budget ChannelCostCheckpointBudget
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", 1).First(&budget).Error; err != nil {
		return nil, err
	}
	return &budget, nil
}

func saveCostReportCheckpoint(tx *gorm.DB, job *ChannelCostReportJob, end int64, rows string, empty bool) error {
	budget, err := lockCostCheckpointBudget(tx)
	if err != nil {
		return err
	}
	points := []ChannelCostReportCheckpoint{{JobID: job.ID, Start: job.Cursor, End: end, Empty: empty}}
	texts := []string{rows}
	var count int64
	if err := tx.Model(&ChannelCostReportCheckpoint{}).Where("job_id = ? AND end_timestamp = ?", job.ID, job.Cursor).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		points = append(points, ChannelCostReportCheckpoint{JobID: job.ID, Start: job.Cursor, End: job.Cursor})
		texts = append(texts, job.RowsJSON)
	}
	var additional int64
	for i, text := range texts {
		if len(text) > costCheckpointDecodedLimit {
			return nil
		}
		var buffer bytes.Buffer
		writer := gzip.NewWriter(&buffer)
		if _, err := writer.Write([]byte(text)); err != nil {
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
		if buffer.Len() > costCheckpointMaxPayload {
			return nil
		}
		points[i].Data = buffer.Bytes()
		points[i].StoredBytes = int64(buffer.Len() + 256)
		additional += points[i].StoredBytes
	}
	for budget.StoredRows+int64(len(points)) > CostCheckpointMaxRows || budget.StoredBytes+additional > CostCheckpointMaxBytes {
		// Keep the current baseline so the new interval remains readable.
		var oldest []ChannelCostReportCheckpoint
		limit := int(max(1, budget.StoredRows+int64(len(points))-CostCheckpointMaxRows))
		if budget.StoredBytes+additional > CostCheckpointMaxBytes {
			limit = max(limit, 64)
		}
		if err := tx.Select("job_id, end_timestamp, stored_bytes").Where("NOT (job_id = ? AND end_timestamp = ?)", job.ID, job.Cursor).Order("end_timestamp, job_id").Limit(limit).Find(&oldest).Error; err != nil {
			return err
		}
		if len(oldest) == 0 {
			return errors.New("invalid checkpoint storage budget")
		}
		for _, point := range oldest {
			deleted := tx.Where("job_id = ? AND end_timestamp = ?", point.JobID, point.End).Delete(&ChannelCostReportCheckpoint{})
			if deleted.Error != nil {
				return deleted.Error
			}
			if deleted.RowsAffected != 1 {
				return errors.New("checkpoint eviction conflict")
			}
			budget.StoredBytes -= point.StoredBytes
			budget.StoredRows--
			budget.EvictedRows++
		}
	}
	if err := tx.Create(&points).Error; err != nil {
		return err
	}
	budget.StoredBytes += additional
	budget.StoredRows += int64(len(points))
	return tx.Model(budget).Select("stored_bytes", "stored_rows", "evicted_rows").Updates(budget).Error
}

func deleteCostReportCheckpoints(tx *gorm.DB, jobs []string) error {
	budget, err := lockCostCheckpointBudget(tx)
	if err != nil {
		return err
	}
	var usage struct {
		StoredBytes int64
		StoredRows  int64
	}
	if err := tx.Model(&ChannelCostReportCheckpoint{}).Select("COALESCE(SUM(stored_bytes),0) AS stored_bytes, COUNT(*) AS stored_rows").Where("job_id IN ?", jobs).Scan(&usage).Error; err != nil {
		return err
	}
	if usage.StoredRows > budget.StoredRows || usage.StoredBytes > budget.StoredBytes {
		return errors.New("invalid checkpoint storage budget")
	}
	deleted := tx.Where("job_id IN ?", jobs).Delete(&ChannelCostReportCheckpoint{})
	if deleted.Error != nil {
		return deleted.Error
	}
	if deleted.RowsAffected != usage.StoredRows {
		return errors.New("checkpoint deletion conflict")
	}
	budget.StoredRows -= usage.StoredRows
	budget.StoredBytes -= usage.StoredBytes
	return tx.Model(budget).Select("stored_bytes", "stored_rows").Updates(budget).Error
}

func decodeCostCheckpoint(data []byte, rows *[]ChannelCostReportRow) error {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer reader.Close()
	text, err := io.ReadAll(io.LimitReader(reader, costCheckpointDecodedLimit+1))
	if err != nil {
		return err
	}
	if len(text) > costCheckpointDecodedLimit {
		return errors.New("checkpoint exceeds decoded limit")
	}
	return common.Unmarshal(text, rows)
}
