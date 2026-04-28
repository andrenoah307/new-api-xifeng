package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExtractSessionFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata json.RawMessage
		want     string
	}{
		{
			name:     "empty metadata",
			metadata: nil,
			want:     "",
		},
		{
			name:     "metadata without user_id",
			metadata: json.RawMessage(`{"foo":"bar"}`),
			want:     "",
		},
		{
			name:     "legacy format with session UUID",
			metadata: json.RawMessage(`{"user_id":"user_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890_account_12345678-1234-1234-1234-123456789012_session_aabbccdd-1122-3344-5566-778899aabbcc"}`),
			want:     "aabbccdd-1122-3344-5566-778899aabbcc",
		},
		{
			name:     "JSON format with session_id",
			metadata: json.RawMessage(`{"user_id":"{\"device_id\":\"dev1\",\"account_uuid\":\"acc1\",\"session_id\":\"11223344-aabb-ccdd-eeff-001122334455\"}"}`),
			want:     "11223344-aabb-ccdd-eeff-001122334455",
		},
		{
			name:     "user_id without session",
			metadata: json.RawMessage(`{"user_id":"plain-user-id"}`),
			want:     "",
		},
		{
			name:     "user_id is number",
			metadata: json.RawMessage(`{"user_id":12345}`),
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSessionFromMetadata(tt.metadata)
			if got != tt.want {
				t.Errorf("extractSessionFromMetadata() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractClaudeSessionID_HeaderPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("x-claude-code-session-id", "header-session-id")

	metadata := json.RawMessage(`{"user_id":"user_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890_session_aabbccdd-1122-3344-5566-778899aabbcc"}`)

	got := ExtractClaudeSessionID(c, metadata)
	if got != "header-session-id" {
		t.Errorf("expected header value to take priority, got %q", got)
	}
}

func TestExtractClaudeSessionID_FallbackToMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	metadata := json.RawMessage(`{"user_id":"user_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890_session_aabbccdd-1122-3344-5566-778899aabbcc"}`)

	got := ExtractClaudeSessionID(c, metadata)
	if got != "aabbccdd-1122-3344-5566-778899aabbcc" {
		t.Errorf("expected metadata session, got %q", got)
	}
}

func TestCaptureUpstreamRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	resp := &http.Response{
		Header: http.Header{},
	}
	resp.Header.Set("request-id", "req_abc123")

	CaptureUpstreamRequestID(c, resp)

	got := c.GetString("upstream_request_id")
	if got != "req_abc123" {
		t.Errorf("expected upstream_request_id = %q, got %q", "req_abc123", got)
	}
}

func TestCaptureUpstreamRequestID_NilResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	CaptureUpstreamRequestID(c, nil)

	got := c.GetString("upstream_request_id")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
