package model

import (
	"errors"
	"strings"

	cfg "github.com/QuantumNous/new-api/setting/captcha_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

func validateCaptchaOptionWrite(key string) error {
	if strings.EqualFold(strings.TrimSpace(key), cfg.OptionKey) {
		return errors.New("use the captcha settings endpoint")
	}
	return nil
}

// Compare-and-swap prevents another node's secret rotation from being lost.
func UpdateCaptchaConfig(previous, next string) error {
	// A failed INSERT/UPDATE must not log its secret-bearing SQL parameters.
	database := DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(logger.Silent)})
	var affected int64
	if previous == "" {
		result := database.Create(&Option{Key: cfg.OptionKey, Value: next})
		if result.Error != nil {
			return result.Error
		}
		affected = result.RowsAffected
	} else {
		err := database.Transaction(func(tx *gorm.DB) error {
			var current Option
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(&Option{Key: cfg.OptionKey}).First(&current).Error; err != nil {
				return err
			}
			if current.Value != previous {
				return errors.New("captcha configuration changed; reload settings")
			}
			return tx.Model(&current).Update("value", next).Error
		})
		if err != nil {
			return err
		}
		affected = 1
	}
	if affected != 1 {
		return errors.New("captcha configuration changed; reload settings")
	}
	return updateOptionMap(cfg.OptionKey, next)
}
