package operation_setting

import (
	"testing"
)

func TestIsModelBlockedForCountry(t *testing.T) {
	// Setup blocked models and rebuild index
	regionRestrictionSetting.BlockedModels = map[string][]string{
		"CN": {"gpt-4o", "claude-*", "*"},
		"RU": {"gpt-4o", "gemini-*"},
		"JP": {"dall-e-3"},
	}
	RebuildRegionRestrictionIndex()

	tests := []struct {
		name        string
		countryCode string
		modelName   string
		want        bool
	}{
		// Exact match
		{"exact match RU gpt-4o", "RU", "gpt-4o", true},
		{"exact match JP dall-e-3", "JP", "dall-e-3", true},
		// Prefix wildcard
		{"prefix match RU gemini-pro", "RU", "gemini-pro", true},
		{"prefix match RU gemini-1.5-flash", "RU", "gemini-1.5-flash", true},
		// Catch-all wildcard
		{"catch-all CN blocks anything", "CN", "some-random-model", true},
		// No match
		{"no match JP gpt-4o", "JP", "gpt-4o", false},
		{"no match RU dall-e-3", "RU", "dall-e-3", false},
		{"unknown country", "DE", "gpt-4o", false},
		// Empty country code returns false
		{"empty country code", "", "gpt-4o", false},
		// Case insensitivity
		{"case insensitive country", "ru", "gpt-4o", true},
		{"case insensitive model", "RU", "GPT-4o", true},
		{"case insensitive both", "jp", "DALL-E-3", true},
		{"case insensitive prefix", "ru", "GEMINI-Pro", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsModelBlockedForCountry(tt.countryCode, tt.modelName)
			if got != tt.want {
				t.Errorf("IsModelBlockedForCountry(%q, %q) = %v, want %v",
					tt.countryCode, tt.modelName, got, tt.want)
			}
		})
	}
}

func TestRebuildRegionRestrictionIndex(t *testing.T) {
	// Start with empty
	regionRestrictionSetting.BlockedModels = map[string][]string{}
	RebuildRegionRestrictionIndex()

	if IsModelBlockedForCountry("CN", "gpt-4o") {
		t.Error("expected no block with empty config")
	}

	// Rebuild with data
	regionRestrictionSetting.BlockedModels = map[string][]string{
		"us": {"claude-3-opus"}, // lowercase country should be uppercased
	}
	RebuildRegionRestrictionIndex()

	if !IsModelBlockedForCountry("US", "claude-3-opus") {
		t.Error("expected block after rebuild")
	}

	// Verify whitespace trimming
	regionRestrictionSetting.BlockedModels = map[string][]string{
		"  KR  ": {"  gpt-4o  "},
	}
	RebuildRegionRestrictionIndex()

	if !IsModelBlockedForCountry("KR", "gpt-4o") {
		t.Error("expected block after trimming whitespace")
	}

	// Verify empty patterns are skipped
	regionRestrictionSetting.BlockedModels = map[string][]string{
		"FR": {"", "  ", "gpt-4o"},
	}
	RebuildRegionRestrictionIndex()

	if !IsModelBlockedForCountry("FR", "gpt-4o") {
		t.Error("expected block for non-empty pattern")
	}

	// Verify empty country key is skipped
	regionRestrictionSetting.BlockedModels = map[string][]string{
		"":   {"gpt-4o"},
		"  ": {"gpt-4o"},
	}
	RebuildRegionRestrictionIndex()

	if IsModelBlockedForCountry("", "gpt-4o") {
		t.Error("expected no block for empty country key")
	}
}

func TestDefaultSettingDisabled(t *testing.T) {
	// Reset to fresh defaults
	saved := regionRestrictionSetting
	defer func() { regionRestrictionSetting = saved }()

	regionRestrictionSetting = RegionRestrictionSetting{
		Enabled:       false,
		FilterConsole: true,
		BlockRelay:    true,
		BlockedModels: map[string][]string{},
		BlockMessage:  "",
		XdbPath:       "data/ip2region.xdb",
	}

	if IsRegionRestrictionEnabled() {
		t.Error("expected disabled by default")
	}
	if IsRegionRestrictionConsoleEnabled() {
		t.Error("expected console disabled when main switch is off")
	}
	if IsRegionRestrictionRelayEnabled() {
		t.Error("expected relay disabled when main switch is off")
	}

	s := GetRegionRestrictionSetting()
	if s.XdbPath != "data/ip2region.xdb" {
		t.Errorf("unexpected default XdbPath: %s", s.XdbPath)
	}
	if s.FilterConsole != true {
		t.Error("expected FilterConsole default true")
	}
	if s.BlockRelay != true {
		t.Error("expected BlockRelay default true")
	}
}
