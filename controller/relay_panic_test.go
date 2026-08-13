package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRelayTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return ctx, recorder
}

// A panicking attempt must come back as a non-nil error, because Relay's refund
// defer keys off `newAPIError != nil`. If the panic escaped, the pre-consumed
// quota would stay deducted with no logs row written.
func TestRelayAttemptConvertsPanicToError(t *testing.T) {
	ctx, _ := newRelayTestContext(t)
	info := &relaycommon.RelayInfo{}

	var apiErr *types.NewAPIError
	require.NotPanics(t, func() {
		apiErr = relayAttempt(ctx, info, func(*gin.Context, *relaycommon.RelayInfo) *types.NewAPIError {
			panic("slice bounds out of range [10:8]")
		})
	})

	require.NotNil(t, apiErr, "panic must surface as an error so the refund defer runs")
	assert.Equal(t, types.ErrorCodeRelayPanic, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	// SkipRetry does double duty here: a payload that panics panics on every
	// retry, and processChannelError only auto-bans retryable errors, so this is
	// also what keeps our own crash from disabling a healthy upstream channel.
	assert.True(t, apiErr.IsSkipRetry())
	assert.Contains(t, apiErr.Error(), "slice bounds out of range")
}

func TestRelayAttemptPassesThroughNormalResults(t *testing.T) {
	ctx, _ := newRelayTestContext(t)
	info := &relaycommon.RelayInfo{}

	assert.Nil(t, relayAttempt(ctx, info, func(*gin.Context, *relaycommon.RelayInfo) *types.NewAPIError {
		return nil
	}))

	want := types.NewError(assert.AnError, types.ErrorCodeDoRequestFailed)
	got := relayAttempt(ctx, info, func(*gin.Context, *relaycommon.RelayInfo) *types.NewAPIError {
		return want
	})
	require.NotNil(t, got)
	assert.Equal(t, types.ErrorCodeDoRequestFailed, got.GetErrorCode())
	assert.False(t, got.IsSkipRetry(), "recover must not add SkipRetry to ordinary errors")
}
