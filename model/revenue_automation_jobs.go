package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Every job owned by automation has a bounded final payload, independently of
// the checkpoint budget. Manual report payload limits remain unchanged.
func validateAutomationReportSize(tx *gorm.DB, job *ChannelCostReportJob, rows string) error {
	if len(rows)+len(job.PolicyJSON) <= 256*1024 {
		return nil
	}
	var count int64
	if err := tx.Model(&RevenueAutomationJob{}).Where("request_key = ?", job.RequestKey).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("Automation report exceeds storage limit")
	}
	return nil
}

func EnsureRevenueAutomationReport(ctx context.Context, config *RevenueAutomation, policy CostReportPolicy) (*ChannelCostReportJob, error) {
	if err := ValidateCostReportPolicy(policy); err != nil {
		return nil, err
	}
	identity, err := ChannelCostReportSourceIdentity(ctx)
	if err != nil {
		return nil, err
	}
	encoded, err := common.Marshal(policy)
	if err != nil {
		return nil, err
	}
	if len(encoded) > 128*1024 {
		return nil, errors.New("Automation policy exceeds storage limit")
	}
	key := uuid.NewSHA1(uuid.NameSpaceOID, append([]byte(identity), encoded...)).String()
	var job ChannelCostReportJob
	err = RevenueAutomationTransaction(ctx, config, func(tx *gorm.DB) error {
		lookup := tx.Where("request_key = ?", key).First(&job)
		if lookup.Error == nil {
			return nil
		}
		if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return lookup.Error
		}
		var count int64
		if err := tx.Model(&RevenueAutomationJob{}).Count(&count).Error; err != nil {
			return err
		}
		if count >= 500 {
			var owned []RevenueAutomationJob
			if err := tx.Order("created_at ASC").Limit(10).Find(&owned).Error; err != nil {
				return err
			}
			removed := false
			for _, item := range owned {
				var old ChannelCostReportJob
				e := tx.Where("request_key = ?", item.RequestKey).First(&old).Error
				if errors.Is(e, gorm.ErrRecordNotFound) {
					if e = tx.Delete(&item).Error; e != nil {
						return e
					}
					removed = true
					break
				}
				if e != nil {
					return e
				}
				// Preserve recent and active work, including all daily reports needed for a year.
				if old.CreatedAt > time.Now().AddDate(-1, 0, -7).Unix() {
					continue
				}
				r := deletableCostReportJobs(tx, config.OwnerID, old.ID).Delete(&ChannelCostReportJob{})
				if r.Error != nil {
					return r.Error
				}
				if r.RowsAffected != 1 {
					continue
				}
				if err := deleteCostReportCheckpoints(tx, []string{old.ID}); err != nil {
					return err
				}
				if err := tx.Where("task_id = ? AND active_key IS NULL AND status IN ?", old.SystemTaskID, []SystemTaskStatus{SystemTaskStatusSucceeded, SystemTaskStatusFailed}).Delete(&SystemTask{}).Error; err != nil {
					return err
				}
				if err := tx.Delete(&item).Error; err != nil {
					return err
				}
				removed = true
				break
			}
			if !removed {
				return errors.New("Automation report retention limit reached")
			}
		}
		job = ChannelCostReportJob{ID: uuid.NewString(), OwnerID: config.OwnerID, RequestKey: key, SourceIdentity: identity, PolicyJSON: string(encoded), RowsJSON: "[]", Status: "pending", Cursor: policy.Start, TotalWindows: int((policy.End - policy.Start + 3599) / 3600), CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix()}
		if err := createCostReportSystemTask(tx, &job); err != nil {
			return err
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		return tx.Create(&RevenueAutomationJob{RequestKey: key, CreatedAt: time.Now().Unix()}).Error
	})
	return &job, err
}

func NextRevenueAutomationRun(ctx context.Context, row *RevenueAutomation, initialData string) (*RevenueAutomationRun, error) {
	var run RevenueAutomationRun
	err := RevenueAutomationTransaction(ctx, row, func(tx *gorm.DB) error {
		err := tx.Where("status IN ?", []string{"pending", "waiting", "sending"}).Order("scheduled_at ASC").First(&run).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if row.NextAt > time.Now().Unix() {
			return nil
		}
		// MySQL requires LIMIT with OFFSET. Keep 127 runs before inserting the next;
		// repeat bounded batches so excess history cannot bypass the retention cap.
		for {
			var old []RevenueAutomationRun
			if err := tx.Omit("data").Order("scheduled_at DESC").Offset(127).Limit(128).Find(&old).Error; err != nil {
				return err
			}
			for _, item := range old {
				if item.Status == "unknown" {
					return errors.New("Resolve uncertain email deliveries before retention cleanup")
				}
				if err := tx.Delete(&item).Error; err != nil {
					return err
				}
			}
			if len(old) < 128 {
				break
			}
		}
		run = RevenueAutomationRun{ID: uuid.NewString(), ScheduledAt: row.NextAt, Revision: row.Revision, Status: "pending", Data: initialData, UpdatedAt: time.Now().Unix()}
		next := time.Unix(row.NextAt, 0).In(RevenueTimezone).AddDate(0, 0, 1).Unix()
		if row.NextAt < time.Now().AddDate(0, 0, -7).Unix() {
			run.Status = "failed"
			run.Error = "Missed more than seven days; backfill historical reports manually"
			var config RevenueAutomationConfig
			if err := common.UnmarshalJsonStr(row.ConfigJSON, &config); err != nil {
				return err
			}
			next = RevenueNextDue(config.Time, time.Now())
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		return tx.Model(&RevenueAutomation{}).Where("id = ?", 1).Update("next_at", next).Error
	})
	if run.ID == "" {
		return nil, err
	}
	return &run, err
}
