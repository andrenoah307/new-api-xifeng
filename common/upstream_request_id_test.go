package common

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamRequestIdFromHeader(t *testing.T) {
	longValue := strings.Repeat("x", 129)
	tests := []struct {
		name   string
		header http.Header
		want   string
	}{
		{
			name: "own header has priority",
			header: http.Header{
				"X-Oneapi-Request-Id": []string{"own-id"},
				"X-Request-Id":        []string{"generic-id"},
			},
			want: "own-id",
		},
		{
			name:   "generic header is accepted",
			header: http.Header{"X-Request-Id": []string{"generic-id"}},
			want:   "generic-id",
		},
		{
			name:   "value is trimmed",
			header: http.Header{"X-Request-Id": []string{"  generic-id  "}},
			want:   "generic-id",
		},
		{
			name: "blank values fall through to the next candidate",
			header: http.Header{
				"X-Oneapi-Request-Id": []string{"   "},
				"X-Request-Id":        []string{"generic-id"},
			},
			want: "generic-id",
		},
		{
			name: "overlong value falls through to the next candidate",
			header: http.Header{
				"X-Oneapi-Request-Id": []string{longValue},
				"X-Request-Id":        []string{"generic-id"},
			},
			want: "generic-id",
		},
		{
			name:   "exact maximum length is retained",
			header: http.Header{"X-Request-Id": []string{strings.Repeat("x", 128)}},
			want:   strings.Repeat("x", 128),
		},
		{
			name:   "over maximum length is rejected",
			header: http.Header{"X-Request-Id": []string{longValue}},
			want:   "",
		},
		{
			name:   "same header skips blank value",
			header: http.Header{"X-Request-Id": []string{"", "generic-id"}},
			want:   "generic-id",
		},
		{
			name: "nil header",
			want: "",
		},
		{
			name:   "missing headers",
			header: http.Header{"Content-Type": []string{"application/json"}},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, UpstreamRequestIdFromHeader(tt.header))
		})
	}
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
