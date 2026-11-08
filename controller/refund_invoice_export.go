package controller

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
		common.ApiError(c, err)
		return
	}
	kind := c.Query("kind")
	if kind == "" {
		common.ApiSuccess(c, data)
		return
	}
	if kind != "history" && kind != "red" && kind != "reissue" {
		c.Status(http.StatusBadRequest)
		return
	}
	var buffer bytes.Buffer
	buffer.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&buffer)
	if err := w.Write([]string{"原发票工单", "关联充值订单", "关联充值实付金额", "发票抬头", "税号", "邮箱", "发票类型", "税率", "金额", "本次退款关联金额"}); err != nil {
		common.ApiError(c, err)
		return
	}
	for _, invoice := range data.Invoices {
		amount, invoiceType, rate := invoice.OriginalCents, invoice.InvoiceType, invoice.VATRateBps
		if kind == "reissue" {
			amount, invoiceType, rate = invoice.ReissueCents, "ordinary", 600
		}
		if amount == 0 {
			continue
		}
		ids := make([]string, len(invoice.OrderIDs))
		for i, id := range invoice.OrderIDs {
			ids[i] = strconv.Itoa(id)
		}
		label := map[string]string{"ordinary": "增值税普通发票", "special": "增值税专用发票"}[invoiceType]
		if label == "" {
			label = "-"
		}
		rateText := "-"
		if rate > 0 {
			rateText = refundCentsText(int64(rate)) + "%"
		}
		row := []string{strconv.Itoa(invoice.InvoiceTicketID), strings.Join(ids, ";"), refundCentsText(invoice.PaidCents), invoice.CompanyName, invoice.TaxNumber, invoice.Email, label, rateText, refundCentsText(amount), refundCentsText(invoice.RefundCents)}
		for i, value := range row {
			trimmed := strings.TrimLeft(value, " \t\r\n")
			if len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0])) {
				row[i] = "'" + value
			}
		}
		if err := w.Write(row); err != nil {
			common.ApiError(c, err)
			return
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
