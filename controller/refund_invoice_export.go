package controller

import (
	"bytes"
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func RefundInvoiceExport(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, ok := parseTicketID(c)
	if !ok {
		return
	}
	var data *model.RefundExportData
	var err error
	if c.Request.Method == http.MethodPost {
		data, err = model.SaveRefundInvoiceExport(id, c.GetInt("id"))
	} else {
		data, err = model.GetRefundInvoiceExport(id)
	}
	if err != nil {
		if errors.Is(err, model.ErrTicketRefundNotPending) || errors.Is(err, model.ErrTicketForbidden) {
			handleTicketError(c, err)
			return
		}
		if errors.Is(err, model.ErrRefundLegacyWorkflow) {
			common.ApiError(c, err)
			return
		}
		var diagnostic *model.RefundCalculationError
		hasDiagnostic := errors.As(err, &diagnostic)
		if c.Request.Method == http.MethodPost && !hasDiagnostic {
			common.SysError("refund recalculation failed: " + err.Error())
			diagnostic = &model.RefundCalculationError{Code: "calculation_failed", TicketID: id}
			hasDiagnostic = true
		}
		if hasDiagnostic {
			if !authorizeRefundFinance(c) {
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": model.ErrRefundExport.Error(), "diagnostic": diagnostic})
			return
		}
		common.ApiError(c, err)
		return
	}
	kind := c.Query("kind")
	if kind == "" {
		actor, err := getTicketCurrentUser(c)
		if err != nil || actor.Status != common.UserStatusEnabled {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if actor.Role != common.RoleAdminUser && actor.Role != common.RoleRootUser {
			for i := range data.Payments {
				data.Payments[i].Calculation = nil
			}
		}
		common.ApiSuccess(c, data)
		return
	}
	if kind != "red" && kind != "reissue" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	items, err := model.RefundInvoiceExportItems(id, data, kind == "reissue")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var buffer bytes.Buffer
	buffer.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&buffer)
	if err := w.Write([]string{"电子邮箱", "数量", "单价", "金额合计", "公司信息", "应税服务名称", "发票类型", "发票税率(%)", "开票方（公账名称）", "关联充值订单号"}); err != nil {
		common.ApiError(c, err)
		return
	}
	for _, invoice := range items {
		if invoice.TotalMoney == 0 {
			continue
		}
		companyInfo := invoice.CompanyName
		if invoice.TaxNumber != "" {
			companyInfo = "发票抬头\n" + invoice.CompanyName + "\n购方税号\n" + invoice.TaxNumber
		}
		label := map[string]string{"ordinary": "增值税普通发票", "special": "增值税专用发票"}[invoice.InvoiceType]
		for _, issuer := range invoice.IssuerAmounts {
			if issuer.AmountCents == 0 {
				continue
			}
			row := []string{invoice.Email, "", "", refundCentsText(issuer.AmountCents), companyInfo, strings.TrimSpace(operation_setting.InvoiceServiceName), label, strconv.FormatFloat(float64(invoice.TaxRateBps)/100, 'f', -1, 64), issuer.AccountName, strings.Join(invoice.OrderNumbers, ";")}
			for i, value := range row {
				trimmed := strings.TrimLeft(value, " \t\r\n")
				if value != "-" && len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0])) {
					row[i] = "'" + value
				}
			}
			if err := w.Write(row); err != nil {
				common.ApiError(c, err)
				return
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		common.ApiError(c, err)
		return
	}
	if !authorizeRefundFinance(c) {
		return
	}
	c.Header("Content-Disposition", "attachment; filename=refund-"+strconv.Itoa(id)+"-"+kind+".csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buffer.Bytes())
}
