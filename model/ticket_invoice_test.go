package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertInvoiceTicketUser(t *testing.T, userId int) *User {
	t.Helper()
	user := &User{
		Id:       userId,
		Username: fmt.Sprintf("invoice_ticket_%d", userId),
		Password: "hashed_password",
		Quota:    0,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func seedInvoiceTicketTopUp(t *testing.T, userId int, tradeNo string, money float64) int {
	t.Helper()
	now := common.GetTimestamp()
	topUp := &TopUp{
		UserId:       userId,
		Money:        money,
		TradeNo:      tradeNo,
		Status:       "success",
		CreateTime:   now,
		CompleteTime: now,
	}
	require.NoError(t, DB.Create(topUp).Error)
	return topUp.Id
}

func cleanupInvoiceTicketData(t *testing.T, userId int) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM ticket_invoices WHERE user_id = ?", userId).Error)
	require.NoError(t, DB.Exec("DELETE FROM ticket_messages WHERE user_id = ?", userId).Error)
	require.NoError(t, DB.Exec("DELETE FROM tickets WHERE user_id = ?", userId).Error)
	require.NoError(t, DB.Exec("DELETE FROM top_ups WHERE user_id = ?", userId).Error)
	require.NoError(t, DB.Exec("DELETE FROM users WHERE id = ?", userId).Error)
}

func createInvoiceTicketParams(user *User, topUpOrderIds []int) CreateInvoiceTicketParams {
	return CreateInvoiceTicketParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Test Company",
		TaxNumber:     "91110000ABCDEFGH1X",
		Email:         "invoice@example.com",
		TopUpOrderIds: topUpOrderIds,
	}
}

func TestCreateInvoiceTicketMinInvoiceAmountDisabled(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 0
	defer func() { operation_setting.MinInvoiceAmount = oldMinInvoiceAmount }()

	const userId = 910001
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	topUpId := seedInvoiceTicketTopUp(t, user.Id, "invoice-min-disabled-910001", 1)

	ticket, invoice, message, orders, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{topUpId}))

	require.NoError(t, err)
	require.NotNil(t, ticket)
	require.NotNil(t, invoice)
	require.NotNil(t, message)
	require.Len(t, orders, 1)
	assert.Equal(t, float64(1), invoice.TotalMoney)
}

func TestCreateInvoiceTicketMinInvoiceAmountBoundary(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 50
	defer func() { operation_setting.MinInvoiceAmount = oldMinInvoiceAmount }()

	const userId = 910002
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	topUpId := seedInvoiceTicketTopUp(t, user.Id, "invoice-min-boundary-910002", 50)

	ticket, invoice, message, orders, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{topUpId}))

	require.NoError(t, err)
	require.NotNil(t, ticket)
	require.NotNil(t, invoice)
	require.NotNil(t, message)
	require.Len(t, orders, 1)
	assert.Equal(t, float64(50), invoice.TotalMoney)
}

func TestCreateInvoiceTicketMinInvoiceAmountBelow(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 50
	defer func() { operation_setting.MinInvoiceAmount = oldMinInvoiceAmount }()

	const userId = 910003
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	topUpId := seedInvoiceTicketTopUp(t, user.Id, "invoice-min-below-910003", 30)

	ticket, invoice, message, orders, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{topUpId}))

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketInvoiceAmountBelowMin))
	assert.Nil(t, ticket)
	assert.Nil(t, invoice)
	assert.Nil(t, message)
	assert.Nil(t, orders)

	var ticketCount int64
	require.NoError(t, DB.Model(&Ticket{}).Where("user_id = ?", user.Id).Count(&ticketCount).Error)
	assert.Zero(t, ticketCount)
}
