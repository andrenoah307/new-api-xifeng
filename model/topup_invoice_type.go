package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrInvoiceAdjustment = errors.New("Invoice fee adjustment is unavailable for this order")
var ErrInvoiceAdjustmentNegative = errors.New("Confirm the negative balance before deducting invoice fees")
var ErrInvoiceAdjustmentGrace = errors.New("Invoice fee deduction exceeds the configured overdraft allowance")

// Immutable receipts also anchor conversion to the original net credit, so
// repeated invoice-type changes cannot accumulate rounding credits.
type TopUpInvoiceAdjustment struct {
	ID           int    `json:"id"`
	TopUpID      int    `json:"topup_id" gorm:"index;not null"`
	UserID       int    `json:"user_id" gorm:"index;not null"`
	ActorID      int    `json:"actor_id"`
	Revision     string `json:"revision" gorm:"size:64;uniqueIndex;not null"`
	BaseNetCents int64  `json:"base_net_cents"`
	BaseQuota    int64  `json:"base_quota,string" gorm:"type:bigint"`
	BeforeJSON   string `json:"-" gorm:"type:text"`
	ResultJSON   string `json:"-" gorm:"type:text"`
	CreatedAt    int64  `json:"created_at"`
}

type TopUpInvoiceAdjustmentInput struct {
	ID                  int     `json:"id"`
	InvoiceType         string  `json:"invoice_type"`
	ExpectedInvoiceType *string `json:"expected_invoice_type"`
	Revision            string  `json:"revision"`
	ConfirmNegative     bool    `json:"confirm_negative"`
}

type TopUpInvoiceAdjustmentPlan struct {
	Revision                     string `json:"revision"`
	TopUpID                      int    `json:"topup_id"`
	UserID                       int    `json:"user_id"`
	InvoiceType                  string `json:"invoice_type"`
	PreviousInvoiceType          string `json:"previous_invoice_type"`
	PaidCents                    int64  `json:"paid_cents"`
	TaxBeforeCents               int64  `json:"tax_before_cents"`
	TaxAfterCents                int64  `json:"tax_after_cents"`
	QuotaDelta                   string `json:"quota_delta"`
	BalanceBefore                string `json:"balance_before"`
	BalanceAfter                 string `json:"balance_after"`
	BalanceChange                string `json:"balance_change"`
	GraceAmount                  string `json:"grace_amount"`
	RequiresNegativeConfirmation bool   `json:"requires_negative_confirmation"`
	CanApply                     bool   `json:"can_apply"`
	Applied                      bool   `json:"applied"`
}

// Preview and commit share the same locks and calculation. apply=false never writes.
func ManageTopUpInvoiceType(actorID int, input TopUpInvoiceAdjustmentInput, apply bool) (*TopUpInvoiceAdjustmentPlan, error) {
	if input.ID <= 0 || input.ExpectedInvoiceType == nil || !slices.Contains([]string{"ordinary", "special", "none"}, input.InvoiceType) || (apply && len(input.Revision) != 64) {
		return nil, ErrInvoiceTaxInvalid
	}
	var identity TopUp
	if err := DB.Select("id,user_id").First(&identity, input.ID).Error; err != nil {
		return nil, err
	}
	var plan *TopUpInvoiceAdjustmentPlan
	written := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Lock before any consistent read: MySQL REPEATABLE READ must see
		// invoice/refund work committed by the preceding owner of this lock.
		var owner User
		if err := lockForUpdate(tx).First(&owner, identity.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := lockForUpdate(tx).First(&actor, actorID).Error; err != nil || actor.Status != common.UserStatusEnabled || (actor.Role != common.RoleAdminUser && actor.Role != common.RoleRootUser) {
			return ErrTicketForbidden
		}
		var order TopUp
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", input.ID, owner.Id).First(&order).Error; err != nil {
			return err
		}
		var option Option
		if err := lockForUpdate(tx).Where(Option{Key: InvoiceTaxConfigKey}).Take(&option).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		config, err := GetInvoiceTaxConfig(tx)
		if err != nil {
			return err
		}
		if !config.ModuleEnabled {
			return ErrTicketForbidden
		}
		if apply {
			var prior TopUpInvoiceAdjustment
			if err := tx.Where("revision = ? AND top_up_id = ? AND user_id = ?", input.Revision, order.Id, owner.Id).First(&prior).Error; err == nil {
				if err := common.UnmarshalJsonStr(prior.ResultJSON, &plan); err != nil {
					return err
				}
				if plan.InvoiceType != input.InvoiceType || plan.PreviousInvoiceType != *input.ExpectedInvoiceType {
					return ErrFinanceRevision
				}
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if order.InvoiceType != *input.ExpectedInvoiceType {
			return ErrInvoiceTaxQuoteChanged
		}
		if order.Status != common.TopUpStatusSuccess {
			return ErrTicketInvoiceOrderInvalid
		}
		var invoices []TicketInvoice
		if err := tx.Where("user_id = ? AND invoice_status IN ?", owner.Id, []int{InvoiceStatusPending, InvoiceStatusIssued}).Find(&invoices).Error; err != nil {
			return err
		}
		for _, invoice := range invoices {
			ids, err := invoice.GetTopUpOrderIDs()
			if err != nil {
				return err
			}
			if slices.Contains(ids, order.Id) {
				return ErrTicketInvoiceOrderDuplicate
			}
		}
		// Changing paid-source capacity can reassign FIFO consumption even when
		// this particular order was not allocated by a prior refund.
		var refunds int64
		if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status IN ?", owner.Id, []int{RefundStatusPending, RefundStatusRefunded}).Count(&refunds).Error; err != nil {
			return err
		}
		if refunds != 0 {
			return ErrInvoiceAdjustment
		}
		paid, err := invoiceMoneyCents(order.Money)
		if err != nil || !IsCNYInvoiceTopUp(&order) || order.Source == "discount_bonus" || order.CheckoutVersion != 1 || order.QuotaGranted <= 0 || order.PaymentTotalCents != paid || order.InvoiceNetCents <= 0 || order.InvoiceTaxCents < 0 || order.InvoiceTaxCents >= paid || order.InvoiceNetCents != paid-order.InvoiceTaxCents || int64(order.CheckoutBaseQuota) != order.QuotaGranted || order.CheckoutQuota < order.CheckoutBaseQuota {
			return ErrInvoiceAdjustment
		}
		quote := InvoiceTaxQuote{BaseCents: paid, TotalCents: paid, Mode: config.Mode}
		if input.InvoiceType != "none" {
			pricing := config
			if input.InvoiceType == "ordinary" && config.LegacyFreeOrdinaryEnabled && config.AllowsLegacyInvoice(&order) {
				pricing.Enabled = false
			}
			quote, err = CalculateInvoiceTaxFromTotal(paid, input.InvoiceType, pricing)
			if err != nil {
				return err
			}
		}
		baseQuota, baseNet := order.QuotaGranted, order.InvoiceNetCents
		var first, latest TopUpInvoiceAdjustment
		if err := tx.Where("top_up_id = ?", order.Id).Order("id ASC").First(&first).Error; err == nil {
			baseQuota, baseNet = first.BaseQuota, first.BaseNetCents
			if err := tx.Where("top_up_id = ?", order.Id).Order("id DESC").First(&latest).Error; err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if baseNet <= 0 || baseQuota <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || common.QuotaPerUnit <= 0 {
			return ErrInvoiceAdjustment
		}
		target, err := common.RechargeQuotaFromDecimal(decimal.NewFromInt(baseQuota).Mul(decimal.NewFromInt(quote.BaseCents)).DivRound(decimal.NewFromInt(baseNet), 0))
		if err != nil {
			return ErrInvoiceAdjustment
		}
		delta := decimal.NewFromInt(int64(target)).Sub(decimal.NewFromInt(order.QuotaGranted))
		after := decimal.NewFromInt(int64(owner.Quota)).Add(delta)
		checkout := decimal.NewFromInt(int64(order.CheckoutQuota)).Add(delta)
		maxInt := decimal.NewFromInt(int64(^uint(0) >> 1))
		if after.GreaterThan(maxInt) || after.LessThan(maxInt.Neg().Sub(decimal.NewFromInt(1))) || checkout.GreaterThan(maxInt) {
			return ErrInvoiceAdjustment
		}
		unit := decimal.NewFromFloat(common.QuotaPerUnit)
		creditUnit := unit
		if order.CheckoutQuotaPerUnit != "" {
			creditUnit, err = decimal.NewFromString(order.CheckoutQuotaPerUnit)
			if err != nil || !creditUnit.IsPositive() {
				return ErrInvoiceAdjustment
			}
		}
		amount, creditAmount, err := checkoutCreditAmount(int(checkout.IntPart()), creditUnit)
		if err != nil {
			return err
		}
		limit := decimal.NewFromInt(config.RefundOverdraftLimitCents).Mul(unit).Div(decimal.NewFromInt(100))
		negative := delta.IsNegative() && after.IsNegative()
		plan = &TopUpInvoiceAdjustmentPlan{
			TopUpID: order.Id, UserID: owner.Id,
			InvoiceType: input.InvoiceType, PreviousInvoiceType: order.InvoiceType,
			PaidCents: paid, TaxBeforeCents: order.InvoiceTaxCents, TaxAfterCents: quote.FeeCents,
			QuotaDelta: delta.String(), BalanceChange: delta.Div(unit).String(),
			BalanceBefore:                decimal.NewFromInt(int64(owner.Quota)).Div(unit).String(),
			BalanceAfter:                 after.Div(unit).String(),
			GraceAmount:                  decimal.NewFromInt(config.RefundOverdraftLimitCents).Div(decimal.NewFromInt(100)).String(),
			RequiresNegativeConfirmation: negative,
			CanApply:                     order.InvoiceType != input.InvoiceType && (!negative || (config.RefundOverdraftLimitCents > 0 && after.Neg().LessThanOrEqual(limit))),
		}
		before, err := common.Marshal(struct {
			Order      TopUp
			Balance    int
			Config     InvoiceTaxConfig
			Unit       string
			Latest     int
			Target     string
			CreditUnit string
		}{order, owner.Quota, config, unit.String(), latest.ID, input.InvoiceType, creditUnit.String()})
		if err != nil {
			return err
		}
		plan.Revision = fmt.Sprintf("%x", sha256.Sum256(before))
		if !apply {
			return nil
		}
		if plan.Revision != input.Revision {
			return ErrFinanceRevision
		}
		if !plan.CanApply {
			return ErrInvoiceAdjustmentGrace
		}
		if negative && !input.ConfirmNegative {
			return ErrInvoiceAdjustmentNegative
		}
		if !delta.IsZero() {
			result := tx.Model(&User{}).Where("id = ? AND quota = ?", owner.Id, owner.Quota).Update("quota", after.IntPart())
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrFinanceRevision
			}
		}
		result := tx.Model(&TopUp{}).
			Where("id = ? AND user_id = ? AND COALESCE(invoice_type, '') = ? AND quota_granted = ?", order.Id, owner.Id, order.InvoiceType, order.QuotaGranted).
			Updates(map[string]interface{}{
				"invoice_type": input.InvoiceType, "invoice_tax_mode": quote.Mode, "invoice_rate_bps": quote.RateBps,
				"invoice_net_cents": quote.BaseCents, "invoice_tax_cents": quote.FeeCents,
				"quota_granted": target, "checkout_base_quota": target, "checkout_quota": checkout.IntPart(),
				"amount": amount, "checkout_credit_amount": creditAmount,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrFinanceRevision
		}
		plan.Applied = true
		raw, err := common.Marshal(plan)
		if err != nil {
			return err
		}
		receipt := TopUpInvoiceAdjustment{TopUpID: order.Id, UserID: owner.Id, ActorID: actorID, Revision: plan.Revision, BaseNetCents: baseNet, BaseQuota: baseQuota, BeforeJSON: string(before), ResultJSON: string(raw), CreatedAt: common.GetTimestamp()}
		result = tx.Create(&receipt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrFinanceRevision
		}
		written = true
		return nil
	})
	if err == nil && written {
		if cacheErr := InvalidateUserCache(identity.UserId); cacheErr != nil {
			common.SysError("invoice adjustment cache invalidation: " + cacheErr.Error())
		}
		RecordLogWithAdminInfo(identity.UserId, LogTypeManage, fmt.Sprintf("Invoice type adjustment for order #%d: %s -> %s; fee %d -> %d cents; wallet change %s, balance %s", input.ID, plan.PreviousInvoiceType, plan.InvoiceType, plan.TaxBeforeCents, plan.TaxAfterCents, plan.BalanceChange, plan.BalanceAfter), map[string]interface{}{"admin_id": actorID, "revision": plan.Revision})
	}
	return plan, err
}
