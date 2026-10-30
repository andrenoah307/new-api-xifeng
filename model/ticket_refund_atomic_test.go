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
	return CreateRefundTicketParams{UserId: user.Id, Username: user.Username, RefundQuota: 400, PayeeType: RefundPayeeTypeAlipay, PayeeName: "person", PayeeAccount: "account", Contact: "contact"}
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
		go func() { defer workers.Done(); _, _, _, err := CreateRefundTicket(params); outcomes <- err }()
	}
	workers.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrTicketRefundQuotaExceed)
		}
	}
	assert.Equal(t, 1, successes)
	var user User
	require.NoError(t, DB.First(&user, params.UserId).Error)
	assert.Equal(t, 300, user.Quota)
}

func TestRefundConcurrentRejectCreditsOnlyOnce(t *testing.T) {
	params := refundAtomicFixture(t)
	ticket, _, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
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
	ticket, _, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
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

func TestRefundSettlementRejectsInsufficientFundsAndOverflow(t *testing.T) {
	for _, test := range []struct {
		name, mode              string
		status, actual, balance int
	}{
		{"subtract-insufficient", RefundQuotaModeSubtract, RefundStatusRefunded, 2000, 600},
		{"reject-overflow", "", RefundStatusRejected, 0, math.MaxInt64},
		{"subtract-credit-overflow", RefundQuotaModeSubtract, RefundStatusRefunded, 200, math.MaxInt64},
	} {
		t.Run(test.name, func(t *testing.T) {
			params := refundAtomicFixture(t)
			ticket, _, _, err := CreateRefundTicket(params)
			require.NoError(t, err)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", params.UserId).Update("quota", test.balance).Error)
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, RefundStatus: test.status, QuotaMode: test.mode, ActualRefundQuota: test.actual})
			require.ErrorIs(t, err, ErrTicketRefundQuotaExceed)
			var user User
			require.NoError(t, DB.First(&user, params.UserId).Error)
			assert.Equal(t, test.balance, user.Quota)
			refund, err := GetTicketRefundByTicketId(ticket.Id)
			require.NoError(t, err)
			assert.Equal(t, RefundStatusPending, refund.RefundStatus)
		})
	}
}

func TestRefundClawbackFailureRollsBackWalletAdjustment(t *testing.T) {
	truncateTables(t)
	fx := seedClawbackScenario(t, 10000, 20000, 5000, 100, 2000)
	failure := errors.New("commission insert failed")
	name := "test:refund-commission-failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "commission_records" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { DB.Callback().Create().Remove(name) })
	_, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{TicketId: fx.ticketId, RefundStatus: RefundStatusRefunded, QuotaMode: RefundQuotaModeSubtract, ActualRefundQuota: 1000, ClawBackCommission: true, ClawBackQuota: 1000})
	require.ErrorIs(t, err, failure)
	var user User
	require.NoError(t, DB.First(&user, fx.inviteeId).Error)
	assert.Equal(t, 1_000_000, user.Quota)
	quota, history := loadUserAff(t, fx.inviterId)
	assert.Equal(t, 10000, quota)
	assert.Equal(t, 20000, history)
	refund, err := GetTicketRefundByTicketId(fx.ticketId)
	require.NoError(t, err)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
}

func TestRefundLegacySettlementModesPreserveBalances(t *testing.T) {
	for _, test := range []struct {
		name, mode       string
		actual, expected int
	}{
		{"write-off", RefundQuotaModeWriteOff, 0, 600},
		{"subtract-less", RefundQuotaModeSubtract, 200, 800},
		{"subtract-more", RefundQuotaModeSubtract, 600, 400},
		{"override", RefundQuotaModeOverride, 123, 123},
		{"override-unchanged", RefundQuotaModeOverride, 600, 600},
	} {
		t.Run(test.name, func(t *testing.T) {
			params := refundAtomicFixture(t)
			ticket, _, _, err := CreateRefundTicket(params)
			require.NoError(t, err)
			refund, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, RefundStatus: RefundStatusRefunded, QuotaMode: test.mode, ActualRefundQuota: test.actual})
			require.NoError(t, err)
			assert.Equal(t, RefundStatusRefunded, refund.RefundStatus)
			var user User
			require.NoError(t, DB.First(&user, params.UserId).Error)
			assert.Equal(t, test.expected, user.Quota)
		})
	}
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
