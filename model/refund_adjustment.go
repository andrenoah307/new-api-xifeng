package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrRefundAdjustment = errors.New("Refund adjustment is unavailable for this ticket")
var ErrRefundAdjustmentConfirmation = errors.New("Please confirm that you understand the consequences")

// Immutable receipt: a ticket can be adjusted once, including after retries or cancellation.
type RefundAdjustment struct {
	ID             int    `json:"id"`
	TicketID       int    `json:"ticket_id" gorm:"uniqueIndex;not null"`
	UserID         int    `json:"user_id" gorm:"index;not null"`
	ActorID        int    `json:"actor_id"`
	SourceRevision string `json:"source_revision" gorm:"size:64;not null"`
	DebitQuota     int64  `json:"debit_quota,string" gorm:"type:bigint"`
	Reason         string `json:"reason" gorm:"type:text"`
	BeforeJSON     string `json:"-" gorm:"type:text"`
	ResultJSON     string `json:"-" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint"`
}

type RefundAdjustmentLiability struct {
	ID          string `json:"id"`
	RequestID   string `json:"request_id"`
	NodeName    string `json:"node_name"`
	CreatedAt   int64  `json:"created_at"`
	RequestDone bool   `json:"request_done"`
	TaskDone    bool   `json:"task_done"`
}

type RefundAdjustmentPlan struct {
	DifferenceAmount   string                      `json:"difference_amount"`
	CalculationMethod  string                      `json:"calculation_method,omitempty"`
	CalculationBasis   string                      `json:"calculation_basis,omitempty"`
	ApplicationBalance *RefundApplicationBalance   `json:"application_balance,omitempty"`
	TicketID           int                         `json:"ticket_id"`
	Revision           string                      `json:"revision"`
	Requested          string                      `json:"requested"`
	BalanceBefore      string                      `json:"balance_before"`
	DebitAmount        string                      `json:"debit_amount"`
	AccountedAmount    string                      `json:"accounted_amount"`
	KnownAmount        string                      `json:"known_amount"`
	BalanceAfter       string                      `json:"balance_after"`
	GraceAmount        string                      `json:"grace_amount"`
	CanApply           bool                        `json:"can_apply"`
	Blocker            string                      `json:"blocker,omitempty"`
	Liabilities        []RefundAdjustmentLiability `json:"liabilities"`
	Calculation        *RefundExportData           `json:"calculation,omitempty"`
	Diagnostic         *RefundCalculationError     `json:"diagnostic,omitempty"`
	Applied            bool                        `json:"applied"`
	ActorID            int                         `json:"actor_id,omitempty"`
	AppliedAt          int64                       `json:"applied_at,omitempty"`
	Reason             string                      `json:"reason,omitempty"`
	debitQuota         int64
	afterQuota         int
	beforeJSON         string
}

type RefundAdjustmentInput struct {
	CalculationMethod         string `json:"calculation_method"`
	CalculationBasis          string `json:"calculation_basis"`
	ConfirmApplicationBalance bool   `json:"confirm_application_balance"`
	ConfirmNegative           bool   `json:"confirm_negative"`
	Revision                  string `json:"revision"`
	Reason                    string `json:"reason"`
	ConfirmRecovery           bool   `json:"confirm_recovery"`
	ConfirmSettlement         bool   `json:"confirm_settlement"`
}

func buildRefundAdjustmentTx(tx *gorm.DB, refund *TicketRefund, owner *User, basis string) (*RefundAdjustmentPlan, error) {
	if refund.RefundStatus != RefundStatusPending || refund.FinanceVersion != 2 || refund.RefundQuota <= 0 || refund.FrozenQuota != refund.RefundQuota || refund.RefundGiftQuota != 0 || owner.UsedQuota < 0 {
		return nil, ErrRefundAdjustment
	}
	var otherPending int64
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND ticket_id != ? AND refund_status = ?", owner.Id, refund.TicketId, RefundStatusPending).Count(&otherPending).Error; err != nil {
		return nil, err
	}
	if otherPending != 0 {
		return nil, ErrRefundFinancePending
	}
	config, err := GetInvoiceTaxConfig(tx)
	if err != nil {
		return nil, err
	}
	report, err := refundReconciliationReportTx(tx, *refund)
	if err != nil {
		return nil, err
	}
	if report.QuotaPerUnit == "" {
		return nil, ErrRefundAdjustment
	}
	// Only the unexplained surplus is classified by the operator. Proven gifts,
	// referral credits, fees and previous refunds remain in their existing ledgers.
	debit, err := strconv.ParseInt(report.Difference, 10, 64)
	if basis == RefundBasisApplicationBalance {
		debit = 0
	}
	if err != nil || debit < 0 || int64(owner.Quota) < math.MinInt64+debit {
		return nil, ErrRefundAdjustment
	}
	after := int64(owner.Quota) - debit
	if after < math.MinInt32 || after > math.MaxInt32 {
		return nil, ErrRefundAdjustment
	}
	unit := decimal.NewFromFloat(common.QuotaPerUnit)
	accounted, err := decimal.NewFromString(report.AccountedQuota)
	if err != nil {
		return nil, err
	}
	known, err := decimal.NewFromString(report.KnownQuota)
	if err != nil {
		return nil, err
	}
	plan := &RefundAdjustmentPlan{
		CalculationBasis: basis,
		TicketID:         refund.TicketId, Requested: decimal.NewFromInt(int64(refund.RefundQuota)).Div(unit).String(),
		BalanceBefore: decimal.NewFromInt(int64(owner.Quota)).Div(unit).String(),
		DebitAmount:   decimal.NewFromInt(debit).Div(unit).String(), BalanceAfter: decimal.NewFromInt(after).Div(unit).String(),
		GraceAmount:     decimal.NewFromInt(config.RefundOverdraftLimitCents).Div(decimal.NewFromInt(100)).String(),
		AccountedAmount: accounted.Div(unit).String(), KnownAmount: known.Div(unit).String(),
		DifferenceAmount: accounted.Sub(known).Div(unit).String(),
		Liabilities:      []RefundAdjustmentLiability{}, debitQuota: debit, afterQuota: int(after),
	}
	if basis == RefundBasisApplicationBalance {
		noncash, err := originalRefundNoncashQuotaTx(tx, owner)
		if err != nil {
			return nil, err
		}
		plan.ApplicationBalance = &RefundApplicationBalance{
			WalletAmount:  plan.BalanceBefore,
			NoncashAmount: decimal.NewFromInt(noncash).Div(unit).String(),
			CashAmount:    decimal.NewFromInt(int64(owner.Quota)).Sub(decimal.NewFromInt(noncash)).Div(unit).String(),
		}
	}
	var liabilities []RefundLiability
	if err := tx.Where("user_id = ?", owner.Id).Order("id ASC").Limit(101).Find(&liabilities).Error; err != nil {
		return nil, err
	}
	if len(liabilities) > 100 {
		return nil, ErrRefundAdjustment
	}
	for _, row := range liabilities {
		plan.Liabilities = append(plan.Liabilities, RefundAdjustmentLiability{row.ID, row.RequestID, row.NodeName, row.CreatedAt, row.RequestDone, row.TaskDone})
		if !row.TaskDone {
			plan.Blocker = "unsettled_tasks"
		}
	}
	calculationRefund := *refund
	calculationRefund.CalculationBasis = basis
	plan.Calculation, err = buildRefundInvoiceExportTx(tx, &calculationRefund, 0, false, &plan.afterQuota)
	if err != nil {
		var diagnostic *RefundCalculationError
		if errors.As(err, &diagnostic) {
			plan.Diagnostic = diagnostic
			plan.Blocker = "calculation_failed"
		} else {
			return nil, err
		}
	}
	if plan.Calculation != nil && plan.Calculation.RefundGiftQuota != 0 {
		return nil, ErrRefundAdjustment
	}
	if plan.Calculation != nil && plan.Calculation.ApplicationBalance != nil {
		plan.ApplicationBalance = plan.Calculation.ApplicationBalance
	}
	if debit == 0 && len(liabilities) == 0 && basis != RefundBasisApplicationBalance {
		plan.Blocker = "no_adjustment"
	}
	plan.CanApply = plan.Blocker == ""
	// Bind the preview to ledger evidence, settings, invoices and the exact
	// liability flags. No raw credentials or password fields enter the receipt.
	before, err := common.Marshal(struct {
		Report *RefundReconciliationReport
		Refund *TicketRefund
		Remark string
		Config InvoiceTaxConfig
		Plan   *RefundAdjustmentPlan
	}{report, refund, owner.Remark, config, plan})
	if err != nil {
		return nil, err
	}
	if len(before) > 60000 {
		return nil, ErrRefundAdjustment
	}
	plan.beforeJSON = string(before)
	plan.Revision = fmt.Sprintf("%x", sha256.Sum256(before))
	return plan, nil
}

// A nil input only previews. The apply path commits the debit, manual settlement
// acknowledgement, saved calculation and immutable receipt in one transaction.
func ManageRefundAdjustment(ticketID, actorID int, input *RefundAdjustmentInput, previewBasis ...string) (*RefundAdjustmentPlan, error) {
	basis := ""
	if len(previewBasis) > 0 {
		basis = previewBasis[0]
	}
	if input != nil {
		copyInput := *input
		input = &copyInput
		if input.CalculationMethod != "" {
			switch input.CalculationMethod {
			case RefundBasisApplicationBalance:
				input.CalculationBasis = RefundBasisApplicationBalance
				input.ConfirmApplicationBalance = true
			case "discount_recovery":
				input.CalculationBasis = ""
				input.ConfirmRecovery = true
			default:
				return nil, ErrRefundAdjustment
			}
			input.ConfirmSettlement = true
			input.Reason = "Administrator selected refund calculation method: " + input.CalculationMethod + ". Residual request settlement risk accepted; no manual settlement verification claimed."
		}
		basis = input.CalculationBasis
	}
	if basis != "" && basis != RefundBasisApplicationBalance {
		return nil, ErrRefundAdjustment
	}
	if input != nil {
		input.Reason = strings.TrimSpace(input.Reason)
		if len(input.Revision) != 64 || utf8.RuneCountInString(input.Reason) < 5 || utf8.RuneCountInString(input.Reason) > 500 || strings.ContainsFunc(input.Reason, unicode.IsControl) {
			return nil, ErrRefundAdjustmentConfirmation
		}
	}
	var identity TicketRefund
	if err := DB.Select("user_id").Where("ticket_id = ?", ticketID).First(&identity).Error; err != nil {
		return nil, err
	}
	var result *RefundAdjustmentPlan
	var committed bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, identity.UserId).Error; err != nil {
			return err
		}
		var actor User
		if err := lockForUpdate(tx).Select("id,username,role,status").First(&actor, actorID).Error; err != nil || actor.Status != common.UserStatusEnabled || (actor.Role != common.RoleFinanceUser && actor.Role != common.RoleAdminUser && actor.Role != common.RoleRootUser) {
			return ErrTicketForbidden
		}
		var ticket Ticket
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", ticketID, owner.Id).First(&ticket).Error; err != nil {
			return err
		}
		var refund TicketRefund
		if err := lockForUpdate(tx).Where("ticket_id = ? AND user_id = ?", ticketID, owner.Id).First(&refund).Error; err != nil {
			return err
		}
		var setting Option
		if err := lockForUpdate(tx).Where(Option{Key: InvoiceTaxConfigKey}).Take(&setting).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var prior RefundAdjustment
		if err := tx.Where("ticket_id = ?", ticketID).First(&prior).Error; err == nil {
			if input != nil && prior.SourceRevision != input.Revision {
				return ErrFinanceRevision
			}
			return common.UnmarshalJsonStr(prior.ResultJSON, &result)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if ticket.Type != TicketTypeRefund || ticket.Status == TicketStatusClosed || ticket.Status == TicketStatusResolved {
			return ErrRefundAdjustment
		}
		var err error
		if refund.CalculationBasis == RefundBasisApplicationBalance {
			if input != nil && input.CalculationMethod == "discount_recovery" {
				return ErrFinanceRevision
			}
			basis = RefundBasisApplicationBalance
		}
		result, err = buildRefundAdjustmentTx(tx, &refund, &owner, basis)
		if err != nil || input == nil {
			return err
		}
		if result.Revision != input.Revision {
			return ErrFinanceRevision
		}
		if !result.CanApply {
			return ErrRefundAdjustment
		}
		if input.CalculationMethod != "" && result.afterQuota < 0 && !input.ConfirmNegative {
			return ErrRefundAdjustmentConfirmation
		}
		if (result.debitQuota > 0 && !input.ConfirmRecovery) || (len(result.Liabilities) > 0 && !input.ConfirmSettlement) {
			return ErrRefundAdjustmentConfirmation
		}
		if basis == RefundBasisApplicationBalance {
			if !input.ConfirmApplicationBalance || result.ApplicationBalance == nil {
				return ErrRefundAdjustmentConfirmation
			}
			cash, err := decimal.NewFromString(result.ApplicationBalance.CashAmount)
			if err != nil || (cash.IsNegative() && !input.ConfirmNegative) {
				return ErrRefundAdjustmentConfirmation
			}
			if refund.CalculationBasis != basis {
				updated := tx.Model(&TicketRefund{}).Where("ticket_id = ? AND user_id = ? AND refund_status = ?", ticketID, owner.Id, RefundStatusPending).Update("calculation_basis", basis)
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != 1 {
					return ErrFinanceRevision
				}
				refund.CalculationBasis = basis
			}
		}
		if result.debitQuota > 0 {
			note := fmt.Sprintf("因退款工单计算扣回历史优惠赠送额度（工单#%d，%s元额度）", ticketID, result.DebitAmount)
			remark := note
			if owner.Remark != "" {
				remark = owner.Remark + "；" + note
			}
			if utf8.RuneCountInString(remark) > 255 {
				// The complete note remains in the ticket and immutable receipt.
				remark = owner.Remark
			}
			updated := tx.Model(&User{}).Where("id = ? AND quota = ? AND used_quota = ?", owner.Id, owner.Quota, owner.UsedQuota).
				Updates(map[string]interface{}{"quota": result.afterQuota, "remark": remark})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return ErrFinanceRevision
			}
		}
		for _, row := range result.Liabilities {
			deleted := tx.Where("id = ? AND user_id = ? AND request_done = ? AND task_done = ?", row.ID, owner.Id, row.RequestDone, true).Delete(&RefundLiability{})
			if deleted.Error != nil {
				return deleted.Error
			}
			if deleted.RowsAffected != 1 {
				return ErrFinanceRevision
			}
		}
		if err := verifyRefundLiabilitiesTx(tx, owner.Id); err != nil {
			return err
		}
		saved, err := saveRefundInvoiceExportTx(tx, &refund, actorID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(saved, result.Calculation) {
			return ErrFinanceRevision
		}
		result.Applied, result.ActorID, result.AppliedAt, result.Reason = true, actorID, common.GetTimestamp(), input.Reason
		result.CalculationMethod = input.CalculationMethod
		result.CanApply = false
		raw, err := common.Marshal(result)
		if err != nil {
			return err
		}
		if len(raw) > 60000 {
			return ErrRefundAdjustment
		}
		receipt := RefundAdjustment{TicketID: ticketID, UserID: owner.Id, ActorID: actorID, SourceRevision: result.Revision, DebitQuota: result.debitQuota, Reason: input.Reason, BeforeJSON: result.beforeJSON, ResultJSON: string(raw), CreatedAt: result.AppliedAt}
		created := tx.Create(&receipt)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected != 1 {
			return ErrFinanceRevision
		}
		message := TicketMessage{TicketId: ticketID, UserId: actorID, Username: actor.Username, Role: actor.Role, CreatedTime: result.AppliedAt,
			Content: fmt.Sprintf("因退款工单计算扣回历史优惠赠送额度：%s元额度；余额由%s变为%s元额度；人工核验结算记录%d条。退款计算已保存，尚未付款。", result.DebitAmount, result.BalanceBefore, result.BalanceAfter, len(result.Liabilities))}
		if result.ApplicationBalance != nil {
			message.Content = fmt.Sprintf("按原申请余额口径核验退款：不追加扣款；总余额%s元，保留非现金额度%s元，现金口径余额%s元；人工核验结算记录%d条。退款计算已保存，尚未付款。", result.BalanceAfter, result.ApplicationBalance.NoncashAmount, result.ApplicationBalance.CashAmount, len(result.Liabilities))
		}
		if input.CalculationMethod != "" {
			message.Content = fmt.Sprintf("管理员选择退款计算方式%s；账面差额%s元，扣回历史优惠赠送额度%s元，余额由%s变为%s元；按所选方式处理结算阻塞记录%d条，未作人工结算核验。退款计算已保存，尚未付款。", input.CalculationMethod, result.DifferenceAmount, result.DebitAmount, result.BalanceBefore, result.BalanceAfter, len(result.Liabilities))
		}
		created = tx.Create(&message)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected != 1 {
			return ErrFinanceRevision
		}
		updated := tx.Model(&Ticket{}).Where("id = ? AND status = ?", ticketID, ticket.Status).Updates(map[string]interface{}{"status": TicketStatusProcessing, "updated_time": result.AppliedAt, "admin_id": actorID})
		if updated.Error != nil {
			return updated.Error
		}
		// MySQL may report zero changed rows when a same-second operation already
		// set these fields; the locked row and status have been verified above.
		if updated.RowsAffected > 1 {
			return ErrFinanceRevision
		}
		committed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if committed {
		if err := InvalidateUserCache(identity.UserId); err != nil {
			common.SysError("refund adjustment cache invalidation failed: " + err.Error())
		}
		RecordLogWithAdminInfo(identity.UserId, LogTypeManage, fmt.Sprintf("Refund #%d manual adjustment: recovered %s credit; wallet %s", ticketID, result.DebitAmount, result.BalanceAfter), map[string]interface{}{"admin_id": actorID, "ticket_id": ticketID, "revision": result.Revision})
	}
	return result, nil
}
