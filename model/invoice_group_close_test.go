package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func restrictedInvoiceCloseFixture(t *testing.T) (*User, *User, *TopUp, *Ticket) {
	t.Helper()
	owner, actor, order := invoiceAdjustmentFixture(t)
	require.NoError(t, DB.Model(actor).Update("role", common.RoleRootUser).Error)
	require.NoError(t, DB.Model(order).Updates(map[string]interface{}{"invoice_type": "ordinary", "invoice_net_cents": 9400, "invoice_tax_cents": 600, "quota_granted": 47000000, "checkout_base_quota": 47000000, "checkout_quota": 47000000}).Error)
	quote, err := QuoteInvoiceTax(DB, owner.Id, []int{order.Id}, "ordinary")
	require.NoError(t, err)
	params := createInvoiceTicketParams(owner, []int{order.Id})
	params.InvoiceType, params.TaxQuote = "ordinary", quote.Token
	ticket, _, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.InvoiceRestrictedGroups = []string{owner.Group}
	config.InvoiceRestrictedGroupsBlockTickets = true
	require.NoError(t, SaveInvoiceTaxConfig(config))
	return owner, actor, order, ticket
}

func TestRestrictedInvoiceClosureReturnsOrderTaxAtomically(t *testing.T) {
	for _, convert := range []bool{false, true} {
		t.Run(map[bool]string{false: "close only", true: "convert order tax"}[convert], func(t *testing.T) {
			owner, actor, order, ticket := restrictedInvoiceCloseFixture(t)
			page, err := PreviewInvoiceGroupClosures(actor.Id, 0, convert)
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			preview := page.Items[0]
			require.Empty(t, preview.Error)
			var current User
			require.NoError(t, DB.First(&current, owner.Id).Error)
			assert.Equal(t, 1000000, current.Quota)
			result, err := CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, convert, preview.Revision, true)
			require.NoError(t, err)
			assert.True(t, result.Applied)
			require.NoError(t, DB.First(&current, owner.Id).Error)
			require.NoError(t, DB.First(order, order.Id).Error)
			if convert {
				assert.Equal(t, int64(600), preview.OrderTaxCents)
				assert.Equal(t, "3000000", preview.QuotaDelta)
				assert.Equal(t, 4000000, current.Quota)
				assert.Equal(t, "none", order.InvoiceType)
				assert.Equal(t, int64(50000000), order.QuotaGranted)
				assert.Zero(t, order.InvoiceTaxCents)
			} else {
				assert.Equal(t, "0", preview.QuotaDelta)
				assert.Equal(t, 1000000, current.Quota)
				assert.Equal(t, "ordinary", order.InvoiceType)
			}
			require.NoError(t, DB.First(ticket, ticket.Id).Error)
			assert.Equal(t, TicketStatusClosed, ticket.Status)
			_, err = CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, convert, preview.Revision, true)
			assert.ErrorIs(t, err, ErrTicketClosed)
			page, err = PreviewInvoiceGroupClosures(actor.Id, 0, convert)
			require.NoError(t, err)
			assert.Empty(t, page.Items)
		})
	}
}

func TestRestrictedInvoiceClosureRollsBackOnOrderFailure(t *testing.T) {
	owner, actor, _, ticket := restrictedInvoiceCloseFixture(t)
	charge := InvoiceTaxCharge{TicketId: ticket.Id, UserId: owner.Id, TradeNo: "close-rollback-tax", Status: "paid", FeeCents: 60, Quota: 300000}
	require.NoError(t, DB.Create(&charge).Error)
	preview, err := CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, true, "", false)
	require.NoError(t, err)
	injected := errors.New("injected order write failure")
	const callback = "test:invoice_group_close_failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "top_ups" {
			tx.AddError(injected)
		}
	}))
	t.Cleanup(func() { DB.Callback().Update().Remove(callback) })
	_, err = CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, true, preview.Revision, true)
	require.ErrorIs(t, err, injected)
	require.NoError(t, DB.First(ticket, ticket.Id).Error)
	assert.Equal(t, TicketStatusOpen, ticket.Status)
	var invoice TicketInvoice
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&invoice).Error)
	assert.Equal(t, InvoiceStatusPending, invoice.InvoiceStatus)
	var current User
	require.NoError(t, DB.First(&current, owner.Id).Error)
	assert.Equal(t, 1000000, current.Quota)
	require.NoError(t, DB.First(&charge, charge.Id).Error)
	assert.Equal(t, "paid", charge.Status)
	var receipts int64
	require.NoError(t, DB.Model(&TopUpInvoiceAdjustment{}).Count(&receipts).Error)
	assert.Zero(t, receipts)
}

func TestRestrictedInvoiceClosureSupplementAndLatePayment(t *testing.T) {
	for _, status := range []string{"paid", "pending"} {
		t.Run(status, func(t *testing.T) {
			owner, actor, _, ticket := restrictedInvoiceCloseFixture(t)
			charge := InvoiceTaxCharge{TicketId: ticket.Id, UserId: owner.Id, TradeNo: "close-tax-" + status, Status: status, PaymentMethod: "alipay", FeeCents: 60, Quota: 300000}
			require.NoError(t, DB.Create(&charge).Error)
			preview, err := CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, true, "", false)
			require.NoError(t, err)
			if status == "paid" {
				assert.Equal(t, int64(60), preview.SupplementTaxCents)
				assert.Equal(t, "3300000", preview.QuotaDelta)
			} else {
				assert.Equal(t, "3000000", preview.QuotaDelta)
			}
			_, err = CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, true, preview.Revision, true)
			require.NoError(t, err)
			for range 2 {
				require.NoError(t, SettleInvoiceTax(charge.TradeNo, "alipay", charge.FeeCents))
			}
			var current User
			require.NoError(t, DB.First(&current, owner.Id).Error)
			assert.Equal(t, 4300000, current.Quota)
			require.NoError(t, DB.First(&charge, charge.Id).Error)
			assert.Equal(t, "refunded", charge.Status)
		})
	}
}

func TestRestrictedInvoiceClosureRejectsRefundConflictAndNonRoot(t *testing.T) {
	owner, actor, _, ticket := restrictedInvoiceCloseFixture(t)
	require.NoError(t, DB.Create(&TicketRefund{TicketId: ticket.Id + 1, UserId: owner.Id, RefundStatus: RefundStatusPending}).Error)
	_, err := CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, false, "", false)
	assert.ErrorIs(t, err, ErrRefundFinancePending)
	require.NoError(t, DB.Model(actor).Update("role", common.RoleAdminUser).Error)
	_, err = PreviewInvoiceGroupClosures(actor.Id, 0, true)
	assert.ErrorIs(t, err, ErrTicketForbidden)
	require.NoError(t, DB.First(ticket, ticket.Id).Error)
	assert.Equal(t, TicketStatusOpen, ticket.Status)
}

func TestRestrictedInvoiceClosureRevalidatesGroupAndSnapshot(t *testing.T) {
	owner, actor, order, ticket := restrictedInvoiceCloseFixture(t)
	preview, err := CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, true, "", false)
	require.NoError(t, err)
	_, err = CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, false, preview.Revision, true)
	assert.ErrorIs(t, err, ErrFinanceRevision)
	require.NoError(t, DB.Model(owner).Update("group", "other").Error)
	_, err = CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, true, preview.Revision, true)
	assert.ErrorIs(t, err, ErrTicketForbidden)
	require.NoError(t, DB.Model(owner).Update("group", "default").Error)
	require.NoError(t, DB.Model(order).Update("trade_no", "changed-source-order").Error)
	_, err = CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, true, preview.Revision, true)
	assert.ErrorIs(t, err, ErrFinanceRevision)
	require.NoError(t, DB.Model(&TicketInvoice{}).Where("ticket_id = ?", ticket.Id).Update("invoice_status", InvoiceStatusIssued).Error)
	_, err = CloseRestrictedInvoiceTicket(actor.Id, ticket.Id, true, "", false)
	assert.ErrorIs(t, err, ErrTicketInvoiceStatusInvalid)
}
