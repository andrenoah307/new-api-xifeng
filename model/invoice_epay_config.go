package model

import (
	"errors"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

// The suffix keeps the serialized credentials out of the generic options API.
const InvoiceEpayConfigKey = "InvoiceEpayConfigSecret"

type InvoiceEpayConfig struct {
	Address   string `json:"address"`
	PartnerID string `json:"partner_id"`
	Key       string `json:"key,omitempty"`
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
