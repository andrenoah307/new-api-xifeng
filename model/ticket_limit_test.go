package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertTicketLimitUser(t *testing.T, userID int, quota int) *User {
	t.Helper()
	user := &User{
		Id:       userID,
		Username: fmt.Sprintf("ticket_limit_%d", userID),
		Password: "hashed_password",
		Quota:    quota,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() {
		_ = DB.Exec("DELETE FROM tickets WHERE user_id = ?", userID).Error
	})
	return user
}

func insertTicketLimitTicket(t *testing.T, userID int, ticketType string, createdTime int64) *Ticket {
	t.Helper()
	ticket := &Ticket{
		UserId:      userID,
		Type:        ticketType,
		Status:      TicketStatusOpen,
		CreatedTime: createdTime,
		Subject:     "x",
		Username:    "x",
	}
	require.NoError(t, DB.Create(ticket).Error)
	return ticket
}

func TestTicketWeeklyLimitErrValue(t *testing.T) {
	require.Error(t, ErrTicketWeeklyLimit)
	assert.Equal(t, "ticket weekly limit exceeded", ErrTicketWeeklyLimit.Error())
}

func TestTicketWeeklyLimitThresholdValue(t *testing.T) {
	assert.Equal(t, int(5*common.QuotaPerUnit), LowBalanceTicketThreshold())
}

func TestTicketWeeklyLimitStatus_AdminExempt(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	user := insertTicketLimitUser(t, 900001, 0)
	insertTicketLimitTicket(t, user.Id, TicketTypeGeneral, now)
	insertTicketLimitTicket(t, user.Id, TicketTypeInvoice, now)
	insertTicketLimitTicket(t, user.Id, TicketTypeRefund, now)

	status, err := GetUserTicketWeeklyLimitStatus(user.Id, common.RoleAdminUser)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.False(t, status.Limited)
	assert.True(t, status.Exempt)
	assert.Equal(t, LowBalanceTicketThreshold(), status.ThresholdQuota)
	assert.EqualValues(t, -1, status.Remaining)
}

func TestTicketWeeklyLimitStatus_EnoughBalanceAtThreshold(t *testing.T) {
	truncateTables(t)

	threshold := LowBalanceTicketThreshold()
	user := insertTicketLimitUser(t, 900002, threshold)

	status, err := GetUserTicketWeeklyLimitStatus(user.Id, common.RoleCommonUser)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.False(t, status.Limited)
	assert.False(t, status.Exempt)
	assert.Equal(t, threshold, status.ThresholdQuota)
	assert.Equal(t, threshold, status.BalanceQuota)
	assert.EqualValues(t, -1, status.Remaining)
}

func TestTicketWeeklyLimitStatus_LowBalanceFirstTicketAllowed(t *testing.T) {
	truncateTables(t)

	threshold := LowBalanceTicketThreshold()
	user := insertTicketLimitUser(t, 900003, threshold-1)

	status, err := GetUserTicketWeeklyLimitStatus(user.Id, common.RoleCommonUser)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.False(t, status.Limited)
	assert.False(t, status.Exempt)
	assert.Equal(t, threshold-1, status.BalanceQuota)
	assert.EqualValues(t, 0, status.Used)
	assert.EqualValues(t, 1, status.Remaining)
}

func TestTicketWeeklyLimitStatus_LowBalanceSecondTicketLimitedAcrossTypes(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	threshold := LowBalanceTicketThreshold()
	user := insertTicketLimitUser(t, 900004, threshold-1)
	insertTicketLimitTicket(t, user.Id, TicketTypeInvoice, now)

	status, err := GetUserTicketWeeklyLimitStatus(user.Id, common.RoleCommonUser)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.Limited)
	assert.EqualValues(t, 1, status.Used)
	assert.EqualValues(t, 0, status.Remaining)

	insertTicketLimitTicket(t, user.Id, TicketTypeGeneral, now)
	insertTicketLimitTicket(t, user.Id, TicketTypeRefund, now)

	status, err = GetUserTicketWeeklyLimitStatus(user.Id, common.RoleCommonUser)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.Limited)
	assert.EqualValues(t, 3, status.Used)
	assert.EqualValues(t, 0, status.Remaining)
}

func TestTicketWeeklyLimitStatus_LastWeekTicketDoesNotLimitCurrentWeek(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	lastWeek := now - 8*86400
	threshold := LowBalanceTicketThreshold()
	user := insertTicketLimitUser(t, 900005, threshold-1)
	insertTicketLimitTicket(t, user.Id, TicketTypeGeneral, lastWeek)

	status, err := GetUserTicketWeeklyLimitStatus(user.Id, common.RoleCommonUser)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.False(t, status.Limited)
	assert.EqualValues(t, 0, status.Used)
	assert.EqualValues(t, 1, status.Remaining)
}

func TestCountUserTicketsCreatedSince_CountsAllTypesSinceWeekStart(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	lastWeek := now - 8*86400
	user := insertTicketLimitUser(t, 900006, 0)
	insertTicketLimitTicket(t, user.Id, TicketTypeGeneral, now)
	insertTicketLimitTicket(t, user.Id, TicketTypeInvoice, now)
	insertTicketLimitTicket(t, user.Id, TicketTypeRefund, now)
	insertTicketLimitTicket(t, user.Id, TicketTypeGeneral, lastWeek)

	count, err := CountUserTicketsCreatedSince(user.Id, common.WeekStartUnixUTC8(now))
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)
}
