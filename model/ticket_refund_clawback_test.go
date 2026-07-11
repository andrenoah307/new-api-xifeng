package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type clawbackFixture struct {
	inviterId int
	inviteeId int
	ticketId  int
}

// seedClawbackScenario 组一套"邀请人 + 被邀请人 + 一条充值返佣 + 待审核退款工单"。
func seedClawbackScenario(t *testing.T, inviterAff, inviterAffHistory, commissionQuota int, topUpMoney float64, refundQuota int) clawbackFixture {
	t.Helper()

	inviter := &User{
		Username:        "clawback-inviter",
		Password:        "test-password",
		AffCode:         "cb-inviter",
		AffQuota:        inviterAff,
		AffHistoryQuota: inviterAffHistory,
	}
	require.NoError(t, DB.Create(inviter).Error)

	invitee := &User{
		Username:  "clawback-invitee",
		Password:  "test-password",
		AffCode:   "cb-invitee",
		InviterId: inviter.Id,
		Quota:     1_000_000,
	}
	require.NoError(t, DB.Create(invitee).Error)

	if commissionQuota > 0 {
		require.NoError(t, DB.Create(&CommissionRecord{
			UserId:          invitee.Id,
			InviterId:       inviter.Id,
			TopUpId:         800001,
			TopUpMoney:      topUpMoney,
			CommissionRate:  10,
			CommissionQuota: commissionQuota,
			Type:            CommissionTypeTopUp,
		}).Error)
	}

	ticket := &Ticket{
		UserId:      invitee.Id,
		Username:    invitee.Username,
		Subject:     "退款申请",
		Type:        TicketTypeRefund,
		Status:      TicketStatusOpen,
		CreatedTime: common.GetTimestamp(),
		UpdatedTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(ticket).Error)
	require.NoError(t, DB.Create(&TicketRefund{
		TicketId:     ticket.Id,
		UserId:       invitee.Id,
		RefundQuota:  refundQuota,
		RefundStatus: RefundStatusPending,
		PayeeType:    RefundPayeeTypeAlipay,
		PayeeName:    "收款人",
		PayeeAccount: "account",
		Contact:      "contact",
	}).Error)

	return clawbackFixture{inviterId: inviter.Id, inviteeId: invitee.Id, ticketId: ticket.Id}
}

func loadUserAff(t *testing.T, userId int) (affQuota, affHistory int) {
	t.Helper()
	var u User
	require.NoError(t, DB.Select("aff_quota, aff_history").Where("id = ?", userId).First(&u).Error)
	return u.AffQuota, u.AffHistoryQuota
}

func TestRefundClawbackHappyPath(t *testing.T) {
	truncateTables(t)
	fx := seedClawbackScenario(t, 10000, 20000, 5000, 100, 25_000_000)

	refund, ticket, _, clawback, err := UpdateRefundStatus(UpdateRefundStatusParams{
		TicketId:           fx.ticketId,
		AdminId:            1,
		RefundStatus:       RefundStatusRefunded,
		QuotaMode:          RefundQuotaModeWriteOff,
		ClawBackCommission: true,
		ClawBackQuota:      3000,
	})
	require.NoError(t, err)
	require.Equal(t, RefundStatusRefunded, refund.RefundStatus)
	require.Equal(t, TicketStatusResolved, ticket.Status)
	require.NotNil(t, clawback)
	assert.Equal(t, fx.inviterId, clawback.InviterId)
	assert.Equal(t, 3000, clawback.ClawedQuota)

	affQuota, affHistory := loadUserAff(t, fx.inviterId)
	assert.Equal(t, 7000, affQuota)
	assert.Equal(t, 17000, affHistory)

	var record CommissionRecord
	require.NoError(t, DB.Where("type = ?", CommissionTypeRefundClawback).First(&record).Error)
	assert.Equal(t, -3000, record.CommissionQuota)
	assert.Equal(t, fx.inviteeId, record.UserId)
	assert.Equal(t, fx.inviterId, record.InviterId)
	assert.Equal(t, -(refundClawbackTopUpIdOffset + fx.ticketId), record.TopUpId)
	assert.Contains(t, record.Remark, fmt.Sprintf("#%d", fx.ticketId))
}

func TestRefundClawbackClampsToInviterAffQuota(t *testing.T) {
	truncateTables(t)
	// 邀请人只剩 1200 aff（已转出一部分），请求扣 5000 → 只能扣 1200。
	fx := seedClawbackScenario(t, 1200, 20000, 5000, 100, 25_000_000)

	_, _, _, clawback, err := UpdateRefundStatus(UpdateRefundStatusParams{
		TicketId:           fx.ticketId,
		AdminId:            1,
		RefundStatus:       RefundStatusRefunded,
		ClawBackCommission: true,
		ClawBackQuota:      5000,
	})
	require.NoError(t, err)
	require.NotNil(t, clawback)
	assert.Equal(t, 1200, clawback.ClawedQuota)

	affQuota, _ := loadUserAff(t, fx.inviterId)
	assert.Equal(t, 0, affQuota)
}

func TestRefundClawbackClampsToNetCommission(t *testing.T) {
	truncateTables(t)
	// 净返佣只有 2000（充值返佣 5000 − 历史已扣 3000），请求 5000 → 只能扣 2000。
	fx := seedClawbackScenario(t, 100000, 200000, 5000, 100, 25_000_000)
	require.NoError(t, DB.Create(&CommissionRecord{
		UserId:          fx.inviteeId,
		InviterId:       fx.inviterId,
		TopUpId:         -(refundClawbackTopUpIdOffset + 999999),
		CommissionQuota: -3000,
		Type:            CommissionTypeRefundClawback,
	}).Error)

	_, _, _, clawback, err := UpdateRefundStatus(UpdateRefundStatusParams{
		TicketId:           fx.ticketId,
		AdminId:            1,
		RefundStatus:       RefundStatusRefunded,
		ClawBackCommission: true,
		ClawBackQuota:      5000,
	})
	require.NoError(t, err)
	require.NotNil(t, clawback)
	assert.Equal(t, 2000, clawback.ClawedQuota)

	affQuota, _ := loadUserAff(t, fx.inviterId)
	assert.Equal(t, 98000, affQuota)
}

func TestRefundClawbackNoInviterIsNoop(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "no-inviter-user", Password: "test-password", AffCode: "cb-none", Quota: 1_000_000}
	require.NoError(t, DB.Create(user).Error)
	ticket := &Ticket{
		UserId: user.Id, Username: user.Username, Subject: "退款申请",
		Type: TicketTypeRefund, Status: TicketStatusOpen,
		CreatedTime: common.GetTimestamp(), UpdatedTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(ticket).Error)
	require.NoError(t, DB.Create(&TicketRefund{
		TicketId: ticket.Id, UserId: user.Id, RefundQuota: 1000,
		RefundStatus: RefundStatusPending,
		PayeeType:    RefundPayeeTypeAlipay, PayeeName: "n", PayeeAccount: "a", Contact: "c",
	}).Error)

	refund, _, _, clawback, err := UpdateRefundStatus(UpdateRefundStatusParams{
		TicketId:           ticket.Id,
		AdminId:            1,
		RefundStatus:       RefundStatusRefunded,
		ClawBackCommission: true,
		ClawBackQuota:      3000,
	})
	require.NoError(t, err)
	require.Equal(t, RefundStatusRefunded, refund.RefundStatus)
	require.NotNil(t, clawback)
	assert.Equal(t, 0, clawback.ClawedQuota)

	var count int64
	require.NoError(t, DB.Model(&CommissionRecord{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestRefundClawbackIgnoredOnReject(t *testing.T) {
	truncateTables(t)
	fx := seedClawbackScenario(t, 10000, 20000, 5000, 100, 25_000_000)

	refund, _, _, clawback, err := UpdateRefundStatus(UpdateRefundStatusParams{
		TicketId:           fx.ticketId,
		AdminId:            1,
		RefundStatus:       RefundStatusRejected,
		ClawBackCommission: true,
		ClawBackQuota:      0, // 驳回时忽略，非法值也不报错
	})
	require.NoError(t, err)
	require.Equal(t, RefundStatusRejected, refund.RefundStatus)
	assert.Nil(t, clawback)

	affQuota, affHistory := loadUserAff(t, fx.inviterId)
	assert.Equal(t, 10000, affQuota)
	assert.Equal(t, 20000, affHistory)
}

func TestRefundClawbackInvalidQuotaRejected(t *testing.T) {
	truncateTables(t)
	fx := seedClawbackScenario(t, 10000, 20000, 5000, 100, 25_000_000)

	_, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{
		TicketId:           fx.ticketId,
		AdminId:            1,
		RefundStatus:       RefundStatusRefunded,
		ClawBackCommission: true,
		ClawBackQuota:      0,
	})
	require.ErrorIs(t, err, ErrTicketRefundClawbackQuotaInvalid)

	// 退款保持 pending，可重试；邀请人余额未动。
	var refund TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", fx.ticketId).First(&refund).Error)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
	affQuota, _ := loadUserAff(t, fx.inviterId)
	assert.Equal(t, 10000, affQuota)
}

func TestGetRefundCommissionInfoProportionalSuggestion(t *testing.T) {
	truncateTables(t)
	// 充值总额 100，返佣 5000；退款 25_000_000 quota = 50 money → 比例 0.5 → 建议 2500。
	fx := seedClawbackScenario(t, 10000, 20000, 5000, 100, 25_000_000)

	var refund TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", fx.ticketId).First(&refund).Error)

	info, err := GetRefundCommissionInfo(&refund)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, fx.inviterId, info.InviterId)
	assert.Equal(t, "clawback-inviter", info.InviterUsername)
	assert.Equal(t, 5000, info.TotalCommissionQuota)
	assert.Equal(t, 0, info.ClawedBackQuota)
	assert.Equal(t, 5000, info.ClawbackableQuota)
	assert.InDelta(t, 100, info.TotalTopUpMoney, 1e-9)
	assert.Equal(t, 2500, info.SuggestedClawbackQuota)
	require.Len(t, info.Records, 1)
}

func TestGetRefundCommissionInfoNoInviter(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "info-no-inviter", Password: "test-password", AffCode: "cb-info"}
	require.NoError(t, DB.Create(user).Error)

	info, err := GetRefundCommissionInfo(&TicketRefund{UserId: user.Id, RefundQuota: 1000})
	require.NoError(t, err)
	assert.Nil(t, info)
}
