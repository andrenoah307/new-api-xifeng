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

// seedInvoiceTicketTopUpWithQuota 建带实际到账额度的充值订单：开票手续费按 quota_granted 计费，
// 老的 seedInvoiceTicketTopUp 不设该字段（保留原样以免影响既有用例）。
func seedInvoiceTicketTopUpWithQuota(t *testing.T, userId int, tradeNo string, money float64, quotaGranted int64) int {
	t.Helper()
	now := common.GetTimestamp()
	topUp := &TopUp{
		UserId:       userId,
		Money:        money,
		QuotaGranted: quotaGranted,
		TradeNo:      tradeNo,
		Status:       "success",
		CreateTime:   now,
		CompleteTime: now,
	}
	require.NoError(t, DB.Create(topUp).Error)
	return topUp.Id
}

func setInvoiceTicketUserQuota(t *testing.T, userId int, quota int) {
	t.Helper()
	require.NoError(t, DB.Model(&User{}).Where("id = ?", userId).Update("quota", quota).Error)
}

func invoiceTicketUserQuota(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.First(&user, "id = ?", userId).Error)
	return user.Quota
}

func reloadTicketInvoice(t *testing.T, ticketId int) *TicketInvoice {
	t.Helper()
	var invoice TicketInvoice
	require.NoError(t, DB.Where("ticket_id = ?", ticketId).First(&invoice).Error)
	return &invoice
}

func cleanupInvoiceTicketData(t *testing.T, userId int) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM ticket_invoices WHERE user_id = ?", userId).Error)
	require.NoError(t, DB.Exec("DELETE FROM ticket_messages WHERE user_id = ?", userId).Error)
	require.NoError(t, DB.Exec("DELETE FROM tickets WHERE user_id = ?", userId).Error)
	require.NoError(t, DB.Exec("DELETE FROM top_ups WHERE user_id = ?", userId).Error)
	require.NoError(t, DB.Exec("DELETE FROM logs WHERE user_id = ?", userId).Error)
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

func TestUpdateInvoiceStatusChargesFeeAtSnapshotRate(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	oldRegularFeeRate := operation_setting.InvoiceRegularFeeRate
	operation_setting.MinInvoiceAmount = 0
	operation_setting.InvoiceRegularFeeRate = 6
	defer func() {
		operation_setting.MinInvoiceAmount = oldMinInvoiceAmount
		operation_setting.InvoiceRegularFeeRate = oldRegularFeeRate
	}()

	const userId = 910020
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	setInvoiceTicketUserQuota(t, userId, 100000)
	orderId1 := seedInvoiceTicketTopUpWithQuota(t, userId, "invoice-fee-snapshot-1-910020", 100, 500000)
	orderId2 := seedInvoiceTicketTopUpWithQuota(t, userId, "invoice-fee-snapshot-2-910020", 100, 500000)

	ticket, invoice, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{orderId1, orderId2}))
	require.NoError(t, err)
	require.Equal(t, float64(6), invoice.FeeRate)

	// 申请之后调高配置费率：扣费必须走申请时的快照，不读当前配置
	operation_setting.InvoiceRegularFeeRate = 20

	updated, updatedTicket, prevStatus, err := UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updatedTicket)
	assert.Equal(t, TicketStatusOpen, prevStatus)
	assert.Equal(t, TicketStatusResolved, updatedTicket.Status)
	assert.Equal(t, InvoiceStatusIssued, updated.InvoiceStatus)
	// (500000 + 500000) * 6% = 60000，而非按 20% 的 200000
	assert.Equal(t, 60000, updated.FeeQuota)
	assert.Greater(t, updated.FeeChargedTime, int64(0))
	assert.Equal(t, 40000, invoiceTicketUserQuota(t, userId))

	persisted := reloadTicketInvoice(t, ticket.Id)
	assert.Equal(t, 60000, persisted.FeeQuota)
	assert.Equal(t, updated.FeeChargedTime, persisted.FeeChargedTime)
	assert.Greater(t, persisted.IssuedTime, int64(0))

	// 扣费必须留下额度流水，否则用户看不到钱去哪了
	var feeLogCount int64
	require.NoError(t, LOG_DB.Model(&Log{}).
		Where("user_id = ? AND type = ?", userId, LogTypeManage).
		Where("content LIKE ?", "%开票手续费%").
		Count(&feeLogCount).Error)
	assert.EqualValues(t, 1, feeLogCount)
}

func TestUpdateInvoiceStatusChargesFeeOnlyOnce(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	oldRegularFeeRate := operation_setting.InvoiceRegularFeeRate
	operation_setting.MinInvoiceAmount = 0
	operation_setting.InvoiceRegularFeeRate = 6
	defer func() {
		operation_setting.MinInvoiceAmount = oldMinInvoiceAmount
		operation_setting.InvoiceRegularFeeRate = oldRegularFeeRate
	}()

	const userId = 910021
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	setInvoiceTicketUserQuota(t, userId, 100000)
	orderId := seedInvoiceTicketTopUpWithQuota(t, userId, "invoice-fee-once-910021", 100, 500000)

	ticket, _, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{orderId}))
	require.NoError(t, err)

	issued, _, _, err := UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.NoError(t, err)
	require.Equal(t, 30000, issued.FeeQuota)
	firstChargedTime := issued.FeeChargedTime
	require.Greater(t, firstChargedTime, int64(0))
	require.Equal(t, 70000, invoiceTicketUserQuota(t, userId))

	// 重复标记已开票是幂等的：不再扣费、幂等位不变
	again, _, _, err := UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.NoError(t, err)
	assert.Equal(t, 30000, again.FeeQuota)
	assert.Equal(t, firstChargedTime, again.FeeChargedTime)
	assert.Equal(t, 70000, invoiceTicketUserQuota(t, userId))

	// 管理员纠错回退：手续费不自动退还
	rejected, _, _, err := UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusRejected)
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusRejected, rejected.InvoiceStatus)
	assert.Equal(t, 30000, reloadTicketInvoice(t, ticket.Id).FeeQuota)
	assert.Equal(t, 70000, invoiceTicketUserQuota(t, userId))

	// 回退后再次开票不能二次扣费
	reissued, _, _, err := UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.NoError(t, err)
	assert.Equal(t, 30000, reissued.FeeQuota)
	assert.Equal(t, firstChargedTime, reissued.FeeChargedTime)
	assert.Equal(t, 70000, invoiceTicketUserQuota(t, userId))
}

func TestUpdateInvoiceStatusInsufficientBalanceRollsBack(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	oldRegularFeeRate := operation_setting.InvoiceRegularFeeRate
	operation_setting.MinInvoiceAmount = 0
	operation_setting.InvoiceRegularFeeRate = 6
	defer func() {
		operation_setting.MinInvoiceAmount = oldMinInvoiceAmount
		operation_setting.InvoiceRegularFeeRate = oldRegularFeeRate
	}()

	const userId = 910022
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	setInvoiceTicketUserQuota(t, userId, 100)
	orderId := seedInvoiceTicketTopUpWithQuota(t, userId, "invoice-fee-insufficient-910022", 100, 500000)

	ticket, _, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{orderId}))
	require.NoError(t, err)

	_, _, _, err = UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.Error(t, err)
	var feeErr *InvoiceFeeInsufficientError
	require.True(t, errors.As(err, &feeErr))
	assert.Equal(t, 30000, feeErr.Required)
	assert.Equal(t, 100, feeErr.Balance)

	// 整个事务回滚：发票仍待开票、未打幂等位，余额与工单状态不变，随后可重试
	persisted := reloadTicketInvoice(t, ticket.Id)
	assert.Equal(t, InvoiceStatusPending, persisted.InvoiceStatus)
	assert.Zero(t, persisted.FeeQuota)
	assert.Zero(t, persisted.FeeChargedTime)
	assert.Zero(t, persisted.IssuedTime)
	assert.Equal(t, 100, invoiceTicketUserQuota(t, userId))
	var reloadedTicket Ticket
	require.NoError(t, DB.First(&reloadedTicket, "id = ?", ticket.Id).Error)
	assert.Equal(t, TicketStatusOpen, reloadedTicket.Status)

	// 用户补足余额后同一张发票可重试
	setInvoiceTicketUserQuota(t, userId, 30000)
	issued, _, _, err := UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.NoError(t, err)
	assert.Equal(t, 30000, issued.FeeQuota)
	assert.Zero(t, invoiceTicketUserQuota(t, userId))
}

func TestUpdateInvoiceStatusZeroFeeRateStillStampsSettlement(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	oldRegularFeeRate := operation_setting.InvoiceRegularFeeRate
	operation_setting.MinInvoiceAmount = 0
	operation_setting.InvoiceRegularFeeRate = 0
	defer func() {
		operation_setting.MinInvoiceAmount = oldMinInvoiceAmount
		operation_setting.InvoiceRegularFeeRate = oldRegularFeeRate
	}()

	const userId = 910023
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	setInvoiceTicketUserQuota(t, userId, 100000)
	orderId := seedInvoiceTicketTopUpWithQuota(t, userId, "invoice-fee-zero-910023", 100, 500000)

	ticket, invoice, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{orderId}))
	require.NoError(t, err)
	require.Equal(t, float64(0), invoice.FeeRate)

	issued, _, _, err := UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.NoError(t, err)
	assert.Zero(t, issued.FeeQuota)
	// 费率 0 也要打戳，否则日后调高费率会追扣这张老发票
	assert.Greater(t, issued.FeeChargedTime, int64(0))
	assert.Equal(t, 100000, invoiceTicketUserQuota(t, userId))

	// 事后把费率快照改成 6%（模拟人工改数据/历史配置变更）并回退状态，幂等位必须挡住追扣
	require.NoError(t, DB.Model(&TicketInvoice{}).Where("id = ?", invoice.Id).Update("fee_rate", 6).Error)
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusRejected)
	require.NoError(t, err)
	reissued, _, _, err := UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.NoError(t, err)
	assert.Zero(t, reissued.FeeQuota)
	assert.Equal(t, 100000, invoiceTicketUserQuota(t, userId))
}

func TestUpdateInvoiceStatusMissingQuotaGrantedFailsFast(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	oldRegularFeeRate := operation_setting.InvoiceRegularFeeRate
	operation_setting.MinInvoiceAmount = 0
	operation_setting.InvoiceRegularFeeRate = 6
	defer func() {
		operation_setting.MinInvoiceAmount = oldMinInvoiceAmount
		operation_setting.InvoiceRegularFeeRate = oldRegularFeeRate
	}()

	const userId = 910024
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	setInvoiceTicketUserQuota(t, userId, 100000)
	// 历史订单没有 quota_granted：按 0 计费会少收钱，必须硬失败让管理员先修数据
	orderId := seedInvoiceTicketTopUp(t, userId, "invoice-fee-no-quota-910024", 100)

	ticket, invoice, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{orderId}))
	require.NoError(t, err)

	_, _, _, err = UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketInvoiceFeeBaseInvalid))

	persisted := reloadTicketInvoice(t, ticket.Id)
	assert.Equal(t, InvoiceStatusPending, persisted.InvoiceStatus)
	assert.Zero(t, persisted.FeeChargedTime)
	assert.Equal(t, 100000, invoiceTicketUserQuota(t, userId))

	// 费率为 0 时不看计费基数，同一张发票应能正常开票
	require.NoError(t, DB.Model(&TicketInvoice{}).Where("id = ?", invoice.Id).Update("fee_rate", 0).Error)
	issued, _, _, err := UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.NoError(t, err)
	assert.Zero(t, issued.FeeQuota)
	assert.Greater(t, issued.FeeChargedTime, int64(0))
	assert.Equal(t, 100000, invoiceTicketUserQuota(t, userId))
}

func TestChargedRejectedInvoiceKeepsOrdersLocked(t *testing.T) {
	truncateTables(t)
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	oldRegularFeeRate := operation_setting.InvoiceRegularFeeRate
	operation_setting.MinInvoiceAmount = 0
	operation_setting.InvoiceRegularFeeRate = 6
	defer func() {
		operation_setting.MinInvoiceAmount = oldMinInvoiceAmount
		operation_setting.InvoiceRegularFeeRate = oldRegularFeeRate
	}()

	const userId = 910025
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	user := insertInvoiceTicketUser(t, userId)
	setInvoiceTicketUserQuota(t, userId, 100000)
	orderId := seedInvoiceTicketTopUpWithQuota(t, userId, "invoice-fee-lock-910025", 100, 500000)

	ticket, invoice, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{orderId}))
	require.NoError(t, err)

	_, _, _, err = UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusIssued)
	require.NoError(t, err)
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, 1, InvoiceStatusRejected)
	require.NoError(t, err)

	// 已扣费的驳回发票继续占用订单，否则用户对同批订单再申请会被二次扣费
	eligible, err := GetEligibleInvoiceOrders(userId)
	require.NoError(t, err)
	assert.Empty(t, eligible)

	_, _, _, _, err = CreateInvoiceTicket(createInvoiceTicketParams(user, []int{orderId}))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketInvoiceOrderDuplicate))

	// 逃生舱：人工退费后把 fee_quota 置 0 即可释放订单
	require.NoError(t, DB.Model(&TicketInvoice{}).Where("id = ?", invoice.Id).Update("fee_quota", 0).Error)
	eligible, err = GetEligibleInvoiceOrders(userId)
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	assert.Equal(t, orderId, eligible[0].Id)

	ticket2, _, _, _, err := CreateInvoiceTicket(createInvoiceTicketParams(user, []int{orderId}))
	require.NoError(t, err)
	assert.NotEqual(t, ticket.Id, ticket2.Id)
}

func TestBackfillInvoiceLegacyFeeChargedOnlyMarksIssued(t *testing.T) {
	truncateTables(t)

	const userId = 910026
	cleanupInvoiceTicketData(t, userId)
	defer cleanupInvoiceTicketData(t, userId)

	insertInvoiceTicketUser(t, userId)
	seed := func(ticketId, status int, feeChargedTime int64) int {
		invoice := &TicketInvoice{
			TicketId:       ticketId,
			UserId:         userId,
			CompanyName:    "Legacy Company",
			TaxNumber:      "91110000ABCDEFGH1X",
			Email:          "invoice@example.com",
			TopUpOrderIds:  "[1]",
			InvoiceStatus:  status,
			FeeChargedTime: feeChargedTime,
		}
		require.NoError(t, DB.Create(invoice).Error)
		return invoice.Id
	}
	pendingId := seed(9100261, InvoiceStatusPending, 0)
	legacyIssuedId := seed(9100262, InvoiceStatusIssued, 0)
	settledIssuedId := seed(9100263, InvoiceStatusIssued, 123)
	rejectedId := seed(9100264, InvoiceStatusRejected, 0)

	feeChargedTimeOf := func(id int) int64 {
		var invoice TicketInvoice
		require.NoError(t, DB.First(&invoice, "id = ?", id).Error)
		return invoice.FeeChargedTime
	}

	backfillInvoiceLegacyFeeCharged()

	// 只有「上线前就已开票」的行被视同已结算，禁止追扣
	assert.EqualValues(t, -1, feeChargedTimeOf(legacyIssuedId))
	assert.EqualValues(t, 0, feeChargedTimeOf(pendingId))
	assert.EqualValues(t, 123, feeChargedTimeOf(settledIssuedId))
	assert.EqualValues(t, 0, feeChargedTimeOf(rejectedId))

	// 天然幂等，重跑不改变任何行
	backfillInvoiceLegacyFeeCharged()
	assert.EqualValues(t, -1, feeChargedTimeOf(legacyIssuedId))
	assert.EqualValues(t, 0, feeChargedTimeOf(pendingId))
	assert.EqualValues(t, 123, feeChargedTimeOf(settledIssuedId))
	assert.EqualValues(t, 0, feeChargedTimeOf(rejectedId))
}
