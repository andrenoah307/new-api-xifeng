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
	for _, order := range orders {
		require.NoError(t, DB.Table("top_ups").Where("id = ?", order.Id).Update("epay_credentials", `{"gateway_id":"public","account_name":"Company account"}`).Error)
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
	require.NoError(t, DB.First(ticket, ticket.Id).Error)
	assert.Equal(t, TicketStatusProcessing, ticket.Status)
	_, err = CloseUserTicket(ticket.Id, f.user.Id)
	require.ErrorIs(t, err, ErrTicketForbidden)
	// Resetting the presentation status must not discard the persisted processing evidence.
	require.NoError(t, DB.Model(ticket).Updates(map[string]interface{}{"status": TicketStatusOpen, "admin_id": 0}).Error)
	_, err = CloseUserTicket(ticket.Id, f.user.Id)
	require.ErrorIs(t, err, ErrTicketForbidden)
	require.Len(t, plan.Payments, 3)
	assert.Equal(t, int64(18000), plan.CashCents)
	assert.Equal(t, []int64{4500, 9000, 4500}, []int64{plan.Payments[0].RefundCents, plan.Payments[1].RefundCents, plan.Payments[2].RefundCents})
	for i, payment := range plan.Payments {
		require.NotNil(t, payment.Calculation)
		assert.Equal(t, []int64{0, 0, 4500}[i], payment.Calculation.RemainingAfterCents)
		assert.Equal(t, payment.PaidCents, payment.UsedCents+payment.Calculation.PreviouslyRefundedCents+payment.RefundCents+payment.Calculation.RemainingAfterCents)
	}
	require.Len(t, plan.Invoices, 2)
	assert.Equal(t, int64(18000), plan.Invoices[0].OriginalCents)
	assert.Equal(t, int64(4500), plan.Invoices[0].ReissueCents)
	assert.Zero(t, plan.Invoices[1].ReissueCents)
	assert.Equal(t, []InvoiceIssuerAmount{{"Company account", 4500}}, plan.Invoices[1].UninvoicedIssuerAmounts)
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
	require.NotNil(t, remaining.Payments[0].Calculation)
	assert.Equal(t, int64(4500), remaining.Payments[0].Calculation.PreviouslyRefundedCents)
	assert.Zero(t, remaining.Payments[0].Calculation.RemainingAfterCents)
	require.Len(t, remaining.Invoices, 1)
	assert.Zero(t, remaining.Invoices[0].OriginalCents)
	assert.Zero(t, remaining.Invoices[0].ReissueCents)
	listed, total, err := ListRefundCorrectionsForExport(InvoiceExportFilter{Keyword: "original@example.com"}, &common.PageInfo{Page: 1, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, listed, 1)
	assert.Equal(t, ticket.Id, listed[0].TicketId)
	require.Len(t, listed[0].CorrectionRows, 3)
	assert.Equal(t, "red", listed[0].CorrectionRows[0].InvoiceColor)
	assert.Equal(t, float64(180), listed[0].CorrectionRows[0].TotalMoney)
	assert.Equal(t, []string{orders[0].TradeNo, orders[1].TradeNo}, listed[0].CorrectionRows[0].OrderNumbers)
	assert.Equal(t, "Company account", listed[0].CorrectionRows[0].IssuerAmounts[0].AccountName)
	assert.Equal(t, []string{"red", "blue", "red"}, []string{listed[0].CorrectionRows[0].InvoiceColor, listed[0].CorrectionRows[1].InvoiceColor, listed[0].CorrectionRows[2].InvoiceColor})
	listed, total, err = ListRefundCorrectionsForExport(InvoiceExportFilter{}, &common.PageInfo{Page: 2, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Empty(t, listed)
	listed, total, err = ListRefundCorrectionsForExport(InvoiceExportFilter{TicketStatus: TicketStatusClosed, StartTime: common.GetTimestamp() + 100}, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, listed)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: second.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.NoError(t, err)
	_, total, err = ListRefundCorrectionsForExport(InvoiceExportFilter{}, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
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

func TestOriginalRefundPreservesLegacyGiftRecovery(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error) })
	order := seedRefundFinanceOrder(t, f.user.Id, 90000, 9000, 0, 1)
	bonus := TopUp{UserId: f.user.Id, TradeNo: order.TradeNo + "_bonus", Source: "discount_bonus", QuotaGranted: 10000, Status: common.TopUpStatusSuccess}
	require.NoError(t, DB.Create(&bonus).Error)
	require.NoError(t, DB.Model(&f.user).Update("quota", 50000).Error)
	ticket := Ticket{UserId: f.user.Id, Type: TicketTypeRefund, Status: TicketStatusOpen}
	require.NoError(t, DB.Create(&ticket).Error)
	refund := TicketRefund{TicketId: ticket.Id, UserId: f.user.Id, RefundStatus: RefundStatusPending, RefundQuota: 45000, FrozenQuota: 50000, RefundGiftQuota: 5000, FinanceVersion: 1, SettlementVerified: true}
	require.NoError(t, DB.Create(&refund).Error)
	allocation := RefundOrderAllocation{RefundTicketID: ticket.Id, TopUpID: order.Id, UserID: f.user.Id, OriginalQuota: 90000, RefundQuota: 45000, OriginalPaidCents: 9000, RefundCents: 4500, RefundGiftQuota: 5000, OriginalGiftQuota: 10000, GiftPolicyVersion: 1, Status: "reserved"}
	require.NoError(t, DB.Create(&allocation).Error)
	attempt := RefundPayoutAttempt{TicketId: ticket.Id, UserId: f.user.Id, RequestKey: uuid.NewString(), Status: "unknown", CashCents: 4500}
	require.NoError(t, DB.Create(&attempt).Error)
	_, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.ErrorIs(t, err, ErrRefundLegacyWorkflow)
	require.NoError(t, DB.First(&refund, refund.Id).Error)
	assert.Equal(t, 1, refund.FinanceVersion)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
	require.NoError(t, DB.Model(&attempt).Update("status", "failed").Error)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 50000, owner.Quota)
	require.NoError(t, DB.First(&refund, refund.Id).Error)
	assert.Equal(t, int64(5000), refund.RefundGiftQuota)
	assert.Equal(t, 2, refund.FinanceVersion)
	require.NoError(t, DB.First(&allocation, allocation.ID).Error)
	assert.Equal(t, "committed", allocation.Status)
	assert.Equal(t, 1, allocation.GiftPolicyVersion)
	assert.Equal(t, int64(5000), allocation.RefundGiftQuota)
}
