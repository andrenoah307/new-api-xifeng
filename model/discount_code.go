package model

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	DiscountCodeStatusEnabled  = 1
	DiscountCodeStatusDisabled = 2
)

type DiscountCode struct {
	Id             int            `json:"id"`
	Code           string         `json:"code" gorm:"type:varchar(64);uniqueIndex"`
	Name           string         `json:"name" gorm:"type:varchar(100);index"`
	DiscountRate   int            `json:"discount_rate"`
	StartTime      int64          `json:"start_time" gorm:"bigint"`
	EndTime        int64          `json:"end_time" gorm:"bigint"`
	MaxUsesTotal   int            `json:"max_uses_total" gorm:"default:0"`
	MaxUsesPerUser int            `json:"max_uses_per_user" gorm:"default:0"`
	UsedCount      int            `json:"used_count" gorm:"default:0"`
	Status         int            `json:"status" gorm:"default:1"`
	CreatedTime    int64          `json:"created_time" gorm:"bigint"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
	Count          int            `json:"count" gorm:"-:all"`
}

func GetAllDiscountCodes(startIdx int, num int) (codes []*DiscountCode, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	err = tx.Model(&DiscountCode{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&codes).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return codes, total, nil
}

func SearchDiscountCodes(keyword string, startIdx int, num int) (codes []*DiscountCode, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	codes = make([]*DiscountCode, 0)
	query := tx.Model(&DiscountCode{})

	if id, parseErr := strconv.Atoi(keyword); parseErr == nil {
		query = query.Where("id = ? OR name LIKE ? OR code LIKE ?", id, keyword+"%", keyword+"%")
	} else {
		query = query.Where("name LIKE ? OR code LIKE ?", keyword+"%", keyword+"%")
	}

	err = query.Session(&gorm.Session{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&codes).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return codes, total, nil
}

func GetDiscountCodeById(id int) (*DiscountCode, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	dc := DiscountCode{Id: id}
	err := DB.First(&dc, "id = ?", id).Error
	return &dc, err
}

func GetDiscountCodeByCode(code string) (*DiscountCode, error) {
	if code == "" {
		return nil, errors.New("折扣码为空")
	}
	dc := &DiscountCode{}
	err := DB.Where("code = ?", code).First(dc).Error
	return dc, err
}

func (dc *DiscountCode) Insert() error {
	return DB.Create(dc).Error
}

func (dc *DiscountCode) Update() error {
	return DB.Model(dc).Select(
		"name", "status", "discount_rate", "start_time", "end_time",
		"max_uses_total", "max_uses_per_user",
	).Updates(dc).Error
}

func (dc *DiscountCode) Delete() error {
	return DB.Delete(dc).Error
}

func DeleteDiscountCodeById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	dc := DiscountCode{Id: id}
	err := DB.Where(dc).First(&dc).Error
	if err != nil {
		return err
	}
	return dc.Delete()
}

// ValidateDiscountCode checks if a discount code is valid for the given user.
// Does NOT increment usage — call RecordDiscountCodeUsage after payment succeeds.
func ValidateDiscountCode(code string, userId int) (*DiscountCode, error) {
	if code == "" {
		return nil, errors.New("未提供折扣码")
	}

	dc := &DiscountCode{}
	err := DB.Where("code = ?", code).First(dc).Error
	if err != nil {
		return nil, errors.New("折扣码不存在")
	}

	if dc.Status != DiscountCodeStatusEnabled {
		return nil, errors.New("该折扣码已禁用")
	}

	now := common.GetTimestamp()
	if dc.StartTime > 0 && now < dc.StartTime {
		return nil, errors.New("该折扣码尚未生效")
	}
	if dc.EndTime > 0 && now > dc.EndTime {
		return nil, errors.New("该折扣码已过期")
	}

	if dc.MaxUsesTotal > 0 && dc.UsedCount >= dc.MaxUsesTotal {
		return nil, errors.New("该折扣码使用次数已达上限")
	}

	if dc.MaxUsesPerUser > 0 {
		userCount, err := GetDiscountCodeUserUsageCount(dc.Id, userId)
		if err != nil {
			return nil, errors.New("查询使用记录失败")
		}
		if userCount >= int64(dc.MaxUsesPerUser) {
			return nil, errors.New("您已达到该折扣码的使用次数上限")
		}
	}

	return dc, nil
}

// IncrementDiscountCodeUsedCount atomically increments the used_count within a transaction.
func IncrementDiscountCodeUsedCount(tx *gorm.DB, discountCodeId int) error {
	return tx.Model(&DiscountCode{}).Where("id = ?", discountCodeId).
		Update("used_count", gorm.Expr("used_count + 1")).Error
}
