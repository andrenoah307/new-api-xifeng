package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundExportsPreserveInvoiceTitlesAndIssuerAmounts(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceExport{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceExport{}).Error) })
	orders := []TopUp{
		{UserId: f.user.Id, TradeNo: "A-5", Money: 5, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"a","account_name":"Issuer A"}`},
		{UserId: f.user.Id, TradeNo: "A-6", Money: 6, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"a","account_name":"Issuer A"}`},
		{UserId: f.user.Id, TradeNo: "B-12", Money: 12, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"b","account_name":"Issuer B"}`},
		{UserId: f.user.Id, TradeNo: "UNKNOWN-31", Money: 31, Status: common.TopUpStatusSuccess},
		{UserId: f.user.Id, TradeNo: "OTHER-TITLE-10", Money: 10, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"a","account_name":"Issuer A"}`},
	}
	require.NoError(t, DB.Create(&orders).Error)
	ticket := Ticket{UserId: f.user.Id, Type: TicketTypeRefund, Status: TicketStatusOpen}
	require.NoError(t, DB.Create(&ticket).Error)
	require.NoError(t, DB.Create(&TicketRefund{TicketId: ticket.Id, UserId: f.user.Id, RefundStatus: RefundStatusPending}).Error)
	data := RefundExportData{TicketID: ticket.Id, CashCents: 1888,
		Payments: []RefundExportPayment{{OrderID: orders[0].Id, RefundCents: 500}, {OrderID: orders[1].Id, RefundCents: 600}, {OrderID: orders[2].Id, RefundCents: 588}, {OrderID: orders[4].Id, RefundCents: 200}},
		Invoices: []RefundExportInvoice{
			{InvoiceTicketID: 10, CompanyName: "Title One", TaxNumber: "TAX-1", Email: "one@example.test", InvoiceType: "special", VATRateBps: 1300, OrderIDs: []int{orders[0].Id, orders[1].Id, orders[2].Id, orders[3].Id}, OriginalCents: 5400, RefundCents: 1688, ReissueCents: 3712},
			{InvoiceTicketID: 11, CompanyName: "Title Two", TaxNumber: "TAX-2", Email: "two@example.test", InvoiceType: "ordinary", VATRateBps: 600, OrderIDs: []int{orders[4].Id}, OriginalCents: 1000, RefundCents: 200, ReissueCents: 800},
		},
	}
	raw, err := common.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, DB.Create(&RefundInvoiceExport{TicketID: ticket.Id, UserID: f.user.Id, CashCents: data.CashCents, DataJSON: string(raw)}).Error)
	red, err := RefundInvoiceExportItems(ticket.Id, &data, false)
	require.NoError(t, err)
	blue, err := RefundInvoiceExportItems(ticket.Id, &data, true)
	require.NoError(t, err)
	assert.Equal(t, []InvoiceIssuerAmount{{"Issuer A", 1100}, {"Issuer B", 1200}, {"-", 3100}}, red[0].IssuerAmounts)
	assert.Equal(t, []InvoiceIssuerAmount{{"Issuer A", 0}, {"Issuer B", 612}, {"-", 3100}}, blue[0].IssuerAmounts)
	assert.Equal(t, []InvoiceIssuerAmount{{"Issuer A", 800}}, blue[1].IssuerAmounts)
	listed, total, err := ListRefundCorrectionsForExport(InvoiceExportFilter{}, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, listed[0].CorrectionRows, 8)
	row := 0
	for i := range red {
		for j, issuer := range red[i].IssuerAmounts {
			redRow, blueRow := listed[0].CorrectionRows[row], listed[0].CorrectionRows[row+1]
			assert.Equal(t, "red", redRow.InvoiceColor)
			assert.Equal(t, "blue", blueRow.InvoiceColor)
			assert.Equal(t, []InvoiceIssuerAmount{issuer}, redRow.IssuerAmounts)
			assert.Equal(t, []InvoiceIssuerAmount{blue[i].IssuerAmounts[j]}, blueRow.IssuerAmounts)
			assert.Equal(t, red[i].CompanyName, redRow.CompanyName)
			assert.Equal(t, red[i].TaxNumber, blueRow.TaxNumber)
			assert.Equal(t, red[i].Email, blueRow.Email)
			assert.Equal(t, float64(issuer.AmountCents)/100, redRow.TotalMoney)
			assert.Equal(t, float64(blue[i].IssuerAmounts[j].AmountCents)/100, blueRow.TotalMoney)
			assert.Equal(t, "ordinary", blueRow.InvoiceType)
			assert.Equal(t, InvoiceVATRateBps, blueRow.TaxRateBps)
			row += 2
		}
	}
	// Missing per-order allocation must not silently assign a partial refund to a known issuer.
	data.Payments = nil
	_, err = RefundInvoiceExportItems(ticket.Id, &data, true)
	require.ErrorIs(t, err, ErrRefundExport)
}

func TestRefundExportsPreserveIssuersAcrossPartialRefunds(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceExport{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceExport{}).Error) })
	a := seedRefundFinanceOrder(t, f.user.Id, 1100, 1100, 0, 1)
	b := seedRefundFinanceOrder(t, f.user.Id, 1200, 1200, 0, 2)
	c := seedRefundFinanceOrder(t, f.user.Id, 3100, 3100, 0, 3)
	require.NoError(t, DB.Table("top_ups").Where("id = ?", a.Id).Update("epay_credentials", `{"gateway_id":"a","account_name":"Issuer A"}`).Error)
	require.NoError(t, DB.Table("top_ups").Where("id = ?", b.Id).Update("epay_credentials", `{"gateway_id":"b","account_name":"Issuer B"}`).Error)
	require.NoError(t, DB.Model(&f.user).Update("quota", 5400).Error)
	invoice := TicketInvoice{TicketId: 810, UserId: f.user.Id, CompanyName: "Original title", InvoiceType: "ordinary", VATRateBps: 600, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 54}
	require.NoError(t, invoice.SetTopUpOrderIDs([]int{a.Id, b.Id, c.Id}))
	require.NoError(t, DB.Create(&invoice).Error)
	params := CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 1688, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()}
	first, _, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: first.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	params.RefundQuota, params.RequestKey = 600, uuid.NewString()
	second, _, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	data, err := SaveRefundInvoiceExport(second.Id, f.admin.Id)
	require.NoError(t, err)
	require.Len(t, data.Invoices, 1)
	assert.Equal(t, int64(3712), data.Invoices[0].OriginalCents)
	red, err := RefundInvoiceExportItems(second.Id, data, false)
	require.NoError(t, err)
	blue, err := RefundInvoiceExportItems(second.Id, data, true)
	require.NoError(t, err)
	assert.Equal(t, []InvoiceIssuerAmount{{"Issuer B", 612}, {"-", 3100}}, red[0].IssuerAmounts)
	assert.Equal(t, []InvoiceIssuerAmount{{"Issuer B", 12}, {"-", 3100}}, blue[0].IssuerAmounts)
	assert.Equal(t, "Original title", blue[0].CompanyName)
}
