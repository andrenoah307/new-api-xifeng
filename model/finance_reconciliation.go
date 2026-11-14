package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrFinanceRevision = errors.New("financial evidence changed; refresh and review again")
var ErrFinanceReviewPending = errors.New("Financial reconciliation required")
var ErrFinanceEvidence = errors.New("financial reconciliation evidence is invalid")
var ErrFinanceReviewer = errors.New("financial reconciliation requires a different administrator to approve")

// Supplemental evidence never rewrites a payment receipt or changes the wallet.
type FinancePaymentEvidence struct {
	TopUpID    int   `json:"top_up_id"`
	PaidCents  int64 `json:"paid_cents"`
	TaxCents   int64 `json:"tax_cents"`
	PaidQuota  int64 `json:"paid_quota"`
	BonusQuota int64 `json:"bonus_quota"`
}

type FinanceReconciliationPlan struct {
	Payments          []FinancePaymentEvidence `json:"payments"`
	NoncashGrantQuota int64                    `json:"noncash_grant_quota"`
	Evidence          string                   `json:"evidence"`
}

type FinanceReconciliation struct {
	ApprovalsJSON  string                          `json:"-" gorm:"type:text"`
	Approvals      []FinanceReconciliationApproval `json:"approvals" gorm:"-"`
	Id             int                             `json:"id"`
	UserId         int                             `json:"user_id" gorm:"index;not null"`
	TicketId       int                             `json:"ticket_id" gorm:"index;not null"`
	RequestKey     string                          `json:"request_key" gorm:"size:64;uniqueIndex;not null"`
	SourceRevision string                          `json:"source_revision" gorm:"size:64;not null"`
	PlanJSON       string                          `json:"-" gorm:"type:text;not null"`
	Plan           FinanceReconciliationPlan       `json:"plan" gorm:"-"`
	Status         string                          `json:"status" gorm:"size:16;index;not null"`
	ProposedBy     int                             `json:"proposed_by"`
	ReviewedBy     int                             `json:"reviewed_by"`
	ReviewReason   string                          `json:"review_reason" gorm:"type:text"`
	CreatedTime    int64                           `json:"created_time"`
	ReviewedTime   int64                           `json:"reviewed_time"`
}

type FinanceReconciliationApproval struct {
	AdminID     int    `json:"admin_id"`
	Reason      string `json:"reason"`
	CreatedTime int64  `json:"created_time"`
}

type FinanceBlocker struct {
	Code    string `json:"code"`
	OrderID int    `json:"order_id,omitempty"`
}

type FinanceSourceOrder struct {
	GiftQuota       int64   `json:"gift_quota"`
	ID              int     `json:"id"`
	Source          string  `json:"source"`
	CheckoutVersion int     `json:"checkout_version"`
	Money           float64 `json:"money"`
	PaidCents       int64   `json:"paid_cents"`
	TaxCents        int64   `json:"tax_cents"`
	Quota           int64   `json:"quota"`
	DiscountRate    int     `json:"discount_rate"`
}

type RefundReconciliationReport struct {
	ReviewEnabled   bool                    `json:"review_enabled"`
	ReviewCount     int                     `json:"review_count"`
	QuotaPerUnit    string                  `json:"quota_per_unit"`
	USDExchangeRate string                  `json:"usd_exchange_rate"`
	Revision        string                  `json:"revision"`
	Stage           string                  `json:"stage"`
	Blockers        []FinanceBlocker        `json:"blockers"`
	Orders          []FinanceSourceOrder    `json:"orders"`
	PaidQuota       int64                   `json:"paid_quota"`
	NoncashQuota    int64                   `json:"noncash_quota"`
	WalletQuota     int                     `json:"wallet_quota"`
	UsedQuota       int                     `json:"used_quota"`
	Difference      string                  `json:"difference"`
	AccountedQuota  string                  `json:"accounted_quota"`
	KnownQuota      string                  `json:"known_quota"`
	Reviews         []FinanceReconciliation `json:"reviews"`
}

func applyFinancePaymentEvidence(orders []TopUp, plan FinanceReconciliationPlan) (int64, error) {
	if plan.NoncashGrantQuota < 0 || plan.NoncashGrantQuota > math.MaxInt32 {
		return 0, ErrFinanceEvidence
	}
	index := map[int]int{}
	for i := range orders {
		index[orders[i].Id] = i
	}
	seen := map[int]bool{}
	extra := plan.NoncashGrantQuota
	for _, entry := range plan.Payments {
		i, exists := index[entry.TopUpID]
		if !exists || seen[entry.TopUpID] {
			return 0, ErrFinanceEvidence
		}
		seen[entry.TopUpID] = true
		order := &orders[i]
		paid, err := invoiceMoneyCents(order.Money)
		if err != nil || order.CheckoutVersion != 0 || !IsCNYInvoiceTopUp(order) || order.Source == "discount_bonus" || entry.PaidCents != paid || entry.TaxCents < 0 || entry.TaxCents >= paid || entry.PaidQuota <= 0 || entry.BonusQuota < 0 || entry.PaidQuota > order.QuotaGranted || entry.BonusQuota != order.QuotaGranted-entry.PaidQuota || entry.BonusQuota > math.MaxInt64-extra {
			return 0, ErrFinanceEvidence
		}
		order.CheckoutVersion, order.PaymentTotalCents = 1, entry.PaidCents
		order.InvoiceTaxCents, order.InvoiceNetCents = entry.TaxCents, entry.PaidCents-entry.TaxCents
		order.QuotaGranted = entry.PaidQuota
		order.RefundGiftQuota = entry.BonusQuota
		extra += entry.BonusQuota
	}
	return extra, nil
}

func loadRefundPaymentSources(tx *gorm.DB, userID int) ([]TopUp, int64, error) {
	var orders []TopUp
	if err := tx.Where("user_id = ? AND status = ?", userID, common.TopUpStatusSuccess).Order("complete_time ASC, id ASC").Limit(10001).Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	if len(orders) > 10000 {
		return nil, 0, ErrRefundFinanceReview
	}
	var records []FinanceReconciliation
	if err := tx.Where("user_id = ? AND status = ?", userID, "applied").Order("id ASC").Find(&records).Error; err != nil {
		return nil, 0, err
	}
	var bonus int64
	for _, record := range records {
		var plan FinanceReconciliationPlan
		if err := common.UnmarshalJsonStr(record.PlanJSON, &plan); err != nil {
			return nil, 0, err
		}
		additional, err := applyFinancePaymentEvidence(orders, plan)
		if err != nil || additional > math.MaxInt64-bonus {
			return nil, 0, ErrFinanceEvidence
		}
		bonus += additional
	}
	paid := make([]TopUp, 0, len(orders))
	gifts := map[string]TopUp{}
	for _, order := range orders {
		if order.Source == "discount_bonus" {
			gifts[order.TradeNo] = order
		}
	}
	for _, order := range orders {
		if order.Source == "discount_bonus" {
			if order.QuotaGranted < 0 || order.QuotaGranted > math.MaxInt64-bonus {
				return nil, 0, ErrFinanceEvidence
			}
			bonus += order.QuotaGranted
		} else {
			if gift, exists := gifts[order.TradeNo+"_bonus"]; exists {
				if gift.DiscountCodeId != order.DiscountCodeId || gift.QuotaGranted < 0 || order.RefundGiftQuota != 0 {
					return nil, 0, ErrFinanceEvidence
				}
				order.RefundGiftQuota = gift.QuotaGranted
			}
			paid = append(paid, order)
		}
	}
	return paid, bonus, nil
}

func refundReconciliationReportTx(tx *gorm.DB, refund TicketRefund) (*RefundReconciliationReport, error) {
	var owner User
	if err := tx.Select("id, quota, used_quota, aff_history, aff_quota").First(&owner, refund.UserId).Error; err != nil {
		return nil, err
	}
	orders, bonus, err := loadRefundPaymentSources(tx, refund.UserId)
	if err != nil {
		return nil, err
	}
	report := &RefundReconciliationReport{Stage: "awaiting_owner_confirmation", Blockers: []FinanceBlocker{}, Orders: []FinanceSourceOrder{}, Reviews: []FinanceReconciliation{}, NoncashQuota: bonus, WalletQuota: owner.Quota, UsedQuota: owner.UsedQuota}
	if common.QuotaPerUnit > 0 && !math.IsNaN(common.QuotaPerUnit) && !math.IsInf(common.QuotaPerUnit, 0) && operation_setting.USDExchangeRate > 0 && !math.IsNaN(operation_setting.USDExchangeRate) && !math.IsInf(operation_setting.USDExchangeRate, 0) {
		report.QuotaPerUnit = decimal.NewFromFloat(common.QuotaPerUnit).String()
		report.USDExchangeRate = decimal.NewFromFloat(operation_setting.USDExchangeRate).String()
	}
	var liabilities []RefundLiability
	if err := tx.Where("user_id = ?", refund.UserId).Order("id ASC").Find(&liabilities).Error; err != nil {
		return nil, err
	}
	if len(liabilities) > 0 {
		report.Blockers = append(report.Blockers, FinanceBlocker{Code: "liability_pending"})
	}
	for _, order := range orders {
		if order.QuotaGranted < 0 || order.QuotaGranted > math.MaxInt64-report.PaidQuota {
			return nil, ErrFinanceEvidence
		}
		report.PaidQuota += order.QuotaGranted
		report.Orders = append(report.Orders, FinanceSourceOrder{ID: order.Id, Source: order.Source, CheckoutVersion: order.CheckoutVersion, Money: order.Money, PaidCents: order.PaymentTotalCents, TaxCents: order.InvoiceTaxCents, Quota: order.QuotaGranted, GiftQuota: order.RefundGiftQuota, DiscountRate: order.DiscountRate})
		if order.CheckoutVersion != 1 {
			report.Blockers = append(report.Blockers, FinanceBlocker{Code: "legacy_payment_snapshot_missing", OrderID: order.Id})
		}
		if !IsCNYInvoiceTopUp(&order) {
			report.Blockers = append(report.Blockers, FinanceBlocker{Code: "unsupported_currency", OrderID: order.Id})
		}
	}
	var refunds []TicketRefund
	if err := tx.Where("user_id = ?", refund.UserId).Order("id ASC").Find(&refunds).Error; err != nil {
		return nil, err
	}
	known := decimal.NewFromInt(report.PaidQuota).Add(decimal.NewFromInt(bonus)).Add(decimal.NewFromInt(max(0, int64(owner.AffHistoryQuota)-int64(owner.AffQuota))))
	for _, row := range refunds {
		if row.RefundStatus == RefundStatusRefunded {
			known = known.Sub(decimal.NewFromInt(int64(row.RefundQuota))).Sub(decimal.NewFromInt(row.RefundGiftQuota))
			if row.FinanceVersion == 0 {
				report.Blockers = append(report.Blockers, FinanceBlocker{Code: "legacy_refund_unallocated"})
			}
		}
		if row.RefundStatus == RefundStatusPending {
			known = known.Sub(decimal.NewFromInt(int64(row.FrozenQuota)))
		}
	}
	var charges []InvoiceTaxCharge
	if err := tx.Where("user_id = ? AND status = ?", refund.UserId, "paid").Order("id ASC").Find(&charges).Error; err != nil {
		return nil, err
	}
	accounted := decimal.NewFromInt(int64(owner.Quota)).Add(decimal.NewFromInt(int64(owner.UsedQuota)))
	for _, charge := range charges {
		if charge.PaymentMethod == "balance" {
			accounted = accounted.Add(decimal.NewFromInt(int64(charge.Quota)))
		}
	}
	report.Difference = accounted.Sub(known).String()
	report.AccountedQuota, report.KnownQuota = accounted.String(), known.String()
	if !known.Equal(accounted) {
		report.Blockers = append(report.Blockers, FinanceBlocker{Code: "ledger_mismatch"})
	}
	if err := tx.Where("user_id = ?", refund.UserId).Order("id DESC").Find(&report.Reviews).Error; err != nil {
		return nil, err
	}
	for i := range report.Reviews {
		if err := common.UnmarshalJsonStr(report.Reviews[i].PlanJSON, &report.Reviews[i].Plan); err != nil {
			return nil, err
		}
		if report.Reviews[i].ApprovalsJSON != "" {
			if err := common.UnmarshalJsonStr(report.Reviews[i].ApprovalsJSON, &report.Reviews[i].Approvals); err != nil {
				return nil, err
			}
		}
	}
	config, err := GetInvoiceTaxConfig(tx)
	if err != nil {
		return nil, err
	}
	report.ReviewEnabled, report.ReviewCount = config.ReconciliationReviewEnabled, config.ReconciliationReviewCount
	// Exclude review proposals from the revision: submitting a proposal must not
	// invalidate its own source snapshot. Applied evidence changes effective orders.
	encoded, err := common.Marshal(struct {
		Owner           User
		Orders          []FinanceSourceOrder
		Bonus           int64
		Refunds         []TicketRefund
		Charges         []InvoiceTaxCharge
		Liabilities     []RefundLiability
		QuotaPerUnit    string
		USDExchangeRate string
	}{owner, report.Orders, bonus, refunds, charges, liabilities, report.QuotaPerUnit, report.USDExchangeRate})
	if err != nil {
		return nil, err
	}
	report.Revision = fmt.Sprintf("%x", sha256.Sum256(encoded))
	if len(report.Blockers) > 0 {
		report.Stage = "needs_reconciliation"
	}
	for _, review := range report.Reviews {
		if review.Status == "submitted" && review.SourceRevision == report.Revision {
			report.Stage = "awaiting_review"
			break
		}
	}
	if len(liabilities) > 0 {
		report.Stage = "waiting_settlement"
	}
	if refund.SettlementVerified {
		report.Stage = "reserved"
	}
	if refund.RefundStatus != RefundStatusPending {
		report.Stage = "closed"
	}
	return report, nil
}

func GetRefundReconciliationReport(ticketID int) (*RefundReconciliationReport, error) {
	if err := SynchronizeRefundHold(ticketID); err != nil {
		return nil, err
	}
	var known TicketRefund
	if err := DB.Select("user_id").Where("ticket_id = ?", ticketID).First(&known).Error; err != nil {
		return nil, err
	}
	var result *RefundReconciliationReport
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).Select("id").First(&owner, known.UserId).Error; err != nil {
			return err
		}
		var refund TicketRefund
		if err := tx.Where("ticket_id = ?", ticketID).First(&refund).Error; err != nil {
			return err
		}
		var err error
		result, err = refundReconciliationReportTx(tx, refund)
		return err
	})
	return result, err
}

func SubmitFinanceReconciliation(ticketID, actorID int, key, revision string, plan FinanceReconciliationPlan) (*FinanceReconciliation, error) {
	plan.Evidence = strings.TrimSpace(plan.Evidence)
	if len(key) < 16 || len(key) > 64 || len(revision) != 64 || len(plan.Payments) > 100 || (len(plan.Payments) == 0 && plan.NoncashGrantQuota == 0) {
		return nil, ErrFinanceEvidence
	}
	encoded, err := common.Marshal(plan)
	if err != nil {
		return nil, err
	}
	var known TicketRefund
	if err := DB.Where("ticket_id = ?", ticketID).First(&known).Error; err != nil {
		return nil, err
	}
	var result FinanceReconciliation
	err = DB.Transaction(func(tx *gorm.DB) error {
		// Acquire the wallet lock before any snapshot reads, including on MySQL
		// REPEATABLE READ, so concurrent submissions see the previous repair.
		var owner User
		if err := lockForUpdate(tx).First(&owner, known.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := tx.Where("id = ? AND status = ? AND role IN ?", actorID, common.UserStatusEnabled, []int{common.RoleFinanceUser, common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
			return ErrTicketForbidden
		}
		var refund TicketRefund
		if err := lockForUpdate(tx).Where("ticket_id = ?", ticketID).First(&refund).Error; err != nil {
			return err
		}
		config, err := GetInvoiceTaxConfig(tx)
		if err != nil {
			return err
		}
		if !config.ReconciliationReviewEnabled && actor.Role != common.RoleAdminUser && actor.Role != common.RoleRootUser {
			return ErrTicketForbidden
		}
		if err := tx.Where("request_key = ?", key).First(&result).Error; err == nil {
			if result.TicketId != ticketID || result.ProposedBy != actorID || result.SourceRevision != revision || result.PlanJSON != string(encoded) {
				return ErrFinanceRevision
			}
			if result.Status != "submitted" || config.ReconciliationReviewEnabled {
				return nil
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if refund.RefundStatus != RefundStatusPending || !refund.hasProvisionalRefundHold() {
			return ErrTicketRefundNotPending
		}
		report, err := refundReconciliationReportTx(tx, refund)
		if err != nil {
			return err
		}
		if report.Revision != revision {
			return ErrFinanceRevision
		}
		for _, review := range report.Reviews {
			if review.Status != "submitted" || review.SourceRevision != revision {
				continue
			}
			if review.ProposedBy == actorID && review.PlanJSON == string(encoded) {
				result = review
				if config.ReconciliationReviewEnabled {
					return nil
				}
				break
			}
			if config.ReconciliationReviewEnabled {
				return ErrFinanceReviewPending
			}
		}
		difference, err := decimal.NewFromString(report.Difference)
		if err != nil || (plan.NoncashGrantQuota > 0 && decimal.NewFromInt(plan.NoncashGrantQuota).GreaterThan(difference)) {
			return ErrFinanceEvidence
		}
		orders, _, err := loadRefundPaymentSources(tx, refund.UserId)
		if err != nil {
			return err
		}
		if _, err = applyFinancePaymentEvidence(orders, plan); err != nil {
			return err
		}
		if result.Id == 0 {
			result = FinanceReconciliation{UserId: refund.UserId, TicketId: ticketID, RequestKey: key, SourceRevision: revision, PlanJSON: string(encoded), Status: "submitted", ProposedBy: actorID, CreatedTime: common.GetTimestamp()}
		}
		if !config.ReconciliationReviewEnabled {
			result.Status, result.ReviewedBy, result.ReviewedTime = "applied", actorID, common.GetTimestamp()
			result.ReviewReason = "Applied on administrator submission; review disabled"
		}
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		if err := recordTicketOperationTx(tx, ticketID, actorID, fmt.Sprintf("Financial reconciliation #%d: %s; plan: %s", result.Id, result.Status, result.PlanJSON)); err != nil {
			return err
		}
		if result.Status == "applied" {
			return tx.Model(&FinanceReconciliation{}).Where("user_id = ? AND id <> ? AND source_revision = ? AND status = ?", result.UserId, result.Id, result.SourceRevision, "submitted").Update("status", "superseded").Error
		}
		// The reconciliation record is the internal audit trail, not a user reply.
		return nil
	})
	result.Plan = plan
	return &result, err
}

func ReviewFinanceReconciliation(id, actorID int, approve bool, reason string) (*FinanceReconciliation, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > 4000 {
		return nil, ErrFinanceEvidence
	}
	var known FinanceReconciliation
	if err := DB.First(&known, id).Error; err != nil {
		return nil, err
	}
	var result FinanceReconciliation
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, known.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := tx.Where("id = ? AND status = ? AND role IN ?", actorID, common.UserStatusEnabled, []int{common.RoleAdminUser, common.RoleRootUser}).First(&actor).Error; err != nil {
			return ErrTicketForbidden
		}
		if err := lockForUpdate(tx).First(&result, id).Error; err != nil {
			return err
		}
		config, err := GetInvoiceTaxConfig(tx)
		if err != nil {
			return err
		}
		if config.ReconciliationReviewEnabled && actorID == result.ProposedBy {
			return ErrFinanceReviewer
		}
		if result.ApprovalsJSON != "" {
			if err := common.UnmarshalJsonStr(result.ApprovalsJSON, &result.Approvals); err != nil {
				return err
			}
		}
		if err := common.UnmarshalJsonStr(result.PlanJSON, &result.Plan); err != nil {
			return err
		}
		target := "rejected"
		if approve {
			target = "applied"
		}
		if result.Status == target && result.ReviewedBy == actorID && result.ReviewReason == reason {
			return nil
		}
		alreadyApproved := false
		for _, approval := range result.Approvals {
			if approval.AdminID == actorID {
				alreadyApproved = true
				if approve && result.Status == "submitted" && (!config.ReconciliationReviewEnabled || len(result.Approvals) >= config.ReconciliationReviewCount) {
					break
				}
				if approve && approval.Reason == reason && (result.Status == "submitted" || result.Status == "applied") {
					return nil
				}
				return ErrFinanceRevision
			}
		}
		if result.Status != "submitted" {
			return ErrFinanceRevision
		}
		if err := common.UnmarshalJsonStr(result.PlanJSON, &result.Plan); err != nil {
			return err
		}
		var refund TicketRefund
		if err := tx.Where("ticket_id = ?", result.TicketId).First(&refund).Error; err != nil {
			return err
		}
		if approve {
			if refund.RefundStatus != RefundStatusPending || !refund.hasProvisionalRefundHold() {
				return ErrTicketRefundNotPending
			}
			report, err := refundReconciliationReportTx(tx, refund)
			if err != nil {
				return err
			}
			if report.Revision != result.SourceRevision {
				return ErrFinanceRevision
			}
			difference, err := decimal.NewFromString(report.Difference)
			if err != nil || (result.Plan.NoncashGrantQuota > 0 && decimal.NewFromInt(result.Plan.NoncashGrantQuota).GreaterThan(difference)) {
				return ErrFinanceEvidence
			}
			orders, _, err := loadRefundPaymentSources(tx, known.UserId)
			if err != nil {
				return err
			}
			if _, err = applyFinancePaymentEvidence(orders, result.Plan); err != nil {
				return err
			}
		}
		if approve {
			if !alreadyApproved {
				result.Approvals = append(result.Approvals, FinanceReconciliationApproval{AdminID: actorID, Reason: reason, CreatedTime: common.GetTimestamp()})
			}
			encoded, err := common.Marshal(result.Approvals)
			if err != nil {
				return err
			}
			result.ApprovalsJSON = string(encoded)
			if config.ReconciliationReviewEnabled && len(result.Approvals) < config.ReconciliationReviewCount {
				target = "submitted"
			}
		}
		result.Status, result.ReviewedBy, result.ReviewReason, result.ReviewedTime = target, actorID, reason, common.GetTimestamp()
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		if err := recordTicketOperationTx(tx, result.TicketId, actorID, fmt.Sprintf("Financial reconciliation #%d: %s; reason: %s", result.Id, target, reason)); err != nil {
			return err
		}
		if target == "applied" {
			// Older deployments allowed multiple proposals for the same source.
			return tx.Model(&FinanceReconciliation{}).Where("user_id = ? AND id <> ? AND source_revision = ? AND status = ?", result.UserId, result.Id, result.SourceRevision, "submitted").Update("status", "superseded").Error
		}
		return nil
	})
	return &result, err
}
