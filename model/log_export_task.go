package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	ExportStatusPending    = 0
	ExportStatusProcessing = 1
	ExportStatusDone       = 2
	ExportStatusFailed     = 3
	ExportStatusCancelled  = 4
)

type LogExportTask struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId        int    `json:"user_id" gorm:"index;not null"`
	Username      string `json:"username" gorm:"size:64"`
	Email         string `json:"email" gorm:"size:128;not null"`
	Status        int    `json:"status" gorm:"default:0;index"`
	Filters       string `json:"filters" gorm:"type:text"`
	FilePath      string `json:"file_path" gorm:"size:512"`
	FileSize      int64  `json:"file_size"`
	RowCount      int    `json:"row_count"`
	Progress      int    `json:"progress" gorm:"default:0"`
	ErrorMessage  string `json:"error_message" gorm:"size:512"`
	CreatedTime   int64  `json:"created_time" gorm:"bigint"`
	StartedTime   int64  `json:"started_time" gorm:"bigint"`
	CompletedTime int64  `json:"completed_time" gorm:"bigint"`
}

func (task *LogExportTask) BeforeCreate(tx *gorm.DB) error {
	if task.CreatedTime == 0 {
		task.CreatedTime = common.GetTimestamp()
	}
	return nil
}

func CreateLogExportTask(task *LogExportTask) error {
	return DB.Create(task).Error
}

func GetLogExportTaskById(id int) (*LogExportTask, error) {
	var task LogExportTask
	if err := DB.First(&task, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func GetUserExportTasks(userId int, page, pageSize int) ([]*LogExportTask, int64, error) {
	var tasks []*LogExportTask
	var total int64

	tx := DB.Model(&LogExportTask{}).Where("user_id = ?", userId)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	if err := tx.Order("created_time DESC").Offset(offset).Limit(pageSize).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func GetAllExportTasks(page, pageSize int) ([]*LogExportTask, int64, error) {
	var tasks []*LogExportTask
	var total int64

	tx := DB.Model(&LogExportTask{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	if err := tx.Order("created_time DESC").Offset(offset).Limit(pageSize).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func UpdateLogExportTask(id int, updates map[string]interface{}) error {
	return DB.Model(&LogExportTask{}).Where("id = ?", id).Updates(updates).Error
}

func GetUserRecentExportCount(userId int, sinceTimestamp int64) (int64, error) {
	var count int64
	err := DB.Model(&LogExportTask{}).
		Where("user_id = ? AND status IN ? AND created_time > ?",
			userId, []int{ExportStatusPending, ExportStatusProcessing, ExportStatusDone}, sinceTimestamp).
		Count(&count).Error
	return count, err
}

func GetPendingExportTasks() ([]*LogExportTask, error) {
	var tasks []*LogExportTask
	err := DB.Where("status IN ?", []int{ExportStatusPending, ExportStatusProcessing}).
		Order("created_time ASC").Find(&tasks).Error
	return tasks, err
}

func GetCompletedExportTasksForCleanup(beforeTimestamp int64) ([]*LogExportTask, error) {
	var tasks []*LogExportTask
	err := DB.Where("status = ? AND completed_time < ? AND file_path != ?",
		ExportStatusDone, beforeTimestamp, "").
		Find(&tasks).Error
	return tasks, err
}

func GetTotalExportFileSize() (int64, error) {
	var total int64
	err := DB.Model(&LogExportTask{}).
		Where("file_path != ?", "").
		Select("COALESCE(SUM(file_size), 0)").
		Scan(&total).Error
	return total, err
}

func GetExportTaskCountByStatus(status int) (int64, error) {
	var count int64
	err := DB.Model(&LogExportTask{}).Where("status = ?", status).Count(&count).Error
	return count, err
}
