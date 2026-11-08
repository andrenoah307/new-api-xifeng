package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOriginalRefundInvoiceExportsAcrossOrdersAndInvoices(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error) })
	orders := []*TopUp{
		seedRefundFinanceOrder(t, f.user.Id, 10000, 9000, 0, 1),
		seedRefundFinanceOrder(t, f.user.Id, 10000, 9000, 0, 2),
		seedRefundFinanceOrder(t, f.user.Id, 10000, 9000, 0, 3),
	}
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": 25000, "used_quota": 5000}).Error)
	for i, ids := range [][]int{{orders[0].Id, orders[1].Id}, {orders[2].Id}} {
		invoice := TicketInvoice{TicketId: 800 + i, UserId: f.user.Id, CompanyName: "Original title", TaxNumber: "Original tax", Email: "original@example.com", InvoiceType: "special", VATRateBps: 600, InvoiceStatus: InvoiceStatusIssued, TotalMoney: float64(len(ids) * 90)}
		require.NoError(t, invoice.SetTopUpOrderIDs(ids))
		require.NoError(t, DB.Create(&invoice).Error)
	}
	params := CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 20000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()}
	ticket, refund, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	assert.Equal(t, 20000, refund.FrozenQuota)
	assert.Nil(t, refund.Finance)
	assert.Empty(t, refund.SettlementStatus)
	_, err = SaveRefundInvoiceExport(ticket.Id, f.user.Id)
	require.ErrorIs(t, err, ErrTicketForbidden)
	plan, err := SaveRefundInvoiceExport(ticket.Id, f.finance.Id)
	require.NoError(t, err)
	require.Len(t, plan.Payments, 3)
	assert.Equal(t, int64(18000), plan.CashCents)
	assert.Equal(t, []int64{4500, 9000, 4500}, []int64{plan.Payments[0].RefundCents, plan.Payments[1].RefundCents, plan.Payments[2].RefundCents})
	require.Len(t, plan.Invoices, 2)
	assert.Equal(t, int64(18000), plan.Invoices[0].OriginalCents)
	assert.Equal(t, int64(4500), plan.Invoices[0].ReissueCents)
	assert.Equal(t, int64(4500), plan.Invoices[1].ReissueCents)
	assert.Equal(t, "original@example.com", plan.Invoices[0].Email)
	repeated, err := SaveRefundInvoiceExport(ticket.Id, f.admin.Id)
	require.NoError(t, err)
	assert.Equal(t, plan, repeated)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 5000, owner.Quota)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.ErrorIs(t, err, ErrTicketRefundNotPending)
	params.RefundQuota, params.RequestKey = 5000, uuid.NewString()
	second, _, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	remaining, err := SaveRefundInvoiceExport(second.Id, f.admin.Id)
	require.NoError(t, err)
	require.Len(t, remaining.Payments, 1)
	assert.Equal(t, orders[2].Id, remaining.Payments[0].OrderID)
	assert.Equal(t, int64(4500), remaining.CashCents)
	require.Len(t, remaining.Invoices, 1)
	assert.Equal(t, int64(4500), remaining.Invoices[0].OriginalCents)
	assert.Zero(t, remaining.Invoices[0].ReissueCents)
	var invoices []TicketInvoice
	require.NoError(t, DB.Where("user_id = ?", f.user.Id).Find(&invoices).Error)
	for _, invoice := range invoices {
		assert.Equal(t, InvoiceStatusIssued, invoice.InvoiceStatus)
		assert.Equal(t, "special", invoice.InvoiceType)
	}
}

func TestOriginalRefundRejectionAndAdjustedApproval(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error) })
	order := seedRefundFinanceOrder(t, f.user.Id, 10000, 9000, 0, 1)
	// Legacy paid orders have no checkout snapshot; their recorded cash is retained.
	require.NoError(t, DB.Model(order).Updates(map[string]interface{}{"checkout_version": 0, "payment_total_cents": 0}).Error)
	require.NoError(t, DB.Model(&f.user).Update("quota", 10000).Error)
	params := CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 8000, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()}
	ticket, _, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	_, err = SaveRefundInvoiceExport(ticket.Id, f.admin.Id)
	require.NoError(t, err)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 10000, owner.Quota)
	params.RequestKey = uuid.NewString()
	ticket, _, _, err = CreateRefundTicket(params)
	require.NoError(t, err)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded, QuotaMode: RefundQuotaModeSubtract, ActualRefundQuota: 5000})
	require.NoError(t, err)
	plan, err := GetRefundInvoiceExport(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(4500), plan.CashCents)
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 5000, owner.Quota)
	var saved TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&saved).Error)
	assert.Equal(t, 5000, saved.RefundQuota)
	assert.Equal(t, int64(4500), saved.RefundCashCents)
	require.NoError(t, DB.First(order, order.Id).Error)
	remaining, err := adjustInvoiceOrdersForRefunds(DB, []*TopUp{order})
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, float64(45), remaining[0].Money)
	assert.Equal(t, float64(90), order.Money)
	assert.Equal(t, common.UserStatusEnabled, owner.Status)
}
