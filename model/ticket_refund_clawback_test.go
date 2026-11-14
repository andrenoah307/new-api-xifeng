package model

import (
	"fmt"
	"gorm.io/gorm"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundClawbackWalletBoundsAndRollback(t *testing.T) {
	for _, balance := range []int{2000, -100, math.MinInt32} {
		t.Run(fmt.Sprint(balance), func(t *testing.T) {
			truncateTables(t)
			fx := seedClawbackScenario(t, 0, 5000, 5000, 100, 1000)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", fx.inviterId).Update("quota", balance).Error)
			var refund TicketRefund
			require.NoError(t, DB.Where("ticket_id = ?", fx.ticketId).First(&refund).Error)
			info, err := GetRefundCommissionInfo(&refund)
			require.NoError(t, err)
			assert.Equal(t, 5000, info.ClawbackableQuota)
			_, err = runHistoricalClawback(t, fx.ticketId, 5000)
			var inviter User
			require.NoError(t, DB.First(&inviter, fx.inviterId).Error)
			if balance == math.MinInt32 {
				require.ErrorIs(t, err, ErrRefundFinanceReview)
				assert.Equal(t, balance, inviter.Quota)
				assert.Equal(t, 5000, inviter.AffHistoryQuota)
			} else {
				require.NoError(t, err)
				assert.Equal(t, balance-5000, inviter.Quota)
				assert.Zero(t, inviter.AffHistoryQuota)
				_, err = runHistoricalClawback(t, fx.ticketId, 5000)
				require.NoError(t, err)
				require.NoError(t, DB.First(&inviter, fx.inviterId).Error)
				assert.Equal(t, balance-5000, inviter.Quota)
			}
		})
	}
}

func TestOriginalRefundClawbackUsesOrderCashAndPrivateLog(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	order := seedRefundFinanceOrder(t, f.user.Id, 90000, 9000, 0, 1)
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": 90000, "inviter_id": f.finance.Id}).Error)
	require.NoError(t, DB.Model(&f.finance).Updates(map[string]interface{}{"quota": 0, "aff_quota": 100, "aff_history": 900}).Error)
	require.NoError(t, DB.Create(&CommissionRecord{UserId: f.user.Id, InviterId: f.finance.Id, TopUpId: order.Id, CommissionQuota: 900, Type: CommissionTypeTopUp}).Error)
	ticket, _, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 45000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()})
	require.NoError(t, err)
	params := UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded, ClawBackCommission: true, ClawBackQuota: 900}
	_, _, _, result, err := UpdateRefundStatus(params)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 450, result.ClawedQuota)
	assert.Equal(t, 350, result.WalletQuota)
	var inviter User
	require.NoError(t, DB.First(&inviter, f.finance.Id).Error)
	assert.Equal(t, -350, inviter.Quota)
	assert.Zero(t, inviter.AffQuota)
	assert.Equal(t, 450, inviter.AffHistoryQuota)
	_, _, _, _, err = UpdateRefundStatus(params)
	require.ErrorIs(t, err, ErrTicketRefundNotPending)
	var logs []*Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", f.finance.Id, LogTypeManage).Find(&logs).Error)
	require.Len(t, logs, 1)
	formatUserLogs(logs, 0)
	assert.Equal(t, "被推荐人退款，扣除推荐人奖励", logs[0].Content)
	assert.NotContains(t, logs[0].Other, "refund_cash")
	assert.NotContains(t, logs[0].Other, "ticket_id")
}

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

	clawback, err := runHistoricalClawback(t, fx.ticketId, 3000)
	require.NoError(t, err)
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
	assert.Equal(t, "被推荐人退款，扣除推荐人奖励", record.Remark)
}

func TestRefundClawbackDebitsTransferredRewardFromWallet(t *testing.T) {
	truncateTables(t)
	// Recover the transferred remainder even when the wallet becomes negative.
	fx := seedClawbackScenario(t, 1200, 20000, 5000, 100, 25_000_000)

	clawback, err := runHistoricalClawback(t, fx.ticketId, 5000)
	require.NoError(t, err)
	require.NotNil(t, clawback)
	assert.Equal(t, 5000, clawback.ClawedQuota)

	affQuota, _ := loadUserAff(t, fx.inviterId)
	assert.Equal(t, 0, affQuota)
	var inviter User
	require.NoError(t, DB.First(&inviter, fx.inviterId).Error)
	assert.Equal(t, -3800, inviter.Quota)
	assert.Equal(t, 15000, inviter.AffHistoryQuota)
	var refund TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", fx.ticketId).First(&refund).Error)
	info, err := GetRefundCommissionInfo(&refund)
	require.NoError(t, err)
	assert.Zero(t, info.ClawbackableQuota)
}

func TestRefundClawbackClampsToInviterAffHistory(t *testing.T) {
	truncateTables(t)
	// 防御历史脏数据：累计返佣低于当前返佣余额时，不得把 aff_history 扣成负数。
	fx := seedClawbackScenario(t, 10000, 800, 5000, 100, 25_000_000)
	var refund TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", fx.ticketId).First(&refund).Error)
	info, err := GetRefundCommissionInfo(&refund)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, 800, info.ClawbackableQuota)

	clawback, err := runHistoricalClawback(t, fx.ticketId, 5000)
	require.NoError(t, err)
	require.NotNil(t, clawback)
	assert.Equal(t, 800, clawback.ClawedQuota)

	affQuota, affHistory := loadUserAff(t, fx.inviterId)
	assert.Equal(t, 9200, affQuota)
	assert.Equal(t, 0, affHistory)
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

	clawback, err := runHistoricalClawback(t, fx.ticketId, 5000)
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

	clawback, err := runHistoricalClawback(t, ticket.Id, 3000)
	require.NoError(t, err)
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

func TestGetRefundCommissionInfoDoesNotInferLegacyCashFromQuota(t *testing.T) {
	truncateTables(t)
	// Historical quota has no authoritative cash allocation, even when it
	// happens to equal 50 units at the current exchange rate.
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
	assert.Zero(t, info.SuggestedClawbackQuota)
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

func runHistoricalClawback(t *testing.T, ticketID, amount int) (*CommissionClawbackResult, error) {
	t.Helper()
	var result *CommissionClawbackResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var refund TicketRefund
		if err := tx.Where("ticket_id = ?", ticketID).First(&refund).Error; err != nil {
			return err
		}
		var err error
		result, err = applyCommissionClawback(tx, &refund, amount)
		return err
	})
	return result, err
}
