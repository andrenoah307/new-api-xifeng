package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func TestGetUserAgreementHashWhenContentSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := *system_setting.GetLegalSettings()
	defer func() { *system_setting.GetLegalSettings() = original }()

	system_setting.GetLegalSettings().UserAgreement = "hello world"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/user-agreement", nil)

	GetUserAgreement(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Success bool   `json:"success"`
		Data    string `json:"data"`
		Hash    string `json:"hash"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success=true, got %s", w.Body.String())
	}
	if resp.Data != "hello world" {
		t.Fatalf("expected data to mirror content, got %q", resp.Data)
	}
	if len(resp.Hash) != 8 {
		t.Fatalf("expected 8-char hash, got %q", resp.Hash)
	}
	if !strings.HasPrefix(resp.Hash, "2aae6c") {
		t.Fatalf("expected stable sha1 prefix for 'hello world', got %q", resp.Hash)
	}
}

func TestGetUserAgreementHashEmptyWhenUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := *system_setting.GetLegalSettings()
	defer func() { *system_setting.GetLegalSettings() = original }()

	system_setting.GetLegalSettings().UserAgreement = ""

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/user-agreement", nil)

	GetUserAgreement(c)

	var resp struct {
		Success bool   `json:"success"`
		Data    string `json:"data"`
		Hash    string `json:"hash"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp.Data != "" || resp.Hash != "" {
		t.Fatalf("expected both data and hash empty, got data=%q hash=%q", resp.Data, resp.Hash)
	}
}
