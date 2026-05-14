package model

import (
	"github.com/QuantumNous/new-api/common"
)

type DiscountCodeUsage struct {
	Id             int   `json:"id"`
	DiscountCodeId int   `json:"discount_code_id" gorm:"index:idx_dc_user"`
	UserId         int   `json:"user_id" gorm:"index:idx_dc_user"`
	TopUpId        int   `json:"top_up_id"`
	CreatedTime    int64 `json:"created_time" gorm:"bigint"`
}

func GetDiscountCodeUserUsageCount(discountCodeId int, userId int) (int64, error) {
	var count int64
	err := DB.Model(&DiscountCodeUsage{}).
		Where("discount_code_id = ? AND user_id = ?", discountCodeId, userId).
		Count(&count).Error
	return count, err
}

func RecordDiscountCodeUsage(discountCodeId int, userId int, topUpId int) error {
	usage := &DiscountCodeUsage{
		DiscountCodeId: discountCodeId,
		UserId:         userId,
		TopUpId:        topUpId,
		CreatedTime:    common.GetTimestamp(),
	}
	return DB.Create(usage).Error
}
