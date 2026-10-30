package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	RefundStatusPending   = 1 // 待审核（冻结中）
	RefundStatusRefunded  = 2 // 已退款
	RefundStatusRejected  = 3 // 已驳回
	RefundStatusCancelled = 4 // 用户主动取消（关闭退款工单）
)

const (
	RefundPayeeTypeAlipay = "alipay"
	RefundPayeeTypeBank   = "bank"
)

// 批准退款时的额度操作模式。默认按 RefundQuotaModeWriteOff（核销冻结）。
const (
	RefundQuotaModeWriteOff = "write_off" // 核销冻结额度（默认）
	RefundQuotaModeSubtract = "subtract"  // 自定义核销金额：先全部解冻，再从 quota 扣除指定金额
	RefundQuotaModeOverride = "override"  // 覆盖最终余额：先全部解冻，再把 quota 设置为指定值
)

var (
	ErrTicketRefundNotFound          = errors.New("ticket refund not found")
	ErrTicketRefundStatusInvalid     = errors.New("ticket refund status invalid")
	ErrTicketRefundQuotaInvalid      = errors.New("ticket refund quota invalid")
	ErrTicketRefundQuotaExceed       = errors.New("ticket refund quota exceed")
	ErrTicketRefundPayeeTypeEmpty    = errors.New("ticket refund payee type empty")
	ErrTicketRefundPayeeNameEmpty    = errors.New("ticket refund payee name empty")
	ErrTicketRefundPayeeAccountEmpty = errors.New("ticket refund payee account empty")
	ErrTicketRefundPayeeBankEmpty    = errors.New("ticket refund payee bank empty")
	ErrTicketRefundContactEmpty      = errors.New("ticket refund contact empty")
	ErrTicketRefundNotPending        = errors.New("ticket refund not pending")
	ErrTicketRefundQuotaModeInvalid  = errors.New("ticket refund quota mode invalid")
	// ErrTicketRefundClawbackQuotaInvalid 勾选扣回返佣但金额非法（≤0）。
	ErrTicketRefundClawbackQuotaInvalid = errors.New("ticket refund clawback quota invalid")
)

type TicketRefund struct {
	CreationReplayed  bool                 `json:"-" gorm:"-"`
	FinanceVersion    int                  `json:"finance_version"`
	RefundCashCents   int64                `json:"refund_cash_cents" gorm:"type:bigint"`
	Finance           *RefundFinanceDetail `json:"finance,omitempty" gorm:"-"`
	FinanceRequestKey *string              `json:"-" gorm:"size:64;uniqueIndex"`
	Id                int                  `json:"id"`
	TicketId          int                  `json:"ticket_id" gorm:"uniqueIndex;not null"`
	UserId            int                  `json:"user_id" gorm:"index;not null"`
	RefundQuota       int                  `json:"refund_quota" gorm:"type:bigint;not null"`
	FrozenQuota       int                  `json:"frozen_quota" gorm:"type:bigint;default:0"`
	UserQuotaSnapshot int                  `json:"user_quota_snapshot" gorm:"type:bigint;default:0"`
	PayeeType         string               `json:"payee_type" gorm:"type:varchar(16);not null"`
	PayeeName         string               `json:"payee_name" gorm:"type:varchar(128);not null"`
	PayeeAccount      string               `json:"payee_account" gorm:"type:varchar(128);not null"`
	PayeeBank         string               `json:"payee_bank" gorm:"type:varchar(255)"`
	Contact           string               `json:"contact" gorm:"type:varchar(128);not null"`
	Reason            string               `json:"reason" gorm:"type:text"`
	RefundStatus      int                  `json:"refund_status" gorm:"type:int;default:1"`
	ProcessedTime     int64                `json:"processed_time" gorm:"bigint;default:0"`
	CreatedTime       int64                `json:"created_time" gorm:"bigint"`
}

type CreateRefundTicketParams struct {
	FinanceQuote string
	RequestKey   string
	UserId       int
	Username     string
	Subject      string
	Priority     int
	RefundQuota  int
	PayeeType    string
	PayeeName    string
	PayeeAccount string
	PayeeBank    string
	Contact      string
	Reason       string
}

func (refund *TicketRefund) BeforeCreate(tx *gorm.DB) error {
	if refund.CreatedTime == 0 {
		refund.CreatedTime = common.GetTimestamp()
	}
	return nil
}

func IsValidRefundStatus(status int) bool {
	switch status {
	case RefundStatusPending, RefundStatusRefunded, RefundStatusRejected:
		return true
	default:
		return false
	}
}

func NormalizeRefundPayeeType(payeeType string) string {
	return strings.ToLower(strings.TrimSpace(payeeType))
}

func IsValidRefundPayeeType(payeeType string) bool {
	switch NormalizeRefundPayeeType(payeeType) {
	case RefundPayeeTypeAlipay, RefundPayeeTypeBank:
		return true
	default:
		return false
	}
}

func refundPayeeTypeText(payeeType string) string {
	switch NormalizeRefundPayeeType(payeeType) {
	case RefundPayeeTypeAlipay:
		return "支付宝"
	case "wechat":
		return "微信"
	case RefundPayeeTypeBank:
		return "银行卡"
	case "other":
		return "其他"
	default:
		return payeeType
	}
}

func buildRefundSummaryMessage(params CreateRefundTicketParams) string {
	lines := []string{
		"退款申请信息：",
		fmt.Sprintf("申请退款额度：%s", logger.LogQuota(params.RefundQuota)),
		fmt.Sprintf("收款方式：%s", refundPayeeTypeText(params.PayeeType)),
		fmt.Sprintf("收款人：%s", strings.TrimSpace(params.PayeeName)),
		fmt.Sprintf("收款账号：%s", strings.TrimSpace(params.PayeeAccount)),
	}
	if bank := strings.TrimSpace(params.PayeeBank); bank != "" {
		lines = append(lines, fmt.Sprintf("开户行：%s", bank))
	}
	lines = append(lines, fmt.Sprintf("联系方式：%s", strings.TrimSpace(params.Contact)))
	if reason := strings.TrimSpace(params.Reason); reason != "" {
		lines = append(lines, "退款原因：")
		lines = append(lines, reason)
	}
	return strings.Join(lines, "\n")
}

// CreateRefundTicket atomically reserves the wallet balance and creates the ticket.
func CreateRefundTicket(params CreateRefundTicketParams) (*Ticket, *TicketRefund, *TicketMessage, error) {
	payeeType := NormalizeRefundPayeeType(params.PayeeType)
	if payeeType == "" {
		return nil, nil, nil, ErrTicketRefundPayeeTypeEmpty
	}
	if !IsValidRefundPayeeType(payeeType) {
		return nil, nil, nil, ErrTicketRefundPayeeTypeEmpty
	}
	if params.RefundQuota <= 0 {
		return nil, nil, nil, ErrTicketRefundQuotaInvalid
	}
	if strings.TrimSpace(params.PayeeName) == "" {
		return nil, nil, nil, ErrTicketRefundPayeeNameEmpty
	}
	if strings.TrimSpace(params.PayeeAccount) == "" {
		return nil, nil, nil, ErrTicketRefundPayeeAccountEmpty
	}
	if payeeType == RefundPayeeTypeBank && strings.TrimSpace(params.PayeeBank) == "" {
		return nil, nil, nil, ErrTicketRefundPayeeBankEmpty
	}
	if strings.TrimSpace(params.Contact) == "" {
		return nil, nil, nil, ErrTicketRefundContactEmpty
	}
	if params.RequestKey != "" {
		if _, err := uuid.Parse(params.RequestKey); err != nil || len(params.RequestKey) != 36 {
			return nil, nil, nil, ErrTicketRefundQuotaInvalid
		}
	}
	subject := strings.TrimSpace(params.Subject)
	if subject == "" {
		subject = fmt.Sprintf("退款申请（%s）", logger.LogQuota(params.RefundQuota))
	}
	replayed := false

	var (
		ticket  *Ticket
		refund  *TicketRefund
		message *TicketMessage
	)
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id, quota").First(&user, params.UserId).Error; err != nil {
			return err
		}
		if params.RequestKey != "" {
			var prior TicketRefund
			err := tx.Where("finance_request_key = ?", fmt.Sprintf("%d:%s", params.UserId, params.RequestKey)).First(&prior).Error
			if err == nil {
				if prior.UserId != params.UserId || prior.RefundQuota != params.RefundQuota || prior.PayeeType != payeeType || prior.PayeeName != strings.TrimSpace(params.PayeeName) || prior.PayeeAccount != strings.TrimSpace(params.PayeeAccount) || prior.PayeeBank != strings.TrimSpace(params.PayeeBank) || prior.Contact != strings.TrimSpace(params.Contact) || prior.Reason != strings.TrimSpace(params.Reason) {
					return ErrTicketRefundQuotaInvalid
				}
				ticket = &Ticket{}
				if err := tx.First(ticket, prior.TicketId).Error; err != nil {
					return err
				}
				if ticket.Subject != subject {
					return ErrTicketRefundQuotaInvalid
				}
				message = &TicketMessage{}
				if err := tx.Where("ticket_id = ?", prior.TicketId).Order("id ASC").First(message).Error; err != nil {
					return err
				}
				refund = &prior
				if err := AttachRefundFinance(tx, refund); err != nil {
					return err
				}
				replayed = true
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		maxRefundable, err := getUserMaxRefundableQuotaTx(tx, params.UserId)
		if err != nil {
			return err
		}
		if params.RefundQuota > maxRefundable {
			return ErrTicketRefundQuotaExceed
		}
		userQuota := user.Quota
		now := common.GetTimestamp()
		ticket = &Ticket{
			UserId:      params.UserId,
			Username:    strings.TrimSpace(params.Username),
			Subject:     subject,
			Type:        TicketTypeRefund,
			Status:      TicketStatusOpen,
			Priority:    NormalizeTicketPriority(params.Priority),
			CreatedTime: now,
			UpdatedTime: now,
		}
		if err := tx.Create(ticket).Error; err != nil {
			return err
		}

		refund = &TicketRefund{
			TicketId:          ticket.Id,
			UserId:            params.UserId,
			RefundQuota:       params.RefundQuota,
			FrozenQuota:       params.RefundQuota,
			UserQuotaSnapshot: userQuota,
			PayeeType:         payeeType,
			PayeeName:         strings.TrimSpace(params.PayeeName),
			PayeeAccount:      strings.TrimSpace(params.PayeeAccount),
			PayeeBank:         strings.TrimSpace(params.PayeeBank),
			Contact:           strings.TrimSpace(params.Contact),
			Reason:            strings.TrimSpace(params.Reason),
			RefundStatus:      RefundStatusPending,
			CreatedTime:       now,
		}
		if params.RequestKey != "" {
			key := fmt.Sprintf("%d:%s", params.UserId, params.RequestKey)
			refund.FinanceRequestKey = &key
		}
		if err := prepareRefundFinanceTx(tx, refund, params); err != nil {
			return err
		}
		result := tx.Model(&User{}).Where("id = ? AND quota >= ?", params.UserId, params.RefundQuota).
			Update("quota", gorm.Expr("quota - ?", params.RefundQuota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTicketRefundQuotaExceed
		}
		if err := tx.Create(refund).Error; err != nil {
			return err
		}

		message = &TicketMessage{
			TicketId:    ticket.Id,
			UserId:      params.UserId,
			Username:    strings.TrimSpace(params.Username),
			Role:        common.RoleCommonUser,
			Content:     buildRefundSummaryMessage(params),
			CreatedTime: now,
		}
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return AttachRefundFinance(tx, refund)
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if replayed {
		refund.CreationReplayed = true
		return ticket, refund, message, nil
	}
	_ = InvalidateUserCache(params.UserId)
	RecordLog(params.UserId, LogTypeSystem,
		fmt.Sprintf("提交退款申请，扣减余额 %s（工单 #%d，冻结待审核）",
			logger.LogQuota(params.RefundQuota), ticket.Id))
	return ticket, refund, message, nil
}

// SumUserPendingRefundQuota 汇总用户当前处于 Pending 状态的退款工单累计申请额度。
//
// 注意：这笔钱在退款工单提交事务内已从 user.quota 中**扣除**，
// 并不是账户层面意义上的"冻结"。此处数字仅供管理员审计参考（"这位用户还有 X 额度在等审核"），
// 切勿把它再加回到可用余额上。对应字段 TicketRefund.RefundQuota（与 FrozenQuota 同值，取任一即可）。
func SumUserPendingRefundQuota(userId int) (int64, error) {
	var total int64
	err := DB.Model(&TicketRefund{}).
		Where("user_id = ? AND refund_status = ?", userId, RefundStatusPending).
		Select("COALESCE(SUM(refund_quota), 0)").
		Scan(&total).Error
	return total, err
}

func GetUserTotalRefundedQuota(userId int) (int64, error) {
	var total int64
	err := DB.Model(&TicketRefund{}).
		Where("user_id = ? AND refund_status = ?", userId, RefundStatusRefunded).
		Select("COALESCE(SUM(refund_quota), 0)").
		Scan(&total).Error
	return total, err
}

func GetUserMaxRefundableQuota(userId int) (int, error) {
	return getUserMaxRefundableQuotaTx(DB, userId)
}

func getUserMaxRefundableQuotaTx(tx *gorm.DB, userId int) (int, error) {
	var totalTopUp, bonusQuota, totalRefunded, pendingRefund int64
	if err := tx.Model(&TopUp{}).Where("user_id = ? AND status = ? AND (source IS NULL OR source = '' OR source != ?)", userId, common.TopUpStatusSuccess, "discount_bonus").
		Select("COALESCE(SUM(quota_granted), 0)").Scan(&totalTopUp).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&TopUp{}).Where("user_id = ? AND status = ? AND source = ?", userId, common.TopUpStatusSuccess, "discount_bonus").
		Select("COALESCE(SUM(quota_granted), 0)").Scan(&bonusQuota).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", userId, RefundStatusRefunded).
		Select("COALESCE(SUM(refund_quota), 0)").Scan(&totalRefunded).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", userId, RefundStatusPending).
		Select("COALESCE(SUM(refund_quota), 0)").Scan(&pendingRefund).Error; err != nil {
		return 0, err
	}
	var user User
	if err := tx.Select("quota, aff_history, aff_quota").First(&user, userId).Error; err != nil {
		return 0, err
	}
	if totalTopUp < 0 || bonusQuota < 0 || totalRefunded < 0 || pendingRefund < 0 {
		return 0, ErrTicketRefundQuotaInvalid
	}
	cap := max(int64(0), totalTopUp-totalRefunded)
	cap = max(int64(0), cap-pendingRefund)
	// Subtract only nonnegative bounded quantities, without overflowing intermediate sums.
	affTransferred := max(int64(0), int64(max(0, user.AffHistoryQuota))-int64(max(0, user.AffQuota)))
	available := max(int64(0), int64(user.Quota))
	available = max(int64(0), available-affTransferred)
	available = max(int64(0), available-bonusQuota)
	return int(min(available, cap)), nil
}

func GetTicketRefundByTicketId(ticketId int) (*TicketRefund, error) {
	var refund TicketRefund
	if err := DB.Where("ticket_id = ?", ticketId).First(&refund).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTicketRefundNotFound
		}
		return nil, err
	}
	if err := AttachRefundFinance(DB, &refund); err != nil {
		return nil, err
	}
	return &refund, nil
}

// UpdateRefundStatusParams 描述管理员处理退款时的参数。
// ActualRefundQuota 的语义随 QuotaMode 变化（仅在 RefundStatus == RefundStatusRefunded 时生效）：
//   - QuotaMode == "" 或 RefundQuotaModeWriteOff：核销已扣余额，ActualRefundQuota 被忽略；
//   - RefundQuotaModeSubtract：实际核销 ActualRefundQuota（可大于或小于原申请额度）；
//     若 Y > RefundQuota 则额外扣减 (Y-RefundQuota)；若 Y < RefundQuota 则退还 (RefundQuota-Y)；
//     ActualRefundQuota 必须 > 0；
//   - RefundQuotaModeOverride：先退还原已扣 RefundQuota，再把用户余额强制设置为 ActualRefundQuota；
//     ActualRefundQuota 必须 ≥ 0。
//
// 驳回（RefundStatusRejected）：忽略 ActualRefundQuota / QuotaMode，永远退还 RefundQuota 到用户余额。
type UpdateRefundStatusParams struct {
	TicketId          int
	AdminId           int
	RefundStatus      int
	QuotaMode         string
	ActualRefundQuota int
	// ClawBackCommission 批准退款时是否同步扣回邀请人返佣（与退款结算同事务）。
	// 仅 RefundStatus == RefundStatusRefunded 时生效；驳回时忽略。
	ClawBackCommission bool
	ClawBackQuota      int
}

// UpdateRefundStatus 管理员处理退款。
// 只有处于 pending 的退款才能被处理，防止二次扣减/退还。
//
// Wallet mutations, status transitions and commission clawback share one transaction.
// Cache invalidation and audit logging happen only after commit.
// 第三个返回值是更新前的工单主状态，供调用方判断是否需要发出状态变更通知；
// 第四个返回值是实际执行的返佣扣回结果（未勾选或无可扣时 ClawedQuota 为 0）。
func UpdateRefundStatus(params UpdateRefundStatusParams) (*TicketRefund, *Ticket, int, *CommissionClawbackResult, error) {
	if params.RefundStatus != RefundStatusRefunded && params.RefundStatus != RefundStatusRejected {
		return nil, nil, 0, nil, ErrTicketRefundStatusInvalid
	}
	mode := strings.TrimSpace(params.QuotaMode)
	if mode == "" {
		mode = RefundQuotaModeWriteOff
	}
	if params.RefundStatus == RefundStatusRefunded {
		switch mode {
		case RefundQuotaModeWriteOff:
			// ok
		case RefundQuotaModeSubtract:
			if params.ActualRefundQuota <= 0 {
				return nil, nil, 0, nil, ErrTicketRefundQuotaInvalid
			}
		case RefundQuotaModeOverride:
			if params.ActualRefundQuota < 0 {
				return nil, nil, 0, nil, ErrTicketRefundQuotaInvalid
			}
		default:
			return nil, nil, 0, nil, ErrTicketRefundQuotaModeInvalid
		}
		if params.ClawBackCommission && params.ClawBackQuota <= 0 {
			return nil, nil, 0, nil, ErrTicketRefundClawbackQuotaInvalid
		}
	}

	// Read only routing identity before acquiring locks in user -> ticket -> refund order.
	var identity TicketRefund
	if err := DB.Select("id, user_id").Where("ticket_id = ?", params.TicketId).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, 0, nil, ErrTicketRefundNotFound
		}
		return nil, nil, 0, nil, err
	}
	var refund TicketRefund
	var ticket Ticket
	var prevStatus, originalQuota, oldQuotaForOverride, finalQuotaForOverride int
	userId := identity.UserId
	var clawback *CommissionClawbackResult
	txErr := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id, quota").First(&user, userId).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&ticket, params.TicketId).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("ticket_id = ? AND user_id = ?", params.TicketId, userId).First(&refund).Error; err != nil {
			return err
		}
		if refund.RefundStatus != RefundStatusPending {
			return ErrTicketRefundNotPending
		}
		if refund.RefundQuota <= 0 {
			return ErrTicketRefundQuotaInvalid
		}
		prevStatus, originalQuota = ticket.Status, refund.RefundQuota
		now := common.GetTimestamp()
		refundUpdates := map[string]interface{}{
			"refund_status":  params.RefundStatus,
			"processed_time": now,
			"frozen_quota":   0,
		}
		ticketUpdates := map[string]interface{}{
			"updated_time": now,
			"admin_id":     params.AdminId,
		}
		if params.RefundStatus == RefundStatusRefunded {
			ticketUpdates["status"] = TicketStatusResolved
		} else {
			ticketUpdates["status"] = TicketStatusProcessing
		}
		res := tx.Model(&TicketRefund{}).
			Where("id = ? AND refund_status = ?", refund.Id, RefundStatusPending).
			Updates(refundUpdates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 并发保护：已被其他管理员处理过。
			return ErrTicketRefundNotPending
		}
		if params.RefundStatus == RefundStatusRefunded {
			if err := finalizeRefundFinanceTx(tx, &refund, params); err != nil {
				return err
			}
		} else {
			if err := releaseRefundFinanceTx(tx, &refund); err != nil {
				return err
			}
		}
		delta := 0
		if params.RefundStatus == RefundStatusRejected {
			delta = originalQuota
		} else if mode == RefundQuotaModeSubtract {
			delta = originalQuota - params.ActualRefundQuota
		} else if mode == RefundQuotaModeOverride {
			if int64(user.Quota) > math.MaxInt64-int64(originalQuota) {
				return ErrTicketRefundQuotaInvalid
			}
			oldQuotaForOverride = user.Quota + originalQuota
			finalQuotaForOverride = params.ActualRefundQuota
			// Locking the wallet serializes this explicit override with concurrent debits.
			if user.Quota != finalQuotaForOverride {
				result := tx.Model(&User{}).Where("id = ? AND quota = ?", userId, user.Quota).Update("quota", finalQuotaForOverride)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return ErrTicketRefundQuotaExceed
				}
			}
		}
		if delta != 0 {
			if err := applyRefundWalletDeltaTx(tx, userId, delta); err != nil {
				return err
			}
		}
		if err := tx.Model(&Ticket{}).Where("id = ?", ticket.Id).Updates(ticketUpdates).Error; err != nil {
			return err
		}
		// 返佣扣回与退款结算同事务：失败整体回滚，退款保持 pending 可重试；
		// 上面的 CAS 保证每张工单最多成功扣回一次。
		if params.RefundStatus == RefundStatusRefunded && params.ClawBackCommission {
			var cbErr error
			clawback, cbErr = applyCommissionClawback(tx, &refund, params.ClawBackQuota)
			if cbErr != nil {
				return cbErr
			}
		}
		refund.RefundStatus = params.RefundStatus
		refund.ProcessedTime = now
		refund.FrozenQuota = 0
		if status, ok := ticketUpdates["status"].(int); ok {
			ticket.Status = status
		}
		ticket.AdminId = params.AdminId
		ticket.UpdatedTime = now
		return AttachRefundFinance(tx, &refund)
	})
	if txErr != nil {
		return nil, nil, 0, nil, txErr
	}

	_ = InvalidateUserCache(userId)
	if clawback != nil && clawback.ClawedQuota > 0 {
		_ = InvalidateUserCache(clawback.InviterId)
	}

	// 4) 写审计日志（账户变更 + 工单动作）。
	adminInfo := map[string]interface{}{"admin_id": params.AdminId}
	switch params.RefundStatus {
	case RefundStatusRefunded:
		var logContent string
		switch mode {
		case RefundQuotaModeWriteOff:
			logContent = fmt.Sprintf("管理员批准退款，核销已扣余额 %s（工单 #%d）",
				logger.LogQuota(originalQuota), ticket.Id)
		case RefundQuotaModeSubtract:
			diff := params.ActualRefundQuota - originalQuota
			var delta string
			switch {
			case diff > 0:
				delta = fmt.Sprintf("追加扣减 %s", logger.LogQuota(diff))
			case diff < 0:
				delta = fmt.Sprintf("退还 %s", logger.LogQuota(-diff))
			default:
				delta = "余额无变动"
			}
			logContent = fmt.Sprintf("管理员批准退款，实际核销 %s（工单 #%d，原申请 %s，%s）",
				logger.LogQuota(params.ActualRefundQuota), ticket.Id,
				logger.LogQuota(originalQuota), delta)
		case RefundQuotaModeOverride:
			logContent = fmt.Sprintf("管理员批准退款并覆盖余额：工单 #%d，原申请 %s；解冻后余额 %s → 覆盖为 %s",
				ticket.Id, logger.LogQuota(originalQuota),
				logger.LogQuota(oldQuotaForOverride), logger.LogQuota(finalQuotaForOverride))
		}
		RecordLogWithAdminInfo(userId, LogTypeManage, logContent, adminInfo)
		if clawback != nil && clawback.ClawedQuota > 0 {
			RecordLogWithAdminInfo(clawback.InviterId, LogTypeManage,
				fmt.Sprintf("因被邀请用户退款，扣回邀请返佣 %s（工单 #%d）",
					logger.LogQuota(clawback.ClawedQuota), ticket.Id), adminInfo)
		}
	case RefundStatusRejected:
		RecordLogWithAdminInfo(userId, LogTypeManage,
			fmt.Sprintf("管理员驳回退款，退还额度 %s（工单 #%d）",
				logger.LogQuota(originalQuota), ticket.Id), adminInfo)
	}

	return &refund, &ticket, prevStatus, clawback, nil
}

// applyRefundWalletDeltaTx guards both insufficient funds and BIGINT overflow.
func applyRefundWalletDeltaTx(tx *gorm.DB, userId, delta int) error {
	if delta == 0 {
		return nil
	}
	query := tx.Model(&User{}).Where("id = ?", userId)
	if delta > 0 {
		query = query.Where("quota <= ?", math.MaxInt64-int64(delta))
	} else {
		if delta == math.MinInt {
			return ErrTicketRefundQuotaInvalid
		}
		query = query.Where("quota >= ?", -delta)
	}
	result := query.Update("quota", gorm.Expr("quota + ?", delta))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTicketRefundQuotaExceed
	}
	return nil
}

// CancelPendingRefund 取消指定工单的待审核退款，将冻结额度退还给用户。
// 仅当 refund_status == RefundStatusPending 时生效；CAS 保证幂等，并发安全。
// 设计用于用户主动关闭退款工单或管理员关闭退款工单场景。
func CancelPendingRefund(tx *gorm.DB, ticketId, userId int) error {
	var user User
	if err := lockForUpdate(tx).Select("id").First(&user, userId).Error; err != nil {
		return err
	}
	var refund TicketRefund
	if err := lockForUpdate(tx).Where("ticket_id = ? AND user_id = ? AND refund_status = ?", ticketId, userId, RefundStatusPending).
		First(&refund).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	now := common.GetTimestamp()
	res := tx.Model(&TicketRefund{}).
		Where("id = ? AND refund_status = ?", refund.Id, RefundStatusPending).
		Updates(map[string]interface{}{
			"refund_status":  RefundStatusCancelled,
			"frozen_quota":   0,
			"processed_time": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}

	if err := releaseRefundFinanceTx(tx, &refund); err != nil {
		return err
	}
	if refund.FrozenQuota < 0 {
		return ErrTicketRefundQuotaInvalid
	}
	if refund.FrozenQuota > 0 {
		if err := applyRefundWalletDeltaTx(tx, refund.UserId, refund.FrozenQuota); err != nil {
			return err
		}
	}

	return nil
}

func GetUserActiveRefundSummaries(userId int) ([]*TicketRefund, error) {
	var refunds []*TicketRefund
	if err := DB.Where("user_id = ? AND refund_status IN ?", userId, []int{RefundStatusPending, RefundStatusRefunded}).
		Order("created_time DESC").Find(&refunds).Error; err != nil {
		return nil, err
	}
	return refunds, nil
}
