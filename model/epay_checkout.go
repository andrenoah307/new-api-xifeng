package model

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type EpayCheckoutRequest struct {
	TotalCents    int64  `json:"total_cents"`
	InvoiceType   string `json:"invoice_type"`
	PaymentMethod string `json:"payment_method"`
	DiscountCode  string `json:"discount_code"`
	Token         string `json:"token"`
	RequestID     string `json:"request_id"`
}

// Checkout quotas are JSON numbers consumed by browsers as well as bigint credits.
const maxEpayCheckoutQuota = 1<<53 - 1

type EpayCheckoutQuote struct {
	PolicyEffectiveAt int64               `json:"policy_effective_at"`
	VATRateBps        int                 `json:"vat_rate_bps"`
	UnitPrice         string              `json:"-"`
	QuotaPerUnit      string              `json:"-"`
	InvoiceType       string              `json:"invoice_type"`
	Mode              string              `json:"mode"`
	RateBps           int                 `json:"rate_bps"`
	TotalCents        int64               `json:"total_cents"`
	NetCents          int64               `json:"net_cents"`
	TaxCents          int64               `json:"tax_cents"`
	BaseQuota         int                 `json:"base_quota"`
	Quota             int                 `json:"quota"`
	CreditAmount      string              `json:"credit_amount"`
	Amount            int64               `json:"-"`
	DiscountCodeID    int                 `json:"discount_code_id"`
	DiscountRate      int                 `json:"discount_rate"`
	Token             string              `json:"token"`
	Routes            []InvoiceEpayConfig `json:"-"`
	Strategy          string              `json:"-"`
}

func CalculateEpayCheckout(totalCents int64, invoiceType string, config InvoiceTaxConfig, price decimal.Decimal, discountRate int) (EpayCheckoutQuote, error) {
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return EpayCheckoutQuote{}, ErrInvoiceTaxInvalid
	}
	return calculateEpayCheckout(totalCents, invoiceType, config, price, discountRate, decimal.NewFromFloat(common.QuotaPerUnit))
}

func calculateEpayCheckout(totalCents int64, invoiceType string, config InvoiceTaxConfig, price decimal.Decimal, discountRate int, quotaPerUnit decimal.Decimal) (EpayCheckoutQuote, error) {
	quote := EpayCheckoutQuote{InvoiceType: invoiceType, TotalCents: totalCents, NetCents: totalCents, DiscountRate: discountRate, PolicyEffectiveAt: config.PolicyEffectiveAt, VATRateBps: InvoiceVATRateBps, UnitPrice: price.String(), QuotaPerUnit: quotaPerUnit.String()}
	if totalCents <= 0 || totalCents > 100000000000 || !price.IsPositive() || !quotaPerUnit.IsPositive() || discountRate < 1 || discountRate > 100 {
		return quote, ErrInvoiceTaxInvalid
	}
	if invoiceType != "none" {
		tax, err := CalculateInvoiceTaxFromTotal(totalCents, invoiceType, config)
		if err != nil {
			return quote, err
		}
		quote.NetCents, quote.TaxCents = tax.BaseCents, tax.FeeCents
		quote.Mode, quote.RateBps = tax.Mode, tax.RateBps
	}
	// Multiply before dividing, then round once to whole quota units. Do not
	// pass wallet credits through the float64/int32 relay-charge converter.
	base := decimal.NewFromInt(quote.NetCents).Mul(quotaPerUnit).DivRound(price.Mul(decimal.NewFromInt(100)), 0)
	var err error
	quote.BaseQuota, err = common.RechargeQuotaFromDecimal(base)
	if err != nil || quote.BaseQuota > maxEpayCheckoutQuota {
		return quote, ErrInvoiceTaxInvalid
	}
	quote.Quota, err = common.RechargeQuotaFromDecimal(decimal.NewFromInt(int64(quote.BaseQuota)).Mul(decimal.NewFromInt(100)).DivRound(decimal.NewFromInt(int64(discountRate)), 0))
	if err != nil || quote.Quota > maxEpayCheckoutQuota {
		return quote, ErrInvoiceTaxInvalid
	}
	quote.Amount, quote.CreditAmount, err = checkoutCreditAmount(quote.Quota, quotaPerUnit)
	if err != nil {
		return quote, err
	}
	return quote, nil
}

// Amount is a legacy whole-credit field; financial settlement uses integer
// quota and cents. Bound it before IntPart and format without double rounding.
func checkoutCreditAmount(quota int, quotaPerUnit decimal.Decimal) (int64, string, error) {
	if quota <= 0 || !quotaPerUnit.IsPositive() {
		return 0, "", ErrInvoiceTaxInvalid
	}
	credit := decimal.NewFromInt(int64(quota))
	whole, _ := credit.QuoRem(quotaPerUnit, 0)
	if whole.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return 0, "", ErrInvoiceTaxInvalid
	}
	return whole.IntPart(), credit.DivRound(quotaPerUnit, 2).StringFixed(2), nil
}

// Creation calls this while holding the user and optional discount-code locks.
func QuoteEpayCheckout(db *gorm.DB, userID int, req EpayCheckoutRequest, reserve bool) (EpayCheckoutQuote, error) {
	var empty EpayCheckoutQuote
	if !operation_setting.IsPaymentComplianceConfirmed() || (req.PaymentMethod != "alipay" && req.PaymentMethod != "wxpay") || req.TotalCents < int64(operation_setting.MinTopUp)*100 {
		return empty, ErrInvoiceTaxInvalid
	}
	var user User
	if err := db.First(&user, userID).Error; err != nil {
		return empty, err
	}
	config, err := GetInvoiceTaxConfig(db)
	if err != nil {
		return empty, err
	}
	if !config.ModuleEnabled {
		return empty, ErrTicketForbidden
	}
	routingDB := db
	if reserve {
		routingDB = lockForUpdate(db)
	}
	routes, strategy, err := GetEpayCheckoutCandidates(routingDB, req.InvoiceType, req.TotalCents)
	if err != nil {
		return empty, err
	}
	eligible := []InvoiceEpayConfig{}
	for _, route := range routes {
		if slices.Contains(route.CheckoutMethods(), req.PaymentMethod) {
			eligible = append(eligible, route)
		}
	}
	if len(eligible) == 0 {
		return empty, ErrInvoiceTaxInvalid
	}
	groupRatio := common.GetTopupGroupRatio(user.Group)
	if groupRatio == 0 {
		groupRatio = 1
	}
	presetDiscount := 1.0
	if req.TotalCents%100 == 0 {
		if ratio, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(req.TotalCents/100)]; ok {
			presetDiscount = ratio
		}
	}
	for _, value := range []float64{operation_setting.Price, groupRatio, presetDiscount} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return empty, ErrInvoiceTaxInvalid
		}
	}
	rate := 100
	var dc DiscountCode
	if req.DiscountCode != "" {
		if !operation_setting.IsDiscountCodeEnabled() {
			return empty, ErrInvoiceTaxInvalid
		}
		query := db
		if reserve {
			query = lockForUpdate(db)
		}
		if err := query.Where("code = ?", req.DiscountCode).First(&dc).Error; err != nil {
			return empty, err
		}
		if err := dc.validateAvailability(db, userID); err != nil {
			return empty, err
		}
		rate = dc.DiscountRate
	}
	price := decimal.NewFromFloat(operation_setting.Price).Mul(decimal.NewFromFloat(groupRatio)).Mul(decimal.NewFromFloat(presetDiscount))
	quote, err := CalculateEpayCheckout(req.TotalCents, req.InvoiceType, config.CheckoutAt(common.GetTimestamp()), price, rate)
	if err != nil {
		return empty, err
	}
	if dc.Id > 0 && dc.MaxAmount > 0 && decimal.NewFromInt(int64(quote.Quota)).Div(decimal.NewFromFloat(common.QuotaPerUnit)).GreaterThan(decimal.NewFromInt(dc.MaxAmount)) {
		return empty, ErrInvoiceTaxInvalid
	}
	quote.DiscountCodeID = dc.Id
	quote.Routes, quote.Strategy = eligible, strategy
	raw, err := common.Marshal(struct {
		Quote        EpayCheckoutQuote
		UnitPrice    string
		QuotaPerUnit string
		UserID       int
		Method       string
		Routes       []InvoiceEpayConfig
		Strategy     string
	}{quote, quote.UnitPrice, quote.QuotaPerUnit, userID, req.PaymentMethod, eligible, strategy})
	if err != nil {
		return empty, err
	}
	quote.Token = common.Sha1(raw)
	return quote, nil
}

var checkoutRequestIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{16,48}$`)

func CreateEpayCheckout(userID int, req EpayCheckoutRequest) (*TopUp, error) {
	if !checkoutRequestIDPattern.MatchString(req.RequestID) {
		return nil, ErrInvoiceTaxInvalid
	}
	legacyTradeNo := "CHECKOUT" + decimal.NewFromInt(int64(userID)).String() + "-" + req.RequestID
	var order TopUp
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, userID).Error; err != nil {
			return err
		}
		// The external order number stays short; request identity is stored
		// separately. Also recognize pending orders created before the upgrade.
		err := tx.Where("user_id = ? AND (checkout_request_id = ? OR trade_no = ?)", userID, req.RequestID, legacyTradeNo).Take(&order).Error
		if err == nil {
			if order.UserId != userID || order.CheckoutToken != req.Token || order.PaymentTotalCents != req.TotalCents || order.InvoiceType != req.InvoiceType || order.PaymentMethod != req.PaymentMethod || order.Status != common.TopUpStatusPending {
				return ErrInvoiceTaxInvalid
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		quote, err := QuoteEpayCheckout(tx, userID, req, true)
		if err != nil {
			return err
		}
		if req.Token == "" || quote.Token != req.Token {
			return ErrInvoiceTaxQuoteChanged
		}
		selected, err := selectEpayCheckoutGateway(tx, quote.Routes, quote.Strategy, req.PaymentMethod)
		if err != nil {
			return err
		}
		credentials, err := common.Marshal(selected)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		order = TopUp{
			UserId: userID, TradeNo: fmt.Sprintf("USR%dNO%s%d", userID, common.GetRandomString(6), now), Status: common.TopUpStatusPending,
			PaymentProvider: PaymentProviderEpay, PaymentMethod: req.PaymentMethod,
			Money:      float64(quote.TotalCents) / 100,
			Amount:     quote.Amount,
			CreateTime: now, DiscountCodeId: quote.DiscountCodeID, DiscountRate: quote.DiscountRate,
			CheckoutVersion: 1, CheckoutToken: req.Token, CheckoutRequestID: &req.RequestID, CheckoutCreditAmount: quote.CreditAmount,
			CheckoutPolicyEffectiveAt: quote.PolicyEffectiveAt, CheckoutUnitPrice: quote.UnitPrice,
			CheckoutQuotaPerUnit: quote.QuotaPerUnit,
			InvoiceType:          quote.InvoiceType, InvoiceTaxMode: quote.Mode, InvoiceRateBps: quote.RateBps,
			InvoiceNetCents: quote.NetCents, InvoiceTaxCents: quote.TaxCents, PaymentTotalCents: quote.TotalCents,
			CheckoutBaseQuota: quote.BaseQuota, CheckoutQuota: quote.Quota, EpayCredentials: string(credentials),
		}
		// Credential-bearing SQL must not be interpolated into the ORM error log.
		result := tx.Session(&gorm.Session{Logger: gormlogger.Discard}).Create(&order)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceTaxInvalid
		}
		return nil
	})
	return &order, err
}

func CompleteEpayCheckout(tradeNo, method string, paidCents int64) (*TopUp, int, error) {
	return completeEpayCheckoutAt(tradeNo, method, paidCents, common.GetTimestamp())
}

func completeEpayCheckoutAt(tradeNo, method string, paidCents, now int64) (*TopUp, int, error) {
	var order TopUp
	credited, bonus := 0, 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).Take(&order).Error; err != nil {
			return err
		}
		if order.CheckoutVersion != 1 || order.PaymentProvider != PaymentProviderEpay || order.PaymentMethod != method || order.PaymentTotalCents != paidCents || paidCents <= 0 || paidCents > 100000000000 || order.InvoiceNetCents <= 0 || order.InvoiceTaxCents < 0 || order.InvoiceTaxCents > paidCents || order.InvoiceNetCents != paidCents-order.InvoiceTaxCents || order.CheckoutBaseQuota <= 0 || order.CheckoutQuota < order.CheckoutBaseQuota || order.CheckoutQuota > maxEpayCheckoutQuota {
			return ErrInvoiceTaxInvalid
		}
		if order.Status == common.TopUpStatusSuccess {
			return nil
		}
		if order.Status != common.TopUpStatusPending && order.Status != common.TopUpStatusExpired {
			return ErrTopUpStatusInvalid
		}
		if order.CheckoutPolicyEffectiveAt == 0 {
			config, err := GetInvoiceTaxConfig(tx)
			if err != nil {
				return err
			}
			if config.PolicyEffectiveAt > 0 {
				if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
					return ErrInvoiceTaxInvalid
				}
				if order.InvoiceType != "ordinary" && order.InvoiceType != "special" && order.InvoiceType != "none" {
					return ErrInvoiceTaxInvalid
				}
				// Older pending checkouts lack a unit-price snapshot. Preserve their
				// original quota-per-net-cent ratio instead of using today's price.
				quote, err := CalculateInvoiceTaxFromTotal(order.PaymentTotalCents, "ordinary", config.CheckoutAt(now))
				if order.InvoiceType == "special" {
					quote, err = CalculateInvoiceTaxFromTotal(order.PaymentTotalCents, "special", config.CheckoutAt(now))
				}
				if err != nil {
					return err
				}
				if order.InvoiceType == "none" {
					quote.BaseCents, quote.FeeCents, quote.RateBps = order.PaymentTotalCents, 0, 0
				}
				base, err := common.RechargeQuotaFromDecimal(decimal.NewFromInt(int64(order.CheckoutBaseQuota)).Mul(decimal.NewFromInt(quote.BaseCents)).DivRound(decimal.NewFromInt(order.InvoiceNetCents), 0))
				if err != nil || base > maxEpayCheckoutQuota {
					return ErrInvoiceTaxInvalid
				}
				total, err := common.RechargeQuotaFromDecimal(decimal.NewFromInt(int64(order.CheckoutQuota)).Mul(decimal.NewFromInt(quote.BaseCents)).DivRound(decimal.NewFromInt(order.InvoiceNetCents), 0))
				if err != nil || total < base || total > maxEpayCheckoutQuota {
					return ErrInvoiceTaxInvalid
				}
				order.CheckoutBaseQuota, order.CheckoutQuota = base, total
				order.InvoiceNetCents, order.InvoiceTaxCents, order.InvoiceRateBps, order.InvoiceTaxMode = quote.BaseCents, quote.FeeCents, quote.RateBps, quote.Mode
				order.Amount, order.CheckoutCreditAmount, err = checkoutCreditAmount(total, decimal.NewFromFloat(common.QuotaPerUnit))
				if err != nil {
					return err
				}
			}
		}
		// Reprice scheduled orders at payment completion, using their original price
		// and discount, so a pre-cutoff unpaid order cannot bypass the new fees.
		if order.CheckoutPolicyEffectiveAt > 0 {
			price, err := decimal.NewFromString(order.CheckoutUnitPrice)
			if err != nil {
				return ErrInvoiceTaxInvalid
			}
			quotaPerUnit, err := decimal.NewFromString(order.CheckoutQuotaPerUnit)
			if err != nil {
				return ErrInvoiceTaxInvalid
			}
			config := (InvoiceTaxConfig{PolicyEffectiveAt: order.CheckoutPolicyEffectiveAt}).CheckoutAt(now)
			quote, err := calculateEpayCheckout(order.PaymentTotalCents, order.InvoiceType, config, price, order.DiscountRate, quotaPerUnit)
			if err != nil {
				return err
			}
			order.InvoiceNetCents, order.InvoiceTaxCents = quote.NetCents, quote.TaxCents
			order.InvoiceRateBps, order.InvoiceTaxMode = quote.RateBps, quote.Mode
			order.CheckoutBaseQuota, order.CheckoutQuota, order.CheckoutCreditAmount = quote.BaseQuota, quote.Quota, quote.CreditAmount
			order.Amount = quote.Amount
		}
		result := tx.Model(&TopUp{}).Where("id = ? AND status = ?", order.Id, order.Status).Updates(map[string]interface{}{
			"status": common.TopUpStatusSuccess, "complete_time": now, "quota_granted": order.CheckoutBaseQuota,
			"invoice_net_cents": order.InvoiceNetCents, "invoice_tax_cents": order.InvoiceTaxCents,
			"invoice_rate_bps": order.InvoiceRateBps, "invoice_tax_mode": order.InvoiceTaxMode,
			"checkout_base_quota": order.CheckoutBaseQuota, "checkout_quota": order.CheckoutQuota,
			"checkout_credit_amount": order.CheckoutCreditAmount, "amount": order.Amount,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTopUpStatusInvalid
		}
		order.Status, order.QuotaGranted, order.CompleteTime = common.TopUpStatusSuccess, int64(order.CheckoutBaseQuota), now
		var err error
		bonus, err = grantDiscountCodeBonusTx(tx, &order)
		if err != nil {
			return err
		}
		if order.CheckoutBaseQuota+bonus != order.CheckoutQuota {
			return ErrInvoiceTaxInvalid
		}
		result = tx.Model(&User{}).Where("id = ? AND quota <= ?", order.UserId, math.MaxInt64-int64(order.CheckoutQuota)).Update("quota", gorm.Expr("quota + ?", order.CheckoutQuota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceTaxInvalid
		}
		credited = order.CheckoutQuota
		return nil
	})
	if err != nil {
		return &order, 0, err
	}
	if credited > 0 {
		if err := invalidateUserCache(order.UserId); err != nil {
			common.SysError("checkout quota cache invalidation failed: " + err.Error())
		}
		recordDiscountCodeBonus(&order, bonus)
	}
	return &order, credited, nil
}
