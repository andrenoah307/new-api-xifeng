package service

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

var legacySessionRe = regexp.MustCompile(`_session_([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

type parsedMetadataUserID struct {
	SessionID string `json:"session_id"`
}

func extractSessionFromMetadata(metadata json.RawMessage) string {
	if len(metadata) == 0 {
		return ""
	}
	var meta struct {
		UserId interface{} `json:"user_id"`
	}
	if err := common.Unmarshal(metadata, &meta); err != nil || meta.UserId == nil {
		return ""
	}
	raw, ok := meta.UserId.(string)
	if !ok || raw == "" {
		return ""
	}
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "{") {
		var parsed parsedMetadataUserID
		if err := common.Unmarshal([]byte(raw), &parsed); err == nil && parsed.SessionID != "" {
			return parsed.SessionID
		}
	}
	if m := legacySessionRe.FindStringSubmatch(raw); len(m) == 2 {
		return m[1]
	}
	return ""
}

func ExtractClaudeSessionID(c *gin.Context, metadata json.RawMessage) string {
	if sid := c.Request.Header.Get("x-claude-code-session-id"); sid != "" {
		return sid
	}
	return extractSessionFromMetadata(metadata)
}

func CaptureUpstreamRequestID(c *gin.Context, resp *http.Response) {
	if resp == nil {
		return
	}
	if rid := resp.Header.Get("request-id"); rid != "" {
		common.SetContextKey(c, constant.ContextKeyUpstreamRequestId, rid)
	}
}
