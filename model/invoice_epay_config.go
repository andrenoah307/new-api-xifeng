package model

import (
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

// The suffix keeps the serialized credentials out of the generic options API.
const InvoiceEpayConfigKey = "InvoiceEpayConfigSecret"

type InvoiceEpayConfig struct {
	Address        string   `json:"address"`
	PartnerID      string   `json:"partner_id"`
	Key            string   `json:"key,omitempty"`
	PaymentMethods []string `json:"payment_methods"`
}

// A missing list preserves legacy configuration; an explicit empty list disables checkout.
func (config InvoiceEpayConfig) CheckoutMethods() []string {
	methods := config.PaymentMethods
	if methods == nil {
		methods = []string{}
		for _, method := range operation_setting.PayMethods {
			if method["type"] == "alipay" || method["type"] == "wxpay" {
				if !slices.Contains(methods, method["type"]) {
					methods = append(methods, method["type"])
				}
			}
		}
	}
	return methods
}

func GetEpayCheckoutGateway(db *gorm.DB, invoiceType string) (InvoiceEpayConfig, error) {
	if invoiceType == "none" {
		return InvoiceEpayConfig{Address: operation_setting.PayAddress, PartnerID: operation_setting.EpayId, Key: operation_setting.EpayKey}, nil
	}
	if invoiceType != "ordinary" && invoiceType != "special" {
		return InvoiceEpayConfig{}, ErrInvoiceTaxInvalid
	}
	return GetInvoiceEpayConfig(db)
}

func GetEpayCheckoutMethods(db *gorm.DB, invoiceType string) ([]string, error) {
	config, err := GetEpayCheckoutGateway(db, invoiceType)
	if err != nil {
		return nil, err
	}
	if !operation_setting.IsPaymentComplianceConfirmed() || config.Address == "" || config.PartnerID == "" || config.Key == "" {
		return []string{}, nil
	}
	return config.CheckoutMethods(), nil
}

func GetInvoiceEpayConfig(db *gorm.DB) (InvoiceEpayConfig, error) {
	var option Option
	var config InvoiceEpayConfig
	err := db.Where(Option{Key: InvoiceEpayConfigKey}).Take(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	err = common.UnmarshalJsonStr(option.Value, &config)
	return config, err
}

func SaveInvoiceEpayConfig(config InvoiceEpayConfig) error {
	for _, method := range config.PaymentMethods {
		if method != "alipay" && method != "wxpay" {
			return ErrInvoiceTaxInvalid
		}
	}
	config.Address = strings.TrimRight(strings.TrimSpace(config.Address), "/")
	config.PartnerID = strings.TrimSpace(config.PartnerID)
	config.Key = strings.TrimSpace(config.Key)
	address, err := url.Parse(config.Address)
	if err != nil || address.Host == "" || (address.Scheme != "https" && address.Scheme != "http") || address.User != nil || address.RawQuery != "" || address.Fragment != "" || config.PartnerID == "" {
		return ErrInvoiceTaxInvalid
	}
	return DB.Session(&gorm.Session{Logger: gormlogger.Discard}).Transaction(func(tx *gorm.DB) error {
		// Create the row before locking, including on the first concurrent save.
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: InvoiceEpayConfigKey, Value: "{}"}).Error; err != nil {
			return err
		}
		var option Option
		if err := lockForUpdate(tx).Where(Option{Key: InvoiceEpayConfigKey}).Take(&option).Error; err != nil {
			return err
		}
		var previous InvoiceEpayConfig
		if err := common.UnmarshalJsonStr(option.Value, &previous); err != nil {
			return err
		}
		if config.Key == "" {
			if config.PartnerID != previous.PartnerID || config.Address != previous.Address {
				return ErrInvoiceTaxInvalid
			}
			config.Key = previous.Key
		}
		if config.PaymentMethods == nil {
			config.PaymentMethods = previous.PaymentMethods
		}
		if config.Key == "" {
			return ErrInvoiceTaxInvalid
		}
		raw, err := common.Marshal(config)
		if err != nil {
			return err
		}
		if string(raw) == option.Value {
			return nil
		}
		result := tx.Model(&Option{}).Where(Option{Key: InvoiceEpayConfigKey}).Update("value", string(raw))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceTaxInvalid
		}
		return nil
	})
}
