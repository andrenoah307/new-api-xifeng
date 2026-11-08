package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrRefundConsumptionFrozen = errors.New("consumption is suspended while a refund is pending")

// Never expire records: elapsed time does not prove that a debit settled.
type RefundLiability struct {
	ID          string `gorm:"primaryKey;size:36"`
	UserID      int    `gorm:"index;not null"`
	RequestID   string `gorm:"type:text"`
	NodeName    string `gorm:"type:text"`
	CreatedAt   int64  `gorm:"type:bigint"`
	RequestDone bool
	TaskDone    bool
}

func BeginRefundLiability(userID int, requestID string) (*common.RefundLiability, error) {
	record := RefundLiability{ID: uuid.NewString(), UserID: userID, RequestID: requestID, NodeName: common.NodeName, CreatedAt: common.GetTimestamp(), TaskDone: true}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).Select("id").First(&owner, userID).Error; err != nil {
			return err
		}
		config, err := GetInvoiceTaxConfig(tx)
		if err != nil {
			return err
		}
		if config.RefundSettlementGuardEnabled {
			var pending int64
			if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", userID, RefundStatusPending).Count(&pending).Error; err != nil {
				return err
			}
			if pending != 0 {
				return ErrRefundConsumptionFrozen
			}
		}
		return tx.Create(&record).Error
	})
	if err != nil {
		return nil, err
	}
	return &common.RefundLiability{ID: record.ID}, nil
}

func CompleteRefundLiability(id string) {
	if id == "" {
		return
	}
	if err := DB.Model(&RefundLiability{}).Where("id = ?", id).Update("task_done", true).Error; err != nil {
		common.SysError(err.Error())
		return
	}
	if err := DB.Where("id = ? AND request_done = ? AND task_done = ?", id, true, true).Delete(&RefundLiability{}).Error; err != nil {
		common.SysError("refund liability release failed: " + err.Error())
	}
}

func FinishRefundLiabilityRequest(id string) {
	if err := DB.Model(&RefundLiability{}).Where("id = ?", id).Update("request_done", true).Error; err != nil {
		common.SysError(err.Error())
		return
	}
	if err := DB.Where("id = ? AND request_done = ? AND task_done = ?", id, true, true).Delete(&RefundLiability{}).Error; err != nil {
		common.SysError(err.Error())
	}
}

func HoldRefundLiability(liability *common.RefundLiability) {
	if liability == nil {
		return
	}
	if err := DB.Model(&RefundLiability{}).Where("id = ?", liability.ID).Update("task_done", false).Error; err != nil {
		liability.Failed.Store(true)
		common.SysError(err.Error())
	}
}

func verifyRefundLiabilitiesTx(tx *gorm.DB, userID int) error {
	var count int64
	if err := tx.Model(&RefundLiability{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return ErrRefundSettlementUnconfirmed
	}
	return nil
}

// Task submissions are recorded as consumption; reversals must also reverse
// that aggregate so the refund ledger can reconcile after queued writes drain.
func AdjustTaskUsedQuota(userID, delta int) error {
	return DB.Model(&User{}).Where("id = ?", userID).Update("used_quota", gorm.Expr("used_quota + ?", delta)).Error
}
