package model

import (
	"math"
	"strconv"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

type CommissionRecord struct {
	Id              int     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId          int     `json:"user_id" gorm:"index"`
	InviterId       int     `json:"inviter_id" gorm:"index"`
	TopUpId         int     `json:"topup_id" gorm:"uniqueIndex"`
	TopUpMoney      float64 `json:"topup_money"`
	CommissionRate  float64 `json:"commission_rate"`
	CommissionQuota int     `json:"commission_quota"`
	IsManual        bool    `json:"is_manual" gorm:"default:false"`
	CreatedAt       int64   `json:"created_at" gorm:"autoCreateTime"`
}

func GrantTopUpCommission(topUp *TopUp, isManual bool) {
	if common.TopUpCommissionRate <= 0 {
		return
	}
	if isManual && !common.TopUpCommissionManualEnabled {
		return
	}
	if topUp == nil || topUp.UserId == 0 {
		return
	}

	user, err := GetUserById(topUp.UserId, false)
	if err != nil || user == nil || user.InviterId == 0 {
		return
	}

	rate := common.TopUpCommissionRate
	commissionMoney := topUp.Money * (rate / 100)
	commissionQuota := int(math.Round(commissionMoney * common.QuotaPerUnit))
	if commissionQuota <= 0 {
		return
	}

	record := &CommissionRecord{
		UserId:          topUp.UserId,
		InviterId:       user.InviterId,
		TopUpId:         topUp.Id,
		TopUpMoney:      topUp.Money,
		CommissionRate:  rate,
		CommissionQuota: commissionQuota,
		IsManual:        isManual,
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		if err := tx.Model(&User{}).Where("id = ?", user.InviterId).Updates(map[string]interface{}{
			"aff_quota":   gorm.Expr("aff_quota + ?", commissionQuota),
			"aff_history": gorm.Expr("aff_history + ?", commissionQuota),
		}).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		common.SysError("grant commission failed: topup_id=" + strconv.Itoa(topUp.Id) + " err=" + err.Error())
	}
}

const commissionCountHardLimit = 10000

func GetCommissionRecordsByInviterId(inviterId int, page int, pageSize int) ([]*CommissionRecord, int64, error) {
	var records []*CommissionRecord
	var total int64

	query := DB.Model(&CommissionRecord{}).Where("inviter_id = ?", inviterId)
	query.Limit(commissionCountHardLimit).Count(&total)

	err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}

func GetAllCommissionRecords(page int, pageSize int) ([]*CommissionRecord, int64, error) {
	var records []*CommissionRecord
	var total int64

	query := DB.Model(&CommissionRecord{})
	query.Limit(commissionCountHardLimit).Count(&total)

	err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error
	return records, total, err
}
