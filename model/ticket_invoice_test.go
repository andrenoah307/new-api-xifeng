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

func TestCloseInvoiceTicketReleasesOrdersForReinvoicing(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 0
	defer func() { operation_setting.MinInvoiceAmount = oldMinInvoiceAmount }()

	const userId = 910004
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	topUpId := seedInvoiceTicketTopUp(t, user.Id, "invoice-cancel-release-910004", 10)

	ticket, invoice, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{topUpId}))
	require.NoError(t, err)
	require.NotNil(t, ticket)
	require.NotNil(t, invoice)

	// 占用期间同一订单不能重复申请
	_, _, _, _, err = CreateInvoiceTicket(createInvoiceTicketParams(user, []int{topUpId}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketInvoiceOrderDuplicate))

	// 用户关闭工单 → 待开票申请转为已取消
	_, err = CloseUserTicket(ticket.Id, user.Id)
	require.NoError(t, err)

	var cancelled TicketInvoice
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&cancelled).Error)
	assert.Equal(t, InvoiceStatusCancelled, cancelled.InvoiceStatus)

	// 已取消的申请禁止复活为已开票，避免订单释放后重复开票
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketInvoiceStatusInvalid))

	// 订单释放后可重新发起开票
	ticket2, invoice2, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{topUpId}))
	require.NoError(t, err)
	require.NotNil(t, ticket2)
	require.NotNil(t, invoice2)
	assert.NotEqual(t, invoice.Id, invoice2.Id)
	assert.Equal(t, InvoiceStatusPending, invoice2.InvoiceStatus)
}

func TestCreateInvoiceTicketInvoiceType(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	oldSpecialEnabled := operation_setting.InvoiceSpecialEnabled
	oldRegularFeeRate := operation_setting.InvoiceRegularFeeRate
	oldSpecialFeeRate := operation_setting.InvoiceSpecialFeeRate
	operation_setting.MinInvoiceAmount = 0
	operation_setting.InvoiceRegularFeeRate = 0
	operation_setting.InvoiceSpecialFeeRate = 6
	defer func() {
		operation_setting.MinInvoiceAmount = oldMinInvoiceAmount
		operation_setting.InvoiceSpecialEnabled = oldSpecialEnabled
		operation_setting.InvoiceRegularFeeRate = oldRegularFeeRate
		operation_setting.InvoiceSpecialFeeRate = oldSpecialFeeRate
	}()

	const userId = 910010
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)
	user := insertInvoiceTicketUser(t, userId)

	// 未指定票种时默认普票，费率取普票配置
	topUpId := seedInvoiceTicketTopUp(t, user.Id, "invoice-type-default-910010", 100)
	_, invoice, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{topUpId}))
	require.NoError(t, err)
	assert.Equal(t, InvoiceTypeRegular, invoice.InvoiceType)
	assert.Equal(t, float64(0), invoice.FeeRate)

	// 增票未开放时拒绝
	operation_setting.InvoiceSpecialEnabled = false
	topUpId2 := seedInvoiceTicketTopUp(t, user.Id, "invoice-type-disabled-910010", 100)
	params := createInvoiceTicketParams(user, []int{topUpId2})
	params.InvoiceType = InvoiceTypeSpecial
	_, _, _, _, err = CreateInvoiceTicket(params)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketInvoiceTypeDisabled))

	// 增票开放后成功，费率快照取增票配置
	operation_setting.InvoiceSpecialEnabled = true
	_, invoice2, message2, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	assert.Equal(t, InvoiceTypeSpecial, invoice2.InvoiceType)
	assert.Equal(t, float64(6), invoice2.FeeRate)
	assert.Contains(t, message2.Content, "增值税专用发票")
	assert.Contains(t, message2.Content, "手续费率：6%")

	// 非法票种拒绝
	topUpId3 := seedInvoiceTicketTopUp(t, user.Id, "invoice-type-invalid-910010", 100)
	params3 := createInvoiceTicketParams(user, []int{topUpId3})
	params3.InvoiceType = 99
	_, _, _, _, err = CreateInvoiceTicket(params3)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketInvoiceTypeInvalid))
}
