package middleware

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	realtimemetrics "github.com/QuantumNous/new-api/pkg/realtime_metrics"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const rejectionRecordedKey = "realtime_rejection_recorded"

// recordRelayRejection attributes one refused request to exactly one gate. The
// first gate to claim it wins, so a specific limiter that then aborts through
// abortWithOpenAiMessage is not counted a second time as a generic rejection.
func recordRelayRejection(c *gin.Context, kind string) {
	if c.GetBool(rejectionRecordedKey) {
		return
	}
	c.Set(rejectionRecordedKey, true)
	realtimemetrics.RecordRejection(kind)
}

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	recordRelayRejection(c, realtimemetrics.RejectionGate)
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	userId := c.GetInt("id")
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":    "new_api_error",
			"code":    codeStr,
		},
	})
	c.Abort()
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
}

func abortWithMidjourneyMessage(c *gin.Context, statusCode int, code int, description string) {
	recordRelayRejection(c, realtimemetrics.RejectionGate)
	c.JSON(statusCode, gin.H{
		"description": description,
		"type":        "new_api_error",
		"code":        code,
	})
	c.Abort()
	logger.LogError(c.Request.Context(), description)
}
