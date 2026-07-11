package model

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	CommissionTypeTopUp          = "topup"
	CommissionTypeInvite         = "invite"
	CommissionTypeRefundClawback = "refund_clawback" // 退款扣回（负数记录）
)

// TopUpId 有唯一索引且被三个来源共用：充值记录用真实正数 id，邀请奖励用
// -inviteeId（小负数），退款扣回用 -(offset+ticketId)。offset 拉开命名空间，
// 保证三者永不碰撞；"每张退款工单最多扣回一次"由 pending→refunded 的 CAS 保证。
const refundClawbackTopUpIdOffset = 1_000_000_000

type CommissionRecord struct {
	Id              int     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId          int     `json:"user_id" gorm:"index"`
	InviterId       int     `json:"inviter_id" gorm:"index"`
	TopUpId         int     `json:"topup_id" gorm:"uniqueIndex"`
	TopUpMoney      float64 `json:"topup_money"`
	CommissionRate  float64 `json:"commission_rate"`
	CommissionQuota int     `json:"commission_quota"`
	IsManual        bool    `json:"is_manual" gorm:"default:false"`
	Type            string  `json:"type" gorm:"type:varchar(20);default:'topup';index"`
	Remark          string  `json:"remark" gorm:"type:varchar(255);default:''"`
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
	if topUp.Source == "discount_bonus" {
		return
	}

	user, err := GetUserById(topUp.UserId, false)
	if err != nil || user == nil || user.InviterId == 0 {
		return
	}

	rate := common.TopUpCommissionRate
	commissionMoney := topUp.Money * (rate / 100)
	commissionQuota, err := common.QuotaRoundStrict(commissionMoney * common.QuotaPerUnit)
	if err != nil {
		common.SysError("grant commission quota invalid: topup_id=" +
			strconv.Itoa(topUp.Id) + " err=" + err.Error())
		return
	}
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
		Type:            CommissionTypeTopUp,
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

func GetRecentCommissionQuota(inviterId int, commissionType string, cooldownHours int) (int64, error) {
	if cooldownHours <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(cooldownHours) * time.Hour).Unix()
	var total int64
	query := DB.Model(&CommissionRecord{}).
		Where("inviter_id = ? AND created_at >= ?", inviterId, cutoff).
		Select("COALESCE(SUM(commission_quota), 0)")
	if commissionType != "" {
		query = query.Where("type = ?", commissionType)
	}
	err := query.Scan(&total).Error
	return total, err
}

func GetTransferableAffQuota(userId int, affQuota int) (int, error) {
	recentTopUp, err := GetRecentCommissionQuota(userId, CommissionTypeTopUp, common.AffTransferCooldownHours)
	if err != nil {
		return 0, err
	}
	recentInvite, err := GetRecentCommissionQuota(userId, CommissionTypeInvite, common.InviteRewardCooldownHours)
	if err != nil {
		return 0, err
	}
	transferable := int64(affQuota) - recentTopUp - recentInvite
	if transferable < 0 {
		transferable = 0
	}
	return int(transferable), nil
}

// RefundCommissionInfo 汇总"退款用户的充值给邀请人带来的返佣"，
// 供管理员处理退款工单时展示与预填扣回金额。
type RefundCommissionInfo struct {
	InviterId              int                 `json:"inviter_id"`
	InviterUsername        string              `json:"inviter_username"`
	Records                []*CommissionRecord `json:"records"`
	TotalCommissionQuota   int                 `json:"total_commission_quota"`
	ClawedBackQuota        int                 `json:"clawed_back_quota"`
	ClawbackableQuota      int                 `json:"clawbackable_quota"`
	InviterAffQuota        int                 `json:"inviter_aff_quota"`
	TotalTopUpMoney        float64             `json:"total_topup_money"`
	SuggestedClawbackQuota int                 `json:"suggested_clawback_quota"`
}

// GetRefundCommissionInfo 返回退款用户产生的充值返佣明细与可扣回额。
// 无邀请人 / 邀请人已删除 / 无返佣记录时返回 (nil, nil)。
func GetRefundCommissionInfo(refund *TicketRefund) (*RefundCommissionInfo, error) {
	var inviterId int
	if err := DB.Model(&User{}).Where("id = ?", refund.UserId).
		Select("inviter_id").Find(&inviterId).Error; err != nil {
		return nil, err
	}
	if inviterId == 0 {
		return nil, nil
	}

	var inviter User
	if err := DB.Select("id, username, aff_quota, aff_history").
		Where("id = ?", inviterId).First(&inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var records []*CommissionRecord
	if err := DB.Where("user_id = ? AND inviter_id = ? AND type IN ?",
		refund.UserId, inviterId,
		[]string{CommissionTypeTopUp, CommissionTypeRefundClawback}).
		Order("id DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	info := &RefundCommissionInfo{
		InviterId:       inviterId,
		InviterUsername: inviter.Username,
		Records:         records,
		InviterAffQuota: inviter.AffQuota,
	}
	for _, record := range records {
		switch record.Type {
		case CommissionTypeTopUp:
			info.TotalCommissionQuota += record.CommissionQuota
			info.TotalTopUpMoney += record.TopUpMoney
		case CommissionTypeRefundClawback:
			info.ClawedBackQuota += -record.CommissionQuota
		}
	}

	clawbackable := info.TotalCommissionQuota - info.ClawedBackQuota
	if clawbackable < 0 {
		clawbackable = 0
	}
	if affAvailable := common.Max(inviter.AffQuota, 0); clawbackable > affAvailable {
		clawbackable = affAvailable
	}
	if historyAvailable := common.Max(inviter.AffHistoryQuota, 0); clawbackable > historyAvailable {
		clawbackable = historyAvailable
	}
	info.ClawbackableQuota = clawbackable

	// 预填金额：按退款额占该用户充值总额的比例折算返佣，再夹到可扣回范围内。
	suggested := info.ClawbackableQuota
	if info.TotalTopUpMoney > 0 {
		refundMoney := float64(refund.RefundQuota) / common.QuotaPerUnit
		ratio := refundMoney / info.TotalTopUpMoney
		if ratio > 1 {
			ratio = 1
		}
		if ratio < 0 {
			ratio = 0
		}
		suggested = common.QuotaRound(float64(info.TotalCommissionQuota) * ratio)
	}
	if suggested > info.ClawbackableQuota {
		suggested = info.ClawbackableQuota
	}
	if suggested < 0 {
		suggested = 0
	}
	info.SuggestedClawbackQuota = suggested
	return info, nil
}

// CommissionClawbackResult 描述退款结算时实际执行的返佣扣回。
type CommissionClawbackResult struct {
	InviterId      int `json:"inviter_id"`
	RequestedQuota int `json:"requested_quota"`
	ClawedQuota    int `json:"clawed_quota"`
}

// applyCommissionClawback 在退款结算事务内扣回邀请人返佣：
// 扣减邀请人 aff_quota 与 aff_history（后者保证邀请人自己的退款上限
// 推导 affTransferred = aff_history - aff_quota 不被扭曲），并插入一条
// 负数返佣记录让邀请人在钱包返佣历史里看到扣回原因。
// 实际扣回额被夹到 min(请求额, 净返佣, 邀请人 aff 余额, 累计返佣)，
// 绝不把 aff_quota 或 aff_history 扣成负数。
func applyCommissionClawback(tx *gorm.DB, refund *TicketRefund, requested int) (*CommissionClawbackResult, error) {
	result := &CommissionClawbackResult{RequestedQuota: requested}

	var inviterId int
	if err := tx.Model(&User{}).Where("id = ?", refund.UserId).
		Select("inviter_id").Find(&inviterId).Error; err != nil {
		return nil, err
	}
	if inviterId == 0 {
		return result, nil
	}

	var inviter User
	if err := lockForUpdate(tx).Select("id, aff_quota, aff_history").
		Where("id = ?", inviterId).First(&inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, nil
		}
		return nil, err
	}
	result.InviterId = inviterId

	var net int64
	if err := tx.Model(&CommissionRecord{}).
		Where("user_id = ? AND inviter_id = ? AND type IN ?",
			refund.UserId, inviterId,
			[]string{CommissionTypeTopUp, CommissionTypeRefundClawback}).
		Select("COALESCE(SUM(commission_quota), 0)").Scan(&net).Error; err != nil {
		return nil, err
	}

	actual64 := int64(requested)
	if actual64 > net {
		actual64 = net
	}
	if actual64 > int64(inviter.AffQuota) {
		actual64 = int64(inviter.AffQuota)
	}
	if actual64 > int64(inviter.AffHistoryQuota) {
		actual64 = int64(inviter.AffHistoryQuota)
	}
	if actual64 <= 0 {
		return result, nil
	}
	actual, err := common.QuotaFromFloatStrict(float64(actual64))
	if err != nil {
		return nil, err
	}

	// SQLite 下 lockForUpdate 不加锁，这里的守卫条件兜底防止并发扣成负数。
	res := tx.Model(&User{}).
		Where("id = ? AND aff_quota >= ? AND aff_history >= ?", inviterId, actual, actual).
		Updates(map[string]interface{}{
			"aff_quota":   gorm.Expr("aff_quota - ?", actual),
			"aff_history": gorm.Expr("aff_history - ?", actual),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, errors.New("邀请人返佣余额不足，扣回失败")
	}

	if err := tx.Create(&CommissionRecord{
		UserId:          refund.UserId,
		InviterId:       inviterId,
		TopUpId:         -(refundClawbackTopUpIdOffset + refund.TicketId),
		TopUpMoney:      0,
		CommissionRate:  0,
		CommissionQuota: -actual,
		Type:            CommissionTypeRefundClawback,
		Remark:          fmt.Sprintf("退款工单 #%d 扣回返佣", refund.TicketId),
	}).Error; err != nil {
		return nil, err
	}

	result.ClawedQuota = actual
	return result, nil
}

func GrantInviteCommission(inviterId int, inviteeId int, quota int) error {
	record := &CommissionRecord{
		UserId:          inviteeId,
		InviterId:       inviterId,
		TopUpId:         -inviteeId,
		TopUpMoney:      0,
		CommissionRate:  0,
		CommissionQuota: quota,
		IsManual:        false,
		Type:            CommissionTypeInvite,
	}
	return DB.Create(record).Error
}
