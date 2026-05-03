package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
)

const (
	InvitationCodeStatusEnabled  = 1
	InvitationCodeStatusDisabled = 2
)

type InvitationCode struct {
	Id          int            `json:"id"`
	Code        string         `json:"code" gorm:"type:varchar(32);uniqueIndex"`
	Status      int            `json:"status" gorm:"type:int;default:1"`
	Name        string         `json:"name" gorm:"type:varchar(64);index"`
	MaxUses     int            `json:"max_uses" gorm:"type:int;default:1"`
	UsedCount   int            `json:"used_count" gorm:"type:int;default:0"`
	CreatedBy   int            `json:"created_by" gorm:"type:int;index"`
	OwnerUserId int            `json:"owner_user_id" gorm:"type:int;column:owner_user_id;index"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	ExpiredTime int64          `json:"expired_time" gorm:"bigint"`
	IsAdmin     bool           `json:"is_admin" gorm:"default:false"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
	Count       int            `json:"count" gorm:"-:all"`
}

type InvitationCodeUsage struct {
	Id               int    `json:"id"`
	InvitationCodeId int    `json:"invitation_code_id" gorm:"index"`
	UserId           int    `json:"user_id" gorm:"index"`
	Username         string `json:"username" gorm:"type:varchar(64);index"`
	UsedTime         int64  `json:"used_time" gorm:"bigint"`
}

type InvitationCodeQuotaInfo struct {
	Limit                int   `json:"limit"`
	Used                 int64 `json:"used"`
	Remaining            int   `json:"remaining"`
	AccountAgeDays       int   `json:"account_age_days"`
	MinAccountAgeDays    int   `json:"min_account_age_days"`
	DefaultCodeMaxUses   int   `json:"default_code_max_uses"`
	DefaultCodeValidDays int   `json:"default_code_valid_days"`
}

func (invitationCode *InvitationCode) IsExpired() bool {
	return invitationCode.ExpiredTime != 0 && invitationCode.ExpiredTime < common.GetTimestamp()
}

func (invitationCode *InvitationCode) IsExhausted() bool {
	return invitationCode.MaxUses > 0 && invitationCode.UsedCount >= invitationCode.MaxUses
}

func validateInvitationCodeRecord(invitationCode *InvitationCode) error {
	if invitationCode.Status != InvitationCodeStatusEnabled {
		return ErrInvitationCodeDisabled
	}
	if invitationCode.IsExpired() {
		return ErrInvitationCodeExpired
	}
	if invitationCode.IsExhausted() {
		return ErrInvitationCodeExhausted
	}
	return nil
}

func ValidateInvitationCode(code string) (*InvitationCode, error) {
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return nil, ErrInvitationCodeRequired
	}
	invitationCode := &InvitationCode{}
	if err := DB.Where("code = ?", trimmedCode).First(invitationCode).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvitationCodeInvalid
		}
		return nil, err
	}
	if err := validateInvitationCodeRecord(invitationCode); err != nil {
		return nil, err
	}
	return invitationCode, nil
}

func GetUsableInvitationCodeWithTx(tx *gorm.DB, code string) (*InvitationCode, error) {
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return nil, ErrInvitationCodeRequired
	}
	invitationCode := &InvitationCode{}
	query := tx
	if !common.UsingSQLite {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}
	if err := query.Where("code = ?", trimmedCode).First(invitationCode).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvitationCodeInvalid
		}
		return nil, err
	}
	if err := validateInvitationCodeRecord(invitationCode); err != nil {
		return nil, err
	}
	return invitationCode, nil
}

func ConsumeInvitationCodeWithTx(tx *gorm.DB, invitationCode *InvitationCode, userId int, username string) error {
	if invitationCode == nil {
		return nil
	}
	if err := validateInvitationCodeRecord(invitationCode); err != nil {
		return err
	}
	if invitationCode.MaxUses > 0 && invitationCode.UsedCount+1 > invitationCode.MaxUses {
		return ErrInvitationCodeExhausted
	}
	if err := tx.Model(invitationCode).Update("used_count", gorm.Expr("used_count + ?", 1)).Error; err != nil {
		return err
	}
	invitationCode.UsedCount++
	usage := &InvitationCodeUsage{
		InvitationCodeId: invitationCode.Id,
		UserId:           userId,
		Username:         username,
		UsedTime:         common.GetTimestamp(),
	}
	return tx.Create(usage).Error
}

func (invitationCode *InvitationCode) Insert() error {
	return DB.Create(invitationCode).Error
}

func (invitationCode *InvitationCode) Update() error {
	updates := map[string]any{
		"name":          invitationCode.Name,
		"status":        invitationCode.Status,
		"max_uses":      invitationCode.MaxUses,
		"expired_time":  invitationCode.ExpiredTime,
		"owner_user_id": invitationCode.OwnerUserId,
	}
	return DB.Model(invitationCode).Updates(updates).Error
}

func GetInvitationCodeById(id int) (*InvitationCode, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	invitationCode := &InvitationCode{}
	if err := DB.First(invitationCode, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return invitationCode, nil
}

func GetAllInvitationCodes(startIdx int, num int) ([]*InvitationCode, int64, error) {
	invitationCodes := make([]*InvitationCode, 0)
	var total int64
	query := DB.Model(&InvitationCode{})
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&invitationCodes).Error; err != nil {
		return nil, 0, err
	}
	return invitationCodes, total, nil
}

func SearchInvitationCodes(keyword string, startIdx int, num int) ([]*InvitationCode, int64, error) {
	invitationCodes := make([]*InvitationCode, 0)
	var total int64
	query := DB.Model(&InvitationCode{})
	if id, err := strconv.Atoi(keyword); err == nil {
		query = query.Where("id = ? OR owner_user_id = ? OR created_by = ? OR name LIKE ? OR code LIKE ?", id, id, id, keyword+"%", keyword+"%")
	} else {
		query = query.Where("name LIKE ? OR code LIKE ?", keyword+"%", keyword+"%")
	}
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&invitationCodes).Error; err != nil {
		return nil, 0, err
	}
	return invitationCodes, total, nil
}

func GetInvitationCodeUsages(invitationCodeId int) ([]*InvitationCodeUsage, error) {
	var usages []*InvitationCodeUsage
	err := DB.Where("invitation_code_id = ?", invitationCodeId).Order("id desc").Find(&usages).Error
	return usages, err
}

func DeleteInvitationCodeById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	return DB.Delete(&InvitationCode{}, "id = ?", id).Error
}

func DeleteInvalidInvitationCodes() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where(
		"status = ? OR (expired_time != 0 AND expired_time < ?) OR (max_uses != 0 AND used_count >= max_uses)",
		InvitationCodeStatusDisabled,
		now,
	).Delete(&InvitationCode{})
	return result.RowsAffected, result.Error
}

func GetOwnedInvitationCodesByUserId(userId int) ([]*InvitationCode, error) {
	var invitationCodes []*InvitationCode
	err := DB.Where("owner_user_id = ?", userId).Order("id desc").Find(&invitationCodes).Error
	return invitationCodes, err
}

func getUserInvitationCodeGenerateLimit(user *User) int {
	policy := setting.GetInvitationCodePolicy()
	if quota, ok := policy.RoleQuotas[strconv.Itoa(user.Role)]; ok {
		return quota
	}
	if quota, ok := policy.GroupQuotas[user.Group]; ok {
		return quota
	}
	return policy.DefaultGenerateQuota
}

func countUserCreatedInvitationCodes(userId int) (int64, error) {
	var count int64
	err := DB.Unscoped().
		Model(&InvitationCode{}).
		Where("created_by = ? AND is_admin = ?", userId, false).
		Count(&count).Error
	return count, err
}

func GetUserInvitationCodeQuotaInfo(user *User) (*InvitationCodeQuotaInfo, error) {
	used, err := countUserCreatedInvitationCodes(user.Id)
	if err != nil {
		return nil, err
	}
	policy := setting.GetInvitationCodePolicy()
	accountAgeDays := policy.MinAccountAgeDays
	if user.CreatedTime > 0 {
		accountAgeDays = int((common.GetTimestamp() - user.CreatedTime) / 86400)
		if accountAgeDays < 0 {
			accountAgeDays = 0
		}
	}
	limit := getUserInvitationCodeGenerateLimit(user)
	remaining := -1
	if limit >= 0 {
		remaining = limit - int(used)
		if remaining < 0 {
			remaining = 0
		}
	}
	return &InvitationCodeQuotaInfo{
		Limit:                limit,
		Used:                 used,
		Remaining:            remaining,
		AccountAgeDays:       accountAgeDays,
		MinAccountAgeDays:    policy.MinAccountAgeDays,
		DefaultCodeMaxUses:   policy.DefaultCodeMaxUses,
		DefaultCodeValidDays: policy.DefaultCodeValidDays,
	}, nil
}

func CanUserGenerateInvitationCode(user *User, quotaInfo *InvitationCodeQuotaInfo) error {
	if !common.InvitationCodeUserGenerateEnabled {
		return ErrInvitationCodeUserGenerateDisabled
	}
	if quotaInfo == nil {
		var err error
		quotaInfo, err = GetUserInvitationCodeQuotaInfo(user)
		if err != nil {
			return err
		}
	}
	if user.CreatedTime > 0 && quotaInfo.AccountAgeDays < quotaInfo.MinAccountAgeDays {
		return ErrInvitationCodeAccountTooYoung
	}
	if quotaInfo.Limit == 0 {
		return ErrInvitationCodeQuotaExceeded
	}
	if quotaInfo.Limit > 0 && quotaInfo.Used >= int64(quotaInfo.Limit) {
		return ErrInvitationCodeQuotaExceeded
	}
	return nil
}

func GenerateUserInvitationCode(user *User) (*InvitationCode, error) {
	policy := setting.GetInvitationCodePolicy()
	invitationCode := &InvitationCode{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		quotaInfo, err := GetUserInvitationCodeQuotaInfo(user)
		if err != nil {
			return err
		}
		if err := CanUserGenerateInvitationCode(user, quotaInfo); err != nil {
			return err
		}
		invitationCode.Name = "用户邀请码"
		invitationCode.Code = strings.ToUpper(common.GetRandomString(12))
		invitationCode.Status = InvitationCodeStatusEnabled
		invitationCode.MaxUses = policy.DefaultCodeMaxUses
		invitationCode.CreatedBy = user.Id
		invitationCode.OwnerUserId = user.Id
		invitationCode.CreatedTime = common.GetTimestamp()
		invitationCode.IsAdmin = false
		if policy.DefaultCodeValidDays > 0 {
			invitationCode.ExpiredTime = invitationCode.CreatedTime + int64(policy.DefaultCodeValidDays*86400)
		}
		return tx.Create(invitationCode).Error
	})
	if err != nil {
		return nil, err
	}
	return invitationCode, nil
}
