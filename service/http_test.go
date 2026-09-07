package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHeaderCaptureContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	return c
}

func TestShouldCopyUpstreamHeader(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		values      []string
		initial     string
		wantCopy    bool
		wantCapture string
	}{
		{
			name:        "generic request id is copied and captured",
			key:         "X-Request-Id",
			values:      []string{" generic-id "},
			wantCopy:    true,
			wantCapture: "generic-id",
		},
		{
			name:        "own request id is hidden and captured",
			key:         "X-Oneapi-Request-Id",
			values:      []string{"own-id"},
			wantCopy:    false,
			wantCapture: "own-id",
		},
		{
			name:        "existing capture is not overwritten",
			key:         "X-Request-Id",
			values:      []string{"generic-id"},
			initial:     "existing-id",
			wantCopy:    true,
			wantCapture: "existing-id",
		},
		{
			name:     "content length is managed separately",
			key:      "Content-Length",
			values:   []string{"12"},
			wantCopy: false,
		},
		{
			name:     "ordinary header is copied without capture",
			key:      "Content-Type",
			values:   []string{"application/json"},
			wantCopy: true,
		},
		{
			name:     "overlong id is not captured but is copied",
			key:      "X-Request-Id",
			values:   []string{strings.Repeat("x", 129)},
			wantCopy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newHeaderCaptureContext(t)
			if tt.initial != "" {
				c.Set(common.UpstreamRequestIdKey, tt.initial)
			}

			assert.Equal(t, tt.wantCopy, ShouldCopyUpstreamHeader(c, tt.key, tt.values))
			require.Equal(t, tt.wantCapture, c.GetString(common.UpstreamRequestIdKey))
		})
	}
}

func TestShouldCopyUpstreamHeaderWithNilContext(t *testing.T) {
	assert.True(t, ShouldCopyUpstreamHeader(nil, "X-Request-Id", []string{"generic-id"}))
	assert.False(t, ShouldCopyUpstreamHeader(nil, "X-Oneapi-Request-Id", []string{"own-id"}))
	assert.False(t, ShouldCopyUpstreamHeader(nil, "Content-Length", []string{"12"}))
}

func TestShouldCopyUpstreamHeaderDoesNotPanicOnEmptyValues(t *testing.T) {
	c := newHeaderCaptureContext(t)
	assert.True(t, ShouldCopyUpstreamHeader(c, "X-Request-Id", nil))
	assert.Empty(t, c.GetString(common.UpstreamRequestIdKey))
}
