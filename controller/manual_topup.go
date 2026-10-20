package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func handleManualTopUpError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrTicketForbidden) || errors.Is(err, model.ErrInvoiceTaxInvalid) || errors.Is(err, model.ErrInvoiceTaxQuoteChanged) {
		handleTicketError(c, err)
		return
	}
	common.SysError("manual topup failed: " + err.Error())
	common.ApiErrorI18n(c, i18n.MsgDatabaseError)
}

func AdminManualTopUpQuote(c *gin.Context) {
	var req model.ManualTopUpRequest
	if c.ShouldBindJSON(&req) != nil {
		handleTicketError(c, model.ErrInvoiceTaxInvalid)
		return
	}
	quote, err := model.QuoteManualTopUp(model.DB, c.GetInt("id"), req)
	if err != nil {
		handleManualTopUpError(c, err)
		return
	}
	common.ApiSuccess(c, quote)
}

func AdminManualTopUp(c *gin.Context) {
	var req model.ManualTopUpRequest
	if c.ShouldBindJSON(&req) != nil {
		handleTicketError(c, model.ErrInvoiceTaxInvalid)
		return
	}
	order, created, err := model.CreateManualTopUp(c.GetInt("id"), req)
	if err != nil {
		handleManualTopUpError(c, err)
		return
	}
	if created {
		recordUserAccountAudit(c, req.UserID, "user.manual_topup", map[string]interface{}{
			"topup_id": order.Id, "trade_no": order.TradeNo, "total_cents": order.PaymentTotalCents, "tax_cents": order.InvoiceTaxCents, "quota": order.QuotaGranted, "invoice_type": order.InvoiceType,
		})
	}
	common.ApiSuccess(c, order)
}
