package model

import (
	"slices"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func UpdateTopUpInvoiceType(id int, expectedType, invoiceType string) (*TopUp, error) {
	if id <= 0 || !slices.Contains([]string{"ordinary", "special", "none"}, invoiceType) {
		return nil, ErrInvoiceTaxInvalid
	}
	var order TopUp
	if err := DB.Select("id", "user_id").First(&order, id).Error; err != nil {
		return nil, err
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		config, err := GetInvoiceTaxConfig(tx)
		if err != nil {
			return err
		}
		if !config.ModuleEnabled {
			return ErrTicketForbidden
		}
		// Match invoice application lock order so eligibility cannot change mid-application.
		var owner User
		if err := lockForUpdate(tx).First(&owner, order.UserId).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&order, id).Error; err != nil {
			return err
		}
		if order.InvoiceType != expectedType {
			return ErrInvoiceTaxQuoteChanged
		}
		if order.Status != common.TopUpStatusSuccess {
			return ErrTicketInvoiceOrderInvalid
		}
		if order.InvoiceType == invoiceType {
			return nil
		}
		var invoices []TicketInvoice
		if err := tx.Where("user_id = ? AND invoice_status IN ?", order.UserId, []int{InvoiceStatusPending, InvoiceStatusIssued}).Find(&invoices).Error; err != nil {
			return err
		}
		for _, invoice := range invoices {
			ids, err := invoice.GetTopUpOrderIDs()
			if err != nil {
				return err
			}
			if slices.Contains(ids, id) {
				return ErrTicketInvoiceOrderDuplicate
			}
		}
		if invoiceType != "none" {
			if !IsCNYInvoiceTopUp(&order) {
				return ErrInvoiceTaxCurrency
			}
			if _, err := invoiceMoneyCents(order.Money); err != nil {
				return err
			}
		}
		result := tx.Model(&TopUp{}).Where("id = ? AND status = ? AND COALESCE(invoice_type, '') = ?", id, common.TopUpStatusSuccess, expectedType).Update("invoice_type", invoiceType)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceTaxQuoteChanged
		}
		order.InvoiceType = invoiceType
		return nil
	})
	return &order, err
}
