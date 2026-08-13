package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The admin console polls this endpoint every ten seconds and both frontends run
// a global axios interceptor that toasts on any non-2xx. A degraded read must
// therefore still be a 200 carrying degraded/warning, or an operator watching a
// Redis blip gets an error popup every ten seconds on top of the outage.
func TestGetRealtimeMetricsAnswers200WhenDegraded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/performance/realtime", nil)

	GetRealtimeMetrics(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			RedisEnabled bool   `json:"redis_enabled"`
			Degraded     bool   `json:"degraded"`
			Warning      string `json:"warning"`
			NowUnix      int64  `json:"now_unix"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))

	assert.True(t, body.Success)
	assert.False(t, body.Data.RedisEnabled)
	assert.True(t, body.Data.Degraded)
	assert.Equal(t, "redis_disabled", body.Data.Warning)
	assert.NotZero(t, body.Data.NowUnix)
}
