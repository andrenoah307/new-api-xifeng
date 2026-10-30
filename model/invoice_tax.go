package model

import (
	"errors"
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const InvoiceTaxConfigKey = "InvoiceTaxConfig"

// Rates use basis points (600 = 6%). Monetary calculations use integer cents.
type InvoiceTaxConfig struct {
	ModuleEnabled       bool   `json:"module_enabled"`
	LegacyTopUpsEnabled bool   `json:"legacy_topups_enabled"`
	Enabled             bool   `json:"enabled"`
	Mode                string `json:"mode"`
	OrdinaryRateBps     int    `json:"ordinary_rate_bps"`
	SpecialRateBps      int    `json:"special_rate_bps"`
}

type InvoiceTaxQuote struct {
	PrepaidOnly     bool   `json:"prepaid_only"`
	PrepaidNetCents int64  `json:"prepaid_net_cents"`
	PrepaidTaxCents int64  `json:"prepaid_tax_cents"`
	InvoiceType     string `json:"invoice_type"`
	Mode            string `json:"mode"`
	RateBps         int    `json:"rate_bps"`
	BaseCents       int64  `json:"base_cents"`
	FeeCents        int64  `json:"fee_cents"`
	TotalCents      int64  `json:"total_cents"`
	Quota           int    `json:"quota"`
	CanBalance      bool   `json:"can_balance"`
	Token           string `json:"token"`
}

var ErrInvoiceTaxInvalid = errors.New("invalid invoice tax parameters")

var (
	ErrInvoiceTaxQuoteChanged = errors.New("invoice tax quote changed")
	ErrInvoiceTaxUnpaid       = errors.New("invoice tax payment required")
	ErrInvoiceTaxCurrency     = errors.New("invoice currency cannot be confirmed as CNY")
)

type InvoiceTaxCharge struct {
	Id            int    `json:"id"`
	TicketId      int    `json:"ticket_id" gorm:"uniqueIndex;not null"`
	UserId        int    `json:"user_id" gorm:"index;not null"`
	TradeNo       string `json:"trade_no" gorm:"size:64;uniqueIndex;not null"`
	InvoiceType   string `json:"invoice_type" gorm:"size:16"`
	Mode          string `json:"mode" gorm:"size:16"`
	RateBps       int    `json:"rate_bps"`
	BaseCents     int64  `json:"base_cents"`
	FeeCents      int64  `json:"fee_cents"`
	TotalCents    int64  `json:"total_cents"`
	Quota         int    `json:"quota"`
	PaymentMethod string `json:"payment_method" gorm:"size:16"`
	Status        string `json:"status" gorm:"size:16"`
	CreatedTime   int64  `json:"created_time"`
	PaidTime      int64  `json:"paid_time"`
	RefundedTime  int64  `json:"refunded_time"`
}

func IsCNYInvoiceTopUp(order *TopUp) bool {
	if order.PaymentProvider == "manual" && order.CheckoutVersion == 1 && order.CreatedBy > 0 {
		return true
	}
	return order.PaymentProvider == PaymentProviderEpay || (order.PaymentProvider == "" && (order.PaymentMethod == "alipay" || order.PaymentMethod == "wxpay"))
}

func QuoteInvoiceTax(db *gorm.DB, userID int, orderIDs []int, invoiceType string) (InvoiceTaxQuote, error) {
	var empty InvoiceTaxQuote
	if len(orderIDs) > 500 {
		return empty, ErrInvoiceTaxInvalid
	}
	ids, err := normalizeTopUpOrderIDs(orderIDs)
	if err != nil {
		return empty, err
	}
	config, err := GetInvoiceTaxConfig(db)
	if err != nil {
		return empty, err
	}
	if !config.ModuleEnabled {
		config.Enabled = false
		config.LegacyTopUpsEnabled = true
	}
	if invoiceType == "" && !config.Enabled {
		invoiceType = "ordinary"
	}
	if invoiceType != "ordinary" && invoiceType != "special" {
		return empty, ErrInvoiceTaxInvalid
	}
	orders, err := fetchSuccessTopUpsByIds(db, userID, ids)
	if err != nil {
		return empty, err
	}
	if len(orders) != len(ids) {
		return empty, ErrTicketInvoiceOrderInvalid
	}
	var cents, prepaidNet, prepaidTax int64
	for _, order := range orders {
		if order.InvoiceType != "" {
			if order.InvoiceType != invoiceType {
				return empty, ErrTicketInvoiceOrderInvalid
			}
			switch order.CheckoutVersion {
			case 1:
				if order.InvoiceNetCents <= 0 || order.InvoiceTaxCents < 0 || order.InvoiceNetCents+order.InvoiceTaxCents != order.PaymentTotalCents {
					return empty, ErrTicketInvoiceOrderInvalid
				}
				prepaidNet += order.InvoiceNetCents
				prepaidTax += order.InvoiceTaxCents
			case 0:
				// An explicit type on a legacy recharge is an administrator's eligibility grant.
				// Keep the original payment amount; do not synthesize tax or debit the wallet.
				if !IsCNYInvoiceTopUp(order) {
					return empty, ErrInvoiceTaxCurrency
				}
				amount, err := invoiceMoneyCents(order.Money)
				if err != nil {
					return empty, err
				}
				prepaidNet += amount
			default:
				return empty, ErrTicketInvoiceOrderInvalid
			}
			continue
		}
		if !config.LegacyTopUpsEnabled {
			return empty, ErrTicketInvoiceOrderInvalid
		}
		if config.ModuleEnabled && !IsCNYInvoiceTopUp(order) {
			return empty, ErrInvoiceTaxCurrency
		}
		amount, err := invoiceMoneyCents(order.Money)
		if err != nil {
			return empty, err
		}
		cents += amount
	}
	quote := InvoiceTaxQuote{InvoiceType: invoiceType, PrepaidOnly: cents == 0}
	if cents > 0 {
		quote, err = CalculateInvoiceTax(cents, invoiceType, config)
		if err != nil {
			return quote, err
		}
	}
	quote.PrepaidNetCents, quote.PrepaidTaxCents = prepaidNet, prepaidTax
	quote.TotalCents += prepaidNet + prepaidTax
	// The configured business convention is CNY 1 = one USD quota unit.
	// Store the converted quota on the charge so later settings cannot alter refunds.
	if math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || common.QuotaPerUnit <= 0 {
		return empty, ErrInvoiceTaxInvalid
	}
	quota, clamp := common.QuotaFromDecimalChecked(decimal.NewFromInt(quote.FeeCents).Div(decimal.NewFromInt(100)).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Ceil())
	if clamp != nil {
		return empty, ErrInvoiceTaxInvalid
	}
	quote.Quota = quota
	raw, err := common.Marshal(struct {
		Quote  InvoiceTaxQuote
		Orders []int
	}{quote, ids})
	if err != nil {
		return empty, err
	}
	quote.Token = common.Sha1(raw)
	var user User
	if err = db.Select("quota").First(&user, userID).Error; err != nil {
		return empty, err
	}
	quote.CanBalance = user.Quota >= quota
	return quote, nil
}

func GetInvoiceTaxCharge(ticketID int) (*InvoiceTaxCharge, error) {
	var charge InvoiceTaxCharge
	err := DB.Where("ticket_id = ?", ticketID).Take(&charge).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &charge, err
}

// Payment and invoice state changes share the same lock order: user, invoice, charge.
// Only the signed gateway callback may settle an online charge.
func SettleInvoiceTax(tradeNo, method string, paidCents int64) error {
	var known InvoiceTaxCharge
	if err := DB.Where("trade_no = ?", tradeNo).Take(&known).Error; err != nil {
		return err
	}
	refunded := false
	settled := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, known.UserId).Error; err != nil {
			return err
		}
		var invoice TicketInvoice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("ticket_id = ?", known.TicketId).Take(&invoice).Error; err != nil {
			return err
		}
		var charge InvoiceTaxCharge
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&charge, known.Id).Error; err != nil {
			return err
		}
		if charge.FeeCents != paidCents || charge.PaymentMethod != method || (method != "alipay" && method != "wxpay") {
			return ErrInvoiceTaxInvalid
		}
		if charge.Status == "paid" || charge.Status == "refunded" {
			return nil
		}
		if charge.Status != "pending" {
			return ErrInvoiceTaxInvalid
		}
		now := common.GetTimestamp()
		result := tx.Model(&InvoiceTaxCharge{}).Where("id = ? AND status = ?", charge.Id, "pending").Updates(map[string]interface{}{"status": "paid", "paid_time": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceTaxInvalid
		}
		charge.Status = "paid"
		if invoice.InvoiceStatus == InvoiceStatusRejected {
			if err := refundPaidInvoiceTax(tx, &charge); err != nil {
				return err
			}
			refunded = true
		}
		settled = true
		return tx.Create(&TicketMessage{TicketId: charge.TicketId, UserId: charge.UserId, Role: common.RoleCommonUser, Content: fmt.Sprintf("Invoice tax payment: CNY %.2f; %s; order %s", float64(charge.FeeCents)/100, charge.Status, charge.TradeNo), CreatedTime: now}).Error
	})
	if err == nil && refunded {
		syncInvoiceTaxQuotaCache(known.UserId, known.Quota)
	}
	if err == nil && settled {
		RecordLog(known.UserId, LogTypeSystem, fmt.Sprintf("Invoice tax online payment: ticket %d, CNY %.2f, refunded to balance: %t", known.TicketId, float64(known.FeeCents)/100, refunded))
	}
	return err
}

// The caller holds user, invoice and charge locks in that order. Both the
// ledger CAS and bounded wallet credit must commit in the same transaction.
func refundPaidInvoiceTax(tx *gorm.DB, charge *InvoiceTaxCharge) error {
	if charge.Status != "paid" || charge.Quota < 0 || charge.Quota > common.MaxQuota {
		return ErrInvoiceTaxInvalid
	}
	now := common.GetTimestamp()
	result := tx.Model(&InvoiceTaxCharge{}).Where("id = ? AND status = ?", charge.Id, "paid").Updates(map[string]interface{}{"status": "refunded", "refunded_time": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvoiceTaxInvalid
	}
	if charge.Quota > 0 {
		result = tx.Model(&User{}).Where("id = ? AND quota <= ?", charge.UserId, math.MaxInt64-int64(charge.Quota)).Update("quota", gorm.Expr("quota + ?", charge.Quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceTaxInvalid
		}
	}
	charge.Status = "refunded"
	charge.RefundedTime = now
	return nil
}

func syncInvoiceTaxQuotaCache(userID, delta int) {
	if err := cacheIncrUserQuota(userID, int64(delta)); err != nil && !errors.Is(err, common.ErrRedisKeyMiss) {
		common.SysError("invoice tax quota cache update failed: " + err.Error())
	}
}

func (config InvoiceTaxConfig) Valid() bool {
	return (config.Mode == "vat" || config.Mode == "compound" || config.Mode == "gross_up") &&
		config.OrdinaryRateBps >= 0 && config.OrdinaryRateBps < 10000 &&
		config.SpecialRateBps >= 0 && config.SpecialRateBps < 10000
}

func GetInvoiceTaxConfig(db *gorm.DB) (InvoiceTaxConfig, error) {
	config := InvoiceTaxConfig{Mode: "vat"}
	var option Option
	err := db.Where(Option{Key: InvoiceTaxConfigKey}).Take(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	if err = common.UnmarshalJsonStr(option.Value, &config); err != nil {
		return config, err
	}
	if !config.Valid() {
		return config, ErrInvoiceTaxInvalid
	}
	return config, nil
}

func SaveInvoiceTaxConfig(config InvoiceTaxConfig) error {
	if !config.Valid() {
		return ErrInvoiceTaxInvalid
	}
	raw, err := common.Marshal(config)
	if err != nil {
		return err
	}
	if err := DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&Option{Key: InvoiceTaxConfigKey, Value: string(raw)}).Error; err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[InvoiceTaxConfigKey] = string(raw)
	common.OptionMapRWMutex.Unlock()
	return nil
}

func CalculateInvoiceTax(baseCents int64, invoiceType string, config InvoiceTaxConfig) (InvoiceTaxQuote, error) {
	quote := InvoiceTaxQuote{InvoiceType: invoiceType, Mode: config.Mode, BaseCents: baseCents}
	if !config.Valid() || baseCents <= 0 || baseCents > 100000000000 || (invoiceType != "ordinary" && invoiceType != "special") {
		return quote, ErrInvoiceTaxInvalid
	}
	quote.RateBps = config.OrdinaryRateBps
	if invoiceType == "special" {
		quote.RateBps = config.SpecialRateBps
	}
	if !config.Enabled {
		quote.RateBps = 0
	}
	base := decimal.NewFromInt(baseCents)
	rate := decimal.NewFromInt(int64(quote.RateBps)).Div(decimal.NewFromInt(10000))
	one := decimal.NewFromInt(1)
	var total decimal.Decimal
	switch config.Mode {
	case "vat":
		total = base.Mul(one.Add(rate))
	case "compound":
		total = base.Mul(one.Add(rate)).Mul(one.Add(rate))
	case "gross_up":
		total = base.DivRound(one.Sub(rate), 8)
	}
	// Round the final amount once, then derive the amount due, preserving the
	// accounting invariant base + fee = total even at fractional-cent rates.
	total = total.Round(0)
	if total.LessThan(base) || total.GreaterThan(decimal.NewFromInt(100000000000)) {
		return quote, ErrInvoiceTaxInvalid
	}
	quote.TotalCents = total.IntPart()
	quote.FeeCents = quote.TotalCents - baseCents
	return quote, nil
}

// CalculateInvoiceTaxFromTotal keeps the user's final payment fixed. The net
// amount is rounded to cents, and the remainder is the invoice charge.
func CalculateInvoiceTaxFromTotal(totalCents int64, invoiceType string, config InvoiceTaxConfig) (InvoiceTaxQuote, error) {
	quote, err := CalculateInvoiceTax(1, invoiceType, config)
	if err != nil || totalCents <= 0 || totalCents > 100000000000 {
		return InvoiceTaxQuote{}, ErrInvoiceTaxInvalid
	}
	total := decimal.NewFromInt(totalCents)
	rate := decimal.NewFromInt(int64(quote.RateBps)).Div(decimal.NewFromInt(10000))
	one := decimal.NewFromInt(1)
	var base decimal.Decimal
	switch config.Mode {
	case "vat":
		base = total.DivRound(one.Add(rate), 16)
	case "compound":
		base = total.DivRound(one.Add(rate).Mul(one.Add(rate)), 16)
	case "gross_up":
		base = total.Mul(one.Sub(rate))
	}
	base = base.Round(0)
	if base.LessThanOrEqual(decimal.Zero) || base.GreaterThan(total) {
		return InvoiceTaxQuote{}, ErrInvoiceTaxInvalid
	}
	quote.BaseCents = base.IntPart()
	quote.TotalCents = totalCents
	quote.FeeCents = totalCents - quote.BaseCents
	return quote, nil
}

func invoiceMoneyCents(money float64) (int64, error) {
	if math.IsNaN(money) || math.IsInf(money, 0) || money <= 0 || money > 1000000000 {
		return 0, ErrInvoiceTaxInvalid
	}
	return decimal.NewFromFloat(money).Mul(decimal.NewFromInt(100)).Round(0).IntPart(), nil
}
