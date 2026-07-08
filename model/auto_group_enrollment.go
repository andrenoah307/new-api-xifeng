package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type AutoGroupEnrollment struct {
	Id            int            `json:"id" gorm:"primaryKey"`
	UserId        int            `json:"user_id" gorm:"uniqueIndex;not null"`
	OriginalGroup string         `json:"original_group" gorm:"type:varchar(64)"`
	CurrentGroup  string         `json:"current_group" gorm:"type:varchar(64)"`
	CurrentRuleId int            `json:"current_rule_id" gorm:"default:0"`
	EnrolledAt    int64          `json:"enrolled_at" gorm:"bigint"`
	UpdatedAt     int64          `json:"updated_at" gorm:"bigint"`
	EnrolledBy    int            `json:"enrolled_by" gorm:"default:0"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

type AutoGroupEnrollmentWithUser struct {
	AutoGroupEnrollment
	Username    string `json:"username" gorm:"column:username"`
	DisplayName string `json:"display_name" gorm:"column:display_name"`
	RuleName    string `json:"rule_name,omitempty" gorm:"-"`
}

func (e *AutoGroupEnrollment) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if e.EnrolledAt == 0 {
		e.EnrolledAt = now
	}
	e.UpdatedAt = now
	return nil
}

func ListAutoGroupEnrollments(offset, limit int, keyword string) ([]*AutoGroupEnrollmentWithUser, int64, error) {
	var enrollments []*AutoGroupEnrollmentWithUser
	var total int64

	tx := DB.Model(&AutoGroupEnrollment{}).
		Select("auto_group_enrollments.*, users.username, users.display_name").
		Joins("LEFT JOIN users ON users.id = auto_group_enrollments.user_id")

	if keyword != "" {
		tx = tx.Where("users.username LIKE ? OR users.display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("auto_group_enrollments.id desc").Offset(offset).Limit(limit).Find(&enrollments).Error; err != nil {
		return nil, 0, err
	}
	return enrollments, total, nil
}

func GetEnrollmentByUserId(userId int) (*AutoGroupEnrollment, error) {
	var enrollment AutoGroupEnrollment
	err := DB.Where("user_id = ?", userId).First(&enrollment).Error
	if err != nil {
		return nil, err
	}
	return &enrollment, nil
}

func ListAllEnrollmentUserIds() ([]int, error) {
	var userIds []int
	err := DB.Model(&AutoGroupEnrollment{}).Pluck("user_id", &userIds).Error
	return userIds, err
}

func ListAllEnrollments() ([]*AutoGroupEnrollment, error) {
	var enrollments []*AutoGroupEnrollment
	err := DB.Find(&enrollments).Error
	return enrollments, err
}

func CreateAutoGroupEnrollment(enrollment *AutoGroupEnrollment) error {
	if enrollment == nil {
		return errors.New("enrollment is nil")
	}
	return DB.Create(enrollment).Error
}

func DeleteAutoGroupEnrollment(id int) (*AutoGroupEnrollment, error) {
	if id <= 0 {
		return nil, errors.New("invalid enrollment id")
	}
	var enrollment AutoGroupEnrollment
	if err := DB.First(&enrollment, "id = ?", id).Error; err != nil {
		return nil, err
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	if err := tx.Delete(&AutoGroupEnrollment{}, "id = ?", id).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if enrollment.CurrentGroup != enrollment.OriginalGroup {
		if err := tx.Model(&User{}).Where("id = ? AND "+commonGroupCol+" = ?", enrollment.UserId, enrollment.CurrentGroup).
			Update(commonGroupCol, enrollment.OriginalGroup).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &enrollment, nil
}

func UpdateEnrollmentGroup(userId int, group string, ruleId int) error {
	now := common.GetTimestamp()
	return DB.Model(&AutoGroupEnrollment{}).
		Where("user_id = ?", userId).
		Updates(map[string]interface{}{
			"current_group":   group,
			"current_rule_id": ruleId,
			"updated_at":      now,
		}).Error
}

func TransactionalGroupChange(userId int, newGroup string, ruleId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).
			Where("id = ?", userId).
			Update(commonGroupCol, newGroup).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		return tx.Model(&AutoGroupEnrollment{}).
			Where("user_id = ?", userId).
			Updates(map[string]interface{}{
				"current_group":   newGroup,
				"current_rule_id": ruleId,
				"updated_at":      now,
			}).Error
	})
}
