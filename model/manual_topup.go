package model

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type ManualTopUpRequest struct {
	UserID      int    `json:"user_id"`
	TotalCents  int64  `json:"total_cents"`
	InvoiceType string `json:"invoice_type"`
	GatewayID   string `json:"gateway_id"`
	Token       string `json:"token"`
	RequestID   string `json:"request_id"`
}

type ManualTopUpAccount struct {
	GatewayID   string `json:"gateway_id"`
	GatewayName string `json:"gateway_name"`
	AccountName string `json:"account_name"`
}

type ManualTopUpQuote struct {
	EpayCheckoutQuote
	ManualTopUpAccount
}

// Expose only receipt metadata, never merchant credentials or payment routing rules.
func GetManualTopUpAccounts(db *gorm.DB, operatorID int) ([]ManualTopUpAccount, error) {
	var operator User
	if err := db.First(&operator, operatorID).Error; err != nil {
		return nil, err
	}
	if operator.Status != common.UserStatusEnabled || operator.Role < common.RoleAdminUser {
		return nil, ErrTicketForbidden
	}
	config, err := GetInvoiceEpayConfig(db)
	if err != nil {
		return nil, err
	}
	accounts := make([]ManualTopUpAccount, 0, len(config.Gateways))
	for _, gateway := range config.Gateways {
		if gateway.ID != "" && strings.TrimSpace(gateway.AccountName) != "" {
			accounts = append(accounts, ManualTopUpAccount{gateway.ID, gateway.Name, gateway.AccountName})
		}
	}
	return accounts, nil
}

// Manual receipts represent money already collected, without gateway promotions.
func QuoteManualTopUp(db *gorm.DB, operatorID int, req ManualTopUpRequest) (ManualTopUpQuote, error) {
	var empty ManualTopUpQuote
	if operatorID <= 0 || req.UserID <= 0 || !slices.Contains([]string{"none", "ordinary", "special"}, req.InvoiceType) {
		return empty, ErrInvoiceTaxInvalid
	}
	var operator, user User
	if err := db.First(&operator, operatorID).Error; err != nil {
		return empty, err
	}
	if err := db.First(&user, req.UserID).Error; err != nil {
		return empty, err
	}
	if operator.Status != common.UserStatusEnabled || operator.Role < common.RoleAdminUser || (operator.Role != common.RoleRootUser && operator.Role <= user.Role) {
		return empty, ErrTicketForbidden
	}
	config, err := GetInvoiceTaxConfig(db)
	if err != nil {
		return empty, err
	}
	if !config.ModuleEnabled {
		return empty, ErrTicketForbidden
	}
	accounts, err := GetManualTopUpAccounts(db, operatorID)
	if err != nil {
		return empty, err
	}
	index := slices.IndexFunc(accounts, func(account ManualTopUpAccount) bool { return account.GatewayID == req.GatewayID })
	if index < 0 {
		return empty, ErrInvoiceTaxInvalid
	}
	checkout, err := CalculateEpayCheckout(req.TotalCents, req.InvoiceType, config.CheckoutAt(common.GetTimestamp()), decimal.NewFromInt(1), 100)
	if err != nil {
		return empty, err
	}
	quote := ManualTopUpQuote{checkout, accounts[index]}
	raw, err := common.Marshal(struct {
		Quote              ManualTopUpQuote
		OperatorID, UserID int
	}{quote, operatorID, req.UserID})
	if err != nil {
		return empty, err
	}
	quote.Token = common.Sha1(raw)
	return quote, nil
}

func CreateManualTopUp(operatorID int, req ManualTopUpRequest) (*TopUp, bool, error) {
	if operatorID <= 0 || req.UserID <= 0 || !checkoutRequestIDPattern.MatchString(req.RequestID) || req.Token == "" {
		return nil, false, ErrInvoiceTaxInvalid
	}
	tradeNo := fmt.Sprintf("MANUAL%d-%s", operatorID, req.RequestID)
	var order TopUp
	created := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Lock accounts in ID order before consistent reads (including MySQL RR).
		ids := []int{operatorID, req.UserID}
		slices.Sort(ids)
		for _, id := range slices.Compact(ids) {
			var account User
			if err := lockForUpdate(tx).First(&account, id).Error; err != nil {
				return err
			}
		}
		// Lock before consistent reads so MySQL RR sees the latest committed configuration.
		var gatewayOption Option
		gatewayErr := lockForUpdate(tx).Where(Option{Key: InvoiceEpayConfigKey}).Take(&gatewayOption).Error
		if gatewayErr != nil && !errors.Is(gatewayErr, gorm.ErrRecordNotFound) {
			return gatewayErr
		}
		// Recheck current account privileges even for a replay.
		var operator, user User
		if err := tx.First(&operator, operatorID).Error; err != nil {
			return err
		}
		if err := tx.First(&user, req.UserID).Error; err != nil {
			return err
		}
		if operator.Status != common.UserStatusEnabled || operator.Role < common.RoleAdminUser || (operator.Role != common.RoleRootUser && operator.Role <= user.Role) {
			return ErrTicketForbidden
		}
		err := tx.Where("trade_no = ?", tradeNo).Take(&order).Error
		if err == nil {
			var account ManualTopUpAccount
			if order.EpayCredentials != "" {
				if err := common.UnmarshalJsonStr(order.EpayCredentials, &account); err != nil {
					return err
				}
			}
			if account.GatewayID != req.GatewayID {
				return ErrInvoiceTaxInvalid
			}
			if order.CreatedBy != operatorID || order.UserId != req.UserID || order.CheckoutToken != req.Token || order.PaymentTotalCents != req.TotalCents || order.InvoiceType != req.InvoiceType || order.PaymentProvider != "manual" || order.Status != common.TopUpStatusSuccess {
				return ErrInvoiceTaxInvalid
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(gatewayErr, gorm.ErrRecordNotFound) {
			return ErrInvoiceTaxInvalid
		}
		quote, err := QuoteManualTopUp(tx, operatorID, req)
		if err != nil {
			return err
		}
		if quote.Token != req.Token {
			return ErrInvoiceTaxQuoteChanged
		}
		snapshot, err := common.Marshal(quote.ManualTopUpAccount)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		order = TopUp{
			UserId: req.UserID, CreatedBy: operatorID, TradeNo: tradeNo,
			Status: common.TopUpStatusSuccess, Source: "manual", PaymentProvider: "manual", PaymentMethod: "manual",
			Money:        float64(quote.TotalCents) / 100,
			Amount:       decimal.NewFromInt(int64(quote.Quota)).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart(),
			QuotaGranted: int64(quote.Quota), CreateTime: now, CompleteTime: now,
			CheckoutVersion: 1, CheckoutToken: quote.Token, CheckoutCreditAmount: quote.CreditAmount,
			CheckoutPolicyEffectiveAt: quote.PolicyEffectiveAt, CheckoutUnitPrice: quote.UnitPrice,
			CheckoutQuotaPerUnit: quote.QuotaPerUnit,
			EpayCredentials:      string(snapshot),
			CheckoutBaseQuota:    quote.Quota, CheckoutQuota: quote.Quota,
			InvoiceType: quote.InvoiceType, InvoiceTaxMode: quote.Mode, InvoiceRateBps: quote.RateBps,
			InvoiceNetCents: quote.NetCents, InvoiceTaxCents: quote.TaxCents, PaymentTotalCents: quote.TotalCents,
		}
		result := tx.Create(&order)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceTaxInvalid
		}
		result = tx.Model(&User{}).Where("id = ? AND quota <= ?", req.UserID, math.MaxInt64-int64(quote.Quota)).Update("quota", gorm.Expr("quota + ?", quote.Quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceTaxInvalid
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if err := invalidateUserCache(req.UserID); err != nil {
		common.SysError("manual topup cache invalidation failed: " + err.Error())
	}
	return &order, created, nil
}
