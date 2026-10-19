package model

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var taxNumberRegex = regexp.MustCompile(`^[A-Z0-9]{18}$`)

const (
	InvoiceStatusPending   = 1
	InvoiceStatusIssued    = 2
	InvoiceStatusRejected  = 3
	MaxInvoiceRemarkLength = 100
)

type TicketInvoice struct {
	InvoiceType     string            `json:"invoice_type" gorm:"size:16"`
	InvoiceNetCents int64             `json:"invoice_net_cents"`
	InvoiceTaxCents int64             `json:"invoice_tax_cents"`
	TaxCharge       *InvoiceTaxCharge `json:"tax_charge,omitempty" gorm:"-"`
	Id              int               `json:"id"`
	TicketId        int               `json:"ticket_id" gorm:"uniqueIndex;not null"`
	UserId          int               `json:"user_id" gorm:"index;not null"`
	CompanyName     string            `json:"company_name" gorm:"type:varchar(255);not null"`
	TaxNumber       string            `json:"tax_number" gorm:"type:varchar(64);not null"`
	BankName        string            `json:"bank_name" gorm:"type:varchar(255)"`
	BankAccount     string            `json:"bank_account" gorm:"type:varchar(128)"`
	CompanyAddress  string            `json:"company_address" gorm:"type:varchar(512)"`
	CompanyPhone    string            `json:"company_phone" gorm:"type:varchar(32)"`
	Remark          string            `json:"remark" gorm:"type:varchar(255)"`
	Email           string            `json:"email" gorm:"type:varchar(128);not null"`
	TopUpOrderIds   string            `json:"topup_order_ids" gorm:"type:text;not null"`
	TotalMoney      float64           `json:"total_money"`
	InvoiceStatus   int               `json:"invoice_status" gorm:"type:int;default:1"`
	IssuedTime      int64             `json:"issued_time" gorm:"bigint;default:0"`
	CreatedTime     int64             `json:"created_time" gorm:"bigint"`
}

type CreateInvoiceTicketParams struct {
	InvoiceType      string
	TaxPaymentMethod string
	TaxQuote         string
	UserId           int
	Username         string
	Subject          string
	Priority         int
	Content          string
	CompanyName      string
	TaxNumber        string
	BankName         string
	BankAccount      string
	CompanyAddress   string
	CompanyPhone     string
	Email            string
	TopUpOrderIds    []int
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
	charge, err := GetInvoiceTaxCharge(ticketId)
	if err != nil {
		return nil, err
	}
	invoice.TaxCharge = charge
	return &invoice, nil
}

func GetUserInvoiceSummaries(userId int) ([]*TicketInvoice, error) {
	var invoices []*TicketInvoice
	if err := activeInvoiceOrders(DB).Where("user_id = ?", userId).Order("created_time DESC").Find(&invoices).Error; err != nil {
		return nil, err
	}
	return invoices, nil
}

func getProtectedInvoiceOrderSet(tx *gorm.DB, userId int) (map[int]struct{}, error) {
	var invoices []*TicketInvoice
	if err := activeInvoiceOrders(tx).Where("user_id = ? AND invoice_status <> ?", userId, InvoiceStatusRejected).Find(&invoices).Error; err != nil {
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

// Closed, unissued applications no longer reserve orders. Reapplying settles
// any legacy tax ledger in the same transaction as the new application.
func activeInvoiceOrders(tx *gorm.DB) *gorm.DB {
	closed := tx.Session(&gorm.Session{NewDB: true}).Model(&Ticket{}).Select("id").Where("status = ?", TicketStatusClosed)
	return tx.Where("invoice_status <> ? OR ticket_id NOT IN (?)", InvoiceStatusPending, closed)
}

// Called inside the ticket-closing transaction; quota cache changes must wait
// until the caller commits. Issued invoices must never be cancelled by closing.
func cancelPendingInvoice(tx *gorm.DB, ticket *Ticket, actorID int) (int, error) {
	var owner User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, ticket.UserId).Error; err != nil {
		return 0, err
	}
	var invoice TicketInvoice
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("ticket_id = ?", ticket.Id).Take(&invoice).Error; err != nil {
		return 0, err
	}
	if invoice.InvoiceStatus != InvoiceStatusPending {
		return 0, nil
	}
	var charge InvoiceTaxCharge
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("ticket_id = ?", ticket.Id).Take(&charge).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	refund := 0
	if err == nil && charge.Status == "paid" {
		if err := refundPaidInvoiceTax(tx, &charge); err != nil {
			return 0, err
		}
		refund = charge.Quota
	}
	result := tx.Model(&TicketInvoice{}).Where("id = ? AND invoice_status = ?", invoice.Id, InvoiceStatusPending).Update("invoice_status", InvoiceStatusRejected)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, ErrTicketInvoiceStatusInvalid
	}
	role := common.RoleAdminUser
	if actorID == ticket.UserId {
		role = common.RoleCommonUser
	}
	err = tx.Create(&TicketMessage{TicketId: ticket.Id, UserId: actorID, Role: role, Content: fmt.Sprintf("Invoice application cancelled on ticket closure; tax refunded to balance: %d quota", refund), CreatedTime: common.GetTimestamp()}).Error
	return refund, err
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

func GetEligibleInvoiceOrders(userId int, invoiceType string) ([]*TopUp, error) {
	config, err := GetInvoiceTaxConfig(DB)
	if err != nil {
		return nil, err
	}
	if invoiceType == "" {
		invoiceType = "ordinary"
	}
	if invoiceType != "ordinary" && invoiceType != "special" {
		return nil, ErrInvoiceTaxInvalid
	}
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
	eligible := make([]*TopUp, 0, len(topUps))
	for _, order := range topUps {
		if order.InvoiceType == "" {
			if config.LegacyTopUpsEnabled && IsCNYInvoiceTopUp(order) {
				eligible = append(eligible, order)
			}
		} else if order.CheckoutVersion == 1 && order.InvoiceType == invoiceType {
			eligible = append(eligible, order)
		}
	}
	return eligible, nil
}

func buildInvoiceSummaryMessage(params CreateInvoiceTicketParams, orderIds []int, totalMoney float64) string {
	lines := []string{
		"发票申请信息：",
		fmt.Sprintf("公司名称：%s", strings.TrimSpace(params.CompanyName)),
		fmt.Sprintf("税号：%s", strings.TrimSpace(params.TaxNumber)),
		fmt.Sprintf("接收邮箱：%s", strings.TrimSpace(params.Email)),
		fmt.Sprintf("关联订单：%v", orderIds),
		fmt.Sprintf("申请金额：%.2f", totalMoney),
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
	if utf8.RuneCountInString(strings.TrimSpace(params.Content)) > MaxInvoiceRemarkLength {
		return nil, nil, nil, nil, ErrTicketInvoiceRemarkTooLong
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
	var refundedQuota int
	err = DB.Transaction(func(tx *gorm.DB) error {
		config, err := GetInvoiceTaxConfig(tx)
		if err != nil {
			return err
		}
		var owner User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, params.UserId).Error; err != nil {
			return err
		}
		// Old versions closed only the ticket. Settle overlapping applications
		// before reuse, without performing financial writes in the listing API.
		var closedInvoices []*TicketInvoice
		closedTickets := tx.Model(&Ticket{}).Select("id").Where("user_id = ? AND status = ?", params.UserId, TicketStatusClosed)
		if err := tx.Where("user_id = ? AND invoice_status = ? AND ticket_id IN (?)", params.UserId, InvoiceStatusPending, closedTickets).Find(&closedInvoices).Error; err != nil {
			return err
		}
		selected := make(map[int]bool, len(orderIds))
		for _, id := range orderIds {
			selected[id] = true
		}
		for _, previous := range closedInvoices {
			ids, err := previous.GetTopUpOrderIDs()
			if err != nil {
				return err
			}
			for _, id := range ids {
				if selected[id] {
					refund, err := cancelPendingInvoice(tx, &Ticket{Id: previous.TicketId, UserId: params.UserId}, params.UserId)
					if err != nil {
						return err
					}
					refundedQuota += refund
					break
				}
			}
		}
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
		quote, err := QuoteInvoiceTax(tx, params.UserId, orderIds, params.InvoiceType)
		if err != nil {
			return err
		}
		totalMoney = float64(quote.TotalCents) / 100
		var charge *InvoiceTaxCharge
		if config.Enabled && !quote.PrepaidOnly {
			if quote.Token != params.TaxQuote {
				return ErrInvoiceTaxQuoteChanged
			}
			if params.TaxPaymentMethod != "balance" && params.TaxPaymentMethod != "online" {
				return ErrInvoiceTaxInvalid
			}
			charge = &InvoiceTaxCharge{UserId: params.UserId, TradeNo: "ITAX" + common.GetRandomString(32), InvoiceType: quote.InvoiceType, Mode: quote.Mode, RateBps: quote.RateBps, BaseCents: quote.BaseCents, FeeCents: quote.FeeCents, TotalCents: quote.BaseCents + quote.FeeCents, Quota: quote.Quota, Status: "pending", CreatedTime: common.GetTimestamp()}
			if quote.FeeCents == 0 {
				charge.Status = "paid"
				charge.PaidTime = common.GetTimestamp()
			}
			if quote.FeeCents > 0 && params.TaxPaymentMethod == "balance" && quote.CanBalance {
				result := tx.Model(&User{}).Where("id = ? AND quota >= ?", params.UserId, quote.Quota).Update("quota", gorm.Expr("quota - ?", quote.Quota))
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected == 1 {
					charge.Status = "paid"
					charge.PaymentMethod = "balance"
					charge.PaidTime = common.GetTimestamp()
				}
			}
			totalMoney = float64(quote.TotalCents) / 100
		}

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

		invoice = &TicketInvoice{
			InvoiceType:     quote.InvoiceType,
			InvoiceNetCents: quote.BaseCents + quote.PrepaidNetCents,
			InvoiceTaxCents: quote.FeeCents + quote.PrepaidTaxCents,
			TicketId:        ticket.Id,
			UserId:          params.UserId,
			CompanyName:     strings.TrimSpace(params.CompanyName),
			TaxNumber:       strings.TrimSpace(params.TaxNumber),
			BankName:        strings.TrimSpace(params.BankName),
			BankAccount:     strings.TrimSpace(params.BankAccount),
			CompanyAddress:  strings.TrimSpace(params.CompanyAddress),
			CompanyPhone:    strings.TrimSpace(params.CompanyPhone),
			Remark:          strings.TrimSpace(params.Content),
			Email:           strings.TrimSpace(params.Email),
			TotalMoney:      totalMoney,
			InvoiceStatus:   InvoiceStatusPending,
			CreatedTime:     now,
		}
		if err := invoice.SetTopUpOrderIDs(orderIds); err != nil {
			return err
		}
		if err := tx.Create(invoice).Error; err != nil {
			return err
		}
		if charge != nil {
			charge.TicketId = ticket.Id
			if err := tx.Create(charge).Error; err != nil {
				return err
			}
			invoice.TaxCharge = charge
		}

		message = &TicketMessage{
			TicketId:    ticket.Id,
			UserId:      params.UserId,
			Username:    strings.TrimSpace(params.Username),
			Role:        common.RoleCommonUser,
			Content:     buildInvoiceSummaryMessage(params, orderIds, totalMoney),
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
	quotaDelta := refundedQuota
	if invoice.TaxCharge != nil && invoice.TaxCharge.PaymentMethod == "balance" {
		quotaDelta -= invoice.TaxCharge.Quota
		RecordLog(params.UserId, LogTypeSystem, fmt.Sprintf("Invoice tax balance payment: ticket %d, CNY %.2f, quota %d", ticket.Id, float64(invoice.TaxCharge.FeeCents)/100, invoice.TaxCharge.Quota))
	}
	if quotaDelta != 0 {
		syncInvoiceTaxQuotaCache(params.UserId, quotaDelta)
	}
	if refundedQuota > 0 {
		RecordLog(params.UserId, LogTypeSystem, fmt.Sprintf("Closed invoice tax refunded before reapplication: quota %d", refundedQuota))
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

// UpdateInvoiceStatus 管理员调整发票状态。
// 第三个返回值是修改前的工单主状态，供调用方触发状态已变化通知。
func UpdateInvoiceStatus(ticketId int, adminId int, invoiceStatus int) (*TicketInvoice, *Ticket, int, error) {
	if !IsValidInvoiceStatus(invoiceStatus) {
		return nil, nil, 0, ErrTicketInvoiceStatusInvalid
	}

	var (
		invoice    TicketInvoice
		ticket     Ticket
		prevStatus int
	)
	var refundedQuota int
	var known TicketInvoice
	if err := DB.Where("ticket_id = ?", ticketId).Take(&known).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, 0, ErrTicketInvoiceNotFound
		}
		return nil, nil, 0, err
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, known.UserId).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("ticket_id = ?", ticketId).First(&invoice).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketInvoiceNotFound
			}
			return err
		}
		if err := tx.First(&ticket, "id = ?", ticketId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		prevStatus = ticket.Status
		if invoiceStatus != InvoiceStatusRejected {
			var others []*TicketInvoice
			if err := activeInvoiceOrders(tx).Where("user_id = ? AND id <> ? AND invoice_status <> ?", invoice.UserId, invoice.Id, InvoiceStatusRejected).Find(&others).Error; err != nil {
				return err
			}
			ids, err := invoice.GetTopUpOrderIDs()
			if err != nil {
				return err
			}
			selected := make(map[int]bool, len(ids))
			for _, id := range ids {
				selected[id] = true
			}
			for _, other := range others {
				otherIDs, err := other.GetTopUpOrderIDs()
				if err != nil {
					return err
				}
				for _, id := range otherIDs {
					if selected[id] {
						return ErrTicketInvoiceOrderDuplicate
					}
				}
			}
		}
		var charge InvoiceTaxCharge
		taxErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("ticket_id = ?", ticketId).Take(&charge).Error
		if taxErr != nil && !errors.Is(taxErr, gorm.ErrRecordNotFound) {
			return taxErr
		}
		if taxErr == nil {
			if invoiceStatus == InvoiceStatusIssued && charge.Status != "paid" {
				return ErrInvoiceTaxUnpaid
			}
			if invoiceStatus != InvoiceStatusRejected && charge.Status == "refunded" {
				return ErrInvoiceTaxUnpaid
			}
			if invoiceStatus == InvoiceStatusRejected && charge.Status == "paid" {
				if err := refundPaidInvoiceTax(tx, &charge); err != nil {
					return err
				}
				refundedQuota = charge.Quota
				if err := tx.Create(&TicketMessage{TicketId: ticketId, UserId: adminId, Role: common.RoleAdminUser, Content: fmt.Sprintf("Invoice tax refunded to balance: CNY %.2f; quota %d; order %s", float64(charge.FeeCents)/100, charge.Quota, charge.TradeNo), CreatedTime: common.GetTimestamp()}).Error; err != nil {
					return err
				}
			}
			invoice.TaxCharge = &charge
		}

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

		if err := tx.Model(&TicketInvoice{}).Where("id = ?", invoice.Id).Updates(invoiceUpdates).Error; err != nil {
			return err
		}
		if err := tx.Model(&Ticket{}).Where("id = ?", ticket.Id).Updates(ticketUpdates).Error; err != nil {
			return err
		}

		invoice.InvoiceStatus = invoiceStatus
		invoice.IssuedTime = invoiceUpdates["issued_time"].(int64)
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
	if refundedQuota > 0 {
		syncInvoiceTaxQuotaCache(invoice.UserId, refundedQuota)
		RecordLog(invoice.UserId, LogTypeSystem, fmt.Sprintf("Invoice tax refunded to balance: ticket %d, quota %d", ticketId, refundedQuota))
	}
	return &invoice, &ticket, prevStatus, nil
}

type InvoiceExportItem struct {
	InvoiceType   string  `json:"invoice_type"`
	TaxMode       string  `json:"tax_mode"`
	TaxRateBps    int     `json:"tax_rate_bps"`
	TaxFeeCents   int64   `json:"tax_fee_cents"`
	TicketId      int     `json:"ticket_id" gorm:"column:ticket_id"`
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
		Select("t.id AS ticket_id, ti.company_name, ti.tax_number, ti.email, ti.total_money, ti.top_up_order_ids, t.status, t.created_time, COALESCE(NULLIF(ti.invoice_type, ''), tc.invoice_type) AS invoice_type, tc.mode AS tax_mode, tc.rate_bps AS tax_rate_bps, CASE WHEN ti.invoice_type IS NOT NULL AND ti.invoice_type <> '' THEN ti.invoice_tax_cents ELSE tc.fee_cents END AS tax_fee_cents").
		Joins("INNER JOIN ticket_invoices ti ON ti.ticket_id = t.id").
		Joins("LEFT JOIN invoice_tax_charges tc ON tc.ticket_id = t.id").
		Where("ti.invoice_status <> ?", InvoiceStatusRejected).
		Where("ti.invoice_status <> ? OR t.status <> ?", InvoiceStatusPending, TicketStatusClosed).
		Where("tc.id IS NULL OR tc.status = ?", "paid").
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
