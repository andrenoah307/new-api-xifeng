package service

import (
	"strings"
	"testing"
)

func TestEstimateTokenByModel_Empty(t *testing.T) {
	if got := EstimateTokenByModel("gpt-4o", ""); got != 0 {
		t.Errorf("empty string: want 0, got %d", got)
	}
}

func TestEstimateTokenByModel_ASCII(t *testing.T) {
	// "hello" = 5 runes → 5*3/2 = 7
	if got := EstimateTokenByModel("gpt-4o", "hello"); got != 7 {
		t.Errorf("ASCII 'hello': want 7, got %d", got)
	}
}

func TestEstimateTokenByModel_CJK(t *testing.T) {
	// "你好世界" = 4 runes → 4*3/2 = 6
	if got := EstimateTokenByModel("claude-3", "你好世界"); got != 6 {
		t.Errorf("CJK '你好世界': want 6, got %d", got)
	}
}

func TestEstimateTokenByModel_Mixed(t *testing.T) {
	// "Hello 你好" = 8 runes (H,e,l,l,o,' ',你,好) → 8*3/2 = 12
	if got := EstimateTokenByModel("gemini-pro", "Hello 你好"); got != 12 {
		t.Errorf("mixed 'Hello 你好': want 12, got %d", got)
	}
}

func TestEstimateTokenByModel_Emoji(t *testing.T) {
	// "😀🎉" = 2 runes → 2*3/2 = 3
	if got := EstimateTokenByModel("gpt-4", "😀🎉"); got != 3 {
		t.Errorf("emoji '😀🎉': want 3, got %d", got)
	}
}

func TestEstimateTokenByModel_ModelIgnored(t *testing.T) {
	text := "same input"
	a := EstimateTokenByModel("gpt-4o", text)
	b := EstimateTokenByModel("claude-3-sonnet", text)
	c := EstimateTokenByModel("gemini-1.5-pro", text)
	if a != b || b != c {
		t.Errorf("model should not affect result: gpt-4o=%d, claude=%d, gemini=%d", a, b, c)
	}
}

func TestEstimateTokenByModel_LargeInput(t *testing.T) {
	text := strings.Repeat("a", 10000)
	want := 10000 * 3 / 2 // 15000
	if got := EstimateTokenByModel("gpt-4o", text); got != want {
		t.Errorf("10k chars: want %d, got %d", want, got)
	}
}

func TestEstimateTokenByModel_SingleChar(t *testing.T) {
	// 1 rune → 1*3/2 = 1 (integer division)
	if got := EstimateTokenByModel("gpt-4o", "a"); got != 1 {
		t.Errorf("single char: want 1, got %d", got)
	}
}

func TestEstimateTokenByModel_Whitespace(t *testing.T) {
	// "a b\nc" = 5 runes → 5*3/2 = 7
	if got := EstimateTokenByModel("gpt-4o", "a b\nc"); got != 7 {
		t.Errorf("whitespace 'a b\\nc': want 7, got %d", got)
	}
}

func BenchmarkEstimateTokenByModel_Short(b *testing.B) {
	text := "Hello, how are you today?"
	for i := 0; i < b.N; i++ {
		EstimateTokenByModel("gpt-4o", text)
	}
}

func BenchmarkEstimateTokenByModel_Medium(b *testing.B) {
	text := strings.Repeat("This is a test sentence with mixed content 你好. ", 100)
	for i := 0; i < b.N; i++ {
		EstimateTokenByModel("claude-3", text)
	}
}

func BenchmarkEstimateTokenByModel_Large(b *testing.B) {
	text := strings.Repeat("a", 100000)
	for i := 0; i < b.N; i++ {
		EstimateTokenByModel("gemini-pro", text)
	}
}
