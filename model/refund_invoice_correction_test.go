package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRefundInvoiceCorrectionChain(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceCorrection{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceCorrection{}).Error)
	owner := insertInvoiceTicketUser(t, 984001)
	actor := insertInvoiceTicketUser(t, 984002)
	require.NoError(t, DB.Model(actor).Update("role", common.RoleFinanceUser).Error)
	invoice := TicketInvoice{TicketId: 984003, UserId: owner.Id, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 10600, TopUpOrderIds: "[11,12]"}
	require.NoError(t, DB.Create(&invoice).Error)
	refund := TicketRefund{TicketId: 984004, UserId: owner.Id, RefundStatus: RefundStatusPending}
	require.NoError(t, DB.Create(&refund).Error)
	allocations := []RefundOrderAllocation{{TopUpID: 11, InvoiceTicketID: invoice.TicketId, RefundCents: 530000}}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return createRefundInvoiceCorrectionsTx(tx, &refund, allocations) }))
	var first RefundInvoiceCorrection
	require.NoError(t, DB.Where("refund_ticket_id = ?", refund.TicketId).First(&first).Error)
	assert.Equal(t, int64(1060000), first.OriginalCents)
	assert.Equal(t, int64(530000), first.ReissueCents)
	_, err := CompleteRefundInvoiceCorrection(first.Id, owner.Id, "RED-1", "BLUE-2", "")
	require.ErrorIs(t, err, ErrTicketForbidden)
	_, err = CompleteRefundInvoiceCorrection(first.Id, actor.Id, "RED-1", "BLUE-2", "")
	require.ErrorIs(t, err, ErrRefundInvoiceCorrection, "external work must be claimed first")
	_, err = StartRefundInvoiceCorrection(first.Id, owner.Id)
	require.ErrorIs(t, err, ErrTicketForbidden)
	started, err := StartRefundInvoiceCorrection(first.Id, actor.Id)
	require.NoError(t, err)
	assert.Equal(t, "processing", started.Status)
	_, err = StartRefundInvoiceCorrection(first.Id, actor.Id)
	require.ErrorIs(t, err, ErrRefundInvoiceCorrection)
	_, err = CompleteRefundInvoiceCorrection(first.Id, actor.Id, "RED-1", "", "")
	require.ErrorIs(t, err, ErrRefundInvoiceCorrection)
	completed, err := CompleteRefundInvoiceCorrection(first.Id, actor.Id, "RED-1", "BLUE-2", "Verified by finance")
	require.NoError(t, err)
	assert.Equal(t, "completed", completed.Status)
	_, err = CompleteRefundInvoiceCorrection(first.Id, actor.Id, "RED-1", "BLUE-2", "")
	require.Error(t, err)
	var messages int64
	require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ?", refund.TicketId).Count(&messages).Error)
	assert.Equal(t, int64(2), messages)

	secondRefund := TicketRefund{TicketId: 984005, UserId: owner.Id, RefundStatus: RefundStatusPending}
	require.NoError(t, DB.Create(&secondRefund).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return createRefundInvoiceCorrectionsTx(tx, &secondRefund, allocations) }))
	var second RefundInvoiceCorrection
	require.NoError(t, DB.Where("refund_ticket_id = ?", secondRefund.TicketId).First(&second).Error)
	assert.Equal(t, int64(530000), second.OriginalCents)
	assert.Equal(t, "BLUE-2", second.OriginalReference)
	assert.Zero(t, second.ReissueCents)
	_, err = StartRefundInvoiceCorrection(second.Id, actor.Id)
	require.NoError(t, err)
	_, err = CompleteRefundInvoiceCorrection(second.Id, actor.Id, "RED-2", "BLUE-3", "")
	require.ErrorIs(t, err, ErrRefundInvoiceCorrection)
	_, err = CompleteRefundInvoiceCorrection(second.Id, actor.Id, "RED-2", "", "")
	require.NoError(t, err)
	var original TicketInvoice
	require.NoError(t, DB.First(&original, invoice.Id).Error)
	assert.Equal(t, InvoiceStatusIssued, original.InvoiceStatus)
	assert.Equal(t, float64(10600), original.TotalMoney)
	var balance User
	require.NoError(t, DB.First(&balance, owner.Id).Error)
	assert.Zero(t, balance.Quota, "document correction must not return tax or change wallet")
}

func TestRefundInvoiceCorrectionReservationsAndOwnership(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&RefundInvoiceCorrection{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceCorrection{}).Error)
	owner := insertInvoiceTicketUser(t, 984011)
	invoice := TicketInvoice{TicketId: 984012, UserId: owner.Id, InvoiceStatus: InvoiceStatusIssued, TotalMoney: 100, TopUpOrderIds: "[21,22]"}
	require.NoError(t, DB.Create(&invoice).Error)
	refund := TicketRefund{TicketId: 984013, UserId: owner.Id, RefundStatus: RefundStatusPending}
	for _, allocation := range []RefundOrderAllocation{
		{TopUpID: 99, InvoiceTicketID: invoice.TicketId, RefundCents: 10},
		{TopUpID: 21, InvoiceTicketID: invoice.TicketId, RefundCents: 10001},
		{TopUpID: 21, InvoiceTicketID: invoice.TicketId, RefundCents: -1},
	} {
		err := DB.Transaction(func(tx *gorm.DB) error {
			return createRefundInvoiceCorrectionsTx(tx, &refund, []RefundOrderAllocation{allocation})
		})
		require.Error(t, err)
	}
	allocation := RefundOrderAllocation{TopUpID: 21, InvoiceTicketID: invoice.TicketId, RefundCents: 1000}
	wrongOwner := refund
	wrongOwner.UserId++
	require.Error(t, DB.Transaction(func(tx *gorm.DB) error {
		return createRefundInvoiceCorrectionsTx(tx, &wrongOwner, []RefundOrderAllocation{allocation})
	}))
	zero := allocation
	zero.RefundCents = 0
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return createRefundInvoiceCorrectionsTx(tx, &refund, []RefundOrderAllocation{zero})
	}))
	var count int64
	require.NoError(t, DB.Model(&RefundInvoiceCorrection{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return createRefundInvoiceCorrectionsTx(tx, &refund, []RefundOrderAllocation{allocation})
	}))
	refund.TicketId++
	require.ErrorIs(t, DB.Transaction(func(tx *gorm.DB) error {
		return createRefundInvoiceCorrectionsTx(tx, &refund, []RefundOrderAllocation{allocation})
	}), ErrRefundInvoiceCorrection)
}
