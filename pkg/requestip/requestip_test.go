package requestip

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func TestGetClientIPAutoPrefersPublicHeader(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	cfg := operation_setting.GetRiskControlSetting()
	cfg.IPMode = ""
	cfg.TrustedIPHeaderEnabled = false
	cfg.TrustedIPHeader = "X-Real-IP"

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:4321"
	req.Header.Set("X-Real-IP", "203.0.113.7")
	ctx.Request = req

	if got := GetClientIP(ctx); got != "203.0.113.7" {
		t.Fatalf("GetClientIP() = %q, want %q", got, "203.0.113.7")
	}
}

func TestGetClientIPAutoFallsBackToRemoteAddr(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	cfg := operation_setting.GetRiskControlSetting()
	cfg.IPMode = ""
	cfg.TrustedIPHeaderEnabled = false
	cfg.TrustedIPHeader = "X-Real-IP"

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:4321"
	ctx.Request = req

	if got := GetClientIP(ctx); got != "198.51.100.10" {
		t.Fatalf("GetClientIP() = %q, want %q", got, "198.51.100.10")
	}
}

func TestGetClientIPUsesTrustedHeaderWhenEnabled(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	cfg := operation_setting.GetRiskControlSetting()
	cfg.TrustedIPHeaderEnabled = true
	cfg.TrustedIPHeader = "CF-Connecting-IP"

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:4321"
	req.Header.Set("CF-Connecting-IP", "203.0.113.8")
	ctx.Request = req

	if got := GetClientIP(ctx); got != "203.0.113.8" {
		t.Fatalf("GetClientIP() = %q, want %q", got, "203.0.113.8")
	}
}

func TestGetClientIPParsesForwardedStyleHeader(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	cfg := operation_setting.GetRiskControlSetting()
	cfg.TrustedIPHeaderEnabled = true
	cfg.TrustedIPHeader = "X-Forwarded-For"

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	ctx.Request = req

	if got := GetClientIP(ctx); got != "203.0.113.9" {
		t.Fatalf("GetClientIP() = %q, want %q", got, "203.0.113.9")
	}
}

func TestGetClientIPFallsBackWhenTrustedHeaderInvalid(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	cfg := operation_setting.GetRiskControlSetting()
	cfg.TrustedIPHeaderEnabled = true
	cfg.TrustedIPHeader = "X-Real-IP"

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:4321"
	req.Header.Set("X-Real-IP", "not-an-ip")
	ctx.Request = req

	if got := GetClientIP(ctx); got != "198.51.100.10" {
		t.Fatalf("GetClientIP() = %q, want %q", got, "198.51.100.10")
	}
}

func TestDiagnoseRequestRecommendsTrustedHeader(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	cfg := operation_setting.GetRiskControlSetting()
	cfg.TrustedIPHeaderEnabled = false
	cfg.TrustedIPHeader = "X-Real-IP"

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.17.0.1:4321"
	req.Header.Set("X-Real-IP", "203.0.113.5")
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 172.17.0.1")
	ctx.Request = req

	diag := DiagnoseRequest(ctx)
	if diag.RecommendedMode != "trusted_header" {
		t.Fatalf("RecommendedMode = %q, want %q", diag.RecommendedMode, "trusted_header")
	}
	if diag.RecommendedHeader != "X-Real-IP" {
		t.Fatalf("RecommendedHeader = %q, want %q", diag.RecommendedHeader, "X-Real-IP")
	}
	// Auto mode heuristic picks the public X-Real-IP; the diagnosis must
	// report the same IP GetClientIP actually returns.
	if diag.EffectiveClientIP != "203.0.113.5" {
		t.Fatalf("EffectiveClientIP = %q, want %q", diag.EffectiveClientIP, "203.0.113.5")
	}
}

func TestGetClientIPXFFModeIndexes(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	gin.SetMode(gin.TestMode)
	newCtx := func(xff string) *gin.Context {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "198.51.100.10:4321"
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		ctx.Request = req
		return ctx
	}

	const xff = "203.0.113.1, 10.0.0.1, 198.51.100.2"
	cases := []struct {
		name  string
		xff   string
		index int
		want  string
	}{
		{"first", xff, 0, "203.0.113.1"},
		{"last", xff, -1, "198.51.100.2"},
		{"second from end", xff, -2, "10.0.0.1"},
		{"out of range positive", xff, 5, "198.51.100.10"},
		{"out of range negative", xff, -5, "198.51.100.10"},
		{"header missing", "", -1, "198.51.100.10"},
	}
	for _, tc := range cases {
		cfg := operation_setting.GetRiskControlSetting()
		cfg.IPMode = operation_setting.IPModeXFF
		cfg.XFFIndex = tc.index
		if got := GetClientIP(newCtx(tc.xff)); got != tc.want {
			t.Fatalf("%s: GetClientIP() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestGetClientIPRemoteAddrModeIgnoresHeaders(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	cfg := operation_setting.GetRiskControlSetting()
	cfg.IPMode = operation_setting.IPModeRemoteAddr

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:4321"
	req.Header.Set("X-Real-IP", "203.0.113.7")
	req.Header.Set("CF-Connecting-IP", "203.0.113.8")
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	ctx.Request = req

	if got := GetClientIP(ctx); got != "198.51.100.10" {
		t.Fatalf("GetClientIP() = %q, want %q", got, "198.51.100.10")
	}
}

func TestEffectiveIPModeCompat(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		enabled bool
		want    string
	}{
		{"unset legacy off", "", false, operation_setting.IPModeAuto},
		{"unset legacy on", "", true, operation_setting.IPModeTrustedHeader},
		{"explicit auto overrides legacy on", operation_setting.IPModeAuto, true, operation_setting.IPModeAuto},
		{"explicit xff", operation_setting.IPModeXFF, false, operation_setting.IPModeXFF},
		{"explicit remote_addr", operation_setting.IPModeRemoteAddr, false, operation_setting.IPModeRemoteAddr},
		{"invalid falls back to legacy", "bogus", true, operation_setting.IPModeTrustedHeader},
		{"nil setting", "", false, operation_setting.IPModeAuto},
	}
	for _, tc := range cases {
		var s *operation_setting.RiskControlSetting
		if tc.name != "nil setting" {
			s = &operation_setting.RiskControlSetting{
				IPMode:                 tc.mode,
				TrustedIPHeaderEnabled: tc.enabled,
			}
		}
		if got := operation_setting.EffectiveIPMode(s); got != tc.want {
			t.Fatalf("%s: EffectiveIPMode() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDiagnoseRequestEffectiveIPMatchesGetClientIP(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	gin.SetMode(gin.TestMode)
	newCtx := func() *gin.Context {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "172.17.0.1:4321"
		req.Header.Set("X-Real-IP", "203.0.113.5")
		req.Header.Set("CF-Connecting-IP", "203.0.113.6")
		req.Header.Set("X-Forwarded-For", "203.0.113.5, 172.17.0.1")
		ctx.Request = req
		return ctx
	}

	for _, mode := range []string{
		"",
		operation_setting.IPModeAuto,
		operation_setting.IPModeTrustedHeader,
		operation_setting.IPModeXFF,
		operation_setting.IPModeRemoteAddr,
	} {
		cfg := operation_setting.GetRiskControlSetting()
		cfg.IPMode = mode
		cfg.TrustedIPHeaderEnabled = false
		cfg.TrustedIPHeader = "CF-Connecting-IP"
		cfg.XFFIndex = -1

		want := GetClientIP(newCtx())
		diag := DiagnoseRequest(newCtx())
		if diag.EffectiveClientIP != want {
			t.Fatalf("mode %q: diagnosis EffectiveClientIP = %q, GetClientIP = %q", mode, diag.EffectiveClientIP, want)
		}
	}
}

func TestDiagnoseModePreviews(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	cfg := operation_setting.GetRiskControlSetting()
	cfg.IPMode = operation_setting.IPModeXFF
	cfg.TrustedIPHeader = "CF-Connecting-IP"
	cfg.XFFIndex = -1

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.17.0.1:4321"
	req.Header.Set("X-Real-IP", "203.0.113.5")
	req.Header.Set("CF-Connecting-IP", "203.0.113.6")
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 198.51.100.2")
	ctx.Request = req

	diag := DiagnoseRequest(ctx)
	got := map[string]ModePreview{}
	for _, p := range diag.ModePreviews {
		got[p.Mode] = p
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 mode previews, got %d", len(diag.ModePreviews))
	}
	if got[operation_setting.IPModeAuto].IP != "203.0.113.5" {
		t.Fatalf("auto preview = %q, want %q", got[operation_setting.IPModeAuto].IP, "203.0.113.5")
	}
	if got[operation_setting.IPModeTrustedHeader].IP != "203.0.113.6" {
		t.Fatalf("trusted_header preview = %q, want %q", got[operation_setting.IPModeTrustedHeader].IP, "203.0.113.6")
	}
	if got[operation_setting.IPModeXFF].IP != "198.51.100.2" {
		t.Fatalf("xff preview = %q, want %q", got[operation_setting.IPModeXFF].IP, "198.51.100.2")
	}
	if got[operation_setting.IPModeRemoteAddr].IP != "172.17.0.1" {
		t.Fatalf("remote_addr preview = %q, want %q", got[operation_setting.IPModeRemoteAddr].IP, "172.17.0.1")
	}
	if !got[operation_setting.IPModeXFF].IsCurrent {
		t.Fatalf("xff preview should be marked current")
	}
	if len(diag.XFFIPs) != 2 || diag.XFFIPs[0] != "203.0.113.7" || diag.XFFIPs[1] != "198.51.100.2" {
		t.Fatalf("XFFIPs = %v, want [203.0.113.7 198.51.100.2]", diag.XFFIPs)
	}
	if diag.CurrentMode != operation_setting.IPModeXFF {
		t.Fatalf("CurrentMode = %q, want %q", diag.CurrentMode, operation_setting.IPModeXFF)
	}
}

func TestDiagnoseRequestRecommendsRemoteAddrWhenPublic(t *testing.T) {
	oldCfg := *operation_setting.GetRiskControlSetting()
	defer func() {
		*operation_setting.GetRiskControlSetting() = oldCfg
	}()

	cfg := operation_setting.GetRiskControlSetting()
	cfg.TrustedIPHeaderEnabled = false
	cfg.TrustedIPHeader = "X-Real-IP"

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.9:4321"
	ctx.Request = req

	diag := DiagnoseRequest(ctx)
	if diag.RecommendedMode != "remote_addr" {
		t.Fatalf("RecommendedMode = %q, want %q", diag.RecommendedMode, "remote_addr")
	}
	if diag.RecommendedHeader != "" {
		t.Fatalf("RecommendedHeader = %q, want empty", diag.RecommendedHeader)
	}
	if diag.EffectiveClientIP != "203.0.113.9" {
		t.Fatalf("EffectiveClientIP = %q, want %q", diag.EffectiveClientIP, "203.0.113.9")
	}
}
