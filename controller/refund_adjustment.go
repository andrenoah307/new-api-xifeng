package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func RefundAdjustment(c *gin.Context) {
	if !authorizeRefundFinance(c) {
		return
	}
	id, ok := parseTicketID(c)
	if !ok {
		return
	}
	var input *model.RefundAdjustmentInput
	if c.Request.Method == http.MethodPost {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		input = &model.RefundAdjustmentInput{}
		if err := common.DecodeJson(c.Request.Body, input); err != nil {
			common.ApiError(c, model.ErrRefundAdjustmentConfirmation)
			return
		}
	}
	result, err := model.ManageRefundAdjustment(id, c.GetInt("id"), input)
	if !authorizeRefundFinance(c) {
		return
	}
	if err != nil {
		if errors.Is(err, model.ErrTicketForbidden) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		for _, exposed := range []error{model.ErrFinanceRevision, model.ErrRefundAdjustmentConfirmation, model.ErrRefundFinancePending} {
			if errors.Is(err, exposed) {
				common.ApiError(c, exposed)
				return
			}
		}
		common.SysError("refund manual adjustment failed: " + err.Error())
		common.ApiError(c, model.ErrRefundAdjustment)
		return
	}
	common.ApiSuccess(c, result)
}
