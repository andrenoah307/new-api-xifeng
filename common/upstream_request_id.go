package common

import (
	"net/http"
	"strings"
)

// UpstreamRequestIdHeaders lists response headers that may carry an upstream
// request ID, in priority order. The own gateway ID is preferred over the
// generic upstream ID when both are present.
var UpstreamRequestIdHeaders = []string{RequestIdKey, "X-Request-Id"}

// maxUpstreamRequestIdLength is aligned with logs.upstream_request_id's
// varchar(128) column. Values are rejected rather than truncated.
const maxUpstreamRequestIdLength = 128

func NormalizeUpstreamRequestId(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > maxUpstreamRequestIdLength {
		return ""
	}
	return v
}

func UpstreamRequestIdFromHeader(h http.Header) string {
	if h == nil {
		return ""
	}
	for _, name := range UpstreamRequestIdHeaders {
		for _, value := range h.Values(name) {
			if normalized := NormalizeUpstreamRequestId(value); normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func IsUpstreamRequestIdHeader(k string) bool {
	for _, name := range UpstreamRequestIdHeaders {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}
