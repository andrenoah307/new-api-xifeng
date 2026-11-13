package model

import "gorm.io/gorm"

// Project current FIFO reservations even when no export snapshot has been saved.
// A failed projection locks the user's pending applications conservatively.
func pendingRefundInvoiceOrders(tx *gorm.DB, userID int) (map[int]bool, bool, error) {
	var refunds []TicketRefund
	if err := tx.Where("user_id = ? AND refund_status = ?", userID, RefundStatusPending).Find(&refunds).Error; err != nil {
		return nil, false, err
	}
	locked := map[int]bool{}
	for i := range refunds {
		plan, err := buildRefundInvoiceExportTx(tx, &refunds[i], 0, false, nil)
		if err != nil {
			return locked, true, nil
		}
		for _, payment := range plan.Payments {
			if payment.RefundQuota > 0 {
				locked[payment.OrderID] = true
			}
		}
	}
	return locked, false, nil
}

// An existing application cannot be issued using its pre-refund amount.
// Rejection remains available; a new application uses adjusted order balances.
func invoiceRefundLocked(tx *gorm.DB, invoice *TicketInvoice) (bool, error) {
	if invoice.InvoiceStatus == InvoiceStatusIssued {
		return false, nil
	}
	ids, err := invoice.GetTopUpOrderIDs()
	if err != nil {
		return false, err
	}
	locked, all, err := pendingRefundInvoiceOrders(tx, invoice.UserId)
	if err != nil || all {
		return all, err
	}
	for _, id := range ids {
		if locked[id] {
			return true, nil
		}
	}
	var count int64
	err = tx.Model(&RefundOrderAllocation{}).
		Where("top_up_id IN ? AND status = ?", ids, "committed").
		Where("refund_ticket_id IN (?)", tx.Model(&TicketRefund{}).Select("ticket_id").Where("user_id = ? AND refund_status = ? AND processed_time >= ?", invoice.UserId, RefundStatusRefunded, invoice.CreatedTime)).Count(&count).Error
	return count > 0, err
}
