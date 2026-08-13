package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	realtimemetrics "github.com/QuantumNous/new-api/pkg/realtime_metrics"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRejectionTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

// A request refused by a specific limiter is then written out through
// abortWithOpenAiMessage, so without the claim it would be counted twice: once as
// the specific gate and once as the generic one.
func TestRelayRejectionIsClaimedOncePerRequest(t *testing.T) {
	c, recorder := newRejectionTestContext()

	recordRelayRejection(c, realtimemetrics.RejectionUserRPM)
	require.True(t, c.GetBool(rejectionRecordedKey))

	abortWithOpenAiMessage(c, http.StatusTooManyRequests, "rate limit exceeded")

	assert.True(t, c.GetBool(rejectionRecordedKey))
	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.True(t, c.IsAborted(), "the response must still be written after the claim is skipped")
}

// The claim must live on the request, not in package state: a process-wide guard
// would record the first rejection after startup and silently drop every one
// after it.
func TestRelayRejectionClaimIsPerRequest(t *testing.T) {
	first, _ := newRejectionTestContext()
	abortWithOpenAiMessage(first, http.StatusForbidden, "denied")
	require.True(t, first.GetBool(rejectionRecordedKey))

	second, recorder := newRejectionTestContext()
	assert.False(t, second.GetBool(rejectionRecordedKey), "a fresh request must be unclaimed")

	abortWithMidjourneyMessage(second, http.StatusTooManyRequests, 429, "too many requests")
	assert.True(t, second.GetBool(rejectionRecordedKey))
	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
}
