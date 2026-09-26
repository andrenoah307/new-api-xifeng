package model

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"

	"gorm.io/gorm"
)

const (
	DiscountCodeStatusEnabled  = 1
	DiscountCodeStatusDisabled = 2
)

// DiscountCodeValidationError carries a business rejection safe to show to users.
type DiscountCodeValidationError string

func (err DiscountCodeValidationError) Error() string {
	return string(err)
}

type DiscountCode struct {
	Id             int            `json:"id"`
	Code           string         `json:"code" gorm:"type:varchar(64);uniqueIndex"`
	Name           string         `json:"name" gorm:"type:varchar(100);index"`
	DiscountRate   int            `json:"discount_rate"`
	StartTime      int64          `json:"start_time" gorm:"bigint"`
	EndTime        int64          `json:"end_time" gorm:"bigint"`
	MaxUsesTotal   int            `json:"max_uses_total" gorm:"default:0"`
	MaxAmount      int64          `json:"max_amount" gorm:"default:0"`
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
		"code", "name", "status", "discount_rate", "start_time", "end_time",
		"max_uses_total", "max_uses_per_user", "max_amount",
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

	if err := dc.validateAvailability(DB, userId); err != nil {
		var businessErr DiscountCodeValidationError
		if errors.As(err, &businessErr) {
			return nil, err
		}
		return nil, errors.New("查询使用记录失败")
	}
	return dc, nil
}

func (dc *DiscountCode) validateAvailability(tx *gorm.DB, userId int) error {
	if dc.Status != DiscountCodeStatusEnabled {
		return DiscountCodeValidationError("该折扣码已禁用")
	}

	now := common.GetTimestamp()
	if dc.StartTime > 0 && now < dc.StartTime {
		return DiscountCodeValidationError("该折扣码尚未生效")
	}
	if dc.EndTime > 0 && now > dc.EndTime {
		return DiscountCodeValidationError("该折扣码已过期")
	}

	if dc.MaxUsesTotal > 0 {
		count, err := GetDiscountCodeTotalCount(tx, dc.Id, now)
		if err != nil {
			return err
		}
		if count >= int64(dc.MaxUsesTotal) {
			pending, err := getPendingDiscountCodeCount(tx, dc.Id, nil, now)
			if err != nil {
				return err
			}
			if pending > 0 {
				return DiscountCodeValidationError("该折扣码名额暂被待支付订单占用，请稍后再试")
			}
			return DiscountCodeValidationError("该折扣码使用次数已达上限")
		}
	}

	if dc.MaxUsesPerUser > 0 {
		userCount, err := GetDiscountCodeUserCount(tx, dc.Id, userId, now)
		if err != nil {
			return err
		}
		if userCount >= int64(dc.MaxUsesPerUser) {
			pending, err := getPendingDiscountCodeCount(tx, dc.Id, &userId, now)
			if err != nil {
				return err
			}
			if pending > 0 {
				return DiscountCodeValidationError(fmt.Sprintf("您有使用该折扣码的待支付订单，请先完成支付；未支付的订单超过 %s后自动释放名额", formatDiscountCodePendingDuration(DiscountCodePendingTTLSeconds())))
			}
			return DiscountCodeValidationError("您已达到该折扣码的使用次数上限")
		}
	}

	return nil
}

func getPendingDiscountCodeCount(tx *gorm.DB, discountCodeId int, userId *int, now int64) (int64, error) {
	query := tx.Model(&TopUp{}).
		Where("discount_code_id = ? AND status = ? AND create_time > ?", discountCodeId, common.TopUpStatusPending, now-DiscountCodePendingTTLSeconds()).
		Where("(source IS NULL OR source <> ?)", "discount_bonus")
	if userId != nil {
		query = query.Where("user_id = ?", *userId)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

func formatDiscountCodePendingDuration(seconds int64) string {
	if seconds%3600 == 0 {
		return fmt.Sprintf("%d 小时", seconds/3600)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("%d 分钟", seconds/60)
	}
	return fmt.Sprintf("%d 秒", seconds)
}

// IncrementDiscountCodeUsedCount atomically increments the used_count within a transaction.
func IncrementDiscountCodeUsedCount(tx *gorm.DB, discountCodeId int) error {
	return tx.Model(&DiscountCode{}).Where("id = ?", discountCodeId).
		Update("used_count", gorm.Expr("used_count + 1")).Error
}

func DiscountCodePendingTTLSeconds() int64 {
	ttl := common.GetEnvOrDefault("DISCOUNT_CODE_PENDING_TTL_SECONDS", 1800)
	if ttl <= 0 {
		return 1800
	}
	return int64(ttl)
}

func (dc *DiscountCode) MaxAmountInInputUnits() decimal.Decimal {
	amount := decimal.NewFromInt(dc.MaxAmount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = amount.Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	return amount
}

func (dc *DiscountCode) CheckMaxAmount(amount decimal.Decimal) error {
	if dc.MaxAmount > 0 && amount.GreaterThan(dc.MaxAmountInInputUnits()) {
		return fmt.Errorf("该折扣码单笔最多充值 %s", dc.MaxAmountInInputUnits().String())
	}
	return nil
}

// ReserveDiscountCodeTopUp serializes availability checks and order creation on the code row.
func ReserveDiscountCodeTopUp(topUp *TopUp) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var dc DiscountCode
		if err := lockForUpdate(tx).First(&dc, topUp.DiscountCodeId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return DiscountCodeValidationError("折扣码不存在")
			}
			return err
		}
		if err := dc.validateAvailability(tx, topUp.UserId); err != nil {
			return err
		}
		return tx.Create(topUp).Error
	})
}

func GetDiscountCodeUserCount(tx *gorm.DB, discountCodeId int, userId int, now int64) (int64, error) {
	var count int64
	err := tx.Model(&TopUp{}).
		Where("discount_code_id = ? AND user_id = ?", discountCodeId, userId).
		Where("(source IS NULL OR source <> ?)", "discount_bonus").
		Where("(status = ? OR (status = ? AND create_time > ?))", common.TopUpStatusSuccess, common.TopUpStatusPending, now-DiscountCodePendingTTLSeconds()).
		Count(&count).Error
	return count, err
}

func GetDiscountCodeTotalCount(tx *gorm.DB, discountCodeId int, now int64) (int64, error) {
	var count int64
	err := tx.Model(&TopUp{}).
		Where("discount_code_id = ?", discountCodeId).
		Where("(source IS NULL OR source <> ?)", "discount_bonus").
		Where("(status = ? OR (status = ? AND create_time > ?))", common.TopUpStatusSuccess, common.TopUpStatusPending, now-DiscountCodePendingTTLSeconds()).
		Count(&count).Error
	return count, err
}

func CleanupPendingOrdersByDiscountCode(discountCodeId int) (int64, error) {
	cutoff := common.GetTimestamp() - DiscountCodePendingTTLSeconds()
	result := DB.Model(&TopUp{}).
		Where("discount_code_id = ? AND status = ? AND create_time < ?",
			discountCodeId, common.TopUpStatusPending, cutoff).
		Where("(source IS NULL OR source <> ?)", "discount_bonus").
		Update("status", common.TopUpStatusExpired)
	return result.RowsAffected, result.Error
}
