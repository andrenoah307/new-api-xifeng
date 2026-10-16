package model

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const FinancePolicyKey = "FinanceBillPolicy"

type FinancePolicy struct {
	InvoiceStatuses []int    `json:"invoice_statuses"`
	TopUpStatuses   []string `json:"topup_statuses"`
	PaymentMethods  []string `json:"payment_methods"`
	Sources         []string `json:"sources"`
}

func DefaultFinancePolicy() FinancePolicy {
	return FinancePolicy{InvoiceStatuses: []int{InvoiceStatusPending, InvoiceStatusIssued, InvoiceStatusRejected}, TopUpStatuses: []string{}, PaymentMethods: []string{}, Sources: []string{}}
}

func (p FinancePolicy) Valid() bool {
	for _, values := range [][]string{p.PaymentMethods, p.Sources} {
		if len(values) > 50 {
			return false
		}
		for i, value := range values {
			if len(value) > 50 || strings.TrimSpace(value) != value || slices.Contains(values[:i], value) {
				return false
			}
		}
	}
	if len(p.InvoiceStatuses) > 3 || len(p.TopUpStatuses) > 4 {
		return false
	}
	seen := make(map[int]bool)
	for _, status := range p.InvoiceStatuses {
		if !IsValidInvoiceStatus(status) || seen[status] {
			return false
		}
		seen[status] = true
	}
	for i, status := range p.TopUpStatuses {
		if !slices.Contains([]string{common.TopUpStatusPending, common.TopUpStatusSuccess, common.TopUpStatusFailed, common.TopUpStatusExpired}, status) || slices.Contains(p.TopUpStatuses[:i], status) {
			return false
		}
	}
	return true
}

// Read from the database on every request so narrowing access takes effect on all nodes.
func GetFinancePolicy() (FinancePolicy, error) {
	var option Option
	err := DB.Where(Option{Key: FinancePolicyKey}).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultFinancePolicy(), nil
	}
	if err != nil {
		return FinancePolicy{}, err
	}
	var policy FinancePolicy
	if err = common.UnmarshalJsonStr(option.Value, &policy); err != nil {
		return FinancePolicy{}, err
	}
	if !policy.Valid() {
		return FinancePolicy{}, errors.New("invalid financial access policy")
	}
	return policy, nil
}

func SaveFinancePolicy(policy FinancePolicy) error {
	if !policy.Valid() {
		return errors.New("invalid financial access policy")
	}
	raw, err := common.Marshal(policy)
	if err != nil {
		return err
	}
	return DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&Option{Key: FinancePolicyKey, Value: string(raw)}).Error
}

type FinancePolicyChoice struct {
	PaymentMethod string `json:"payment_method"`
	Source        string `json:"source"`
}

func GetFinancePolicyChoices(ctx context.Context) ([]FinancePolicyChoice, error) {
	choices := make([]FinancePolicyChoice, 0)
	err := DB.WithContext(ctx).Table("top_ups").Distinct("COALESCE(payment_method, '') AS payment_method", "COALESCE(source, '') AS source").Scan(&choices).Error
	return choices, err
}

type FinanceFilter struct {
	SkipTotal     bool
	BeforeID      int
	View          string
	Keyword       string
	Status        string
	InvoiceStatus int
	StartTime     int64
	EndTime       int64
}

type FinanceTopUp struct {
	Id            int     `json:"id"`
	UserId        int     `json:"-"`
	TradeNo       string  `json:"trade_no"`
	Money         float64 `json:"money"`
	PaymentMethod string  `json:"payment_method"`
	Source        string  `json:"source"`
	Status        string  `json:"status"`
	CreateTime    int64   `json:"create_time"`
	CompleteTime  int64   `json:"complete_time"`
}

type FinanceRecord struct {
	Id               int            `json:"id"`
	UserId           int            `json:"user_id"`
	Username         string         `json:"username"`
	TicketId         int            `json:"ticket_id"`
	CompanyName      string         `json:"company_name"`
	Money            float64        `json:"money"`
	InvoiceStatus    int            `json:"invoice_status"`
	Status           string         `json:"status"`
	CreatedTime      int64          `json:"created_time"`
	TopUpOrderIds    string         `json:"-"`
	TopUps           []FinanceTopUp `json:"topups" gorm:"-"`
	IncompleteOrders bool           `json:"incomplete_orders" gorm:"-"`
}

// Invoice totals are immutable application snapshots, never sums of joined rows.
func ListFinanceRecords(ctx context.Context, filter FinanceFilter, policy FinancePolicy, page *common.PageInfo) ([]FinanceRecord, int64, error) {
	db := DB.WithContext(ctx)
	items := make([]FinanceRecord, 0)
	var query *gorm.DB
	var timeColumn, orderColumn string
	switch filter.View {
	case "invoices":
		if len(policy.InvoiceStatuses) == 0 {
			return items, 0, nil
		}
		query = db.Table("ticket_invoices AS ti").
			Joins("JOIN tickets AS t ON t.id = ti.ticket_id AND t.user_id = ti.user_id AND t.deleted_at IS NULL").
			Joins("LEFT JOIN users AS u ON u.id = ti.user_id").
			Where("t.type = ? AND ti.invoice_status IN ?", TicketTypeInvoice, policy.InvoiceStatuses).
			Select("ti.id, ti.user_id, COALESCE(u.username, t.username) AS username, ti.ticket_id, ti.company_name, ti.total_money AS money, ti.invoice_status, ti.created_time, ti.top_up_order_ids")
		timeColumn, orderColumn = "ti.created_time", "ti.id"
		if filter.InvoiceStatus != 0 {
			query = query.Where("ti.invoice_status = ?", filter.InvoiceStatus)
		}
	case "topups":
		if len(policy.TopUpStatuses) == 0 {
			return items, 0, nil
		}
		query = db.Table("top_ups AS tu").Joins("LEFT JOIN users AS u ON u.id = tu.user_id").
			Where("tu.status IN ?", policy.TopUpStatuses).
			Select("tu.id, tu.user_id, COALESCE(u.username, '') AS username, tu.money, tu.status, tu.create_time AS created_time")
		timeColumn, orderColumn = "tu.create_time", "tu.id"
		if filter.Status != "" {
			query = query.Where("tu.status = ?", filter.Status)
		}
		if len(policy.PaymentMethods) > 0 {
			query = query.Where("COALESCE(tu.payment_method, '') IN ?", policy.PaymentMethods)
		}
		if len(policy.Sources) > 0 {
			query = query.Where("COALESCE(tu.source, '') IN ?", policy.Sources)
		}
	default:
		return nil, 0, errors.New("invalid financial record view")
	}
	if filter.StartTime > 0 {
		query = query.Where(timeColumn+" >= ?", filter.StartTime)
	}
	if filter.BeforeID > 0 {
		query = query.Where(orderColumn+" < ?", filter.BeforeID)
	}
	if filter.EndTime > 0 {
		query = query.Where(timeColumn+" <= ?", filter.EndTime)
	}
	if filter.Keyword != "" {
		pattern, err := sanitizeLikePattern(filter.Keyword)
		if err != nil {
			return nil, 0, err
		}
		id, _ := strconv.Atoi(filter.Keyword)
		if filter.View == "invoices" {
			query = query.Where("(ti.company_name LIKE ? ESCAPE '!' OR u.username LIKE ? ESCAPE '!' OR ti.user_id = ? OR ti.ticket_id = ?)", pattern, pattern, id, id)
		} else {
			query = query.Where("(tu.trade_no LIKE ? ESCAPE '!' OR u.username LIKE ? ESCAPE '!' OR tu.user_id = ?)", pattern, pattern, id)
		}
	}
	var total int64
	if filter.View == "invoices" && (len(policy.PaymentMethods) > 0 || len(policy.Sources) > 0) {
		// Historical associations are JSON text. Parse bounded batches rather than
		// using database-specific JSON operators or partially exposing an invoice.
		cursor := 0
		for {
			var batch []FinanceRecord
			batchQuery := query.Session(&gorm.Session{}).Order("ti.id DESC").Limit(100)
			if cursor > 0 {
				batchQuery = batchQuery.Where("ti.id < ?", cursor)
			}
			if err := batchQuery.Scan(&batch).Error; err != nil {
				return nil, 0, err
			}
			if len(batch) == 0 {
				break
			}
			if err := hydrateFinanceOrders(db, batch, true); err != nil {
				return nil, 0, err
			}
			for _, item := range batch {
				if !policy.allowsInvoice(item) {
					continue
				}
				if total >= int64(page.GetStartIdx()) && len(items) < page.GetPageSize() {
					items = append(items, item)
				}
				total++
				if filter.SkipTotal && len(items) == page.GetPageSize() {
					return items, total, nil
				}
			}
			cursor = batch[len(batch)-1].Id
		}
		return items, total, nil
	}
	if !filter.SkipTotal {
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, 0, err
		}
	}
	if err := query.Order(orderColumn + " DESC").Offset(page.GetStartIdx()).Limit(page.GetPageSize()).Scan(&items).Error; err != nil {
		return nil, 0, err
	}
	if len(items) == 0 {
		return items, total, nil
	}
	if err := hydrateFinanceOrders(db, items, filter.View == "invoices"); err != nil {
		return nil, 0, err
	}
	if filter.View == "topups" && len(policy.InvoiceStatuses) > 0 {
		userIDs := make([]int, 0, len(items))
		for _, item := range items {
			userIDs = append(userIDs, item.UserId)
		}
		var invoices []FinanceRecord
		if err := db.Table("ticket_invoices AS ti").Select("ti.id, ti.user_id, ti.company_name, ti.top_up_order_ids").Joins("JOIN tickets AS t ON t.id = ti.ticket_id AND t.user_id = ti.user_id AND t.deleted_at IS NULL").Where("ti.user_id IN ? AND ti.invoice_status IN ? AND t.type = ?", userIDs, policy.InvoiceStatuses, TicketTypeInvoice).Order("ti.id DESC").Find(&invoices).Error; err != nil {
			return nil, 0, err
		}
		if err := hydrateFinanceOrders(db, invoices, true); err != nil {
			return nil, 0, err
		}
		titles := make(map[string][]string)
		for _, invoice := range invoices {
			if !policy.allowsInvoice(invoice) {
				continue
			}
			for _, order := range invoice.TopUps {
				key := fmt.Sprintf("%d:%d", invoice.UserId, order.Id)
				if !slices.Contains(titles[key], invoice.CompanyName) {
					titles[key] = append(titles[key], invoice.CompanyName)
				}
			}
		}
		for i := range items {
			items[i].CompanyName = strings.Join(titles[fmt.Sprintf("%d:%d", items[i].UserId, items[i].Id)], " / ")
		}
	}
	return items, total, nil
}

func (p FinancePolicy) allowsInvoice(item FinanceRecord) bool {
	if item.IncompleteOrders || len(item.TopUps) == 0 {
		return false
	}
	for _, order := range item.TopUps {
		if len(p.PaymentMethods) > 0 && !slices.Contains(p.PaymentMethods, order.PaymentMethod) {
			return false
		}
		if len(p.Sources) > 0 && !slices.Contains(p.Sources, order.Source) {
			return false
		}
	}
	return true
}

func hydrateFinanceOrders(db *gorm.DB, items []FinanceRecord, invoices bool) error {
	ids := make([]int, 0)
	ordersByRecord := make(map[int][]int, len(items))
	for i := range items {
		item := &items[i]
		item.TopUps = make([]FinanceTopUp, 0)
		orderIDs := []int{item.Id}
		if invoices {
			invoice := TicketInvoice{TopUpOrderIds: item.TopUpOrderIds}
			var err error
			orderIDs, err = invoice.GetTopUpOrderIDs()
			if err != nil {
				item.IncompleteOrders = true
				continue
			}
		}
		ordersByRecord[item.Id] = orderIDs
		ids = append(ids, orderIDs...)
	}
	var orders []FinanceTopUp
	for start := 0; start < len(ids); start += 500 {
		var batch []FinanceTopUp
		if err := db.Table("top_ups").Select("id, user_id, trade_no, money, payment_method, source, status, create_time, complete_time").Where("id IN ?", ids[start:min(start+500, len(ids))]).Find(&batch).Error; err != nil {
			return err
		}
		orders = append(orders, batch...)
	}
	byID := make(map[int]FinanceTopUp, len(orders))
	for _, order := range orders {
		byID[order.Id] = order
	}
	for i := range items {
		for _, id := range ordersByRecord[items[i].Id] {
			order, ok := byID[id]
			if !ok || order.UserId != items[i].UserId {
				items[i].IncompleteOrders = true
				continue
			}
			items[i].TopUps = append(items[i].TopUps, order)
		}
	}
	return nil
}
