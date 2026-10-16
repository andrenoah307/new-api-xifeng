package controller

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func GetInvoiceTaxConfig(c *gin.Context) {
	config, err := model.GetInvoiceTaxConfig(model.DB.WithContext(c.Request.Context()))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, config)
}

func QuoteInvoiceTax(c *gin.Context) {
	var req struct {
		OrderIDs    []int  `json:"topup_order_ids"`
		InvoiceType string `json:"invoice_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	quote, err := model.QuoteInvoiceTax(model.DB.WithContext(c.Request.Context()), c.GetInt("id"), req.OrderIDs, req.InvoiceType)
	if err != nil {
		handleTicketError(c, err)
		return
	}
	common.ApiSuccess(c, quote)
}

func GetInvoiceTaxPayment(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	var ticket model.Ticket
	if err := model.DB.Where("id = ? AND user_id = ?", id, c.GetInt("id")).First(&ticket).Error; err != nil {
		common.ApiErrorI18n(c, i18n.MsgTicketNotFound)
		return
	}
	charge, err := model.GetInvoiceTaxCharge(id)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	methods := []string{}
	if isEpayTopUpEnabled() {
		for _, method := range []string{"alipay", "wxpay"} {
			if operation_setting.ContainsPayMethod(method) {
				methods = append(methods, method)
			}
		}
	}
	common.ApiSuccess(c, gin.H{"charge": charge, "payment_methods": methods})
}

func RequestInvoiceTaxPayment(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	var req struct {
		PaymentMethod string `json:"payment_method"`
	}
	if err != nil || c.ShouldBindJSON(&req) != nil || (req.PaymentMethod != "alipay" && req.PaymentMethod != "wxpay") || !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	client := GetEpayClient()
	if !isEpayTopUpEnabled() || client == nil {
		common.ApiErrorI18n(c, i18n.MsgPaymentNotConfigured)
		return
	}
	charge, err := model.GetInvoiceTaxCharge(id)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	if charge == nil || charge.UserId != c.GetInt("id") || charge.Status != "pending" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	invoice, err := model.GetTicketInvoiceByTicketId(id)
	if err != nil || invoice.InvoiceStatus == model.InvoiceStatusRejected {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	// One persisted trade number per application. Retrying a checkout must not
	// create a second payable order; its gateway method is fixed on first use.
	result := model.DB.Model(&model.InvoiceTaxCharge{}).Where("id = ? AND status = ? AND payment_method = ?", charge.Id, "pending", "").Update("payment_method", req.PaymentMethod)
	if result.Error != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	charge, err = model.GetInvoiceTaxCharge(id)
	if err != nil || charge == nil || charge.Status != "pending" || charge.PaymentMethod != req.PaymentMethod {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	returnPath := fmt.Sprintf("/tickets/%d", id)
	if common.GetTheme() != "default" {
		returnPath = fmt.Sprintf("/console/ticket/%d", id)
	}
	returnURL, _ := url.Parse(system_setting.ServerAddress + returnPath)
	notifyURL, _ := url.Parse(service.GetCallbackAddress() + "/api/invoice-tax/epay/notify")
	uri, params, err := client.Purchase(&epay.PurchaseArgs{Type: charge.PaymentMethod, ServiceTradeNo: charge.TradeNo, Name: fmt.Sprintf("Invoice tax %d", id), Money: decimal.NewFromInt(charge.FeeCents).Div(decimal.NewFromInt(100)).StringFixed(2), Device: epay.PC, NotifyUrl: notifyURL, ReturnUrl: returnURL})
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgPaymentStartFailed)
		return
	}
	common.ApiSuccess(c, gin.H{"url": uri, "params": params})
}

func InvoiceTaxEpayNotify(c *gin.Context) {
	if !isEpayWebhookConfigured() {
		c.String(200, "fail")
		return
	}
	client := GetEpayClient()
	if client == nil || c.Request.ParseForm() != nil {
		c.String(200, "fail")
		return
	}
	values := c.Request.URL.Query()
	if c.Request.Method == "POST" {
		values = c.Request.PostForm
	}
	params := make(map[string]string, len(values))
	for key, values := range values {
		if len(values) != 1 {
			c.String(200, "fail")
			return
		}
		params[key] = values[0]
	}
	verified, err := client.Verify(params)
	if err != nil || !verified.VerifyStatus || params["pid"] != operation_setting.EpayId {
		c.String(200, "fail")
		return
	}
	if verified.TradeStatus != epay.StatusTradeSuccess {
		c.String(200, "success")
		return
	}
	if !invoiceTaxAmountPattern.MatchString(params["money"]) {
		c.String(200, "fail")
		return
	}
	amount, err := decimal.NewFromString(params["money"])
	cents := amount.Mul(decimal.NewFromInt(100))
	if err != nil || !cents.IsInteger() || cents.LessThanOrEqual(decimal.Zero) || cents.GreaterThan(decimal.NewFromInt(100000000000)) {
		c.String(200, "fail")
		return
	}
	if err = model.SettleInvoiceTax(verified.ServiceTradeNo, verified.Type, cents.IntPart()); err != nil {
		common.SysError("invoice tax payment settlement failed")
		c.String(200, "fail")
		return
	}
	c.String(200, "success")
}

var invoiceTaxAmountPattern = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,2})?$`)

func UpdateInvoiceTaxConfig(c *gin.Context) {
	var config model.InvoiceTaxConfig
	if err := c.ShouldBindJSON(&config); err != nil || !config.Valid() {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.SaveInvoiceTaxConfig(config); err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	recordManageAudit(c, "option.update", map[string]interface{}{"key": model.InvoiceTaxConfigKey, "value": config})
	common.ApiSuccess(c, config)
}
