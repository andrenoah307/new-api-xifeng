package controller

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func authorizeRefundFinance(c *gin.Context) bool {
	var actor model.User
	if err := model.DB.WithContext(c.Request.Context()).Select("id,role,status").First(&actor, c.GetInt("id")).Error; err != nil || actor.Status != common.UserStatusEnabled || (actor.Role != common.RoleFinanceUser && actor.Role != common.RoleAdminUser && actor.Role != common.RoleRootUser) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "permission denied"})
		return false
	}
	c.Header("Cache-Control", "no-store")
	return true
}

func QuoteRefundFinance(c *gin.Context) {
	user, err := getTicketCurrentUser(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	quota, err := strconv.Atoi(c.Query("refund_quota"))
	if err != nil || quota <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	detail, err := model.QuoteRefundFinance(user.Id, quota)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, detail)
}

func ListRefundFinance(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 1000000 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || size < 1 || size > 100 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	query := model.DB.WithContext(c.Request.Context()).Model(&model.TicketRefund{}).Where("finance_version = ?", 1)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var items []model.TicketRefund
	if err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	for i := range items {
		if err := model.AttachRefundFinance(model.DB.WithContext(c.Request.Context()), &items[i]); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if !authorizeRefundFinance(c) {
		return
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": total})
}

func CompleteRefundInvoiceCorrection(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	var request struct {
		RedReference     string `json:"red_reference"`
		ReissueReference string `json:"reissue_reference"`
		Evidence         string `json:"evidence"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if common.DecodeJson(c.Request.Body, &request) != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	item, err := model.CompleteRefundInvoiceCorrection(id, c.GetInt("id"), request.RedReference, request.ReissueReference, request.Evidence)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !authorizeRefundFinance(c) {
		return
	}
	common.ApiSuccess(c, item)
}

func StartRefundInvoiceCorrection(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	item, err := model.StartRefundInvoiceCorrection(id, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !authorizeRefundFinance(c) {
		return
	}
	common.ApiSuccess(c, item)
}

func ExportRefundFinance(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Query("ticket_id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	refund, err := model.GetTicketRefundByTicketId(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if refund.Finance == nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	corrections := map[int]model.RefundInvoiceCorrection{}
	for _, correction := range refund.Finance.InvoiceCorrections {
		corrections[correction.InvoiceTicketId] = correction
	}
	seenInvoices := map[int]bool{}
	var buffer bytes.Buffer
	buffer.WriteString("\xef\xbb\xbf")
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"退款工单", "用户ID", "充值订单", "原发票工单", "实付金额", "实付到账额度", "实付已用额度", "可退额度", "本次扣除额度", "退款本金", "退款税费", "退款合计", "已用部分应开发票面额", "本次退款后订单剩余票额", "原有效发票金额", "整票红冲金额", "整票重开金额", "原票凭证", "红票凭证", "重开凭证", "财务处理状态", "财务处理凭证", "退款状态"}); err != nil {
		common.ApiError(c, err)
		return
	}
	for _, row := range refund.Finance.Allocations {
		correction := corrections[row.InvoiceTicketID]
		values := []string{strconv.Itoa(id), strconv.Itoa(refund.UserId), row.TradeNo, strconv.Itoa(row.InvoiceTicketID), refundCentsText(row.OriginalPaidCents), strconv.FormatInt(row.OriginalQuota, 10), strconv.FormatInt(row.UsedQuota, 10), strconv.FormatInt(row.RefundableQuota, 10), strconv.FormatInt(row.RefundQuota, 10), refundCentsText(row.RefundCents - row.RefundTaxCents), refundCentsText(row.RefundTaxCents), refundCentsText(row.RefundCents), refundCentsText(row.UsedInvoiceCents), refundCentsText(row.RemainingInvoiceCents), refundCentsText(correction.OriginalCents), refundCentsText(correction.OriginalCents), refundCentsText(correction.ReissueCents), correction.OriginalReference, correction.RedReference, correction.ReissueReference, correction.Status, correction.Evidence}
		status := map[int]string{model.RefundStatusPending: "待审核", model.RefundStatusRefunded: "已退款", model.RefundStatusRejected: "已驳回", model.RefundStatusCancelled: "已取消"}
		values = append(values, status[refund.RefundStatus])
		denominator, err := decimal.NewFromString(row.QuotaPerUnit)
		if err != nil || !denominator.IsPositive() {
			common.ApiError(c, model.ErrRefundFinanceReview)
			return
		}
		for i, quota := range []int64{row.OriginalQuota, row.UsedQuota, row.RefundableQuota, row.RefundQuota} {
			values[5+i] = decimal.NewFromInt(quota).Div(denominator).StringFixed(6)
		}
		// A merged invoice is exported once, not once for every linked recharge.
		if correction.Id == 0 || seenInvoices[row.InvoiceTicketID] {
			for i := 14; i <= 16; i++ {
				values[i] = ""
			}
		}
		seenInvoices[row.InvoiceTicketID] = true
		for i, value := range values {
			trimmed := strings.TrimLeft(value, " \t\r\n")
			if len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0])) {
				values[i] = "'" + value
			}
		}
		if err := writer.Write(values); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	writer.Flush()
	if writer.Error() != nil {
		common.ApiError(c, writer.Error())
		return
	}
	if !authorizeRefundFinance(c) {
		return
	}
	c.Header("Content-Disposition", "attachment; filename=refund-finance-"+strconv.Itoa(id)+".csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buffer.Bytes())
}

func refundCentsText(cents int64) string {
	return decimal.NewFromInt(cents).Div(decimal.NewFromInt(100)).StringFixed(2)
}
