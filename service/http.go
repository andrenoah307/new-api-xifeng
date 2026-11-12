package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

// ShouldCopyUpstreamHeader checks whether a given upstream response header
// should be copied to the client response. Content-Length is managed
// separately and X-Oneapi-Request-Id is hidden to preserve the local instance
// ID. Request-ID capture is handled separately by CaptureUpstreamRequestIdFallback.
func ShouldCopyUpstreamHeader(k string) bool {
	if strings.EqualFold(k, "Content-Length") {
		return false
	}
	if common.IsUpstreamRequestIdHeader(k) {
		return !strings.EqualFold(k, common.RequestIdKey)
	}
	return true
}

// CaptureUpstreamRequestIdFallback captures an upstream request ID once from
// the complete response header set when the request has not already captured one.
func CaptureUpstreamRequestIdFallback(c *gin.Context, h http.Header) {
	if c == nil || c.GetString(common.UpstreamRequestIdKey) != "" {
		return
	}
	if id, source := common.UpstreamRequestIdFromHeader(h); id != "" {
		common.SetUpstreamRequestId(c, id, source)
	}
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	redacted := false
	if src != nil {
		original := data
		data, redacted = common.RedactPublicErrorJSON(data, src.StatusCode, c.GetString(common.RequestIdKey))
		var parsed interface{}
		if !redacted && src.StatusCode >= 400 {
			if common.Unmarshal(data, &parsed) == nil {
				probe, _ := common.Marshal(map[string]interface{}{"error": parsed})
				if clean, changed := common.RedactPublicErrorJSON(probe, src.StatusCode, c.GetString(common.RequestIdKey)); changed {
					data, redacted = clean, true
				}
			} else if common.HasSensitiveErrorDetails(string(data)) {
				data, _ = common.Marshal(map[string]interface{}{"error": map[string]string{"type": "upstream_error", "message": common.PublicErrorMessage(src.StatusCode, c.GetString(common.RequestIdKey))}})
				redacted = true
			}
		}
		if redacted {
			logger.LogWarn(c, "redacted upstream error body: "+common.LocalLogPreview(common.MaskSensitiveInfo(string(original))))
		}
	}
	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		CaptureUpstreamRequestIdFallback(c, src.Header)
		for k, v := range src.Header {
			if (redacted || src.StatusCode >= 400) && (common.HasSensitiveErrorDetails(k) || common.HasSensitiveErrorDetails(strings.Join(v, " ")) || strings.EqualFold(k, "Set-Cookie") || strings.EqualFold(k, "WWW-Authenticate")) {
				continue
			}
			if redacted && (common.IsUpstreamRequestIdHeader(k) || strings.EqualFold(k, "Content-Encoding") || strings.EqualFold(k, "ETag") || strings.EqualFold(k, "Content-MD5")) {
				continue
			}
			if !ShouldCopyUpstreamHeader(k) {
				continue
			}
			if len(v) > 0 {
				c.Writer.Header().Set(k, v[0])
			}
		}
	}
	if redacted {
		c.Writer.Header().Set("Content-Type", "application/json")
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	c.Writer.Flush()
}
