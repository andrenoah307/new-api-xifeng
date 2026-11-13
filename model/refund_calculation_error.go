package model

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// Error() deliberately excludes financial evidence: shared/user-facing error
// handlers must not disclose diagnostics. Only the finance endpoint serializes it.
type RefundCalculationError struct {
	Code     string            `json:"code"`
	TicketID int               `json:"ticket_id,omitempty"`
	OrderID  int               `json:"order_id,omitempty"`
	Amounts  map[string]string `json:"amounts,omitempty"`
	cause    error
}

func (e *RefundCalculationError) Error() string { return ErrRefundExport.Error() }
func (e *RefundCalculationError) Unwrap() error { return e.cause }

func refundCalculationError(cause error, code string, ticketID, orderID int) *RefundCalculationError {
	var existing *RefundCalculationError
	if errors.As(cause, &existing) {
		return existing
	}
	return &RefundCalculationError{Code: code, TicketID: ticketID, OrderID: orderID, cause: cause}
}

func refundShortfallError(code string, orders []TopUp, lots map[int]RefundCreditLot, refund *TicketRefund, owner *User, available, limitCents int64) *RefundCalculationError {
	result := refundCalculationError(ErrRefundExport, code, refund.TicketId, 0)
	if math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || common.QuotaPerUnit <= 0 {
		return result
	}
	// These are credit balances, not assertions about gateway cash receipts.
	// Decimal accumulation prevents overflow even with inconsistent source data.
	recharged, consumed, refunded := decimal.Zero, decimal.Zero, decimal.Zero
	for _, order := range orders {
		lot := lots[order.Id]
		recharged = recharged.Add(decimal.NewFromInt(order.QuotaGranted))
		consumed = consumed.Add(decimal.NewFromInt(lot.UsedPaid))
		refunded = refunded.Add(decimal.NewFromInt(lot.RefundedPaid))
	}
	unit := decimal.NewFromFloat(common.QuotaPerUnit)
	result.Amounts = map[string]string{
		"recharged": recharged.Div(unit).String(),
		"consumed":  consumed.Div(unit).String(),
		"refunded":  refunded.Div(unit).String(),
		"available": decimal.NewFromInt(available).Div(unit).String(),
		"requested": decimal.NewFromInt(int64(refund.RefundQuota)).Div(unit).String(),
		"shortfall": decimal.NewFromInt(int64(refund.RefundQuota)).Sub(decimal.NewFromInt(available)).Div(unit).String(),
		"wallet":    decimal.NewFromInt(int64(owner.Quota)).Div(unit).String(),
		"limit":     decimal.NewFromInt(limitCents).Div(decimal.NewFromInt(100)).String(),
	}
	if refund.CalculationBasis == RefundBasisApplicationBalance {
		referral := decimal.NewFromInt(originalRefundReferralQuota(owner)).Div(unit)
		result.Amounts["configured_limit"] = result.Amounts["limit"]
		result.Amounts["referral_grace"] = referral.String()
		result.Amounts["limit"] = decimal.NewFromInt(max(int64(0), limitCents)).Div(decimal.NewFromInt(100)).Add(referral).String()
	}
	return result
}
