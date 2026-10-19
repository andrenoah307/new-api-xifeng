package controller

import (
	"net/url"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func QuoteEpayCheckout(c *gin.Context) {
	var req model.EpayCheckoutRequest
	if c.ShouldBindJSON(&req) != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	quote, err := model.QuoteEpayCheckout(model.DB.WithContext(c.Request.Context()), c.GetInt("id"), req, false)
	if err != nil {
		handleTicketError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, quote)
}

func RequestEpayCheckout(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var req model.EpayCheckoutRequest
	if c.ShouldBindJSON(&req) != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	order, err := model.CreateEpayCheckout(c.GetInt("id"), req)
	if err != nil {
		handleTicketError(c, err)
		return
	}
	var credentials model.InvoiceEpayConfig
	if common.UnmarshalJsonStr(order.EpayCredentials, &credentials) != nil {
		common.ApiErrorI18n(c, i18n.MsgPaymentNotConfigured)
		return
	}
	client, err := epay.NewClient(&epay.Config{PartnerID: credentials.PartnerID, Key: credentials.Key}, credentials.Address)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgPaymentNotConfigured)
		return
	}
	notifyURL, _ := url.Parse(service.GetCallbackAddress() + "/api/user/epay/checkout/notify")
	returnURL, _ := url.Parse(service.GetPaymentReturnURL("billing", ""))
	uri, params, err := client.Purchase(&epay.PurchaseArgs{Type: order.PaymentMethod, ServiceTradeNo: order.TradeNo, Name: "Recharge", Money: decimal.NewFromInt(order.PaymentTotalCents).Div(decimal.NewFromInt(100)).StringFixed(2), Device: epay.PC, NotifyUrl: notifyURL, ReturnUrl: returnURL})
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, gin.H{"url": uri, "params": params})
}

func EpayCheckoutNotify(c *gin.Context) {
	// In-flight orders keep their own credentials even after settings are changed.
	if c.Request.ParseForm() != nil {
		c.String(200, "fail")
		return
	}
	values := c.Request.URL.Query()
	if c.Request.Method == "POST" {
		values = c.Request.PostForm
	}
	params := make(map[string]string, len(values))
	for key, value := range values {
		if len(value) != 1 {
			c.String(200, "fail")
			return
		}
		params[key] = value[0]
	}
	var order model.TopUp
	if model.DB.Where("trade_no = ? AND checkout_version = ?", params["out_trade_no"], 1).Take(&order).Error != nil {
		c.String(200, "fail")
		return
	}
	var credentials model.InvoiceEpayConfig
	if common.UnmarshalJsonStr(order.EpayCredentials, &credentials) != nil || credentials.Key == "" || credentials.PartnerID != params["pid"] {
		c.String(200, "fail")
		return
	}
	client, err := epay.NewClient(&epay.Config{PartnerID: credentials.PartnerID, Key: credentials.Key}, credentials.Address)
	if err != nil {
		c.String(200, "fail")
		return
	}
	verified, err := client.Verify(params)
	if err != nil || !verified.VerifyStatus || verified.TradeStatus != epay.StatusTradeSuccess || !invoiceTaxAmountPattern.MatchString(params["money"]) {
		c.String(200, "fail")
		return
	}
	amount, err := decimal.NewFromString(params["money"])
	if err != nil {
		c.String(200, "fail")
		return
	}
	settled, credited, err := model.CompleteEpayCheckout(verified.ServiceTradeNo, verified.Type, amount.Mul(decimal.NewFromInt(100)).IntPart())
	if err != nil {
		c.String(200, "fail")
		return
	}
	c.String(200, "success")
	if credited > 0 {
		model.RecordTopupLog(settled.UserId, "Recharge checkout paid: "+settled.TradeNo, c.ClientIP(), settled.PaymentMethod, "epay")
		service.NotifyTopUpSuccess(settled)
		service.NotifyAutoGroupEvaluation(settled.UserId)
		model.GrantTopUpCommission(settled, false)
	}
}
