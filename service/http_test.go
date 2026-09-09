package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newHeaderCaptureContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	return c
}

func TestCaptureUpstreamRequestIdFallbackUsesPriority(t *testing.T) {
	for i := 0; i < 200; i++ {
		c := newHeaderCaptureContext(t)
		CaptureUpstreamRequestIdFallback(c, http.Header{
			"X-Request-Id":        []string{"generic-id"},
			"X-Oneapi-Request-Id": []string{"own-id"},
		})
		assert.Equal(t, "own-id", c.GetString(common.UpstreamRequestIdKey))
		assert.Equal(t, "X-Oneapi-Request-Id", c.GetString(common.UpstreamRequestIdSourceKey))
	}
}

func TestCaptureUpstreamRequestIdFallbackFillsOnlyEmptyContext(t *testing.T) {
	c := newHeaderCaptureContext(t)
	common.SetUpstreamRequestId(c, "existing-id", "X-Request-Id")

	CaptureUpstreamRequestIdFallback(c, http.Header{
		"X-Oneapi-Request-Id": []string{"own-id"},
	})

	assert.Equal(t, "existing-id", c.GetString(common.UpstreamRequestIdKey))
	assert.Equal(t, "X-Request-Id", c.GetString(common.UpstreamRequestIdSourceKey))
}

func TestCaptureUpstreamRequestIdFallbackHandlesNilAndEmptyInputs(t *testing.T) {
	assert.NotPanics(t, func() { CaptureUpstreamRequestIdFallback(nil, nil) })
	c := newHeaderCaptureContext(t)
	CaptureUpstreamRequestIdFallback(c, nil)
	assert.Empty(t, c.GetString(common.UpstreamRequestIdKey))
	assert.Empty(t, c.GetString(common.UpstreamRequestIdSourceKey))
}

func TestShouldCopyUpstreamHeader(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "content length", key: "Content-Length", want: false},
		{name: "content length lower case", key: "content-length", want: false},
		{name: "own request id", key: "X-Oneapi-Request-Id", want: false},
		{name: "own request id lower case", key: "x-oneapi-request-id", want: false},
		{name: "own request id upper case", key: "X-ONEAPI-REQUEST-ID", want: false},
		{name: "generic request id", key: "X-Request-Id", want: true},
		{name: "content type", key: "Content-Type", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ShouldCopyUpstreamHeader(tt.key))
		})
	}
}
