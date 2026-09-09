package common

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamRequestIdFromHeader(t *testing.T) {
	longValue := strings.Repeat("x", 129)
	tests := []struct {
		name       string
		header     http.Header
		wantID     string
		wantSource string
	}{
		{
			name: "own header has priority",
			header: http.Header{
				"X-Oneapi-Request-Id": []string{"own-id"},
				"X-Request-Id":        []string{"generic-id"},
			},
			wantID: "own-id", wantSource: "X-Oneapi-Request-Id",
		},
		{
			name:       "generic header is accepted",
			header:     http.Header{"X-Request-Id": []string{"generic-id"}},
			wantID:     "generic-id",
			wantSource: "X-Request-Id",
		},
		{
			name:       "value is trimmed",
			header:     http.Header{"X-Request-Id": []string{"  generic-id  "}},
			wantID:     "generic-id",
			wantSource: "X-Request-Id",
		},
		{
			name: "blank values fall through to the next candidate",
			header: http.Header{
				"X-Oneapi-Request-Id": []string{"   "},
				"X-Request-Id":        []string{"generic-id"},
			},
			wantID: "generic-id", wantSource: "X-Request-Id",
		},
		{
			name: "overlong value falls through to the next candidate",
			header: http.Header{
				"X-Oneapi-Request-Id": []string{longValue},
				"X-Request-Id":        []string{"generic-id"},
			},
			wantID: "generic-id", wantSource: "X-Request-Id",
		},
		{
			name:       "exact maximum length is retained",
			header:     http.Header{"X-Request-Id": []string{strings.Repeat("x", 128)}},
			wantID:     strings.Repeat("x", 128),
			wantSource: "X-Request-Id",
		},
		{
			name:   "over maximum length is rejected",
			header: http.Header{"X-Request-Id": []string{longValue}},
		},
		{
			name: "all candidate values are blank",
			header: http.Header{
				"X-Oneapi-Request-Id": []string{" \t"},
				"X-Request-Id":        []string{"\n"},
			},
		},
		{
			name:       "same header skips blank value",
			header:     http.Header{"X-Request-Id": []string{"", "generic-id"}},
			wantID:     "generic-id",
			wantSource: "X-Request-Id",
		},
		{
			name: "nil header",
		},
		{
			name:   "missing headers",
			header: http.Header{"Content-Type": []string{"application/json"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, source := UpstreamRequestIdFromHeader(tt.header)
			require.Equal(t, tt.wantID, id)
			require.Equal(t, tt.wantSource, source)
		})
	}
}

func TestSetUpstreamRequestId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	SetUpstreamRequestId(c, "upstream-id", "X-Request-Id")
	assert.Equal(t, "upstream-id", c.GetString(UpstreamRequestIdKey))
	assert.Equal(t, "X-Request-Id", c.GetString(UpstreamRequestIdSourceKey))

	SetUpstreamRequestId(c, "", "")
	assert.Empty(t, c.GetString(UpstreamRequestIdKey))
	assert.Empty(t, c.GetString(UpstreamRequestIdSourceKey))
}

func TestSetUpstreamRequestIdNilContext(t *testing.T) {
	assert.NotPanics(t, func() { SetUpstreamRequestId(nil, "id", "source") })
}

func TestNormalizeUpstreamRequestId(t *testing.T) {
	assert.Equal(t, "abc", NormalizeUpstreamRequestId("  abc  "))
	assert.Empty(t, NormalizeUpstreamRequestId(" \t\n "))
	assert.Equal(t, strings.Repeat("x", 128), NormalizeUpstreamRequestId(strings.Repeat("x", 128)))
	assert.Empty(t, NormalizeUpstreamRequestId(strings.Repeat("x", 129)))
}

func TestIsUpstreamRequestIdHeader(t *testing.T) {
	assert.True(t, IsUpstreamRequestIdHeader("x-oneapi-request-id"))
	assert.True(t, IsUpstreamRequestIdHeader("X-REQUEST-ID"))
	assert.False(t, IsUpstreamRequestIdHeader("Request-Id"))
	assert.False(t, IsUpstreamRequestIdHeader("Content-Type"))
}
