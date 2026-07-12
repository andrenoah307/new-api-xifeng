package geoip

import "testing"

func TestChineseNameToCountryCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"中国", "CN"},
		{"美国", "US"},
		{"日本", "JP"},
		{"中国香港", "HK"},
		{"", ""},
		{"0", ""},
		{"Unknown", "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ChineseNameToCountryCode(tt.input)
			if got != tt.want {
				t.Errorf("ChineseNameToCountryCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLookupCountryNotReady(t *testing.T) {
	t.Parallel()
	got := LookupCountry("1.2.3.4")
	if got != "" {
		t.Errorf("LookupCountry before Init should return empty, got %q", got)
	}
}

func TestParseCountryFromRegion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"中国|0|上海|上海市|电信", "CN"},
		{"美国|0|加利福尼亚|旧金山|谷歌", "US"},
		{"0|0|0|0|0", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseCountryFromRegion(tt.input)
			if got != tt.want {
				t.Errorf("parseCountryFromRegion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
