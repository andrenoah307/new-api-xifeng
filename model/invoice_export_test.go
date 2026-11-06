package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceExportIssuerAmounts(t *testing.T) {
	truncateTables(t)
	orders := []TopUp{
		{UserId: 767, TradeNo: "issuer-a", Money: 11, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"a","account_name":"公账A","key":"never-export"}`},
		{UserId: 767, TradeNo: "issuer-b", Money: 30, Status: common.TopUpStatusSuccess, CheckoutVersion: 1, PaymentTotalCents: 3000, EpayCredentials: `{"gateway_id":"b","account_name":"公账B"}`},
		{UserId: 767, TradeNo: "issuer-a2", Money: 2, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"a2","account_name":"公账A"}`},
		{UserId: 767, TradeNo: "issuer-private", Money: 3, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"private","account_name":"private-name"}`},
		{UserId: 768, TradeNo: "issuer-other-user", Money: 4, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"c","account_name":"other-user"}`},
	}
	require.NoError(t, DB.Create(&orders).Error)
	for _, tc := range []struct {
		name  string
		total float64
		ids   []int
		want  []InvoiceIssuerAmount
	}{
		{"split", 41, []int{orders[0].Id, orders[1].Id}, []InvoiceIssuerAmount{{"公账A", 1100}, {"公账B", 3000}}},
		{"aggregate and deduplicate", 43, []int{orders[0].Id, orders[1].Id, orders[2].Id, orders[0].Id}, []InvoiceIssuerAmount{{"公账A", 1300}, {"公账B", 3000}}},
		{"unknown and supplemental", 17, []int{orders[0].Id, orders[3].Id}, []InvoiceIssuerAmount{{"公账A", 1100}, {"-", 600}}},
		{"ownership and missing", 15, []int{orders[0].Id, orders[4].Id, -1}, []InvoiceIssuerAmount{{"公账A", 1100}, {"-", 400}}},
		{"refund allocation unknown", 35, []int{orders[0].Id, orders[1].Id}, []InvoiceIssuerAmount{{"-", 3500}}},
		{"no orders", 41, nil, []InvoiceIssuerAmount{{"-", 4100}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := []*InvoiceExportItem{{UserId: 767, TotalMoney: tc.total}, {UserId: 767, TotalMoney: 11}}
			require.NoError(t, attachInvoiceIssuerAmounts(DB, items, [][]int{tc.ids, {orders[0].Id}}))
			assert.Equal(t, tc.want, items[0].IssuerAmounts)
			assert.Equal(t, []InvoiceIssuerAmount{{"公账A", 1100}}, items[1].IssuerAmounts)
			encoded, err := common.Marshal(items)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "never-export")
		})
	}

	ticket := Ticket{UserId: 767, Type: TicketTypeInvoice, Subject: "Issuer export", Status: TicketStatusOpen}
	require.NoError(t, DB.Create(&ticket).Error)
	encodedIDs, err := common.Marshal([]int{orders[0].Id, orders[1].Id})
	require.NoError(t, err)
	invoice := TicketInvoice{TicketId: ticket.Id, UserId: 767, CompanyName: "Export company", Email: "invoice@example.test", TotalMoney: 41, TopUpOrderIds: string(encodedIDs), InvoiceStatus: InvoiceStatusPending}
	require.NoError(t, DB.Create(&invoice).Error)
	items, total, err := ListInvoicesForExport(InvoiceExportFilter{Keyword: "Export company"}, &common.PageInfo{Page: 1, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, ticket.Id, items[0].TicketId)
	assert.Equal(t, 2, items[0].OrderCount)
	assert.Equal(t, []InvoiceIssuerAmount{{"公账A", 1100}, {"公账B", 3000}}, items[0].IssuerAmounts)
}
