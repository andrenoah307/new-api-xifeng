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

func TestGetCnDisclaimerHashChangesWhenContentChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := *system_setting.GetCnDisclaimerSettings()
	defer func() { *system_setting.GetCnDisclaimerSettings() = original }()

	s := system_setting.GetCnDisclaimerSettings()
	s.Title = "Title A"
	s.Content = "Body A"
	s.BlockedCountries = []string{"CN"}

	first := callCnDisclaimer(t)

	s.Content = "Body B"
	second := callCnDisclaimer(t)

	if first.Hash == "" || second.Hash == "" {
		t.Fatalf("expected non-empty hashes, got %q / %q", first.Hash, second.Hash)
	}
	if first.Hash == second.Hash {
		t.Fatalf("hash should change when content changes, got %q both times", first.Hash)
	}
}

func TestGetCnDisclaimerHashStableWhenOnlySilenceChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := *system_setting.GetCnDisclaimerSettings()
	defer func() { *system_setting.GetCnDisclaimerSettings() = original }()

	s := system_setting.GetCnDisclaimerSettings()
	s.Title = "Title"
	s.Content = "Body"
	s.BlockedCountries = []string{"CN"}
	s.SilenceMinutes = 60

	first := callCnDisclaimer(t)

	s.SilenceMinutes = 120
	second := callCnDisclaimer(t)

	if first.Hash != second.Hash {
		t.Fatalf("silence change must not affect hash; got %q -> %q", first.Hash, second.Hash)
	}
	if second.SilenceMinutes != 120 {
		t.Fatalf("expected silence_minutes=120 in response, got %d", second.SilenceMinutes)
	}
}

func TestGetCnDisclaimerEmptyContentReturnsEmptyHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := *system_setting.GetCnDisclaimerSettings()
	defer func() { *system_setting.GetCnDisclaimerSettings() = original }()

	s := system_setting.GetCnDisclaimerSettings()
	s.Title = "Title"
	s.Content = ""
	s.BlockedCountries = []string{"CN"}

	resp := callCnDisclaimer(t)
	if resp.Hash != "" {
		t.Fatalf("empty content should yield empty hash, got %q", resp.Hash)
	}
}

type cnDisclaimerResp struct {
	Success          bool     `json:"success"`
	Enabled          bool     `json:"enabled"`
	Title            string   `json:"title"`
	Content          string   `json:"content"`
	BlockedCountries []string `json:"blocked_countries"`
	SilenceMinutes   int      `json:"silence_minutes"`
	Hash             string   `json:"hash"`
}

func callCnDisclaimer(t *testing.T) cnDisclaimerResp {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/cn-disclaimer", nil)
	GetCnDisclaimer(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var r cnDisclaimerResp
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return r
}
