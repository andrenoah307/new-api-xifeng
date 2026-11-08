package model

import (
	"errors"
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrRefundExport = errors.New("refund invoice export requires a valid cash amount and complete payment history")

// RefundInvoiceExport preserves the cash allocation and invoice identity used by finance.
// It does not move wallet funds or mark invoices as reversed/issued.
type RefundInvoiceExport struct {
	ID          int    `json:"id"`
	TicketID    int    `json:"ticket_id" gorm:"uniqueIndex;not null"`
	UserID      int    `json:"user_id" gorm:"index;not null"`
	CashCents   int64  `json:"cash_cents" gorm:"type:bigint;not null"`
	DataJSON    string `json:"-" gorm:"type:text;not null"`
	UpdatedBy   int    `json:"updated_by"`
	UpdatedTime int64  `json:"updated_time" gorm:"type:bigint"`
}

type RefundExportPayment struct {
	OriginalTaxCents int64  `json:"original_tax_cents"`
	RefundTaxCents   int64  `json:"refund_tax_cents"`
	OriginalQuota    int64  `json:"original_quota"`
	RefundQuota      int64  `json:"refund_quota"`
	OrderID          int    `json:"order_id"`
	TradeNo          string `json:"trade_no"`
	PaidCents        int64  `json:"paid_cents"`
	RefundCents      int64  `json:"refund_cents"`
}

type RefundExportInvoice struct {
	InvoiceTicketID int    `json:"invoice_ticket_id"`
	CompanyName     string `json:"company_name"`
	TaxNumber       string `json:"tax_number"`
	Email           string `json:"email"`
	InvoiceType     string `json:"invoice_type"`
	VATRateBps      int    `json:"vat_rate_bps"`
	OrderIDs        []int  `json:"order_ids"`
	PaidCents       int64  `json:"paid_cents"`
	OriginalCents   int64  `json:"original_cents"`
	RefundCents     int64  `json:"refund_cents"`
	ReissueCents    int64  `json:"reissue_cents"`
}

type RefundExportData struct {
	TicketID  int                   `json:"ticket_id"`
	CashCents int64                 `json:"cash_cents"`
	Payments  []RefundExportPayment `json:"payments"`
	Invoices  []RefundExportInvoice `json:"invoices"`
}

func GetRefundInvoiceExport(ticketID int) (*RefundExportData, error) {
	var refund TicketRefund
	if err := DB.Where("ticket_id = ?", ticketID).First(&refund).Error; err != nil {
		return nil, err
	}
	if refund.RefundStatus != RefundStatusPending && refund.RefundStatus != RefundStatusRefunded {
		return nil, ErrTicketRefundNotPending
	}
	var saved RefundInvoiceExport
	if err := DB.Where("ticket_id = ?", ticketID).First(&saved).Error; err != nil {
		return nil, err
	}
	var data RefundExportData
	if err := common.UnmarshalJsonStr(saved.DataJSON, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// SaveRefundInvoiceExport allocates cash FIFO, independently from consumption quotas.
// Later saved allocations prevent editing an earlier allocation underneath them.
func SaveRefundInvoiceExport(ticketID, actorID int) (*RefundExportData, error) {
	var identity TicketRefund
	if err := DB.Where("ticket_id = ?", ticketID).First(&identity).Error; err != nil {
		return nil, err
	}
	var result *RefundExportData
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, identity.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := tx.Where("id = ? AND status = ? AND role IN ?", actorID, common.UserStatusEnabled, []int{common.RoleFinanceUser, common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
			return ErrTicketForbidden
		}
		var refund TicketRefund
		if err := tx.Where("ticket_id = ?", ticketID).First(&refund).Error; err != nil {
			return err
		}
		if refund.RefundStatus != RefundStatusPending && refund.RefundStatus != RefundStatusRefunded {
			return ErrTicketRefundNotPending
		}
		var err error
		result, err = saveRefundInvoiceExportTx(tx, &refund, actorID)
		return err
	})
	if err != nil {
		return nil, err
	}
	RecordLogWithAdminInfo(identity.UserId, LogTypeManage, fmt.Sprintf("Refund #%d invoice export recalculated", ticketID), map[string]interface{}{"admin_id": actorID, "refund_cash_cents": result.CashCents})
	return result, nil
}

func saveRefundInvoiceExportTx(tx *gorm.DB, refund *TicketRefund, actorID int) (*RefundExportData, error) {
	var result *RefundExportData
	var owner User
	if err := tx.First(&owner, refund.UserId).Error; err != nil {
		return nil, err
	}
	var saved RefundInvoiceExport
	err := tx.Where("ticket_id = ?", refund.TicketId).First(&saved).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if saved.ID > 0 && refund.RefundStatus == RefundStatusRefunded {
		result = &RefundExportData{}
		err := common.UnmarshalJsonStr(saved.DataJSON, result)
		return result, err
	}
	if refund.RefundStatus == RefundStatusRefunded {
		return nil, ErrRefundExport
	}
	var prior []RefundInvoiceExport
	if err := tx.Where("user_id = ? AND ticket_id != ?", refund.UserId, refund.TicketId).Order("id ASC").Find(&prior).Error; err != nil {
		return nil, err
	}
	var previousAllocations []RefundOrderAllocation
	invoiceRemaining := map[int]int64{}
	covered := map[int]bool{}
	for _, plan := range prior {
		var other TicketRefund
		if err := tx.Where("ticket_id = ?", plan.TicketID).First(&other).Error; err != nil {
			return nil, err
		}
		if other.RefundStatus == RefundStatusRejected || other.RefundStatus == RefundStatusCancelled {
			continue
		}
		if saved.ID > 0 && plan.ID > saved.ID {
			return nil, ErrRefundExport
		}
		covered[plan.TicketID] = true
		var data RefundExportData
		if err := common.UnmarshalJsonStr(plan.DataJSON, &data); err != nil {
			return nil, err
		}
		for _, row := range data.Payments {
			previousAllocations = append(previousAllocations, RefundOrderAllocation{TopUpID: row.OrderID, OriginalQuota: row.OriginalQuota, RefundQuota: row.RefundQuota, RefundCents: row.RefundCents})
		}
		for _, row := range data.Invoices {
			invoiceRemaining[row.InvoiceTicketID] = row.ReissueCents
		}
	}
	var historical []TicketRefund
	if err := tx.Where("user_id = ? AND ticket_id != ? AND refund_status = ?", refund.UserId, refund.TicketId, RefundStatusRefunded).Find(&historical).Error; err != nil {
		return nil, err
	}
	for _, old := range historical {
		if covered[old.TicketId] {
			continue
		}
		var allocations []RefundOrderAllocation
		if err := tx.Where("refund_ticket_id = ? AND status = ?", old.TicketId, "committed").Find(&allocations).Error; err != nil {
			return nil, err
		}
		if len(allocations) == 0 {
			return nil, fmt.Errorf("%w: historical refund #%d has no cash allocation", ErrRefundExport, old.TicketId)
		}
		previousAllocations = append(previousAllocations, allocations...)
	}
	orders, _, err := loadRefundPaymentSources(tx, refund.UserId)
	if err != nil {
		return nil, err
	}
	paidOrders := orders[:0]
	for i := range orders {
		order := &orders[i]
		if order.CheckoutVersion == 0 && order.Money == 0 {
			continue
		}
		if order.CheckoutVersion == 0 {
			order.PaymentTotalCents, err = invoiceMoneyCents(order.Money)
			if err != nil {
				return nil, err
			}
		}
		if order.PaymentTotalCents < 0 || order.QuotaGranted < 0 {
			return nil, ErrRefundExport
		}
		if order.PaymentTotalCents == 0 {
			continue
		}
		if !IsCNYInvoiceTopUp(order) || order.PaymentTotalCents > 1000000000000000 || order.QuotaGranted == 0 {
			return nil, ErrRefundExport
		}
		paidOrders = append(paidOrders, *order)
	}
	orders = paidOrders
	var feeQuota int64
	if err := tx.Model(&InvoiceTaxCharge{}).Where("user_id = ? AND status = ? AND payment_method = ?", refund.UserId, "paid", "balance").Select("COALESCE(SUM(quota),0)").Scan(&feeQuota).Error; err != nil {
		return nil, err
	}
	if owner.UsedQuota < 0 || feeQuota < 0 || feeQuota > math.MaxInt64-int64(owner.UsedQuota) {
		return nil, ErrRefundExport
	}
	lots, err := projectRefundCreditLots(int64(owner.UsedQuota)+feeQuota, orders, previousAllocations)
	if err != nil {
		return nil, err
	}
	result = &RefundExportData{TicketID: refund.TicketId, Payments: []RefundExportPayment{}, Invoices: []RefundExportInvoice{}}
	paid := map[int]int64{}
	allocated := map[int]int64{}
	remaining := int64(refund.RefundQuota)
	for _, order := range orders {
		amount := order.PaymentTotalCents
		if amount < 0 || amount > 1000000000000000 || order.QuotaGranted < 0 {
			return nil, ErrRefundExport
		}
		paid[order.Id] = amount
		lot := lots[order.Id]
		take := min(remaining, lot.PaidRemaining)
		if take <= 0 {
			continue
		}
		remaining -= take
		cash := refundProportionalCents(amount, lot.RefundedPaid+take, order.QuotaGranted) - lot.RefundedCash
		if cash < 0 || cash > amount-lot.RefundedCash {
			return nil, ErrRefundExport
		}
		allocated[order.Id] = cash
		if cash > math.MaxInt64-result.CashCents {
			return nil, ErrRefundExport
		}
		result.CashCents += cash
		tax := refundProportionalCents(order.InvoiceTaxCents, lot.RefundedPaid+take, order.QuotaGranted) - refundProportionalCents(order.InvoiceTaxCents, lot.RefundedPaid, order.QuotaGranted)
		result.Payments = append(result.Payments, RefundExportPayment{OrderID: order.Id, TradeNo: order.TradeNo, PaidCents: amount, RefundCents: cash, OriginalQuota: order.QuotaGranted, RefundQuota: take, OriginalTaxCents: order.InvoiceTaxCents, RefundTaxCents: tax})
	}
	if remaining != 0 {
		return nil, ErrRefundExport
	}
	if result.CashCents <= 0 {
		return nil, ErrTicketRefundQuotaInvalid
	}
	var invoices []TicketInvoice
	if err := tx.Where("user_id = ? AND invoice_status = ?", refund.UserId, InvoiceStatusIssued).Order("id ASC").Find(&invoices).Error; err != nil {
		return nil, err
	}
	linked := map[int]bool{}
	for _, invoice := range invoices {
		ids, err := invoice.GetTopUpOrderIDs()
		if err != nil {
			return nil, err
		}
		affected := false
		for _, id := range ids {
			if allocated[id] > 0 {
				affected = true
				break
			}
		}
		if !affected {
			continue
		}
		var reduction, totalPaid int64
		for _, id := range ids {
			amount, exists := paid[id]
			if !exists || amount > math.MaxInt64-totalPaid || allocated[id] > math.MaxInt64-reduction {
				return nil, ErrRefundExport
			}
			reduction += allocated[id]
			totalPaid += amount
		}
		if reduction == 0 {
			continue
		}
		var pendingCorrections int64
		if err := tx.Model(&RefundInvoiceCorrection{}).Where("invoice_ticket_id = ? AND refund_ticket_id != ? AND status IN ?", invoice.TicketId, refund.TicketId, []string{"pending", "processing"}).Count(&pendingCorrections).Error; err != nil {
			return nil, err
		}
		if pendingCorrections > 0 {
			return nil, ErrRefundFinanceCorrectionPending
		}
		for _, id := range ids {
			if allocated[id] > 0 && linked[id] {
				return nil, ErrRefundExport
			}
			linked[id] = true
		}
		baseline, err := invoiceMoneyCents(invoice.TotalMoney)
		if err != nil {
			return nil, err
		}
		if previous, ok := invoiceRemaining[invoice.TicketId]; ok {
			baseline = previous
		} else {
			var correction RefundInvoiceCorrection
			err := tx.Where("invoice_ticket_id = ? AND status = ?", invoice.TicketId, "completed").Order("id DESC").First(&correction).Error
			if err == nil {
				baseline = correction.ReissueCents
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
		if baseline < reduction {
			return nil, ErrRefundExport
		}
		result.Invoices = append(result.Invoices, RefundExportInvoice{InvoiceTicketID: invoice.TicketId, CompanyName: invoice.CompanyName, TaxNumber: invoice.TaxNumber, Email: invoice.Email, InvoiceType: invoice.InvoiceType, VATRateBps: invoice.VATRateBps, OrderIDs: ids, PaidCents: totalPaid, OriginalCents: baseline, RefundCents: reduction, ReissueCents: baseline - reduction})
	}
	raw, err := common.Marshal(result)
	if err != nil {
		return nil, err
	}
	saved.TicketID, saved.UserID, saved.CashCents = refund.TicketId, refund.UserId, result.CashCents
	saved.DataJSON, saved.UpdatedBy, saved.UpdatedTime = string(raw), actorID, common.GetTimestamp()
	if saved.ID == 0 {
		return result, tx.Create(&saved).Error
	}
	return result, tx.Save(&saved).Error
}
