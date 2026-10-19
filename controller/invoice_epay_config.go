package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetInvoiceEpayConfig(c *gin.Context) {
	config, err := model.GetInvoiceEpayConfig(model.DB.WithContext(c.Request.Context()))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, gin.H{"address": config.Address, "partner_id": config.PartnerID, "key_configured": config.Key != ""})
}

func UpdateInvoiceEpayConfig(c *gin.Context) {
	var config model.InvoiceEpayConfig
	if c.ShouldBindJSON(&config) != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.SaveInvoiceEpayConfig(config); err != nil {
		if errors.Is(err, model.ErrInvoiceTaxInvalid) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		} else {
			common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		}
		return
	}
	recordManageAudit(c, "option.update", map[string]interface{}{"key": model.InvoiceEpayConfigKey, "credentials_updated": true})
	GetInvoiceEpayConfig(c)
}
