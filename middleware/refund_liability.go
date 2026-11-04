package middleware

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"net/http"
)

func RefundLiability() gin.HandlerFunc {
	return func(c *gin.Context) {
		liability, err := model.BeginRefundLiability(c.GetInt("id"), common.GetContextKeyString(c, common.RequestIdKey))
		if err != nil {
			message := "refund settlement registration unavailable"
			if errors.Is(err, model.ErrRefundConsumptionFrozen) {
				message = err.Error()
			} else {
				common.SysError(message + ": " + err.Error())
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "refund_settlement_guard", "message": message}})
			return
		}
		c.Set(common.RefundLiabilityContextKey, liability)
		// A panic leaves durable evidence for reconciliation.
		c.Next()
		if !liability.Failed.Load() {
			model.FinishRefundLiabilityRequest(liability.ID)
		}
	}
}
