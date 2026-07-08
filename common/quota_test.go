package common

import "testing"

func TestClampPreConsumeCompletionTokens(t *testing.T) {
	orig := MaxPreConsumeCompletionTokens
	defer func() { MaxPreConsumeCompletionTokens = orig }()

	MaxPreConsumeCompletionTokens = 256000
	if got := ClampPreConsumeCompletionTokens(100); got != 100 {
		t.Fatalf("under cap should pass through, got %d", got)
	}
	if got := ClampPreConsumeCompletionTokens(256000); got != 256000 {
		t.Fatalf("at cap should pass through, got %d", got)
	}
	if got := ClampPreConsumeCompletionTokens(10_000_000); got != 256000 {
		t.Fatalf("over cap should clamp to %d, got %d", 256000, got)
	}

	MaxPreConsumeCompletionTokens = 0 // 0 表示禁用钳制
	if got := ClampPreConsumeCompletionTokens(10_000_000); got != 10_000_000 {
		t.Fatalf("disabled cap should not clamp, got %d", got)
	}
}
