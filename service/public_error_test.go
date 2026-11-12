package service

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPPublicErrorProjection(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		redacted   bool
	}{
		{"json", `{"error":{"message":"group private-group unavailable","metadata":{"key":"private-key"}}}`, 503, true},
		{"text", `channel private-channel unavailable`, 502, true},
		{"message envelope", `{"message":"group private-group unavailable","key":"private-key"}`, 403, true},
		{"embedded error", `{"error":{"message":"account private-account disabled"}}`, 200, true},
		{"validation", `{"error":{"message":"group: Extra inputs are not permitted"}}`, 400, false},
		{"success", `{"choices":[{"message":{"content":"group private-group"}}],"usage":{"total_tokens":9007199254740993}}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(r)
			c.Set(common.RequestIdKey, "local-id")
			response := &http.Response{StatusCode: tc.status, Header: http.Header{"X-Request-Id": {"upstream-id"}, "Content-Type": {"application/json"}, "X-Provider": {"private-provider"}}}
			original := []byte(tc.body)
			IOCopyBytesGracefully(c, response, original)
			assert.Equal(t, tc.status, r.Code)
			assert.Equal(t, tc.body, string(original))
			assert.Equal(t, strconv.Itoa(r.Body.Len()), r.Header().Get("Content-Length"))
			if tc.redacted {
				assert.NotContains(t, r.Body.String(), "private-")
				assert.Contains(t, r.Body.String(), "local-id")
				assert.Empty(t, r.Header().Get("X-Request-Id"))
			} else {
				assert.Equal(t, tc.body, r.Body.String())
			}
			if tc.status >= 400 {
				assert.Empty(t, r.Header().Get("X-Provider"))
			}
			require.Equal(t, "upstream-id", c.GetString(common.UpstreamRequestIdKey))
		})
	}
}
