package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
)

// ErrTicketWeeklyLimit 余额不足用户超出每周新建次数。
var ErrTicketWeeklyLimit = errors.New("ticket weekly limit exceeded")

// LowBalanceTicketThreshold 触发周限流的余额阈值（5 货币单位）。
func LowBalanceTicketThreshold() int {
	return int(5 * common.QuotaPerUnit)
}

// CountUserTicketsCreatedSince 统计该用户自 sinceUnix(含) 起新建的计入周限的工单数(general+refund，排除 invoice)。
// 用户不能自删工单，故非删计数即可。
func CountUserTicketsCreatedSince(userId int, sinceUnix int64) (int64, error) {
	var n int64
	err := DB.Model(&Ticket{}).
		Where("user_id = ? AND created_time >= ? AND type IN (?, ?)", userId, sinceUnix, TicketTypeGeneral, TicketTypeRefund).
		Count(&n).Error
	return n, err
}

type TicketWeeklyLimitStatus struct {
	Limited        bool  `json:"limited"`
	Exempt         bool  `json:"exempt"`
	ThresholdQuota int   `json:"threshold_quota"`
	BalanceQuota   int   `json:"balance_quota"`
	Used           int64 `json:"used"`
	Remaining      int64 `json:"remaining"` // -1 = 无限（豁免或余额>=阈值）
	ResetAt        int64 `json:"reset_at"`  // 下周一 00:00 UTC+8
}

// GetUserTicketWeeklyLimitStatus 计算用户的周限流状态。
func GetUserTicketWeeklyLimitStatus(userId int, role int) (*TicketWeeklyLimitStatus, error) {
	now := common.GetTimestamp()
	st := &TicketWeeklyLimitStatus{
		ThresholdQuota: LowBalanceTicketThreshold(),
		ResetAt:        common.WeekEndUnixUTC8(now),
		Remaining:      -1,
	}
	if role >= common.RoleAdminUser {
		st.Exempt = true
		return st, nil
	}
	quota, err := GetUserQuota(userId, false)
	if err != nil {
		return nil, err
	}
	st.BalanceQuota = quota
	if quota >= st.ThresholdQuota {
		return st, nil
	}
	used, err := CountUserTicketsCreatedSince(userId, common.WeekStartUnixUTC8(now))
	if err != nil {
		return nil, err
	}
	st.Used = used
	if used >= 1 {
		st.Limited = true
		st.Remaining = 0
	} else {
		st.Remaining = 1
	}
	return st, nil
}
