package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"math"
)

// RefundCreditLot is a projection of settled usage, not another writable wallet.
type RefundCreditLot struct {
	UsedPaid       int64
	PaidRemaining  int64
	GiftRemaining  int64
	RefundedPaid   int64
	RefundedCash   int64
	RecoveredGift  int64
	GiftPolicyCash int64
}

func refundCreditLotsTx(tx *gorm.DB, userID int, orders []TopUp) (map[int]RefundCreditLot, error) {
	var owner User
	if err := tx.Select("used_quota").First(&owner, userID).Error; err != nil {
		return nil, err
	}
	var feeQuota int64
	if err := tx.Model(&InvoiceTaxCharge{}).Where("user_id = ? AND status = ? AND payment_method = ?", userID, "paid", "balance").Select("COALESCE(SUM(quota),0)").Scan(&feeQuota).Error; err != nil {
		return nil, err
	}
	if owner.UsedQuota < 0 || feeQuota < 0 || int64(owner.UsedQuota) > math.MaxInt64-feeQuota {
		return nil, ErrRefundFinanceReview
	}
	spent := int64(owner.UsedQuota) + feeQuota
	var rows []RefundOrderAllocation
	if err := tx.Where("user_id = ? AND status IN ?", userID, []string{"committed", "reserved"}).Limit(100001).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 100000 {
		return nil, ErrRefundFinanceReview
	}
	return projectRefundCreditLots(spent, orders, rows)
}

func projectRefundCreditLots(spent int64, orders []TopUp, rows []RefundOrderAllocation) (map[int]RefundCreditLot, error) {
	sources := map[int]TopUp{}
	for _, order := range orders {
		sources[order.Id] = order
	}
	lots := map[int]RefundCreditLot{}
	for _, row := range rows {
		order, exists := sources[row.TopUpID]
		lot := lots[row.TopUpID]
		if !exists || row.OriginalQuota != order.QuotaGranted || row.RefundQuota < 0 || row.RefundGiftQuota < 0 || row.RefundCents < 0 || row.RefundQuota > order.QuotaGranted-lot.RefundedPaid || row.RefundGiftQuota > order.RefundGiftQuota-lot.RecoveredGift {
			return nil, ErrRefundFinanceReview
		}
		if row.GiftPolicyVersion != 0 && row.OriginalGiftQuota != order.RefundGiftQuota {
			return nil, ErrRefundFinanceReview
		}
		lot.RefundedPaid += row.RefundQuota
		if row.RefundCents > order.PaymentTotalCents-lot.RefundedCash {
			return nil, ErrRefundFinanceReview
		}
		lot.RefundedCash += row.RefundCents
		lot.RecoveredGift += row.RefundGiftQuota
		if row.GiftPolicyVersion != 0 {
			if row.RefundCents > math.MaxInt64-lot.GiftPolicyCash {
				return nil, ErrRefundFinanceReview
			}
			lot.GiftPolicyCash += row.RefundCents
		}
		lots[row.TopUpID] = lot
	}
	for _, order := range orders {
		if order.QuotaGranted < 0 || order.RefundGiftQuota < 0 {
			return nil, ErrRefundFinanceReview
		}
		lot := lots[order.Id]
		available := order.QuotaGranted - lot.RefundedPaid
		lot.UsedPaid = min(spent, available)
		spent -= lot.UsedPaid
		lot.PaidRemaining = available - lot.UsedPaid
		lots[order.Id] = lot
	}
	// Spend all paid balances before consuming any order's noncash gift.
	for _, order := range orders {
		lot := lots[order.Id]
		gift := order.RefundGiftQuota - lot.RecoveredGift
		consumedGift := min(spent, gift)
		spent -= consumedGift
		lot.GiftRemaining = gift - consumedGift
		lots[order.Id] = lot
	}
	return lots, nil
}

const (
	RefundGiftPriorExpiry  = "prior_expiry"
	RefundGiftProportional = "proportional"
	RefundGiftNone         = "none"
)

// Gift recovery is independent of cash and invoice amounts.
// Gift-only allocations carry no cash or invoice correction, but are held and
// released in the same transaction as the paid refund.
func applyRefundGiftRecovery(orders []TopUp, lots map[int]RefundCreditLot, detail *RefundFinanceDetail, policy string) error {
	if policy != RefundGiftPriorExpiry && policy != RefundGiftProportional && policy != RefundGiftNone {
		return ErrInvoiceTaxInvalid
	}
	if len(detail.Allocations) == 0 {
		return ErrTicketRefundQuotaInvalid
	}
	lastID := detail.Allocations[len(detail.Allocations)-1].TopUpID
	selected := map[int]RefundOrderAllocation{}
	for _, row := range detail.Allocations {
		selected[row.TopUpID] = row
	}
	detail.Allocations = nil
	for _, order := range orders {
		lot := lots[order.Id]
		row, exists := selected[order.Id]
		if !exists && (lot.GiftRemaining == 0 || policy != RefundGiftPriorExpiry) {
			continue
		}
		if !exists {
			row = RefundOrderAllocation{TopUpID: order.Id, UserID: order.UserId, TradeNo: order.TradeNo, OriginalQuota: order.QuotaGranted, OriginalPaidCents: order.PaymentTotalCents, OriginalTaxCents: order.InvoiceTaxCents, UsedQuota: lot.UsedPaid, UsedInvoiceCents: refundProportionalCents(order.PaymentTotalCents, lot.UsedPaid, order.QuotaGranted), RemainingInvoiceCents: order.PaymentTotalCents - lot.RefundedCash, Status: "reserved", QuotaPerUnit: decimal.NewFromFloat(common.QuotaPerUnit).String()}
		}
		row.GiftPolicyVersion, row.OriginalGiftQuota = 1, order.RefundGiftQuota
		if policy == RefundGiftNone {
			row.GiftPolicyVersion, row.RefundGiftQuota = 0, 0
			row.GiftRecoveryReason = "none"
		} else if order.Id == lastID || policy == RefundGiftProportional {
			if policy == RefundGiftProportional {
				row.GiftPolicyVersion = 2
			}
			row.GiftRecoveryReason = "proportional_refund"
			row.RefundGiftQuota = refundProportionalCents(order.RefundGiftQuota, lot.GiftPolicyCash+row.RefundCents, order.PaymentTotalCents) - lot.RecoveredGift
		} else {
			if lot.PaidRemaining-row.RefundQuota != 0 {
				return ErrRefundFinanceReview
			}
			row.GiftRecoveryReason = "prior_order_expired"
			row.RefundGiftQuota = lot.GiftRemaining
		}
		if row.RefundGiftQuota < 0 || row.RefundGiftQuota > lot.GiftRemaining || row.RefundGiftQuota > math.MaxInt64-detail.RefundGiftQuota {
			return ErrRefundFinanceReview
		}
		detail.RefundGiftQuota += row.RefundGiftQuota
		unit := decimal.NewFromInt(order.InvoiceNetCents).Div(decimal.NewFromInt(order.QuotaGranted))
		recovered := unit.Mul(decimal.NewFromInt(row.RefundGiftQuota)).Round(0)
		remaining := unit.Mul(decimal.NewFromInt(lot.GiftRemaining - row.RefundGiftQuota)).Round(0)
		if recovered.GreaterThan(decimal.NewFromInt(maxRefundEligibilityJSONQuota)) || remaining.GreaterThan(decimal.NewFromInt(maxRefundEligibilityJSONQuota)) {
			return ErrRefundFinanceReview
		}
		row.RefundGiftValueCents, row.RemainingGiftValueCents = recovered.IntPart(), remaining.IntPart()
		if row.RefundGiftValueCents > maxRefundEligibilityJSONQuota-detail.RefundGiftValueCents {
			return ErrRefundFinanceReview
		}
		detail.RefundGiftValueCents += row.RefundGiftValueCents
		detail.Allocations = append(detail.Allocations, row)
		if order.Id == lastID {
			return nil
		}
	}
	return ErrRefundFinanceReview
}

// Gift recovery is additional to refundable paid quota and stays in the hold.
func (refund *TicketRefund) heldRefundQuota() (int64, bool) {
	if refund.RefundQuota < 0 || refund.RefundGiftQuota < 0 || refund.RefundGiftQuota > math.MaxInt64-int64(refund.RefundQuota) {
		return 0, false
	}
	return int64(refund.RefundQuota) + refund.RefundGiftQuota, true
}

func (refund *TicketRefund) hasVerifiedRefundHold() bool {
	total, valid := refund.heldRefundQuota()
	return valid && refund.SettlementVerified && int64(refund.FrozenQuota) == total
}

func (refund *TicketRefund) hasValidRefundHold() bool {
	total, valid := refund.heldRefundQuota()
	return valid && refund.FrozenQuota >= 0 && int64(refund.FrozenQuota) <= total
}

func (refund *TicketRefund) hasProvisionalRefundHold() bool {
	return !refund.SettlementVerified && refund.RefundQuota > 0 && refund.RefundGiftQuota == 0 && refund.hasValidRefundHold()
}
