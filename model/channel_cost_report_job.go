package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const SystemTaskTypeChannelCostReport = "channel_cost_report"

var ErrChannelCostReportConflict = errors.New("cost report task is busy or state changed")
var ErrChannelCostReportSourceChanged = errors.New("cost report source changed; create a new report")

type ChannelCostReportJob struct {
	ID               string `json:"id" gorm:"type:varchar(36);primaryKey"`
	OwnerID          int    `json:"owner_id"`
	RequestKey       string `json:"-" gorm:"type:varchar(36);uniqueIndex"`
	SourceIdentity   string `json:"-" gorm:"type:varchar(64)"`
	PolicyJSON       string `json:"-" gorm:"size:10485760"`
	RowsJSON         string `json:"-" gorm:"size:10485760"`
	Status           string `json:"status" gorm:"type:varchar(16);index"`
	Control          string `json:"control" gorm:"type:varchar(16)"`
	Cursor           int64  `json:"cursor" gorm:"column:cursor_timestamp;type:bigint"`
	ProcessedWindows int    `json:"processed_windows"`
	TotalWindows     int    `json:"total_windows"`
	SystemTaskID     string `json:"-" gorm:"type:varchar(64);index"`
	Error            string `json:"error" gorm:"type:varchar(128)"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt        int64  `json:"updated_at" gorm:"bigint"`
}

func CreateChannelCostReportJob(ctx context.Context, ownerID int, requestKey string, policy CostReportPolicy) (*ChannelCostReportJob, error) {
	if _, err := uuid.Parse(requestKey); err != nil {
		return nil, errors.New("invalid request key")
	}
	if err := ValidateCostReportPolicy(policy); err != nil {
		return nil, err
	}
	sourceIdentity, err := ChannelCostReportSourceIdentity(ctx)
	if err != nil {
		return nil, err
	}
	if sourceIdentity == "" {
		return nil, ErrChannelCostReportSourceChanged
	}
	encoded, err := common.Marshal(policy)
	if err != nil {
		return nil, err
	}
	job := &ChannelCostReportJob{ID: uuid.NewString(), OwnerID: ownerID, RequestKey: requestKey, PolicyJSON: string(encoded), RowsJSON: "[]", Status: "pending", Cursor: policy.Start, TotalWindows: int((policy.End - policy.Start + 3599) / 3600), CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}
	job.SourceIdentity = sourceIdentity
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeCostReportOwner(tx, ownerID); err != nil {
			return err
		}
		var prior ChannelCostReportJob
		lookup := tx.Where("request_key = ?", requestKey).First(&prior)
		if lookup.Error == nil {
			if prior.OwnerID != ownerID || prior.PolicyJSON != job.PolicyJSON {
				return ErrChannelCostReportConflict
			}
			*job = prior
			return nil
		}
		if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return lookup.Error
		}
		if err := createCostReportSystemTask(tx, job); err != nil {
			return err
		}
		return tx.Create(job).Error
	})
	if err != nil {
		return nil, err
	}
	return job, nil
}

func authorizeCostReportOwner(tx *gorm.DB, ownerID int) error {
	var count int64
	if err := tx.Model(&User{}).Where("id = ? AND role = ? AND status = ?", ownerID, common.RoleRootUser, common.UserStatusEnabled).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return errors.New("cost report owner is not authorized")
	}
	return nil
}

func AuthorizeChannelCostReportOwner(ctx context.Context, ownerID int) error {
	return authorizeCostReportOwner(DB.WithContext(ctx), ownerID)
}

func createCostReportSystemTask(tx *gorm.DB, job *ChannelCostReportJob) error {
	id, err := GenerateSystemTaskID()
	if err != nil {
		return err
	}
	payload, err := common.Marshal(map[string]string{"job_id": job.ID})
	if err != nil {
		return err
	}
	kind := SystemTaskTypeChannelCostReport
	task := &SystemTask{TaskID: id, Type: kind, Status: SystemTaskStatusPending, ActiveKey: &kind, Payload: string(payload)}
	if err := tx.Create(task).Error; err != nil {
		return ErrChannelCostReportConflict
	}
	job.SystemTaskID = id
	return nil
}

func GetChannelCostReportJob(ctx context.Context, id string) (*ChannelCostReportJob, error) {
	var job ChannelCostReportJob
	if err := DB.WithContext(ctx).Where("id = ?", id).First(&job).Error; err != nil {
		return nil, err
	}
	if err := reconcileCostReportJobStatus(ctx, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func GetChannelCostReportJobState(ctx context.Context, id string) (*ChannelCostReportJob, error) {
	var job ChannelCostReportJob
	if err := DB.WithContext(ctx).Omit("rows_json", "policy_json").Where("id = ?", id).First(&job).Error; err != nil {
		return nil, err
	}
	if err := reconcileCostReportJobStatus(ctx, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func reconcileCostReportJobStatus(ctx context.Context, job *ChannelCostReportJob) error {
	if job.Status == "pending" || job.Status == "running" {
		var task SystemTask
		if err := DB.WithContext(ctx).Where("task_id = ?", job.SystemTaskID).First(&task).Error; err != nil {
			return err
		}
		if task.Status == SystemTaskStatusFailed {
			job.Status = "failed"
			job.Error = "Report interrupted; resume to continue"
		}
	}
	return nil
}

func ListChannelCostReportJobs(ctx context.Context, limit int) ([]*ChannelCostReportJob, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	var jobs []*ChannelCostReportJob
	if err := DB.WithContext(ctx).Omit("rows_json").Order("created_at DESC, id DESC").Limit(limit).Find(&jobs).Error; err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if err := reconcileCostReportJobStatus(ctx, job); err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

func ControlChannelCostReportJob(ctx context.Context, id, action string) error {
	if action != "pause" && action != "resume" && action != "cancel" {
		return errors.New("invalid report action")
	}
	if action == "resume" {
		job, err := GetChannelCostReportJobState(ctx, id)
		if err != nil {
			return err
		}
		if err := ValidateChannelCostReportSource(ctx, job); err != nil {
			return err
		}
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job ChannelCostReportJob
		if err := tx.Where("id = ?", id).First(&job).Error; err != nil {
			return err
		}
		var task SystemTask
		if err := tx.Where("task_id = ?", job.SystemTaskID).First(&task).Error; err != nil {
			return err
		}
		active := task.Status == SystemTaskStatusPending || task.Status == SystemTaskStatusRunning
		oldTaskID := job.SystemTaskID
		updates := map[string]any{"updated_at": common.GetTimestamp()}
		if action == "resume" {
			if active || job.Status == "cancelled" || job.Status == "completed" || (job.Status != "paused" && job.Status != "failed" && task.Status != SystemTaskStatusFailed) {
				return ErrChannelCostReportConflict
			}
			if err := authorizeCostReportOwner(tx, job.OwnerID); err != nil {
				return err
			}
			if err := createCostReportSystemTask(tx, &job); err != nil {
				return err
			}
			updates["system_task_id"], updates["status"], updates["control"], updates["error"] = job.SystemTaskID, "pending", "", ""
		} else if active {
			if job.Status != "pending" && job.Status != "running" {
				return ErrChannelCostReportConflict
			}
			updates["control"] = action
		} else {
			if job.Status == "completed" || job.Status == "cancelled" {
				return ErrChannelCostReportConflict
			}
			if action == "cancel" {
				updates["status"] = "cancelled"
			} else {
				updates["status"] = "paused"
			}
			updates["control"] = ""
		}
		result := tx.Model(&ChannelCostReportJob{}).Where("id = ? AND system_task_id = ? AND status = ? AND control = ?", id, oldTaskID, job.Status, job.Control).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrChannelCostReportConflict
		}
		return nil
	})
}

// The lease and owner predicates are evaluated in the same statement as publication.
// A replayed window, revoked owner, control request or stale runner cannot add totals.
func costReportFencedUpdate(tx *gorm.DB, job *ChannelCostReportJob, runner string) *gorm.DB {
	return tx.Model(&ChannelCostReportJob{}).
		Where("id = ? AND system_task_id = ?", job.ID, job.SystemTaskID).
		Where("EXISTS (SELECT 1 FROM system_tasks WHERE system_tasks.task_id = channel_cost_report_jobs.system_task_id AND system_tasks.status = ? AND system_tasks.locked_by = ?)", SystemTaskStatusRunning, runner).
		Where("EXISTS (SELECT 1 FROM system_task_locks WHERE system_task_locks.task_id = channel_cost_report_jobs.system_task_id AND system_task_locks.locked_by = ? AND system_task_locks.locked_until >= ?)", runner, common.GetTimestamp())
}

func BeginChannelCostReportJob(ctx context.Context, job *ChannelCostReportJob, runner string) error {
	if err := ValidateChannelCostReportSource(ctx, job); err != nil {
		return err
	}
	result := costReportFencedUpdate(DB.WithContext(ctx), job, runner).Where("status = ? AND control = ?", "pending", "").Where("EXISTS (SELECT 1 FROM users WHERE users.id = channel_cost_report_jobs.owner_id AND users.role = ? AND users.status = ?)", common.RoleRootUser, common.UserStatusEnabled).Updates(map[string]any{"status": "running", "updated_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrChannelCostReportConflict
	}
	return nil
}

func PublishChannelCostReportWindow(ctx context.Context, job *ChannelCostReportJob, runner string, end int64, rows []ChannelCostReportRow) error {
	var policy CostReportPolicy
	if err := common.UnmarshalJsonStr(job.PolicyJSON, &policy); err != nil {
		return err
	}
	if end <= job.Cursor || end != min(job.Cursor+3600, policy.End) {
		return errors.New("invalid report window")
	}
	var prior []ChannelCostReportRow
	if err := common.UnmarshalJsonStr(job.RowsJSON, &prior); err != nil {
		return err
	}
	merged, err := MergeChannelCostReportRows(prior, rows)
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(merged)
	if err != nil {
		return err
	}
	if err := ValidateChannelCostReportSource(ctx, job); err != nil {
		return err
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := costReportFencedUpdate(tx, job, runner)
		result = result.Where("status = ? AND control = ? AND cursor_timestamp = ? AND processed_windows = ?", "running", "", job.Cursor, job.ProcessedWindows).
			Where("EXISTS (SELECT 1 FROM users WHERE users.id = channel_cost_report_jobs.owner_id AND users.role = ? AND users.status = ?)", common.RoleRootUser, common.UserStatusEnabled).
			Updates(map[string]any{"rows_json": string(encoded), "cursor_timestamp": end, "processed_windows": job.ProcessedWindows + 1, "updated_at": common.GetTimestamp()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrChannelCostReportConflict
		}
		return nil
	})
}

func ValidateChannelCostReportSource(ctx context.Context, job *ChannelCostReportJob) error {
	if job.SourceIdentity == "" {
		return ErrChannelCostReportSourceChanged
	}
	identity, err := ChannelCostReportSourceIdentity(ctx)
	if err != nil {
		return err
	}
	if identity != job.SourceIdentity {
		return ErrChannelCostReportSourceChanged
	}
	return nil
}

func FinishChannelCostReportJob(ctx context.Context, job *ChannelCostReportJob, runner, status string) error {
	updates := map[string]any{"status": gorm.Expr("CASE WHEN control = ? THEN ? WHEN control = ? THEN ? ELSE ? END", "cancel", "cancelled", "pause", "paused", status), "updated_at": common.GetTimestamp()}
	if status == "failed" {
		updates["error"] = "Report interrupted; resume to continue"
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := costReportFencedUpdate(tx, job, runner).Where("status IN ?", []string{"pending", "running"}).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrChannelCostReportConflict
		}
		// Separate statement avoids MySQL's left-to-right SET evaluation clearing
		// the control value before the status expression reads it.
		return tx.Model(&ChannelCostReportJob{}).Where("id = ? AND system_task_id = ?", job.ID, job.SystemTaskID).Update("control", "").Error
	})
}
