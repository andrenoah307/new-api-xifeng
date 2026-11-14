package model

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func refundAtomicFixture(t *testing.T) CreateRefundTicketParams {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	user := User{Username: "refund-atomic", Quota: 1000}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&TopUp{UserId: user.Id, TradeNo: "refund-atomic", QuotaGranted: 1000, Status: common.TopUpStatusSuccess}).Error)
	return CreateRefundTicketParams{UserId: user.Id, Username: user.Username, RefundQuota: 400, PayeeType: RefundPayeeTypeAlipay, PayeeName: "person", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()}
}

func TestRefundCreateRollbackIncludesWallet(t *testing.T) {
	params := refundAtomicFixture(t)
	failure := errors.New("message insert failed")
	name := "test:refund-message-failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "ticket_messages" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { DB.Callback().Create().Remove(name) })
	_, _, _, err := CreateRefundTicket(params)
	require.ErrorIs(t, err, failure)
	var user User
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, 1000, user.Quota)
	var count int64
	require.NoError(t, DB.Model(&Ticket{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&TicketRefund{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRefundConcurrentCreateRespectsWalletAndRefundCap(t *testing.T) {
	params := refundAtomicFixture(t)
	params.RefundQuota = 700
	outcomes := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		request := params
		request.RequestKey = uuid.NewString()
		go func() { defer workers.Done(); _, _, _, err := CreateRefundTicket(request); outcomes <- err }()
	}
	workers.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrRefundFinancePending)
		}
	}
	assert.Equal(t, 1, successes)
	var user User
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, 300, user.Quota)
}

func legacyFrozenRefundFixture(t *testing.T, params CreateRefundTicketParams) *Ticket {
	t.Helper()
	ticket, _, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&TicketRefund{}).Where("ticket_id = ?", ticket.Id).Updates(map[string]any{"finance_version": 0, "frozen_quota": params.RefundQuota}).Error)
	return ticket
}

func TestRefundConcurrentRejectCreditsOnlyOnce(t *testing.T) {
	params := refundAtomicFixture(t)
	ticket := legacyFrozenRefundFixture(t, params)
	outcomes := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, RefundStatus: RefundStatusRejected})
			outcomes <- err
		}()
	}
	workers.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrTicketRefundNotPending)
		}
	}
	assert.Equal(t, 1, successes)
	var user User
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, 1000, user.Quota)
}

func TestRefundCancelParticipatesInCallerTransaction(t *testing.T) {
	params := refundAtomicFixture(t)
	ticket := legacyFrozenRefundFixture(t, params)
	failure := errors.New("outer transaction failed")
	require.ErrorIs(t, DB.Transaction(func(tx *gorm.DB) error {
		if err := CancelPendingRefund(tx, ticket.Id, params.UserId); err != nil {
			return err
		}
		return failure
	}), failure)
	var user User
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, 600, user.Quota)
	refund, err := GetTicketRefundByTicketId(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return CancelPendingRefund(tx, ticket.Id, params.UserId) }))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return CancelPendingRefund(tx, ticket.Id, params.UserId) }))
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, 1000, user.Quota)
}

func TestRefundRejectOverflowPreservesHistoricalFreeze(t *testing.T) {
	params := refundAtomicFixture(t)
	ticket := legacyFrozenRefundFixture(t, params)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", params.UserId).Update("quota", math.MaxInt64).Error)
	_, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, RefundStatus: RefundStatusRejected})
	require.ErrorIs(t, err, ErrTicketRefundQuotaExceed)
	var user User
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, math.MaxInt64, user.Quota)
	refund, err := GetTicketRefundByTicketId(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
	assert.Equal(t, 400, refund.FrozenQuota)
}

func TestRefundCancelRejectsInconsistentHistoricalFreeze(t *testing.T) {
	params := refundAtomicFixture(t)
	ticket := legacyFrozenRefundFixture(t, params)
	require.NoError(t, DB.Model(&TicketRefund{}).Where("ticket_id = ?", ticket.Id).Update("frozen_quota", params.RefundQuota+1).Error)
	err := DB.Transaction(func(tx *gorm.DB) error {
		return CancelPendingRefund(tx, ticket.Id, params.UserId)
	})
	require.ErrorIs(t, err, ErrTicketRefundQuotaInvalid)
	var owner User
	require.NoError(t, DB.First(&owner, params.UserId).Error)
	assert.Equal(t, 600, owner.Quota)
	refund, err := GetTicketRefundByTicketId(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
	assert.Equal(t, 401, refund.FrozenQuota)
}

func TestHistoricalFrozenRefundCannotBypassWaitingWithAnyApprovalMode(t *testing.T) {
	params := refundAtomicFixture(t)
	ticket := legacyFrozenRefundFixture(t, params)
	for _, mode := range []string{"", RefundQuotaModeWriteOff, RefundQuotaModeSubtract, RefundQuotaModeOverride} {
		_, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, RefundStatus: RefundStatusRefunded, QuotaMode: mode, ActualRefundQuota: 1})
		require.ErrorIs(t, err, ErrRefundSettlementUnconfirmed)
	}
	var user User
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, 600, user.Quota)
	refund, err := GetTicketRefundByTicketId(ticket.Id)
	require.NoError(t, err)
	assert.Zero(t, refund.FinanceVersion)
	assert.Equal(t, 400, refund.FrozenQuota)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
}

func TestRefundWalletDeltaBounds(t *testing.T) {
	for _, test := range []struct{ balance, delta int }{{600, -2000}, {math.MaxInt64, 200}} {
		name := "insufficient"
		if test.delta > 0 {
			name = "overflow"
		}
		t.Run(name, func(t *testing.T) {
			params := refundAtomicFixture(t)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", params.UserId).Update("quota", test.balance).Error)
			err := DB.Transaction(func(tx *gorm.DB) error { return applyRefundWalletDeltaTx(tx, params.UserId, test.delta) })
			require.ErrorIs(t, err, ErrTicketRefundQuotaExceed)
			var user User
			require.NoError(t, DB.First(&user, params.UserId).Error)
			assert.Equal(t, test.balance, user.Quota)
		})
	}
}

func TestRefundClawbackFailureRollsBackWalletAdjustment(t *testing.T) {
	truncateTables(t)
	fx := seedClawbackScenario(t, 100, 20000, 5000, 100, 2000)
	failure := errors.New("commission insert failed")
	name := "test:refund-commission-failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "commission_records" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { DB.Callback().Create().Remove(name) })
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := applyRefundWalletDeltaTx(tx, fx.inviteeId, 1000); err != nil {
			return err
		}
		var refund TicketRefund
		if err := tx.Where("ticket_id = ?", fx.ticketId).First(&refund).Error; err != nil {
			return err
		}
		_, err := applyCommissionClawback(tx, &refund, 1000)
		return err
	})
	require.ErrorIs(t, err, failure)
	var user User
	require.NoError(t, DB.First(&user, fx.inviteeId).Error)
	assert.Equal(t, 1_000_000, user.Quota)
	quota, history := loadUserAff(t, fx.inviterId)
	assert.Equal(t, 100, quota)
	assert.Equal(t, 20000, history)
	user = User{}
	require.NoError(t, DB.First(&user, fx.inviterId).Error)
	assert.Zero(t, user.Quota)
	refund, err := GetTicketRefundByTicketId(fx.ticketId)
	require.NoError(t, err)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
}

func TestRefundRequestReplayPreservesBalanceAndRejectsChangedPayload(t *testing.T) {
	params := refundAtomicFixture(t)
	params.RequestKey = uuid.NewString()
	ticket, refund, message, err := CreateRefundTicket(params)
	require.NoError(t, err)
	replayedTicket, replayedRefund, replayedMessage, err := CreateRefundTicket(params)
	require.NoError(t, err)
	assert.Equal(t, ticket.Id, replayedTicket.Id)
	assert.Equal(t, refund.Id, replayedRefund.Id)
	assert.Equal(t, message.Id, replayedMessage.Id)
	var user User
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, 600, user.Quota)
	params.PayeeAccount = "changed"
	_, _, _, err = CreateRefundTicket(params)
	require.ErrorIs(t, err, ErrTicketRefundQuotaInvalid)
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, 600, user.Quota)
}
