package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func TestBuildModerationRequestEmitsMultiModalArray(t *testing.T) {
	body, err := buildModerationRequest("omni-moderation-latest",
		[]string{"hello", ""}, []string{"https://example.com/a.png", " "})
	if err != nil {
		t.Fatalf("buildModerationRequest err: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, `"model":"omni-moderation-latest"`) {
		t.Fatalf("model field missing: %s", s)
	}
	if !strings.Contains(s, `"type":"text"`) || !strings.Contains(s, `"text":"hello"`) {
		t.Fatalf("text item missing: %s", s)
	}
	if !strings.Contains(s, `"type":"image_url"`) || !strings.Contains(s, `"url":"https://example.com/a.png"`) {
		t.Fatalf("image_url item missing: %s", s)
	}
}

func TestBuildModerationRequestRejectsEmpty(t *testing.T) {
	if _, err := buildModerationRequest("m", []string{""}, []string{" "}); err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseModerationResponseFoldsMaxScore(t *testing.T) {
	body := strings.NewReader(`{"id":"x","model":"m","results":[{
		"flagged":false,
		"categories":{"violence":false,"sexual":false},
		"category_scores":{"violence":0.12,"sexual":0.88,"hate":0.42},
		"category_applied_input_types":{"sexual":["text","image"]}
	}]}`)
	out := &ModerationResult{Categories: map[string]float64{}, AppliedTypes: map[string][]string{}}
	if err := parseModerationResponse(body, out); err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if out.MaxCategory != "sexual" {
		t.Errorf("MaxCategory=%q want sexual", out.MaxCategory)
	}
	if out.MaxScore != 0.88 {
		t.Errorf("MaxScore=%v want 0.88", out.MaxScore)
	}
	if got := out.AppliedTypes["sexual"]; len(got) != 2 || got[0] != "text" || got[1] != "image" {
		t.Errorf("AppliedTypes=%v want [text image]", got)
	}
}

func TestParseRetryAfterPlainSeconds(t *testing.T) {
	if d := parseRetryAfter("5"); d.Seconds() != 5 {
		t.Errorf("got %v want 5s", d)
	}
	if d := parseRetryAfter("garbage"); d != 0 {
		t.Errorf("got %v want 0 for invalid header", d)
	}
	if d := parseRetryAfter(""); d != 0 {
		t.Errorf("empty header should give 0, got %v", d)
	}
}

func TestModerationSettingNormalizeFiltersAutoAndClampsRate(t *testing.T) {
	cfg := &operation_setting.ModerationSetting{
		Mode:                "weird",
		EnabledGroups:       []string{"vip", "auto", "vip", " "},
		GroupModes:          map[string]string{"vip": "enforce", "auto": "enforce", "free": "garbage"},
		APIKeys:             []string{"sk-1", "sk-1", " "},
		SamplingRatePercent: 250,
		FlagScoreThreshold:  -1,
	}
	operation_setting.NormalizeModerationSetting(cfg)
	if cfg.Mode != operation_setting.ModerationModeOff {
		t.Errorf("invalid mode should fall back to off, got %q", cfg.Mode)
	}
	if len(cfg.EnabledGroups) != 1 || cfg.EnabledGroups[0] != "vip" {
		t.Errorf("EnabledGroups not deduped/auto-filtered: %v", cfg.EnabledGroups)
	}
	if _, ok := cfg.GroupModes["auto"]; ok {
		t.Error("auto must be removed from GroupModes")
	}
	if _, ok := cfg.GroupModes["free"]; ok {
		t.Error("invalid mode entry must be dropped")
	}
	if len(cfg.APIKeys) != 1 {
		t.Errorf("APIKeys should be deduped: %v", cfg.APIKeys)
	}
	if cfg.SamplingRatePercent != 100 {
		t.Errorf("SamplingRatePercent should clamp to 100, got %d", cfg.SamplingRatePercent)
	}
	if cfg.FlagScoreThreshold != 0.5 {
		t.Errorf("FlagScoreThreshold should reset to default, got %v", cfg.FlagScoreThreshold)
	}
}

func TestIsModerationEnabledForGroupTruthTable(t *testing.T) {
	cfg := &operation_setting.ModerationSetting{
		Enabled:       true,
		Mode:          operation_setting.ModerationModeObserveOnly,
		EnabledGroups: []string{"vip", "free"},
		GroupModes: map[string]string{
			"vip":  operation_setting.ModerationModeEnforce,
			"free": "",
		},
	}
	cases := []struct {
		group string
		want  bool
	}{
		{"vip", true},
		{"free", true},
		{"default", false},
		{"", false},
		{operation_setting.ModerationAutoGroup, false},
	}
	for _, tc := range cases {
		if got := operation_setting.IsModerationEnabledForGroup(cfg, tc.group); got != tc.want {
			t.Errorf("group=%q got=%v want=%v", tc.group, got, tc.want)
		}
	}
	cfg.Enabled = false
	if operation_setting.IsModerationEnabledForGroup(cfg, "vip") {
		t.Error("global disabled should override")
	}
}

// TestPreflightModerationHookIsAllowAll documents the v2 stub behavior; if
// future enforce-mode work changes this, the new behavior must be covered by
// a replacement test, not a silent edit.
func TestPreflightModerationHookIsAllowAll(t *testing.T) {
	allow, reason := PreflightModerationHook(nil, nil, nil)
	if !allow {
		t.Fatalf("PreflightModerationHook must allow in v2; got allow=false reason=%q", reason)
	}
}
