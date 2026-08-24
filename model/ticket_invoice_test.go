package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
		AffCode:  fmt.Sprintf("invoice_ticket_aff_%d", userId),
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

func TestCreateInvoiceTicketRemark(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 0
	defer func() { operation_setting.MinInvoiceAmount = oldMinInvoiceAmount }()

	testCases := []struct {
		name              string
		userId            int
		content           string
		wantRemark        string
		wantRemarkSection bool
	}{
		{
			name:       "empty remark",
			userId:     910010,
			content:    "",
			wantRemark: "",
		},
		{
			name:              "trims surrounding whitespace",
			userId:            910011,
			content:           " \n 用于项目报销\t ",
			wantRemark:        "用于项目报销",
			wantRemarkSection: true,
		},
		{
			name:              "accepts exactly one hundred Chinese characters",
			userId:            910012,
			content:           strings.Repeat("注", 100),
			wantRemark:        strings.Repeat("注", 100),
			wantRemarkSection: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			user := insertInvoiceTicketUser(t, testCase.userId)
			topUpId := seedInvoiceTicketTopUp(
				t,
				user.Id,
				fmt.Sprintf("invoice-remark-%d", testCase.userId),
				1,
			)
			params := createInvoiceTicketParams(user, []int{topUpId})
			params.Content = testCase.content

			ticket, invoice, message, orders, err := CreateInvoiceTicket(params)

			require.NoError(t, err)
			require.NotNil(t, ticket)
			require.NotNil(t, invoice)
			require.NotNil(t, message)
			require.Len(t, orders, 1)

			var persistedInvoice TicketInvoice
			require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&persistedInvoice).Error)
			assert.Equal(t, testCase.wantRemark, invoice.Remark)
			assert.Equal(t, testCase.wantRemark, persistedInvoice.Remark)
			assert.Equal(t, testCase.wantRemarkSection, strings.Contains(message.Content, "\n备注：\n"))

			if testCase.wantRemarkSection {
				_, summaryRemark, found := strings.Cut(message.Content, "\n备注：\n")
				require.True(t, found)
				assert.Equal(t, persistedInvoice.Remark, summaryRemark)
			}
		})
	}
}

func TestCreateInvoiceTicketRejectsRemarkOverOneHundredCharacters(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 0
	defer func() { operation_setting.MinInvoiceAmount = oldMinInvoiceAmount }()

	const userId = 910013
	user := insertInvoiceTicketUser(t, userId)
	topUpId := seedInvoiceTicketTopUp(t, user.Id, "invoice-remark-too-long-910013", 1)
	params := createInvoiceTicketParams(user, []int{topUpId})
	params.Content = strings.Repeat("注", 101)

	ticket, invoice, message, orders, err := CreateInvoiceTicket(params)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTicketInvoiceRemarkTooLong)
	assert.Nil(t, ticket)
	assert.Nil(t, invoice)
	assert.Nil(t, message)
	assert.Nil(t, orders)

	modelCases := []struct {
		name  string
		value any
	}{
		{name: "ticket", value: &Ticket{}},
		{name: "invoice", value: &TicketInvoice{}},
		{name: "message", value: &TicketMessage{}},
	}
	for _, modelCase := range modelCases {
		t.Run(modelCase.name, func(t *testing.T) {
			var count int64
			require.NoError(t, DB.Model(modelCase.value).Where("user_id = ?", user.Id).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestCreateInvoiceTicketValidation(t *testing.T) {
	testCases := []struct {
		name    string
		params  CreateInvoiceTicketParams
		wantErr error
	}{
		{
			name: "company name required",
			params: CreateInvoiceTicketParams{
				TaxNumber:     "91110000ABCDEFGH1X",
				Email:         "invoice@example.com",
				TopUpOrderIds: []int{1},
			},
			wantErr: ErrTicketInvoiceCompanyEmpty,
		},
		{
			name: "tax number required",
			params: CreateInvoiceTicketParams{
				CompanyName:   "Test Company",
				Email:         "invoice@example.com",
				TopUpOrderIds: []int{1},
			},
			wantErr: ErrTicketInvoiceTaxNumberEmpty,
		},
		{
			name: "tax number format",
			params: CreateInvoiceTicketParams{
				CompanyName:   "Test Company",
				TaxNumber:     "invalid",
				Email:         "invoice@example.com",
				TopUpOrderIds: []int{1},
			},
			wantErr: ErrTicketInvoiceTaxNumberFormat,
		},
		{
			name: "email required",
			params: CreateInvoiceTicketParams{
				CompanyName:   "Test Company",
				TaxNumber:     "91110000ABCDEFGH1X",
				TopUpOrderIds: []int{1},
			},
			wantErr: ErrTicketInvoiceEmailEmpty,
		},
		{
			name: "order required",
			params: CreateInvoiceTicketParams{
				CompanyName: "Test Company",
				TaxNumber:   "91110000ABCDEFGH1X",
				Email:       "invoice@example.com",
			},
			wantErr: ErrTicketInvoiceOrderEmpty,
		},
		{
			name: "order id positive",
			params: CreateInvoiceTicketParams{
				CompanyName:   "Test Company",
				TaxNumber:     "91110000ABCDEFGH1X",
				Email:         "invoice@example.com",
				TopUpOrderIds: []int{0},
			},
			wantErr: ErrTicketInvoiceOrderInvalid,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ticket, invoice, message, orders, err := CreateInvoiceTicket(testCase.params)

			require.Error(t, err)
			assert.ErrorIs(t, err, testCase.wantErr)
			assert.Nil(t, ticket)
			assert.Nil(t, invoice)
			assert.Nil(t, message)
			assert.Nil(t, orders)
		})
	}
}

func TestCreateInvoiceTicketRejectsInvalidAndDuplicateOrders(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 0
	defer func() { operation_setting.MinInvoiceAmount = oldMinInvoiceAmount }()

	t.Run("invalid order", func(t *testing.T) {
		const userId = 910020
		user := insertInvoiceTicketUser(t, userId)
		params := createInvoiceTicketParams(user, []int{99999999})

		ticket, invoice, message, orders, err := CreateInvoiceTicket(params)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
		assert.Nil(t, ticket)
		assert.Nil(t, invoice)
		assert.Nil(t, message)
		assert.Nil(t, orders)
	})

	t.Run("duplicate order", func(t *testing.T) {
		const userId = 910021
		user := insertInvoiceTicketUser(t, userId)
		topUpId := seedInvoiceTicketTopUp(t, user.Id, "invoice-duplicate-910021", 1)
		params := createInvoiceTicketParams(user, []int{topUpId})
		_, _, _, _, err := CreateInvoiceTicket(params)
		require.NoError(t, err)

		ticket, invoice, message, orders, err := CreateInvoiceTicket(params)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTicketInvoiceOrderDuplicate)
		assert.Nil(t, ticket)
		assert.Nil(t, invoice)
		assert.Nil(t, message)
		assert.Nil(t, orders)

		for _, modelValue := range []any{&Ticket{}, &TicketInvoice{}, &TicketMessage{}} {
			var count int64
			require.NoError(t, DB.Model(modelValue).Where("user_id = ?", user.Id).Count(&count).Error)
			assert.Equal(t, int64(1), count)
		}
	})
}

func TestCreateInvoiceTicketReturnsDatabaseError(t *testing.T) {
	originalDB := DB
	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = brokenDB
	t.Cleanup(func() { DB = originalDB })

	params := CreateInvoiceTicketParams{
		CompanyName:   "Test Company",
		TaxNumber:     "91110000ABCDEFGH1X",
		Email:         "invoice@example.com",
		TopUpOrderIds: []int{1},
	}

	ticket, invoice, message, orders, err := CreateInvoiceTicket(params)

	require.Error(t, err)
	assert.Nil(t, ticket)
	assert.Nil(t, invoice)
	assert.Nil(t, message)
	assert.Nil(t, orders)
}
