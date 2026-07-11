package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedInvoiceForStatus(t *testing.T, ticketId, userId, status int, orderIds string) {
	t.Helper()
	require.NoError(t, DB.Create(&TicketInvoice{
		TicketId:      ticketId,
		UserId:        userId,
		CompanyName:   "测试公司",
		TaxNumber:     "91330100MA27XW0000",
		Email:         "invoice@test.local",
		TopUpOrderIds: orderIds,
		InvoiceType:   InvoiceTypeRegular,
		InvoiceStatus: status,
	}).Error)
}

func TestTopUpListsCarryInvoiceStatus(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM top_ups").Error)
	require.NoError(t, DB.Exec("DELETE FROM ticket_invoices").Error)
	const uid = 90101

	seed := func(tradeNo, status string) int {
		tu := &TopUp{
			UserId:       uid,
			Money:        10,
			TradeNo:      tradeNo,
			Status:       status,
			CreateTime:   common.GetTimestamp(),
			CompleteTime: common.GetTimestamp(),
		}
		require.NoError(t, DB.Create(tu).Error)
		return tu.Id
	}
	idA := seed("inv-a", common.TopUpStatusSuccess)
	idB := seed("inv-b", common.TopUpStatusSuccess)
	idC := seed("inv-c", common.TopUpStatusSuccess)
	idD := seed("inv-d", common.TopUpStatusPending)

	seedInvoiceForStatus(t, 91001, uid, InvoiceStatusPending, fmt.Sprintf("[%d]", idA))
	seedInvoiceForStatus(t, 91002, uid, InvoiceStatusIssued, fmt.Sprintf("[%d]", idB))
	// 已驳回的发票释放订单，不得标记 C
	seedInvoiceForStatus(t, 91003, uid, InvoiceStatusRejected, fmt.Sprintf("[%d]", idC))

	items, total, err := GetUserTopUps(uid, TopUpFilter{}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(4), total)

	byId := make(map[int]*TopUp, len(items))
	for _, it := range items {
		byId[it.Id] = it
	}
	assert.Equal(t, InvoiceStatusPending, byId[idA].InvoiceStatus)
	assert.Equal(t, InvoiceStatusIssued, byId[idB].InvoiceStatus)
	assert.Equal(t, 0, byId[idC].InvoiceStatus)
	assert.Equal(t, 0, byId[idD].InvoiceStatus)

	// 管理员带用户名的列表同样回填（内嵌结构桥接路径）
	adminItems, _, err := GetAllTopUpsWithUsername(TopUpFilter{}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	adminById := make(map[int]*TopUpWithUsername, len(adminItems))
	for _, it := range adminItems {
		adminById[it.Id] = it
	}
	assert.Equal(t, InvoiceStatusPending, adminById[idA].InvoiceStatus)
	assert.Equal(t, InvoiceStatusIssued, adminById[idB].InvoiceStatus)
}

func TestTopUpInvoiceStatusSkipsMalformedOrderIds(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM top_ups").Error)
	require.NoError(t, DB.Exec("DELETE FROM ticket_invoices").Error)
	const uid = 90102

	tu := &TopUp{
		UserId:       uid,
		Money:        10,
		TradeNo:      "inv-bad-json",
		Status:       common.TopUpStatusSuccess,
		CreateTime:   common.GetTimestamp(),
		CompleteTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(tu).Error)
	seedInvoiceForStatus(t, 91011, uid, InvoiceStatusPending, "not-json")

	items, total, err := GetUserTopUps(uid, TopUpFilter{}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	assert.Equal(t, 0, items[0].InvoiceStatus)
}
