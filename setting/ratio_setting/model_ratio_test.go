package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func mustRatioJSONString(t *testing.T, ratios map[string]float64) string {
	t.Helper()

	jsonBytes, err := common.Marshal(ratios)
	if err != nil {
		t.Fatalf("marshal ratios: %v", err)
	}
	return string(jsonBytes)
}

func TestFormatMatchingModelName_StripsContextWindowSuffix(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "claude-fable-5[1m]", want: "claude-fable-5"},
		{name: "claude-fable-5[1M]", want: "claude-fable-5"},
		{name: "claude-opus-4-5[1m]", want: "claude-opus-4-5"},
		{name: "claude-fable-5", want: "claude-fable-5"},
		{name: "foo[1m]bar", want: "foo[1m]bar"},
		{name: "[1m]", want: "[1m]"},
		{name: "gpt-4-gizmo-abc[1m]", want: "gpt-4-gizmo-*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatMatchingModelName(tt.name); got != tt.want {
				t.Fatalf("FormatMatchingModelName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestFormatMatchingModelName_GeminiThinkingWith1m(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "gemini-2.5-pro[1m]", want: "gemini-2.5-pro"},
		{name: "gemini-2.5-flash-thinking-1024[1m]", want: "gemini-2.5-flash-thinking-*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatMatchingModelName(tt.name); got != tt.want {
				t.Fatalf("FormatMatchingModelName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestGetModelRatio_Context1mSuffixResolvesBase(t *testing.T) {
	oldModelRatio := ModelRatio2JSONString()
	t.Cleanup(func() {
		if err := UpdateModelRatioByJSONString(oldModelRatio); err != nil {
			t.Errorf("restore model ratio map: %v", err)
		}
	})

	if err := UpdateModelRatioByJSONString(mustRatioJSONString(t, map[string]float64{"claude-fable-5": 5})); err != nil {
		t.Fatalf("UpdateModelRatioByJSONString failed: %v", err)
	}

	for _, name := range []string{"claude-fable-5[1m]", "claude-fable-5"} {
		ratio, ok, matchedName := GetModelRatio(name)
		if !ok {
			t.Fatalf("GetModelRatio(%q) ok = false, matched name %q", name, matchedName)
		}
		if ratio != 5 {
			t.Fatalf("GetModelRatio(%q) ratio = %v, want 5", name, ratio)
		}
	}
}

func TestGetCompletionRatio_Context1mSuffixResolvesBase(t *testing.T) {
	oldCompletionRatio := CompletionRatio2JSONString()
	t.Cleanup(func() {
		if err := UpdateCompletionRatioByJSONString(oldCompletionRatio); err != nil {
			t.Errorf("restore completion ratio map: %v", err)
		}
	})

	if err := UpdateCompletionRatioByJSONString(mustRatioJSONString(t, map[string]float64{"claude-fable-5": 5})); err != nil {
		t.Fatalf("UpdateCompletionRatioByJSONString failed: %v", err)
	}

	if got := GetCompletionRatio("claude-fable-5[1m]"); got != 5 {
		t.Fatalf("GetCompletionRatio(%q) = %v, want 5", "claude-fable-5[1m]", got)
	}
}

func TestGetModelPrice_Context1mSuffixResolvesBase(t *testing.T) {
	oldModelPrice := ModelPrice2JSONString()
	t.Cleanup(func() {
		if err := UpdateModelPriceByJSONString(oldModelPrice); err != nil {
			t.Errorf("restore model price map: %v", err)
		}
	})

	if err := UpdateModelPriceByJSONString(mustRatioJSONString(t, map[string]float64{"some-priced-model": 2.0})); err != nil {
		t.Fatalf("UpdateModelPriceByJSONString failed: %v", err)
	}

	price, ok := GetModelPrice("some-priced-model[1m]", false)
	if !ok {
		t.Fatalf("GetModelPrice(%q) ok = false", "some-priced-model[1m]")
	}
	if price != 2.0 {
		t.Fatalf("GetModelPrice(%q) = %v, want 2.0", "some-priced-model[1m]", price)
	}
}
