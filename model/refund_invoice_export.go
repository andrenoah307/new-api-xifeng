package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrRefundExport = errors.New("refund invoice export requires a valid cash amount and complete payment history")
var ErrRefundLegacyWorkflow = errors.New("Resolve legacy invoice and payment operations in refund recovery before continuing")

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
	UsedQuota         int64  `json:"used_quota,omitempty"`
	UsedCents         int64  `json:"used_cents,omitempty"`
	GiftPolicyVersion int    `json:"gift_policy_version,omitempty"`
	OriginalGiftQuota int64  `json:"original_gift_quota,omitempty"`
	RefundGiftQuota   int64  `json:"refund_gift_quota,omitempty"`
	OriginalTaxCents  int64  `json:"original_tax_cents"`
	RefundTaxCents    int64  `json:"refund_tax_cents"`
	OriginalQuota     int64  `json:"original_quota"`
	RefundQuota       int64  `json:"refund_quota"`
	OrderID           int    `json:"order_id"`
	TradeNo           string `json:"trade_no"`
	PaidCents         int64  `json:"paid_cents"`
	RefundCents       int64  `json:"refund_cents"`
}

type RefundExportInvoice struct {
	ConsumedOnly            bool                               `json:"consumed_only,omitempty"`
	AffectedIssuers         []string                           `json:"affected_issuers,omitempty"`
	UninvoicedIssuerAmounts []InvoiceIssuerAmount              `json:"uninvoiced_issuer_amounts,omitempty"`
	ReissueIssuerAmounts    []InvoiceIssuerAmount              `json:"reissue_issuer_amounts,omitempty"`
	IssuerInvoiceTypes      map[string]RefundIssuerInvoiceType `json:"issuer_invoice_types,omitempty"`
	IssuerAmounts           []InvoiceIssuerAmount              `json:"issuer_amounts,omitempty"`
	InvoiceTicketID         int                                `json:"invoice_ticket_id"`
	CompanyName             string                             `json:"company_name"`
	TaxNumber               string                             `json:"tax_number"`
	Email                   string                             `json:"email"`
	InvoiceType             string                             `json:"invoice_type"`
	VATRateBps              int                                `json:"vat_rate_bps"`
	OrderIDs                []int                              `json:"order_ids"`
	PaidCents               int64                              `json:"paid_cents"`
	OriginalCents           int64                              `json:"original_cents"`
	RefundCents             int64                              `json:"refund_cents"`
	ReissueCents            int64                              `json:"reissue_cents"`
}

type RefundIssuerInvoiceType struct {
	InvoiceType string `json:"invoice_type"`
	VATRateBps  int    `json:"vat_rate_bps"`
}

type RefundExportData struct {
	GiftRecoveryPolicy string                `json:"gift_recovery_policy,omitempty"`
	PaidQuota          int64                 `json:"paid_quota,omitempty"`
	RefundGiftQuota    int64                 `json:"refund_gift_quota,omitempty"`
	TicketID           int                   `json:"ticket_id"`
	CashCents          int64                 `json:"cash_cents"`
	Payments           []RefundExportPayment `json:"payments"`
	Invoices           []RefundExportInvoice `json:"invoices"`
}

type RefundInvoiceExportItem struct {
	InvoiceExportItem
	OrderNumbers       []string
	issuerOrderNumbers map[string][]string
	issuerInvoiceTypes map[string]RefundIssuerInvoiceType
}

// RefundInvoiceExportItems exports only issuer invoices reduced by this refund.
// The internal projection retains untouched balances for subsequent FIFO refunds.
func RefundInvoiceExportItems(ticketID int, data *RefundExportData, reissue bool) ([]RefundInvoiceExportItem, error) {
	red, err := refundInvoiceExportItemsTx(DB, ticketID, data, false)
	if err != nil {
		return nil, err
	}
	blue, err := refundInvoiceExportItemsTx(DB, ticketID, data, true)
	if err != nil {
		return nil, err
	}
	items := make([]RefundInvoiceExportItem, 0)
	for i := range red {
		for j, issuer := range red[i].IssuerAmounts {
			affected := issuer.AmountCents != blue[i].IssuerAmounts[j].AmountCents
			if data.Invoices[i].ConsumedOnly {
				affected = false
				for _, name := range data.Invoices[i].AffectedIssuers {
					if name == issuer.AccountName {
						affected = true
					}
				}
			}
			if !affected {
				continue
			}
			item := red[i]
			if kind, exists := item.issuerInvoiceTypes[issuer.AccountName]; exists {
				item.InvoiceType, item.TaxRateBps = kind.InvoiceType, kind.VATRateBps
			}
			if reissue {
				item = blue[i]
				issuer = blue[i].IssuerAmounts[j]
			}
			item.OrderNumbers = item.issuerOrderNumbers[issuer.AccountName]
			// Historical unattributed invoices may combine otherwise known accounts.
			if len(red[i].IssuerAmounts) == 1 && issuer.AccountName == "-" {
				item.OrderNumbers = red[i].OrderNumbers
			}
			item.IssuerAmounts = []InvoiceIssuerAmount{issuer}
			item.TotalMoney = float64(issuer.AmountCents) / 100
			items = append(items, item)
		}
	}
	return items, nil
}

func refundInvoiceExportItemsTx(db *gorm.DB, ticketID int, data *RefundExportData, reissue bool) ([]RefundInvoiceExportItem, error) {
	var refund TicketRefund
	if err := db.Where("ticket_id = ?", ticketID).First(&refund).Error; err != nil {
		return nil, err
	}
	items := make([]RefundInvoiceExportItem, len(data.Invoices))
	invoiceItems := make([]*InvoiceExportItem, 0, len(items))
	orderIDs := make([][]int, 0, len(items))
	orderIssuers := map[int]string{}
	for i, invoice := range data.Invoices {
		amount, invoiceType, rate := invoice.OriginalCents, invoice.InvoiceType, invoice.VATRateBps
		if reissue {
			amount, invoiceType, rate = invoice.ReissueCents, "ordinary", InvoiceVATRateBps
		}
		if amount < 0 || invoice.OriginalCents < 0 || invoice.RefundCents < 0 || (!invoice.ConsumedOnly && (invoice.OriginalCents == 0 || invoice.ReissueCents != invoice.OriginalCents-invoice.RefundCents)) {
			return nil, ErrRefundExport
		}
		items[i].InvoiceExportItem = InvoiceExportItem{UserId: refund.UserId, CompanyName: invoice.CompanyName, TaxNumber: invoice.TaxNumber, Email: invoice.Email, InvoiceType: invoiceType, TaxRateBps: rate, TotalMoney: float64(invoice.OriginalCents) / 100}
		items[i].issuerInvoiceTypes = invoice.IssuerInvoiceTypes
		if len(invoice.IssuerAmounts) == 0 {
			invoiceItems = append(invoiceItems, &items[i].InvoiceExportItem)
			orderIDs = append(orderIDs, invoice.OrderIDs)
		} else {
			var total int64
			seen := map[string]bool{}
			for _, issuer := range invoice.IssuerAmounts {
				if issuer.AccountName == "" || seen[issuer.AccountName] || issuer.AmountCents < 0 || issuer.AmountCents > invoice.OriginalCents-total {
					return nil, ErrRefundExport
				}
				seen[issuer.AccountName] = true
				total += issuer.AmountCents
				if issuer.AmountCents > 0 || invoice.ConsumedOnly {
					items[i].IssuerAmounts = append(items[i].IssuerAmounts, issuer)
				}
			}
			if total != invoice.OriginalCents {
				return nil, ErrRefundExport
			}
		}
		var orders []TopUp
		if len(invoice.OrderIDs) > 0 {
			if err := db.Select("id", "trade_no", "status", "epay_credentials").Where("user_id = ? AND id IN ?", refund.UserId, invoice.OrderIDs).Find(&orders).Error; err != nil {
				return nil, err
			}
		}
		numbers := make(map[int]string, len(orders))
		for _, order := range orders {
			numbers[order.Id] = strings.TrimSpace(order.TradeNo)
			if order.Status == common.TopUpStatusSuccess {
				name, err := invoiceOrderIssuerName(order)
				if err != nil {
					return nil, err
				}
				orderIssuers[order.Id] = name
			}
		}
		items[i].issuerOrderNumbers = map[string][]string{}
		for _, id := range invoice.OrderIDs {
			number := numbers[id]
			if number == "" {
				number = "-"
			}
			items[i].OrderNumbers = append(items[i].OrderNumbers, number)
			issuer := orderIssuers[id]
			if issuer == "" {
				issuer = "-"
			}
			items[i].issuerOrderNumbers[issuer] = append(items[i].issuerOrderNumbers[issuer], number)
		}
	}
	if err := attachInvoiceIssuerAmounts(db, invoiceItems, orderIDs); err != nil {
		return nil, err
	}
	if reissue {
		for i, invoice := range data.Invoices {
			items[i].TotalMoney = float64(invoice.ReissueCents) / 100
			if invoice.ConsumedOnly && len(invoice.ReissueIssuerAmounts) == 0 {
				return nil, ErrRefundExport
			}
			if len(invoice.ReissueIssuerAmounts) > 0 {
				remaining := map[string]int64{}
				var total int64
				for _, issuer := range invoice.ReissueIssuerAmounts {
					if _, exists := remaining[issuer.AccountName]; exists || issuer.AmountCents < 0 || issuer.AmountCents > invoice.ReissueCents-total {
						return nil, ErrRefundExport
					}
					remaining[issuer.AccountName] = issuer.AmountCents
					total += issuer.AmountCents
				}
				if total != invoice.ReissueCents || len(remaining) != len(items[i].IssuerAmounts) {
					return nil, ErrRefundExport
				}
				for j, issuer := range items[i].IssuerAmounts {
					amount, exists := remaining[issuer.AccountName]
					if !exists || (!invoice.ConsumedOnly && amount > issuer.AmountCents) {
						return nil, ErrRefundExport
					}
					items[i].IssuerAmounts[j].AmountCents = amount
				}
				continue
			}
			if len(items[i].IssuerAmounts) == 1 {
				items[i].IssuerAmounts[0].AmountCents = invoice.ReissueCents
				continue
			}
			linked := map[int]bool{}
			for _, id := range invoice.OrderIDs {
				linked[id] = true
			}
			remaining := map[string]int64{}
			for _, issuer := range items[i].IssuerAmounts {
				remaining[issuer.AccountName] = issuer.AmountCents
			}
			var deducted int64
			for _, payment := range data.Payments {
				if !linked[payment.OrderID] {
					continue
				}
				issuer := orderIssuers[payment.OrderID]
				if issuer == "" {
					issuer = "-"
				}
				if payment.RefundCents < 0 || payment.RefundCents > remaining[issuer] || payment.RefundCents > invoice.RefundCents-deducted {
					return nil, ErrRefundExport
				}
				remaining[issuer] -= payment.RefundCents
				deducted += payment.RefundCents
			}
			if deducted != invoice.RefundCents {
				return nil, ErrRefundExport
			}
			for j := range items[i].IssuerAmounts {
				items[i].IssuerAmounts[j].AmountCents = remaining[items[i].IssuerAmounts[j].AccountName]
			}
		}
	}
	return items, nil
}

func calculateOriginalRefundPayments(orders []TopUp, lots map[int]RefundCreditLot, requested int64, policy string) (*RefundExportData, error) {
	result := &RefundExportData{GiftRecoveryPolicy: policy, PaidQuota: requested, Payments: []RefundExportPayment{}, Invoices: []RefundExportInvoice{}}
	detail := &RefundFinanceDetail{}
	remaining := requested
	for _, order := range orders {
		lot := lots[order.Id]
		take := min(remaining, lot.PaidRemaining)
		if take <= 0 {
			continue
		}
		remaining -= take
		cash := refundProportionalCents(order.PaymentTotalCents, lot.RefundedPaid+take, order.QuotaGranted) - lot.RefundedCash
		if cash < 0 || cash > order.PaymentTotalCents-lot.RefundedCash || cash > math.MaxInt64-result.CashCents {
			return nil, ErrRefundExport
		}
		result.CashCents += cash
		tax := refundProportionalCents(order.InvoiceTaxCents, lot.RefundedPaid+take, order.QuotaGranted) - refundProportionalCents(order.InvoiceTaxCents, lot.RefundedPaid, order.QuotaGranted)
		detail.Allocations = append(detail.Allocations, RefundOrderAllocation{TopUpID: order.Id, UserID: order.UserId, TradeNo: order.TradeNo, OriginalPaidCents: order.PaymentTotalCents, RefundCents: cash, OriginalQuota: order.QuotaGranted, RefundQuota: take, OriginalTaxCents: order.InvoiceTaxCents, RefundTaxCents: tax})
	}
	if remaining != 0 || result.CashCents <= 0 {
		return nil, ErrRefundExport
	}
	if err := applyRefundGiftRecovery(orders, lots, detail, policy); err != nil {
		return nil, err
	}
	if detail.RefundGiftQuota > math.MaxInt32-requested {
		return nil, ErrTicketRefundQuotaInvalid
	}
	result.RefundGiftQuota = detail.RefundGiftQuota
	for _, row := range detail.Allocations {
		if row.OriginalGiftQuota == 0 {
			row.GiftPolicyVersion = 0
		}
		result.Payments = append(result.Payments, RefundExportPayment{UsedQuota: lots[row.TopUpID].UsedPaid, UsedCents: refundProportionalCents(row.OriginalPaidCents, lots[row.TopUpID].UsedPaid, row.OriginalQuota), OrderID: row.TopUpID, TradeNo: row.TradeNo, PaidCents: row.OriginalPaidCents, RefundCents: row.RefundCents, OriginalQuota: row.OriginalQuota, RefundQuota: row.RefundQuota, OriginalTaxCents: row.OriginalTaxCents, RefundTaxCents: row.RefundTaxCents, GiftPolicyVersion: row.GiftPolicyVersion, OriginalGiftQuota: row.OriginalGiftQuota, RefundGiftQuota: row.RefundGiftQuota})
	}
	return result, nil
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

func saveRefundInvoiceExportTx(tx *gorm.DB, refund *TicketRefund, actorID int, quotaIncludesGift ...bool) (*RefundExportData, error) {
	return buildRefundInvoiceExportTx(tx, refund, actorID, true, quotaIncludesGift...)
}

func buildRefundInvoiceExportTx(tx *gorm.DB, refund *TicketRefund, actorID int, persist bool, quotaIncludesGift ...bool) (*RefundExportData, error) {
	if refund.FinanceVersion == 1 {
		var external int64
		if err := tx.Model(&RefundInvoiceCorrection{}).Where("refund_ticket_id = ? AND status IN ?", refund.TicketId, []string{"processing", "completed"}).Count(&external).Error; err != nil {
			return nil, err
		}
		if external > 0 {
			return nil, ErrRefundLegacyWorkflow
		}
		if err := tx.Model(&RefundPayoutAttempt{}).Where("ticket_id = ? AND status != ?", refund.TicketId, "failed").Count(&external).Error; err != nil {
			return nil, err
		}
		if external > 0 {
			return nil, ErrRefundLegacyWorkflow
		}
	}
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
	invoiceIssuers := map[int][]InvoiceIssuerAmount{}
	invoiceTypes := map[int]map[string]RefundIssuerInvoiceType{}
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
		previousBlue, err := refundInvoiceExportItemsTx(tx, plan.TicketID, &data, true)
		if err != nil {
			return nil, err
		}
		previousRed, err := refundInvoiceExportItemsTx(tx, plan.TicketID, &data, false)
		if err != nil {
			return nil, err
		}
		for _, row := range data.Payments {
			previousAllocations = append(previousAllocations, RefundOrderAllocation{TopUpID: row.OrderID, OriginalQuota: row.OriginalQuota, RefundQuota: row.RefundQuota, RefundCents: row.RefundCents, GiftPolicyVersion: row.GiftPolicyVersion, OriginalGiftQuota: row.OriginalGiftQuota, RefundGiftQuota: row.RefundGiftQuota})
		}
		for i, row := range data.Invoices {
			invoiceRemaining[row.InvoiceTicketID] = row.ReissueCents
			invoiceIssuers[row.InvoiceTicketID] = previousBlue[i].IssuerAmounts
			if invoiceTypes[row.InvoiceTicketID] == nil {
				invoiceTypes[row.InvoiceTicketID] = map[string]RefundIssuerInvoiceType{}
			}
			for j, issuer := range previousRed[i].IssuerAmounts {
				kind, exists := row.IssuerInvoiceTypes[issuer.AccountName]
				if !exists {
					kind, exists = invoiceTypes[row.InvoiceTicketID][issuer.AccountName]
					if !exists {
						kind = RefundIssuerInvoiceType{row.InvoiceType, row.VATRateBps}
					}
				}
				affected := issuer.AmountCents > previousBlue[i].IssuerAmounts[j].AmountCents
				if row.ConsumedOnly {
					for _, name := range row.AffectedIssuers {
						affected = affected || name == issuer.AccountName
					}
				}
				if affected {
					kind = RefundIssuerInvoiceType{"ordinary", InvoiceVATRateBps}
				}
				invoiceTypes[row.InvoiceTicketID][issuer.AccountName] = kind
			}
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
	requested := int64(refund.RefundQuota)
	config, err := GetInvoiceTaxConfig(tx)
	if err != nil {
		return nil, err
	}
	if len(quotaIncludesGift) == 1 && quotaIncludesGift[0] {
		// An explicit final-balance override includes the gift debit in its total.
		var available int64
		for _, lot := range lots {
			if lot.PaidRemaining > math.MaxInt64-available {
				return nil, ErrRefundExport
			}
			available += lot.PaidRemaining
		}
		low, high := int64(1), min(requested, available)
		for low <= high {
			mid := low + (high-low)/2
			candidate, err := calculateOriginalRefundPayments(orders, lots, mid, config.RefundGiftRecoveryPolicy)
			if err != nil {
				return nil, err
			}
			total := candidate.PaidQuota + candidate.RefundGiftQuota
			if total == requested {
				result = candidate
				break
			}
			if total < requested {
				low = mid + 1
			} else {
				high = mid - 1
			}
		}
		if result == nil {
			return nil, ErrTicketRefundQuotaInvalid
		}
	} else {
		result, err = calculateOriginalRefundPayments(orders, lots, requested, config.RefundGiftRecoveryPolicy)
		if err != nil {
			return nil, err
		}
	}
	result.TicketID = refund.TicketId
	paid := map[int]int64{}
	allocated := map[int]int64{}
	for _, order := range orders {
		paid[order.Id] = order.PaymentTotalCents
	}
	for _, payment := range result.Payments {
		allocated[payment.OrderID] = payment.RefundCents
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
		if baseline == 0 && len(invoiceIssuers[invoice.TicketId]) == 0 {
			continue // A fully reversed invoice has no active blue amount to reverse again.
		}
		result.Invoices = append(result.Invoices, RefundExportInvoice{ConsumedOnly: true, InvoiceTicketID: invoice.TicketId, CompanyName: invoice.CompanyName, TaxNumber: invoice.TaxNumber, Email: invoice.Email, InvoiceType: invoice.InvoiceType, VATRateBps: invoice.VATRateBps, OrderIDs: ids, PaidCents: totalPaid, OriginalCents: baseline, RefundCents: reduction})
		result.Invoices[len(result.Invoices)-1].IssuerAmounts = invoiceIssuers[invoice.TicketId]
		result.Invoices[len(result.Invoices)-1].IssuerInvoiceTypes = invoiceTypes[invoice.TicketId]
	}
	red, err := refundInvoiceExportItemsTx(tx, refund.TicketId, result, false)
	if err != nil {
		return nil, err
	}
	for i := range result.Invoices {
		result.Invoices[i].IssuerAmounts = red[i].IssuerAmounts
	}
	if err := applyConsumedInvoiceAmounts(result, orders, lots); err != nil {
		return nil, err
	}
	if !persist {
		return result, nil
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
