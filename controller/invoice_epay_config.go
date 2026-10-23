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
	var gateways []gin.H
	if config.Gateways != nil {
		gateways = []gin.H{}
	}
	for _, gateway := range config.Gateways {
		gateways = append(gateways, gin.H{"id": gateway.ID, "name": gateway.Name, "account_name": gateway.AccountName, "address": gateway.Address, "partner_id": gateway.PartnerID, "key_configured": gateway.Key != "", "payment_methods": gateway.PaymentMethods})
	}
	common.ApiSuccess(c, gin.H{"address": config.Address, "partner_id": config.PartnerID, "key_configured": config.Key != "", "payment_methods": config.CheckoutMethods(), "gateways": gateways, "strategy": config.Strategy, "private_below_cents": config.PrivateBelowCents, "private_gateway_name": config.PrivateGatewayName})
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
