package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrRefundFinanceReview = errors.New("refund requires financial reconciliation")
var ErrRefundFinancePending = errors.New("complete the pending refund or invoice application first")
var ErrRefundFinanceCorrectionPending = errors.New("complete invoice reversal and reissue before confirming the refund")

type RefundOrderAllocation struct {
	ID                    int    `json:"id"`
	RefundTicketID        int    `json:"refund_ticket_id" gorm:"uniqueIndex:idx_refund_order;index"`
	TopUpID               int    `json:"top_up_id" gorm:"uniqueIndex:idx_refund_order;index"`
	UserID                int    `json:"user_id" gorm:"index"`
	TradeNo               string `json:"trade_no" gorm:"size:255"`
	InvoiceTicketID       int    `json:"invoice_ticket_id"`
	OriginalQuota         int64  `json:"original_quota" gorm:"type:bigint"`
	QuotaPerUnit          string `json:"quota_per_unit" gorm:"size:32"`
	UsedQuota             int64  `json:"used_quota" gorm:"type:bigint"`
	RefundableQuota       int64  `json:"refundable_quota" gorm:"type:bigint"`
	RefundQuota           int64  `json:"refund_quota" gorm:"type:bigint"`
	OriginalPaidCents     int64  `json:"original_paid_cents" gorm:"type:bigint"`
	OriginalTaxCents      int64  `json:"original_tax_cents" gorm:"type:bigint"`
	RefundCents           int64  `json:"refund_cents" gorm:"type:bigint"`
	RefundTaxCents        int64  `json:"refund_tax_cents" gorm:"type:bigint"`
	RemainingInvoiceCents int64  `json:"remaining_invoice_cents" gorm:"type:bigint"`
	UsedInvoiceCents      int64  `json:"used_invoice_cents" gorm:"type:bigint"`
	Status                string `json:"status" gorm:"size:16;index"`
}

type RefundFinanceDetail struct {
	QuoteToken         string                    `json:"quote_token,omitempty"`
	Version            int                       `json:"version"`
	RefundCashCents    int64                     `json:"refund_cash_cents"`
	RefundTaxCents     int64                     `json:"refund_tax_cents"`
	Allocations        []RefundOrderAllocation   `json:"allocations"`
	InvoiceCorrections []RefundInvoiceCorrection `json:"invoice_corrections"`
}

// Cumulative rounding makes repeated partial refunds conserve the original cents.
func refundProportionalCents(cents, quota, totalQuota int64) int64 {
	return decimal.NewFromInt(cents).Mul(decimal.NewFromInt(quota)).DivRound(decimal.NewFromInt(totalQuota), 0).IntPart()
}

func quoteRefundFinanceTx(tx *gorm.DB, userID, quota int) (*RefundFinanceDetail, error) {
	if quota <= 0 {
		return nil, ErrTicketRefundQuotaInvalid
	}
	var pending int64
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", userID, RefundStatusPending).Count(&pending).Error; err != nil {
		return nil, err
	}
	if pending != 0 {
		return nil, ErrRefundFinancePending
	}
	var legacy int64
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ? AND finance_version = 0", userID, RefundStatusRefunded).Count(&legacy).Error; err != nil {
		return nil, err
	}
	if legacy != 0 {
		return nil, ErrRefundFinanceReview
	}
	cap, err := getUserMaxRefundableQuotaTx(tx, userID)
	if err != nil {
		return nil, err
	}
	if quota > cap {
		return nil, ErrTicketRefundQuotaExceed
	}
	var orders []TopUp
	if err := tx.Where("user_id = ? AND status = ? AND (source IS NULL OR source != ?)", userID, common.TopUpStatusSuccess, "discount_bonus").Order("complete_time ASC, id ASC").Limit(10001).Find(&orders).Error; err != nil {
		return nil, err
	}
	if len(orders) == 0 || len(orders) > 10000 {
		return nil, ErrRefundFinanceReview
	}
	var previous []RefundOrderAllocation
	if err := tx.Where("user_id = ? AND status = ?", userID, "committed").Limit(100001).Find(&previous).Error; err != nil {
		return nil, err
	}
	if len(previous) > 100000 {
		return nil, ErrRefundFinanceReview
	}
	prior := map[int]RefundOrderAllocation{}
	for _, row := range previous {
		old := prior[row.TopUpID]
		if row.RefundQuota <= 0 || row.RefundCents < 0 || row.RefundTaxCents < 0 || old.RefundQuota > math.MaxInt64-row.RefundQuota || old.RefundCents > math.MaxInt64-row.RefundCents || old.RefundTaxCents > math.MaxInt64-row.RefundTaxCents {
			return nil, ErrRefundFinanceReview
		}
		old.RefundQuota += row.RefundQuota
		old.RefundCents += row.RefundCents
		old.RefundTaxCents += row.RefundTaxCents
		prior[row.TopUpID] = old
	}
	var totalRemaining int64
	for _, order := range orders {
		// Old floating-point prices and unallocated historic tax payments need review.
		if order.CheckoutVersion != 1 || !IsCNYInvoiceTopUp(&order) || order.QuotaGranted <= 0 || order.PaymentTotalCents <= 0 || order.PaymentTotalCents > 1000000000000000 || order.InvoiceNetCents < 0 || order.InvoiceTaxCents < 0 || order.InvoiceTaxCents > order.PaymentTotalCents || order.InvoiceNetCents != order.PaymentTotalCents-order.InvoiceTaxCents {
			return nil, ErrRefundFinanceReview
		}
		old := prior[order.Id]
		if old.RefundQuota > order.QuotaGranted || old.RefundCents > order.PaymentTotalCents || old.RefundTaxCents > order.InvoiceTaxCents {
			return nil, ErrRefundFinanceReview
		}
		remaining := order.QuotaGranted - old.RefundQuota
		if totalRemaining > math.MaxInt64-remaining {
			return nil, ErrRefundFinanceReview
		}
		totalRemaining += remaining
	}
	if int64(cap) > totalRemaining {
		return nil, ErrRefundFinanceReview
	}
	// A wallet balance alone cannot distinguish unused cash from manual credits.
	// Require the durable aggregate ledger to reconcile before attributing usage.
	var owner User
	if err := tx.Select("quota, used_quota, aff_history, aff_quota").First(&owner, userID).Error; err != nil {
		return nil, err
	}
	if owner.Quota < 0 || owner.UsedQuota < 0 || owner.AffHistoryQuota < 0 || owner.AffQuota < 0 {
		return nil, ErrRefundFinanceReview
	}
	var bonus, balanceTax int64
	if err := tx.Model(&TopUp{}).Where("user_id = ? AND status = ? AND source = ?", userID, common.TopUpStatusSuccess, "discount_bonus").Select("COALESCE(SUM(quota_granted),0)").Scan(&bonus).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&InvoiceTaxCharge{}).Where("user_id = ? AND status = ? AND payment_method = ?", userID, "paid", "balance").Select("COALESCE(SUM(quota),0)").Scan(&balanceTax).Error; err != nil {
		return nil, err
	}
	if bonus < 0 || balanceTax < 0 {
		return nil, ErrRefundFinanceReview
	}
	known := decimal.NewFromInt(totalRemaining).Add(decimal.NewFromInt(bonus)).Add(decimal.NewFromInt(max(int64(0), int64(owner.AffHistoryQuota)-int64(owner.AffQuota))))
	accounted := decimal.NewFromInt(int64(owner.Quota)).Add(decimal.NewFromInt(int64(owner.UsedQuota))).Add(decimal.NewFromInt(balanceTax))
	if !known.Equal(accounted) {
		return nil, ErrRefundFinanceReview
	}
	var invoices []TicketInvoice
	if err := activeInvoiceOrders(tx).Where("ticket_invoices.user_id = ? AND ticket_invoices.invoice_status IN ?", userID, []int{InvoiceStatusPending, InvoiceStatusIssued}).Find(&invoices).Error; err != nil {
		return nil, err
	}
	invoiceByOrder := map[int]TicketInvoice{}
	for _, invoice := range invoices {
		ids, err := invoice.GetTopUpOrderIDs()
		if err != nil {
			return nil, ErrRefundFinanceReview
		}
		for _, id := range ids {
			if _, exists := invoiceByOrder[id]; exists {
				return nil, ErrRefundFinanceReview
			}
			invoiceByOrder[id] = invoice
		}
	}
	result := &RefundFinanceDetail{Version: 1, Allocations: []RefundOrderAllocation{}, InvoiceCorrections: []RefundInvoiceCorrection{}}
	used, requested := totalRemaining-int64(cap), int64(quota)
	for _, order := range orders {
		old := prior[order.Id]
		remaining := order.QuotaGranted - old.RefundQuota
		consumed := min(used, remaining)
		used -= consumed
		available := remaining - consumed
		take := min(requested, available)
		if take == 0 {
			continue
		}
		invoice := invoiceByOrder[order.Id]
		if invoice.InvoiceStatus == InvoiceStatusPending {
			return nil, ErrRefundFinancePending
		}
		if invoice.TicketId != 0 {
			var chargeCount int64
			if err := tx.Model(&InvoiceTaxCharge{}).Where("ticket_id = ? AND status = ? AND fee_cents > 0", invoice.TicketId, "paid").Count(&chargeCount).Error; err != nil {
				return nil, err
			}
			if chargeCount != 0 {
				return nil, ErrRefundFinanceReview
			}
		}
		cash := refundProportionalCents(order.PaymentTotalCents, old.RefundQuota+take, order.QuotaGranted) - old.RefundCents
		tax := refundProportionalCents(order.InvoiceTaxCents, old.RefundCents+cash, order.PaymentTotalCents) - old.RefundTaxCents
		if cash < 0 || tax < 0 || tax > cash || result.RefundCashCents > math.MaxInt64-cash || result.RefundTaxCents > math.MaxInt64-tax {
			return nil, ErrRefundFinanceReview
		}
		row := RefundOrderAllocation{TopUpID: order.Id, UserID: userID, TradeNo: order.TradeNo, InvoiceTicketID: invoice.TicketId, OriginalQuota: order.QuotaGranted, UsedQuota: consumed, RefundableQuota: available, RefundQuota: take, OriginalPaidCents: order.PaymentTotalCents, OriginalTaxCents: order.InvoiceTaxCents, RefundCents: cash, RefundTaxCents: tax, RemainingInvoiceCents: order.PaymentTotalCents - old.RefundCents - cash, UsedInvoiceCents: refundProportionalCents(order.PaymentTotalCents, consumed, order.QuotaGranted), Status: "reserved"}
		row.QuotaPerUnit = decimal.NewFromFloat(common.QuotaPerUnit).String()
		result.Allocations = append(result.Allocations, row)
		result.RefundCashCents += cash
		result.RefundTaxCents += tax
		requested -= take
		if requested == 0 {
			break
		}
	}
	if requested != 0 {
		return nil, ErrTicketRefundQuotaExceed
	}
	if result.RefundCashCents > 9007199254740991 {
		return nil, ErrRefundFinanceReview
	}
	encoded, err := common.Marshal(result)
	if err != nil {
		return nil, err
	}
	result.QuoteToken = fmt.Sprintf("%x", sha256.Sum256(encoded))
	return result, nil
}

func QuoteRefundFinance(userID, quota int) (*RefundFinanceDetail, error) {
	var detail *RefundFinanceDetail
	err := DB.Transaction(func(tx *gorm.DB) error {
		required, err := refundFinanceRequired(tx, userID)
		if err != nil {
			return err
		}
		if !required {
			return nil
		}
		var user User
		if err := lockForUpdate(tx).Select("id").First(&user, userID).Error; err != nil {
			return err
		}
		detail, err = quoteRefundFinanceTx(tx, userID, quota)
		return err
	})
	return detail, err
}

func prepareRefundFinanceTx(tx *gorm.DB, refund *TicketRefund, params CreateRefundTicketParams) error {
	required, err := refundFinanceRequired(tx, refund.UserId)
	if err != nil {
		return err
	}
	if !required {
		return nil
	}
	if _, err := uuid.Parse(params.RequestKey); err != nil {
		return ErrTicketRefundQuotaInvalid
	}
	detail, err := quoteRefundFinanceTx(tx, refund.UserId, refund.RefundQuota)
	if err != nil {
		return err
	}
	if params.FinanceQuote != detail.QuoteToken {
		return ErrInvoiceTaxQuoteChanged
	}
	refund.FinanceVersion, refund.RefundCashCents = 1, detail.RefundCashCents
	for i := range detail.Allocations {
		detail.Allocations[i].RefundTicketID = refund.TicketId
	}
	if err := tx.Create(&detail.Allocations).Error; err != nil {
		return err
	}
	if err := createRefundInvoiceCorrectionsTx(tx, refund, detail.Allocations); err != nil {
		return err
	}
	refund.Finance = detail
	return nil
}

func refundFinanceRequired(tx *gorm.DB, userID int) (bool, error) {
	config, err := GetInvoiceTaxConfig(tx)
	if err != nil {
		return false, err
	}
	if config.ModuleEnabled {
		return true, nil
	}
	var count int64
	err = tx.Model(&TopUp{}).Where("user_id = ? AND status = ? AND checkout_version = ?", userID, common.TopUpStatusSuccess, 1).Limit(1).Count(&count).Error
	return count > 0, err
}

func finalizeRefundFinanceTx(tx *gorm.DB, refund *TicketRefund, params UpdateRefundStatusParams) error {
	if refund.FinanceVersion != 1 {
		return nil
	}
	if params.QuotaMode != "" && params.QuotaMode != RefundQuotaModeWriteOff {
		return ErrTicketRefundQuotaModeInvalid
	}
	var actor User
	if err := tx.Where("id = ? AND status = ? AND role IN ?", params.AdminId, common.UserStatusEnabled, []int{common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
		return ErrTicketForbidden
	}
	var pending int64
	if err := tx.Model(&RefundInvoiceCorrection{}).Where("refund_ticket_id = ? AND status != ?", refund.TicketId, "completed").Count(&pending).Error; err != nil {
		return err
	}
	if pending != 0 {
		return ErrRefundFinanceCorrectionPending
	}
	var rows []RefundOrderAllocation
	if err := tx.Where("refund_ticket_id = ?", refund.TicketId).Find(&rows).Error; err != nil {
		return err
	}
	var quota, cash int64
	for _, row := range rows {
		if row.Status != "reserved" || row.RefundQuota <= 0 || row.RefundCents < 0 || quota > math.MaxInt64-row.RefundQuota || cash > math.MaxInt64-row.RefundCents {
			return ErrRefundFinanceReview
		}
		quota += row.RefundQuota
		cash += row.RefundCents
	}
	if len(rows) == 0 || quota != int64(refund.RefundQuota) || cash != refund.RefundCashCents {
		return ErrRefundFinanceReview
	}
	result := tx.Model(&RefundOrderAllocation{}).Where("refund_ticket_id = ? AND status = ?", refund.TicketId, "reserved").Update("status", "committed")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != int64(len(rows)) {
		return ErrRefundFinanceReview
	}
	return nil
}

func releaseRefundFinanceTx(tx *gorm.DB, refund *TicketRefund) error {
	if refund.FinanceVersion != 1 {
		return nil
	}
	var completed int64
	if err := tx.Model(&RefundInvoiceCorrection{}).Where("refund_ticket_id = ? AND status IN ?", refund.TicketId, []string{"processing", "completed"}).Count(&completed).Error; err != nil {
		return err
	}
	if completed != 0 {
		return ErrRefundFinanceCorrectionPending
	}
	if err := tx.Model(&RefundInvoiceCorrection{}).Where("refund_ticket_id = ? AND status = ?", refund.TicketId, "pending").Update("status", "cancelled").Error; err != nil {
		return err
	}
	return tx.Model(&RefundOrderAllocation{}).Where("refund_ticket_id = ? AND status = ?", refund.TicketId, "reserved").Update("status", "released").Error
}

func AttachRefundFinance(tx *gorm.DB, refund *TicketRefund) error {
	if refund.FinanceVersion != 1 {
		return nil
	}
	detail := &RefundFinanceDetail{Version: 1, RefundCashCents: refund.RefundCashCents, Allocations: []RefundOrderAllocation{}, InvoiceCorrections: []RefundInvoiceCorrection{}}
	if err := tx.Where("refund_ticket_id = ?", refund.TicketId).Order("id ASC").Find(&detail.Allocations).Error; err != nil {
		return err
	}
	if err := tx.Where("refund_ticket_id = ?", refund.TicketId).Order("id ASC").Find(&detail.InvoiceCorrections).Error; err != nil {
		return err
	}
	for _, row := range detail.Allocations {
		detail.RefundTaxCents += row.RefundTaxCents
	}
	refund.Finance = detail
	return nil
}

// Invoice eligibility uses remaining paid amounts without rewriting payment snapshots.
func adjustInvoiceOrdersForRefunds(tx *gorm.DB, orders []*TopUp) ([]*TopUp, error) {
	if len(orders) == 0 {
		return orders, nil
	}
	ids := make([]int, 0, len(orders))
	for _, order := range orders {
		ids = append(ids, order.Id)
	}
	var allocations []RefundOrderAllocation
	if err := tx.Where("top_up_id IN ? AND status IN ?", ids, []string{"reserved", "committed"}).Find(&allocations).Error; err != nil {
		return nil, err
	}
	paid, taxes := map[int]int64{}, map[int]int64{}
	reserved := map[int]bool{}
	for _, row := range allocations {
		if row.Status == "reserved" {
			reserved[row.TopUpID] = true
			continue
		}
		if row.RefundCents < 0 || row.RefundTaxCents < 0 || paid[row.TopUpID] > math.MaxInt64-row.RefundCents || taxes[row.TopUpID] > math.MaxInt64-row.RefundTaxCents {
			return nil, ErrRefundFinanceReview
		}
		paid[row.TopUpID] += row.RefundCents
		taxes[row.TopUpID] += row.RefundTaxCents
	}
	result := make([]*TopUp, 0, len(orders))
	for _, order := range orders {
		if reserved[order.Id] {
			continue
		}
		if paid[order.Id] == 0 {
			result = append(result, order)
			continue
		}
		if paid[order.Id] > order.PaymentTotalCents || taxes[order.Id] > order.InvoiceTaxCents {
			return nil, ErrRefundFinanceReview
		}
		copyOrder := *order
		copyOrder.PaymentTotalCents -= paid[order.Id]
		copyOrder.InvoiceTaxCents -= taxes[order.Id]
		copyOrder.InvoiceNetCents = copyOrder.PaymentTotalCents - copyOrder.InvoiceTaxCents
		copyOrder.Money = decimal.NewFromInt(copyOrder.PaymentTotalCents).Div(decimal.NewFromInt(100)).InexactFloat64()
		if copyOrder.PaymentTotalCents > 0 {
			result = append(result, &copyOrder)
		}
	}
	return result, nil
}
