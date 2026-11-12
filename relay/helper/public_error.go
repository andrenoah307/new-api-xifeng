package helper

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

// Only downstream error frames are projected. The caller retains its original
// frame for usage accounting and stream outcome detection.
func PublicErrorData(c *gin.Context, data string) string {
	if clean, changed := common.RedactPublicErrorJSON([]byte(data), 0, c.GetString(common.RequestIdKey)); changed {
		logger.LogWarn(c, "redacted upstream error frame: "+common.LocalLogPreview(common.MaskSensitiveInfo(data)))
		return string(clean)
	}
	return data
}
