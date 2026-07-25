package model

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var taxNumberRegex = regexp.MustCompile(`^[A-Z0-9]{18}$`)

const (
	InvoiceStatusPending   = 1
	InvoiceStatusIssued    = 2
	InvoiceStatusRejected  = 3
	InvoiceStatusCancelled = 4 // 用户主动取消（关闭开票工单），释放占用的充值订单
)

const (
	InvoiceTypeRegular = 1 // 普票（增值税普通发票）
	InvoiceTypeSpecial = 2 // 增票（增值税专用发票），需管理员开启后才可申请
)

type TicketInvoice struct {
	Id             int     `json:"id"`
	TicketId       int     `json:"ticket_id" gorm:"uniqueIndex;not null"`
	UserId         int     `json:"user_id" gorm:"index;not null"`
	CompanyName    string  `json:"company_name" gorm:"type:varchar(255);not null"`
	TaxNumber      string  `json:"tax_number" gorm:"type:varchar(64);not null"`
	BankName       string  `json:"bank_name" gorm:"type:varchar(255)"`
	BankAccount    string  `json:"bank_account" gorm:"type:varchar(128)"`
	CompanyAddress string  `json:"company_address" gorm:"type:varchar(512)"`
	CompanyPhone   string  `json:"company_phone" gorm:"type:varchar(32)"`
	Email          string  `json:"email" gorm:"type:varchar(128);not null"`
	Remark         string  `json:"remark" gorm:"type:varchar(255)"`
	TopUpOrderIds  string  `json:"topup_order_ids" gorm:"type:text;not null"`
	InvoiceType    int     `json:"invoice_type" gorm:"type:int;default:1"`
	FeeRate        float64 `json:"fee_rate"` // 申请时的手续费率快照（%），标记已开票时按此快照扣费，不读当前配置
	TotalMoney     float64 `json:"total_money"`
	InvoiceStatus  int     `json:"invoice_status" gorm:"type:int;default:1"`
	IssuedTime     int64   `json:"issued_time" gorm:"bigint;default:0"`
	// FeeQuota 实际扣除的手续费额度（额度单位，非人民币）。0 = 未扣或费率为 0。
	FeeQuota int `json:"fee_quota" gorm:"type:int;default:0"`
	// FeeChargedTime 手续费结算时间戳，是唯一的扣费幂等位：
	//   0  = 从未结算（可结算）
	//   >0 = 已结算（费率为 0 也会打戳），同一张发票一生只扣一次
	//   -1 = 本功能上线前就已是「已开票」的历史数据，视同已结算，禁止追扣
	FeeChargedTime int64 `json:"fee_charged_time" gorm:"bigint;default:0"`
	CreatedTime    int64 `json:"created_time" gorm:"bigint"`
}

// InvoiceFeeInsufficientError 携带扣费所需额度与用户当前余额，供 controller 渲染带参错误文案。
type InvoiceFeeInsufficientError struct {
	Required int
	Balance  int
}

func (e *InvoiceFeeInsufficientError) Error() string {
	return fmt.Sprintf("invoice fee insufficient: required=%d balance=%d", e.Required, e.Balance)
}

type CreateInvoiceTicketParams struct {
	UserId         int
	Username       string
	Subject        string
	Priority       int
	Content        string
	CompanyName    string
	TaxNumber      string
	BankName       string
	BankAccount    string
	CompanyAddress string
	CompanyPhone   string
	Email          string
	InvoiceType    int
	TopUpOrderIds  []int
}

func (invoice *TicketInvoice) BeforeCreate(tx *gorm.DB) error {
	if invoice.CreatedTime == 0 {
		invoice.CreatedTime = common.GetTimestamp()
	}
	return nil
}

func IsValidInvoiceStatus(status int) bool {
	switch status {
	case InvoiceStatusPending, InvoiceStatusIssued, InvoiceStatusRejected:
		return true
	default:
		return false
	}
}

func normalizeTopUpOrderIDs(orderIds []int) ([]int, error) {
	seen := make(map[int]struct{}, len(orderIds))
	result := make([]int, 0, len(orderIds))
	for _, orderId := range orderIds {
		if orderId <= 0 {
			return nil, ErrTicketInvoiceOrderInvalid
		}
		if _, ok := seen[orderId]; ok {
			continue
		}
		seen[orderId] = struct{}{}
		result = append(result, orderId)
	}
	if len(result) == 0 {
		return nil, ErrTicketInvoiceOrderEmpty
	}
	return result, nil
}

func (invoice *TicketInvoice) GetTopUpOrderIDs() ([]int, error) {
	if strings.TrimSpace(invoice.TopUpOrderIds) == "" {
		return []int{}, nil
	}
	var orderIds []int
	if err := common.UnmarshalJsonStr(invoice.TopUpOrderIds, &orderIds); err != nil {
		return nil, err
	}
	return normalizeTopUpOrderIDs(orderIds)
}

func (invoice *TicketInvoice) SetTopUpOrderIDs(orderIds []int) error {
	normalizedOrderIds, err := normalizeTopUpOrderIDs(orderIds)
	if err != nil {
		return err
	}
	raw, err := common.Marshal(normalizedOrderIds)
	if err != nil {
		return err
	}
	invoice.TopUpOrderIds = string(raw)
	return nil
}

func GetTicketInvoiceByTicketId(ticketId int) (*TicketInvoice, error) {
	var invoice TicketInvoice
	if err := DB.Where("ticket_id = ?", ticketId).First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTicketInvoiceNotFound
		}
		return nil, err
	}
	return &invoice, nil
}

// GetLatestInvoiceProfile 返回用户最近一次发票申请（含抬头信息），用于新申请时预填。
// 未申请过时返回 (nil, nil)。
func GetLatestInvoiceProfile(userId int) (*TicketInvoice, error) {
	var invoice TicketInvoice
	if err := DB.Where("user_id = ?", userId).Order("id DESC").First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &invoice, nil
}

func GetUserInvoiceSummaries(userId int) ([]*TicketInvoice, error) {
	var invoices []*TicketInvoice
	if err := DB.Where("user_id = ?", userId).Order("created_time DESC").Find(&invoices).Error; err != nil {
		return nil, err
	}
	return invoices, nil
}

func getProtectedInvoiceOrderSet(tx *gorm.DB, userId int) (map[int]struct{}, error) {
	var invoices []*TicketInvoice
	// 已驳回/已取消的发票正常释放订单占用，但「已扣过手续费的已驳回发票」例外：
	// 手续费不会自动退还，若放开订单，用户对同批订单再次申请开票会被再扣一次。
	// 人工退费后把该行 fee_quota 置 0 即可释放订单。
	if err := tx.Where("user_id = ?", userId).
		Where("invoice_status NOT IN ? OR (invoice_status = ? AND fee_quota > 0)",
			[]int{InvoiceStatusRejected, InvoiceStatusCancelled}, InvoiceStatusRejected).
		Find(&invoices).Error; err != nil {
		return nil, err
	}

	used := make(map[int]struct{})
	for _, invoice := range invoices {
		orderIds, err := invoice.GetTopUpOrderIDs()
		if err != nil {
			return nil, err
		}
		for _, orderId := range orderIds {
			used[orderId] = struct{}{}
		}
	}
	return used, nil
}

func fetchSuccessTopUpsByIds(tx *gorm.DB, userId int, orderIds []int) ([]*TopUp, error) {
	var topUps []*TopUp
	if err := tx.Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).
		Where("id IN ?", orderIds).
		Find(&topUps).Error; err != nil {
		return nil, err
	}
	return topUps, nil
}

func orderTopUps(orderIds []int, topUps []*TopUp) []*TopUp {
	topUpMap := make(map[int]*TopUp, len(topUps))
	for _, topUp := range topUps {
		topUpMap[topUp.Id] = topUp
	}
	ordered := make([]*TopUp, 0, len(orderIds))
	for _, orderId := range orderIds {
		if topUp, ok := topUpMap[orderId]; ok {
			ordered = append(ordered, topUp)
		}
	}
	return ordered
}

func GetEligibleInvoiceOrders(userId int) ([]*TopUp, error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	usedSet, err := getProtectedInvoiceOrderSet(tx, userId)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	query := tx.Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess)
	if len(usedSet) > 0 {
		usedIds := make([]int, 0, len(usedSet))
		for orderId := range usedSet {
			usedIds = append(usedIds, orderId)
		}
		query = query.Not("id IN ?", usedIds)
	}

	var topUps []*TopUp
	if err = query.Order("complete_time desc, id desc").Find(&topUps).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err = tx.Commit().Error; err != nil {
		return nil, err
	}
	return topUps, nil
}

func invoiceTypeName(invoiceType int) string {
	if invoiceType == InvoiceTypeSpecial {
		return "增票（增值税专用发票）"
	}
	return "普票（增值税普通发票）"
}

func buildInvoiceSummaryMessage(params CreateInvoiceTicketParams, orderIds []int, totalMoney float64, feeRate float64) string {
	lines := []string{
		"发票申请信息：",
		fmt.Sprintf("票种：%s", invoiceTypeName(params.InvoiceType)),
		fmt.Sprintf("公司名称：%s", strings.TrimSpace(params.CompanyName)),
		fmt.Sprintf("税号：%s", strings.TrimSpace(params.TaxNumber)),
		fmt.Sprintf("接收邮箱：%s", strings.TrimSpace(params.Email)),
		fmt.Sprintf("关联订单：%v", orderIds),
		fmt.Sprintf("申请金额：%.2f", totalMoney),
	}
	if feeRate > 0 {
		lines = append(lines, fmt.Sprintf("手续费率：%g%%（开票通过后将从账户余额中扣除，按关联订单实际到账额度计算）", feeRate))
	}
	if bankName := strings.TrimSpace(params.BankName); bankName != "" {
		lines = append(lines, fmt.Sprintf("开户行：%s", bankName))
	}
	if bankAccount := strings.TrimSpace(params.BankAccount); bankAccount != "" {
		lines = append(lines, fmt.Sprintf("银行账号：%s", bankAccount))
	}
	if companyAddress := strings.TrimSpace(params.CompanyAddress); companyAddress != "" {
		lines = append(lines, fmt.Sprintf("注册地址：%s", companyAddress))
	}
	if companyPhone := strings.TrimSpace(params.CompanyPhone); companyPhone != "" {
		lines = append(lines, fmt.Sprintf("联系电话：%s", companyPhone))
	}
	if content := strings.TrimSpace(params.Content); content != "" {
		lines = append(lines, "备注：")
		lines = append(lines, content)
	}
	return strings.Join(lines, "\n")
}

// calcInvoiceFeeQuota 计算本次开票应扣的手续费额度。
// 计费基数是关联充值订单的实际到账额度之和（TopUp.QuotaGranted），不走人民币换算：
// TicketInvoice.TotalMoney 是混合单位（易支付/Waffo 存人民币、Stripe 存美元），
// 且充值可能带折扣码或分组倍率，按元换算会与用户实际获得的额度不成比例。
// feeRate 传入的是申请时的快照 invoice.FeeRate。
func calcInvoiceFeeQuota(topUps []*TopUp, feeRate float64) (int, error) {
	if feeRate <= 0 {
		return 0, nil
	}
	var baseQuota int64
	for _, topUp := range topUps {
		// quota_granted 由 backfillTopUpQuotaGranted 回填保证为正；为 0 说明该订单到账额度
		// 不可知，静默按 0 计费会少收钱，硬失败让管理员先核对订单数据。
		if topUp.QuotaGranted <= 0 {
			return 0, ErrTicketInvoiceFeeBaseInvalid
		}
		baseQuota += topUp.QuotaGranted
	}
	if baseQuota <= 0 {
		return 0, ErrTicketInvoiceFeeBaseInvalid
	}
	feeQuota, clamp := common.QuotaFromDecimalChecked(
		decimal.NewFromInt(baseQuota).
			Mul(decimal.NewFromFloat(feeRate)).
			Div(decimal.NewFromInt(100)),
	)
	if clamp != nil {
		common.SysError(fmt.Sprintf("invoice fee quota clamped: base=%d rate=%v clamped=%d",
			baseQuota, feeRate, feeQuota))
	}
	if feeQuota < 0 {
		return 0, ErrTicketInvoiceFeeBaseInvalid
	}
	return feeQuota, nil
}

func CreateInvoiceTicket(params CreateInvoiceTicketParams) (*Ticket, *TicketInvoice, *TicketMessage, []*TopUp, error) {
	if strings.TrimSpace(params.CompanyName) == "" {
		return nil, nil, nil, nil, ErrTicketInvoiceCompanyEmpty
	}
	taxNumber := strings.TrimSpace(params.TaxNumber)
	if taxNumber == "" {
		return nil, nil, nil, nil, ErrTicketInvoiceTaxNumberEmpty
	}
	if !taxNumberRegex.MatchString(strings.ToUpper(taxNumber)) {
		return nil, nil, nil, nil, ErrTicketInvoiceTaxNumberFormat
	}
	params.TaxNumber = strings.ToUpper(taxNumber)
	if strings.TrimSpace(params.Email) == "" {
		return nil, nil, nil, nil, ErrTicketInvoiceEmailEmpty
	}
	if params.InvoiceType == 0 {
		params.InvoiceType = InvoiceTypeRegular
	}
	var feeRate float64
	switch params.InvoiceType {
	case InvoiceTypeRegular:
		feeRate = operation_setting.InvoiceRegularFeeRate
	case InvoiceTypeSpecial:
		if !operation_setting.InvoiceSpecialEnabled {
			return nil, nil, nil, nil, ErrTicketInvoiceTypeDisabled
		}
		feeRate = operation_setting.InvoiceSpecialFeeRate
	default:
		return nil, nil, nil, nil, ErrTicketInvoiceTypeInvalid
	}
	if feeRate < 0 {
		feeRate = 0
	}

	orderIds, err := normalizeTopUpOrderIDs(params.TopUpOrderIds)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	var (
		ticket        *Ticket
		message       *TicketMessage
		invoice       *TicketInvoice
		orderedTopUps []*TopUp
	)
	err = DB.Transaction(func(tx *gorm.DB) error {
		usedSet, err := getProtectedInvoiceOrderSet(tx, params.UserId)
		if err != nil {
			return err
		}
		for _, orderId := range orderIds {
			if _, ok := usedSet[orderId]; ok {
				return ErrTicketInvoiceOrderDuplicate
			}
		}

		topUps, err := fetchSuccessTopUpsByIds(tx, params.UserId, orderIds)
		if err != nil {
			return err
		}
		if len(topUps) != len(orderIds) {
			return ErrTicketInvoiceOrderInvalid
		}

		var totalMoney float64
		for _, topUp := range topUps {
			totalMoney += topUp.Money
		}
		if operation_setting.MinInvoiceAmount > 0 && totalMoney < float64(operation_setting.MinInvoiceAmount) {
			return ErrTicketInvoiceAmountBelowMin
		}
		orderedTopUps = orderTopUps(orderIds, topUps)

		subject := strings.TrimSpace(params.Subject)
		if subject == "" {
			subject = fmt.Sprintf("发票申请（%d 笔订单）", len(orderIds))
		}

		now := common.GetTimestamp()
		ticket = &Ticket{
			UserId:      params.UserId,
			Username:    strings.TrimSpace(params.Username),
			Subject:     subject,
			Type:        TicketTypeInvoice,
			Status:      TicketStatusOpen,
			Priority:    NormalizeTicketPriority(params.Priority),
			CreatedTime: now,
			UpdatedTime: now,
		}
		if err := tx.Create(ticket).Error; err != nil {
			return err
		}

		remark := strings.TrimSpace(params.Content)
		if runes := []rune(remark); len(runes) > 255 {
			remark = string(runes[:255])
		}
		invoice = &TicketInvoice{
			TicketId:       ticket.Id,
			UserId:         params.UserId,
			CompanyName:    strings.TrimSpace(params.CompanyName),
			TaxNumber:      strings.TrimSpace(params.TaxNumber),
			BankName:       strings.TrimSpace(params.BankName),
			BankAccount:    strings.TrimSpace(params.BankAccount),
			CompanyAddress: strings.TrimSpace(params.CompanyAddress),
			CompanyPhone:   strings.TrimSpace(params.CompanyPhone),
			Email:          strings.TrimSpace(params.Email),
			Remark:         remark,
			InvoiceType:    params.InvoiceType,
			FeeRate:        feeRate,
			TotalMoney:     totalMoney,
			InvoiceStatus:  InvoiceStatusPending,
			CreatedTime:    now,
		}
		if err := invoice.SetTopUpOrderIDs(orderIds); err != nil {
			return err
		}
		if err := tx.Create(invoice).Error; err != nil {
			return err
		}

		message = &TicketMessage{
			TicketId:    ticket.Id,
			UserId:      params.UserId,
			Username:    strings.TrimSpace(params.Username),
			Role:        common.RoleCommonUser,
			Content:     buildInvoiceSummaryMessage(params, orderIds, totalMoney, feeRate),
			CreatedTime: now,
		}
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return ticket, invoice, message, orderedTopUps, nil
}

func GetTicketInvoiceDetail(ticketId int) (*TicketInvoice, []*TopUp, error) {
	invoice, err := GetTicketInvoiceByTicketId(ticketId)
	if err != nil {
		return nil, nil, err
	}
	orderIds, err := invoice.GetTopUpOrderIDs()
	if err != nil {
		return nil, nil, err
	}
	topUps, err := fetchSuccessTopUpsByIds(DB, invoice.UserId, orderIds)
	if err != nil {
		return nil, nil, err
	}
	return invoice, orderTopUps(orderIds, topUps), nil
}

// CancelPendingInvoice 取消指定工单的待开票申请，释放其占用的充值订单以便重新开票。
// 仅当 invoice_status == InvoiceStatusPending 时生效（已开票/已驳回不受影响）；幂等，并发安全。
// 设计用于用户主动关闭开票工单或管理员关闭开票工单场景。
func CancelPendingInvoice(tx *gorm.DB, ticketId, userId int) error {
	return tx.Model(&TicketInvoice{}).
		Where("ticket_id = ? AND user_id = ? AND invoice_status = ?", ticketId, userId, InvoiceStatusPending).
		Update("invoice_status", InvoiceStatusCancelled).Error
}

// UpdateInvoiceStatus 管理员调整发票状态。
// 标记为「已开票」时按 invoice.FeeRate 快照从用户额度中扣除开票手续费：余额不足则整个
// 事务回滚，发票保持原状可重试。fee_charged_time 是落库的幂等位，同一张发票一生只扣一次，
// 因此「已开票 → 驳回 → 再已开票」的纠错回退不会二次扣费。
// 第三个返回值是修改前的工单主状态，供调用方触发状态已变化通知。
func UpdateInvoiceStatus(ticketId int, adminId int, invoiceStatus int) (*TicketInvoice, *Ticket, int, error) {
	if !IsValidInvoiceStatus(invoiceStatus) {
		return nil, nil, 0, ErrTicketInvoiceStatusInvalid
	}

	var (
		invoice     TicketInvoice
		ticket      Ticket
		prevStatus  int
		chargedUser int
	)
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 行锁挡住并发的第二个客服；SQLite 下 lockForUpdate 退化为普通读，由下面的
		// invoice_status / quota CAS 兜底。
		if err := lockForUpdate(tx).Where("ticket_id = ?", ticketId).First(&invoice).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketInvoiceNotFound
			}
			return err
		}
		// 已取消的申请释放了订单占用，订单可能已被重新申请开票，禁止复活避免重复开票
		if invoice.InvoiceStatus == InvoiceStatusCancelled {
			return ErrTicketInvoiceStatusInvalid
		}
		if err := tx.First(&ticket, "id = ?", ticketId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		prevStatus = ticket.Status

		now := common.GetTimestamp()
		invoiceUpdates := map[string]interface{}{
			"invoice_status": invoiceStatus,
		}
		ticketUpdates := map[string]interface{}{
			"updated_time": now,
			"admin_id":     adminId,
		}
		switch invoiceStatus {
		case InvoiceStatusIssued:
			invoiceUpdates["issued_time"] = now
			ticketUpdates["status"] = TicketStatusResolved
		case InvoiceStatusRejected:
			invoiceUpdates["issued_time"] = int64(0)
			ticketUpdates["status"] = TicketStatusProcessing
		default:
			invoiceUpdates["issued_time"] = int64(0)
		}

		if invoiceStatus == InvoiceStatusIssued && invoice.FeeChargedTime == 0 {
			orderIds, err := invoice.GetTopUpOrderIDs()
			if err != nil {
				return err
			}
			topUps, err := fetchSuccessTopUpsByIds(tx, invoice.UserId, orderIds)
			if err != nil {
				return err
			}
			if len(topUps) != len(orderIds) {
				return ErrTicketInvoiceOrderInvalid
			}
			feeQuota, err := calcInvoiceFeeQuota(topUps, invoice.FeeRate)
			if err != nil {
				return err
			}
			if feeQuota > 0 {
				var user User
				if err := lockForUpdate(tx).Where("id = ?", invoice.UserId).First(&user).Error; err != nil {
					return err
				}
				if user.Quota < feeQuota {
					return &InvoiceFeeInsufficientError{Required: feeQuota, Balance: user.Quota}
				}
				// SQLite 下没有行锁，quota >= ? 的 CAS 兜底防止并发扣成负数
				res := tx.Model(&User{}).
					Where("id = ? AND quota >= ?", invoice.UserId, feeQuota).
					Update("quota", gorm.Expr("quota - ?", feeQuota))
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected == 0 {
					return &InvoiceFeeInsufficientError{Required: feeQuota, Balance: user.Quota}
				}
				chargedUser = invoice.UserId
			}
			// 费率为 0 时同样打戳：让「未结算」与「结算金额为 0」可区分，否则日后把费率
			// 调高，这张老发票会在下一次标记已开票时被追扣。
			invoiceUpdates["fee_quota"] = feeQuota
			invoiceUpdates["fee_charged_time"] = now
		}

		// 状态 CAS：发票状态必须仍是本事务读到的值；扣费时额外要求 fee_charged_time 未变，
		// 保证扣款与打戳在并发下严格一对一。
		invoiceGuard := tx.Model(&TicketInvoice{}).
			Where("id = ? AND invoice_status = ?", invoice.Id, invoice.InvoiceStatus)
		if _, ok := invoiceUpdates["fee_charged_time"]; ok {
			invoiceGuard = invoiceGuard.Where("fee_charged_time = ?", invoice.FeeChargedTime)
		}
		res := invoiceGuard.Updates(invoiceUpdates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 已被其他管理员并发处理，整个事务回滚，刚才的扣款一并撤销
			return ErrTicketInvoiceStatusChanged
		}
		if err := tx.Model(&Ticket{}).Where("id = ?", ticket.Id).Updates(ticketUpdates).Error; err != nil {
			return err
		}

		invoice.InvoiceStatus = invoiceStatus
		invoice.IssuedTime = invoiceUpdates["issued_time"].(int64)
		if v, ok := invoiceUpdates["fee_quota"].(int); ok {
			invoice.FeeQuota = v
		}
		if v, ok := invoiceUpdates["fee_charged_time"].(int64); ok {
			invoice.FeeChargedTime = v
		}
		if status, ok := ticketUpdates["status"].(int); ok {
			ticket.Status = status
		}
		ticket.AdminId = adminId
		ticket.UpdatedTime = now
		return nil
	})
	if err != nil {
		return nil, nil, 0, err
	}

	// 这里绕过了 DecreaseUserQuota 的 Redis 增量路径，必须失效缓存，否则缓存余额长期偏高
	if chargedUser > 0 {
		_ = InvalidateUserCache(chargedUser)
		RecordLogWithAdminInfo(invoice.UserId, LogTypeManage,
			fmt.Sprintf("发票已开具，按 %g%% 手续费率扣除开票手续费 %s（工单 #%d）",
				invoice.FeeRate, logger.LogQuota(invoice.FeeQuota), ticket.Id),
			map[string]interface{}{
				"admin_id":  adminId,
				"ticket_id": ticket.Id,
				"fee_quota": invoice.FeeQuota,
				"fee_rate":  invoice.FeeRate,
			})
	}
	return &invoice, &ticket, prevStatus, nil
}

type InvoiceExportItem struct {
	TicketId      int     `json:"ticket_id" gorm:"column:ticket_id"`
	InvoiceType   int     `json:"invoice_type" gorm:"column:invoice_type"`
	FeeRate       float64 `json:"fee_rate" gorm:"column:fee_rate"`
	CompanyName   string  `json:"company_name" gorm:"column:company_name"`
	TaxNumber     string  `json:"tax_number" gorm:"column:tax_number"`
	Email         string  `json:"email" gorm:"column:email"`
	TotalMoney    float64 `json:"total_money" gorm:"column:total_money"`
	TopUpOrderIds string  `json:"-" gorm:"column:top_up_order_ids"`
	OrderCount    int     `json:"order_count" gorm:"-"`
	Status        int     `json:"status" gorm:"column:status"`
	CreatedTime   int64   `json:"created_time" gorm:"column:created_time"`
}

type InvoiceExportFilter struct {
	Keyword      string
	TicketStatus int
	StartTime    int64
	EndTime      int64
}

func ListInvoicesForExport(filter InvoiceExportFilter, pageInfo *common.PageInfo) ([]*InvoiceExportItem, int64, error) {
	var total int64
	items := make([]*InvoiceExportItem, 0)

	query := DB.Table("tickets t").
		Select("t.id AS ticket_id, ti.invoice_type, ti.fee_rate, ti.company_name, ti.tax_number, ti.email, ti.total_money, ti.top_up_order_ids, t.status, t.created_time").
		Joins("INNER JOIN ticket_invoices ti ON ti.ticket_id = t.id").
		Where("t.type = ? AND t.deleted_at IS NULL", TicketTypeInvoice)

	if filter.TicketStatus > 0 {
		query = query.Where("t.status = ?", filter.TicketStatus)
	}
	if filter.Keyword != "" {
		pattern, err := sanitizeLikePattern(filter.Keyword)
		if err != nil {
			return nil, 0, err
		}
		keywordScope := func(db *gorm.DB) *gorm.DB {
			sub := db.Where("ti.company_name LIKE ? ESCAPE '!'", pattern).
				Or("ti.email LIKE ? ESCAPE '!'", pattern)
			if amount, parseErr := strconv.ParseFloat(filter.Keyword, 64); parseErr == nil {
				sub = sub.Or("ti.total_money = ?", amount)
			}
			return sub
		}
		query = query.Where(keywordScope)
	}
	if filter.StartTime > 0 {
		query = query.Where("t.created_time >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("t.created_time <= ?", filter.EndTime)
	}

	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("t.id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&items).Error; err != nil {
		return nil, 0, err
	}

	for _, item := range items {
		var orderIds []int
		if strings.TrimSpace(item.TopUpOrderIds) != "" {
			_ = common.UnmarshalJsonStr(item.TopUpOrderIds, &orderIds)
		}
		item.OrderCount = len(orderIds)
	}

	return items, total, nil
}
