package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinanceInvoicesKeepInvoiceAmountAndUserBoundary(t *testing.T) {
	truncateTables(t)
	u := insertInvoiceTicketUser(t, 920001)
	other := insertInvoiceTicketUser(t, 920002)
	a := seedInvoiceTicketTopUp(t, u.Id, "finance-a", 12.5)
	b := seedInvoiceTicketTopUp(t, u.Id, "finance-b", 20)
	foreign := seedInvoiceTicketTopUp(t, other.Id, "finance-foreign", 999)
	ticket := &Ticket{UserId: u.Id, Username: u.Username, Type: TicketTypeInvoice, Status: TicketStatusOpen}
	require.NoError(t, DB.Create(ticket).Error)
	invoice := &TicketInvoice{TicketId: ticket.Id, UserId: u.Id, CompanyName: "Example Co", TaxNumber: "tax", Email: "finance@example.test", TotalMoney: 30, InvoiceStatus: InvoiceStatusPending}
	require.NoError(t, invoice.SetTopUpOrderIDs([]int{a, b, foreign}))
	require.NoError(t, DB.Create(invoice).Error)
	items, total, err := ListFinanceRecords(context.Background(), FinanceFilter{View: "invoices"}, DefaultFinancePolicy(), &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, 30.0, items[0].Money)
	assert.Equal(t, "Example Co", items[0].CompanyName)
	require.Len(t, items[0].TopUps, 2)
	assert.Equal(t, "finance-a", items[0].TopUps[0].TradeNo)
	assert.Equal(t, "finance-b", items[0].TopUps[1].TradeNo)
	assert.True(t, items[0].IncompleteOrders)

	policy := DefaultFinancePolicy()
	policy.InvoiceStatuses = []int{InvoiceStatusIssued}
	items, total, err = ListFinanceRecords(context.Background(), FinanceFilter{View: "invoices", InvoiceStatus: InvoiceStatusPending}, policy, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Zero(t, total)

	items, total, err = ListFinanceRecords(context.Background(), FinanceFilter{View: "topups"}, DefaultFinancePolicy(), &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Zero(t, total)
}

func TestFinancePolicyDefaultsAndInvalidStoredPolicy(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where(Option{Key: FinancePolicyKey}).Delete(&Option{}).Error)
	policy, err := GetFinancePolicy()
	require.NoError(t, err)
	assert.Equal(t, DefaultFinancePolicy(), policy)
	policy.TopUpStatuses = []string{common.TopUpStatusSuccess}
	require.NoError(t, SaveFinancePolicy(policy))
	got, err := GetFinancePolicy()
	require.NoError(t, err)
	assert.Equal(t, policy, got)
	require.NoError(t, DB.Model(&Option{}).Where(Option{Key: FinancePolicyKey}).Update("value", "invalid").Error)
	_, err = GetFinancePolicy()
	require.Error(t, err)
	t.Cleanup(func() { DB.Where(Option{Key: FinancePolicyKey}).Delete(&Option{}) })
}

func TestFinancePaymentScopeNeverExposesPartialInvoices(t *testing.T) {
	truncateTables(t)
	u := insertInvoiceTicketUser(t, 920003)
	a := seedInvoiceTicketTopUp(t, u.Id, "scope-alipay", 10)
	b := seedInvoiceTicketTopUp(t, u.Id, "scope-stripe", 20)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", a).Updates(map[string]interface{}{"payment_method": "alipay", "source": "online"}).Error)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", b).Updates(map[string]interface{}{"payment_method": "stripe", "source": "online"}).Error)
	for _, ids := range [][]int{{a}, {a, b}} {
		ticket := &Ticket{UserId: u.Id, Username: u.Username, Type: TicketTypeInvoice, Status: TicketStatusOpen}
		require.NoError(t, DB.Create(ticket).Error)
		invoice := &TicketInvoice{TicketId: ticket.Id, UserId: u.Id, CompanyName: "Invoice title", TaxNumber: "tax", Email: "test@example.test", TotalMoney: 10, InvoiceStatus: InvoiceStatusPending}
		require.NoError(t, invoice.SetTopUpOrderIDs(ids))
		require.NoError(t, DB.Create(invoice).Error)
	}
	policy := DefaultFinancePolicy()
	policy.PaymentMethods = []string{"alipay"}
	policy.Sources = []string{"online"}
	policy.TopUpStatuses = []string{common.TopUpStatusSuccess}
	page := &common.PageInfo{Page: 1, PageSize: 1}
	items, total, err := ListFinanceRecords(context.Background(), FinanceFilter{View: "invoices"}, policy, page)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, 10.0, items[0].Money)
	require.Len(t, items[0].TopUps, 1)
	assert.Equal(t, a, items[0].TopUps[0].Id)
	page.Page = 2
	items, total, err = ListFinanceRecords(context.Background(), FinanceFilter{View: "invoices"}, policy, page)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Empty(t, items)
	page.Page = 1
	items, total, err = ListFinanceRecords(context.Background(), FinanceFilter{View: "topups", Status: "success"}, policy, page)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, a, items[0].Id)
	assert.Equal(t, "Invoice title", items[0].CompanyName)
	policy.Sources = []string{"manual"}
	items, total, err = ListFinanceRecords(context.Background(), FinanceFilter{View: "invoices"}, policy, page)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Zero(t, total)
}
