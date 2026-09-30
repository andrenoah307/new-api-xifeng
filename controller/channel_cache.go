package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	realtimemetrics "github.com/QuantumNous/new-api/pkg/realtime_metrics"
	"github.com/gin-gonic/gin"
)

func GetChannelCacheRates(c *gin.Context) {
	rates, err := realtimemetrics.ReadChannelCacheRates(c.Request.Context())
	if err != nil {
		common.SysError("channel cache metrics read failed: " + err.Error())
		// An unavailable observation is not a measured zero. Keep this read-only
		// console endpoint quiet and let the UI render its no-data placeholder.
		rates = map[int]float64{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rates})
}
