package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	realtimemetrics "github.com/QuantumNous/new-api/pkg/realtime_metrics"

	"github.com/gin-gonic/gin"
)

// GetRealtimeMetrics serves the admin console's live load view.
//
// It always answers 200, including when Redis is unreachable: the console polls
// this every ten seconds, and a non-2xx would make the frontend's global error
// interceptor raise a toast on every tick. Degradation is carried in the payload
// (degraded / warning) so the page can say it is showing one instance only.
func GetRealtimeMetrics(c *gin.Context) {
	snapshot := realtimemetrics.Read(c.Request.Context(), func(channelID int) string {
		// Cache lookup only — the dashboard must never put the channel table on
		// the poll path.
		channel, err := model.CacheGetChannel(channelID)
		if err != nil || channel == nil {
			return ""
		}
		return channel.Name
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    snapshot,
	})
}
