package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// Gate new finance operations, never settlement callbacks or existing tickets.
func FinanceModuleRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		config, err := model.GetInvoiceTaxConfig(model.DB.WithContext(c.Request.Context()))
		if err != nil {
			common.ApiErrorI18n(c, i18n.MsgDatabaseError)
			c.Abort()
			return
		}
		if !config.ModuleEnabled {
			common.ApiErrorI18n(c, i18n.MsgForbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}
