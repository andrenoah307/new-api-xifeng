package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundReplacementInvoicesContainOnlyConsumedCash(t *testing.T) {
	f := newRefundFinanceFixture(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceExport{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceExport{}).Error) })
	order := seedRefundFinanceOrder(t, f.user.Id, 9000, 9000, 0, 1)
	require.NoError(t, DB.Table("top_ups").Where("id = ?", order.Id).Update("epay_credentials", `{"gateway_id":"a","account_name":"Issuer A"}`).Error)
	require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": 4500, "used_quota": 4500}).Error)
	invoice := TicketInvoice{TicketId: 820, UserId: f.user.Id, CompanyName: "Original buyer", TaxNumber: "TAX", Email: "buyer@example.test", InvoiceType: "special", VATRateBps: 1300, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 90}
	require.NoError(t, invoice.SetTopUpOrderIDs([]int{order.Id}))
	require.NoError(t, DB.Create(&invoice).Error)
	for _, tc := range []struct{ refund, red, blue, unused int64 }{
		{2000, 9000, 4500, 2500},
		{500, 4500, 4500, 2000},
		{1000, 4500, 5500, 0},
	} {
		if tc.blue == 5500 {
			require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": 1000, "used_quota": 5500}).Error)
		}
		params := CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: int(tc.refund), PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()}
		ticket, _, _, err := CreateRefundTicket(params)
		require.NoError(t, err)
		_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
		require.NoError(t, err)
		plan, err := GetRefundInvoiceExport(ticket.Id)
		require.NoError(t, err)
		assert.Equal(t, tc.refund, plan.CashCents)
		require.Len(t, plan.Invoices, 1)
		assert.True(t, plan.Invoices[0].ConsumedOnly)
		assert.Equal(t, tc.red, plan.Invoices[0].OriginalCents)
		assert.Equal(t, tc.blue, plan.Invoices[0].ReissueCents)
		assert.Equal(t, []InvoiceIssuerAmount{{"Issuer A", tc.unused}}, plan.Invoices[0].UninvoicedIssuerAmounts)
		assert.Equal(t, tc.blue, plan.Payments[0].UsedCents)
		listed, _, err := ListRefundCorrectionsForExport(InvoiceExportFilter{}, &common.PageInfo{Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.NotEmpty(t, listed)
		require.Equal(t, ticket.Id, listed[0].TicketId)
		require.Len(t, listed[0].CorrectionRows, 2)
		assert.Equal(t, float64(tc.red)/100, listed[0].CorrectionRows[0].TotalMoney)
		assert.Equal(t, float64(tc.blue)/100, listed[0].CorrectionRows[1].TotalMoney)
		assert.Equal(t, "Original buyer", listed[0].CorrectionRows[1].CompanyName)
		assert.Equal(t, []string{order.TradeNo}, listed[0].CorrectionRows[1].OrderNumbers)
	}
	eligibility, err := OriginalRefundEligibility(f.user.Id)
	require.NoError(t, err)
	assert.Zero(t, eligibility.WalletQuota)
	assert.Zero(t, eligibility.MaxRefundableQuota)
}
