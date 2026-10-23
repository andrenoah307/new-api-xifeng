package model

import (
	"errors"
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

type EpayCheckoutQuote struct {
	InvoiceType    string              `json:"invoice_type"`
	Mode           string              `json:"mode"`
	RateBps        int                 `json:"rate_bps"`
	TotalCents     int64               `json:"total_cents"`
	NetCents       int64               `json:"net_cents"`
	TaxCents       int64               `json:"tax_cents"`
	BaseQuota      int                 `json:"base_quota"`
	Quota          int                 `json:"quota"`
	CreditAmount   string              `json:"credit_amount"`
	DiscountCodeID int                 `json:"discount_code_id"`
	DiscountRate   int                 `json:"discount_rate"`
	Token          string              `json:"token"`
	Routes         []InvoiceEpayConfig `json:"-"`
	Strategy       string              `json:"-"`
}

func CalculateEpayCheckout(totalCents int64, invoiceType string, config InvoiceTaxConfig, price decimal.Decimal, discountRate int) (EpayCheckoutQuote, error) {
	quote := EpayCheckoutQuote{InvoiceType: invoiceType, TotalCents: totalCents, NetCents: totalCents, DiscountRate: discountRate}
	if totalCents <= 0 || totalCents > 100000000000 || !price.IsPositive() || discountRate < 1 || discountRate > 100 || common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
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
	base := decimal.NewFromInt(quote.NetCents).Div(decimal.NewFromInt(100)).Div(price).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	var clamp *common.QuotaClamp
	quote.BaseQuota, clamp = common.QuotaFromDecimalChecked(base)
	if clamp != nil || quote.BaseQuota <= 0 {
		return quote, ErrInvoiceTaxInvalid
	}
	quote.Quota, clamp = common.QuotaFromDecimalChecked(decimal.NewFromInt(int64(quote.BaseQuota)).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(int64(discountRate))))
	if clamp != nil || quote.Quota <= 0 {
		return quote, ErrInvoiceTaxInvalid
	}
	quote.CreditAmount = decimal.NewFromInt(int64(quote.Quota)).Div(decimal.NewFromFloat(common.QuotaPerUnit)).StringFixed(2)
	return quote, nil
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
	quote, err := CalculateEpayCheckout(req.TotalCents, req.InvoiceType, config, price, rate)
	if err != nil {
		return empty, err
	}
	if dc.Id > 0 && dc.MaxAmount > 0 && decimal.NewFromInt(int64(quote.Quota)).Div(decimal.NewFromFloat(common.QuotaPerUnit)).GreaterThan(decimal.NewFromInt(dc.MaxAmount)) {
		return empty, ErrInvoiceTaxInvalid
	}
	quote.DiscountCodeID = dc.Id
	quote.Routes, quote.Strategy = eligible, strategy
	raw, err := common.Marshal(struct {
		Quote    EpayCheckoutQuote
		UserID   int
		Method   string
		Routes   []InvoiceEpayConfig
		Strategy string
	}{quote, userID, req.PaymentMethod, eligible, strategy})
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
	tradeNo := "CHECKOUT" + decimal.NewFromInt(int64(userID)).String() + "-" + req.RequestID
	var order TopUp
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).First(&owner, userID).Error; err != nil {
			return err
		}
		err := tx.Where("trade_no = ?", tradeNo).Take(&order).Error
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
		order = TopUp{
			UserId: userID, TradeNo: tradeNo, Status: common.TopUpStatusPending,
			PaymentProvider: PaymentProviderEpay, PaymentMethod: req.PaymentMethod,
			Money:      float64(quote.TotalCents) / 100,
			Amount:     decimal.NewFromInt(int64(quote.Quota)).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart(),
			CreateTime: common.GetTimestamp(), DiscountCodeId: quote.DiscountCodeID, DiscountRate: quote.DiscountRate,
			CheckoutVersion: 1, CheckoutToken: req.Token, CheckoutCreditAmount: quote.CreditAmount,
			InvoiceType: quote.InvoiceType, InvoiceTaxMode: quote.Mode, InvoiceRateBps: quote.RateBps,
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
	var order TopUp
	credited, bonus := 0, 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).Take(&order).Error; err != nil {
			return err
		}
		if order.CheckoutVersion != 1 || order.PaymentProvider != PaymentProviderEpay || order.PaymentMethod != method || order.PaymentTotalCents != paidCents || order.InvoiceNetCents <= 0 || order.InvoiceTaxCents < 0 || order.InvoiceNetCents+order.InvoiceTaxCents != order.PaymentTotalCents || order.CheckoutBaseQuota <= 0 || order.CheckoutQuota < order.CheckoutBaseQuota || order.CheckoutQuota > common.MaxQuota {
			return ErrInvoiceTaxInvalid
		}
		if order.Status == common.TopUpStatusSuccess {
			return nil
		}
		if order.Status != common.TopUpStatusPending && order.Status != common.TopUpStatusExpired {
			return ErrTopUpStatusInvalid
		}
		result := tx.Model(&TopUp{}).Where("id = ? AND status = ?", order.Id, order.Status).Updates(map[string]interface{}{"status": common.TopUpStatusSuccess, "complete_time": common.GetTimestamp(), "quota_granted": order.CheckoutBaseQuota})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTopUpStatusInvalid
		}
		order.Status, order.QuotaGranted = common.TopUpStatusSuccess, int64(order.CheckoutBaseQuota)
		var err error
		bonus, err = grantDiscountCodeBonusTx(tx, &order)
		if err != nil {
			return err
		}
		if order.CheckoutBaseQuota+bonus != order.CheckoutQuota {
			return ErrInvoiceTaxInvalid
		}
		result = tx.Model(&User{}).Where("id = ? AND quota <= ?", order.UserId, common.MaxQuota-order.CheckoutQuota).Update("quota", gorm.Expr("quota + ?", order.CheckoutQuota))
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
