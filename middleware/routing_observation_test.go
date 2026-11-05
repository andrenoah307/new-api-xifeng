package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutingObservationOnDistributorRejection(t *testing.T) {
	require.NoError(t, i18n.Init())
	var output bytes.Buffer
	old := gin.DefaultWriter
	gin.DefaultWriter = &output
	t.Cleanup(func() { gin.DefaultWriter = old })
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(common.RequestIdKey, "rejected-routing-request")
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "invalid")
	}, Distribute(), func(c *gin.Context) { t.Error("invalid explicit channel must not reach relay") })
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Session_id", "private-client-session")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.NotContains(t, response.Body.String(), "routing_observation")
	assert.NotContains(t, output.String(), "private-client-session")
	assert.Contains(t, output.String(), "rejected-routing-request")
	assert.Contains(t, output.String(), `"reason":"explicit_channel"`)
	assert.Contains(t, output.String(), `"http_status":400`)
	assert.Contains(t, output.String(), `"write_result":"not_attempted"`)
}
