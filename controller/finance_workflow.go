package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetRefundReconciliation(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	data, err := model.GetRefundReconciliationReport(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func SubmitRefundReconciliation(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	var request struct {
		RequestKey string                          `json:"request_key"`
		Revision   string                          `json:"revision"`
		Plan       model.FinanceReconciliationPlan `json:"plan"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	data, err := model.SubmitFinanceReconciliation(id, c.GetInt("id"), request.RequestKey, request.Revision, request.Plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func ReviewRefundReconciliation(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	var request struct {
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	data, err := model.ReviewFinanceReconciliation(id, c.GetInt("id"), request.Approve, request.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func GetInvoiceCorrections(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	invoice, err := model.GetTicketInvoiceByTicketId(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, invoice)
}

func CreateInvoiceCorrection(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	var request struct {
		RequestKey string `json:"request_key"`
		Reason     string `json:"reason"`
		Reissue    bool   `json:"reissue"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	data, err := model.CreateIndependentInvoiceCorrection(id, c.GetInt("id"), request.RequestKey, request.Reason, request.Reissue)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func TransitionInvoiceCorrection(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	var request struct {
		Action    string                            `json:"action"`
		Documents []model.InvoiceCorrectionDocument `json:"documents"`
		Evidence  string                            `json:"evidence"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, model.ErrFinanceEvidence)
		return
	}
	data, err := model.TransitionInvoiceCorrection(id, c.GetInt("id"), request.Action, request.Documents, request.Evidence)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func GetRefundPayouts(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrRefundPayout)
		return
	}
	refund, err := model.GetTicketRefundForView(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := []model.RefundPayoutAttempt{}
	if err := model.DB.Where("ticket_id = ?", id).Order("id DESC").Find(&items).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	ready := refund.Finance != nil && refund.SettlementVerified && refund.RefundStatus == model.RefundStatusPending
	if refund.Finance != nil {
		for _, correction := range refund.Finance.InvoiceCorrections {
			if correction.Status != "completed" {
				ready = false
			}
		}
	}
	common.ApiSuccess(c, gin.H{"items": items, "ready": ready, "cash_cents": refund.RefundCashCents, "refund_status": refund.RefundStatus})
}

func StartRefundPayout(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrRefundPayout)
		return
	}
	var request struct {
		RequestKey string `json:"request_key"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, model.ErrRefundPayout)
		return
	}
	data, err := model.StartRefundPayout(id, c.GetInt("id"), request.RequestKey)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func ReviewRefundPayout(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrRefundPayout)
		return
	}
	var request struct {
		Status             string `json:"status"`
		Evidence           string `json:"evidence"`
		Reference          string `json:"reference"`
		ClawBackCommission bool   `json:"claw_back_commission"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, model.ErrRefundPayout)
		return
	}
	if request.Status == "succeeded" {
		var before model.RefundPayoutAttempt
		if err := model.DB.First(&before, id).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		var ticket model.Ticket
		if err := model.DB.First(&ticket, before.TicketId).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		previousStatus := ticket.Status
		data, err := model.CompleteRefundPayout(id, c.GetInt("id"), request.Reference, request.Evidence, request.ClawBackCommission)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if before.Status != "succeeded" {
			ticket.Status = model.TicketStatusResolved
			service.NotifyIfTicketStatusChanged(&ticket, previousStatus, service.TicketStatusReasonRefundApproved)
			service.NotifyAutoGroupEvaluation(data.UserId)
		}
		common.ApiSuccess(c, data)
		return
	}
	data, err := model.RecordRefundPayoutReview(id, c.GetInt("id"), request.Status, request.Evidence)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}
