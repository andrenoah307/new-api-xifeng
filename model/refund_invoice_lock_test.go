package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingInvoiceRefundLock(t *testing.T) {
	for _, complete := range []bool{false, true} {
		name := "cancel"
		if complete {
			name = "complete"
		}
		t.Run(name, func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
			require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error)
			t.Cleanup(func() { require.NoError(t, DB.Where("1=1").Delete(&RefundInvoiceExport{}).Error) })
			first := seedRefundFinanceOrder(t, f.user.Id, 10000, 10000, 0, 1)
			second := seedRefundFinanceOrder(t, f.user.Id, 10000, 10000, 0, 2)
			require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": 15000, "used_quota": 5000}).Error)
			invoices := make([]TicketInvoice, 2)
			for i, order := range []*TopUp{first, second} {
				ticket := Ticket{UserId: f.user.Id, Type: TicketTypeInvoice, Status: TicketStatusOpen}
				require.NoError(t, DB.Create(&ticket).Error)
				invoices[i] = TicketInvoice{TicketId: ticket.Id, UserId: f.user.Id, InvoiceStatus: InvoiceStatusPending, InvoiceType: "ordinary", VATRateBps: 600, TotalMoney: 100, CreatedTime: 1}
				require.NoError(t, invoices[i].SetTopUpOrderIDs([]int{order.Id}))
				require.NoError(t, DB.Create(&invoices[i]).Error)
			}
			refund, _, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: 2500, PayeeType: "alipay", PayeeName: "name", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()})
			require.NoError(t, err)
			// No saved export is needed to lock the affected pending application.
			_, _, _, err = UpdateInvoiceStatus(invoices[0].TicketId, f.admin.Id, InvoiceStatusIssued)
			require.ErrorIs(t, err, ErrRefundFinancePending)
			rows, total, err := ListInvoicesForExport(InvoiceExportFilter{}, &common.PageInfo{Page: 1, PageSize: 20})
			require.NoError(t, err)
			assert.Equal(t, int64(1), total)
			require.Len(t, rows, 1)
			assert.Equal(t, invoices[1].TicketId, rows[0].TicketId)
			eligible, err := adjustInvoiceOrdersForRefunds(DB, []*TopUp{first, second})
			require.NoError(t, err)
			require.Len(t, eligible, 1)
			assert.Equal(t, second.Id, eligible[0].Id)
			_, _, _, err = UpdateInvoiceStatus(invoices[1].TicketId, f.admin.Id, InvoiceStatusIssued)
			require.NoError(t, err)
			var snapshots int64
			require.NoError(t, DB.Model(&RefundInvoiceExport{}).Count(&snapshots).Error)
			assert.Zero(t, snapshots, "read-only lock checks must not write snapshots")
			status := RefundStatusRejected
			if complete {
				status = RefundStatusRefunded
			}
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: refund.Id, AdminId: f.admin.Id, RefundStatus: status})
			require.NoError(t, err)
			_, _, _, err = UpdateInvoiceStatus(invoices[0].TicketId, f.admin.Id, InvoiceStatusIssued)
			if !complete {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrRefundFinancePending)
			// A stale full-price application remains rejectable after cash is refunded.
			_, _, _, err = UpdateInvoiceStatus(invoices[0].TicketId, f.admin.Id, InvoiceStatusRejected)
			require.NoError(t, err)
			eligible, err = adjustInvoiceOrdersForRefunds(DB, []*TopUp{first})
			require.NoError(t, err)
			require.Len(t, eligible, 1)
			assert.Equal(t, int64(7500), eligible[0].PaymentTotalCents)
		})
	}
}
