package service

import (
	"testing"
	"time"
)

func TestMemoryRiskMetricStoreReadPathDoesNotCreateState(t *testing.T) {
	store := newMemoryRiskMetricStore()
	now := time.Unix(1713175200, 0)

	block, err := store.GetBlock(RiskSubjectTypeToken, 1001)
	if err != nil {
		t.Fatalf("GetBlock returned error: %v", err)
	}
	if block != nil {
		t.Fatalf("GetBlock returned unexpected block: %#v", block)
	}
	if err = store.ClearBlock(RiskSubjectTypeToken, 1001); err != nil {
		t.Fatalf("ClearBlock returned error: %v", err)
	}
	inflight, err := store.RecordFinish(RiskSubjectTypeToken, 1001, now)
	if err != nil {
		t.Fatalf("RecordFinish returned error: %v", err)
	}
	if inflight != 0 {
		t.Fatalf("RecordFinish returned unexpected inflight: %d", inflight)
	}
	hitCount, err := store.GetRuleHitCount(RiskSubjectTypeToken, 1001, now)
	if err != nil {
		t.Fatalf("GetRuleHitCount returned error: %v", err)
	}
	if hitCount != 0 {
		t.Fatalf("GetRuleHitCount returned unexpected count: %d", hitCount)
	}
	if got := len(store.subject); got != 0 {
		t.Fatalf("read-only paths should not create memory states, got %d", got)
	}
}

func TestMemoryRiskMetricStoreSweepsIdleSubjectState(t *testing.T) {
	store := newMemoryRiskMetricStore()
	now := time.Unix(1713175200, 0)

	metrics, err := store.RecordStart(RiskSubjectTypeToken, 1001, "ip-1", "ua-1", now)
	if err != nil {
		t.Fatalf("RecordStart returned error: %v", err)
	}
	if metrics.RequestCount1M != 1 {
		t.Fatalf("RecordStart returned unexpected metrics: %#v", metrics)
	}
	if metrics.TokensPerIP10M != 1 {
		t.Fatalf("expected tokens_per_ip_10m to be 1, got %#v", metrics)
	}
	if _, err = store.RecordFinish(RiskSubjectTypeToken, 1001, now); err != nil {
		t.Fatalf("RecordFinish returned error: %v", err)
	}
	if got := len(store.subject); got != 1 {
		t.Fatalf("expected one active memory state, got %d", got)
	}
	if got := len(store.ipTokens); got != 1 {
		t.Fatalf("expected one active ip->tokens state, got %d", got)
	}

	// Trigger a global sweep from another key before the original subject goes idle.
	if _, err = store.GetRuleHitCount(RiskSubjectTypeToken, 2002, now); err != nil {
		t.Fatalf("GetRuleHitCount before idle window returned error: %v", err)
	}
	if got := len(store.subject); got != 1 {
		t.Fatalf("state should still exist before retention windows expire, got %d", got)
	}

	later := now.Add(2 * time.Hour)
	if _, err = store.GetRuleHitCount(RiskSubjectTypeToken, 3003, later); err != nil {
		t.Fatalf("GetRuleHitCount during sweep returned error: %v", err)
	}
	if got := len(store.subject); got != 0 {
		t.Fatalf("idle subject state should be swept after retention windows expire, got %d", got)
	}
	if got := len(store.ipTokens); got != 0 {
		t.Fatalf("idle ip->tokens state should be swept after retention windows expire, got %d", got)
	}
}

func TestMemoryRiskMetricStoreCountsTokensPerIP(t *testing.T) {
	store := newMemoryRiskMetricStore()
	now := time.Unix(1713175200, 0)

	metrics1, err := store.RecordStart(RiskSubjectTypeToken, 1001, "ip-shared", "ua-1", now)
	if err != nil {
		t.Fatalf("RecordStart token 1001 returned error: %v", err)
	}
	if metrics1.TokensPerIP10M != 1 {
		t.Fatalf("expected first token to see tokens_per_ip_10m=1, got %#v", metrics1)
	}

	metrics2, err := store.RecordStart(RiskSubjectTypeToken, 1002, "ip-shared", "ua-2", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RecordStart token 1002 returned error: %v", err)
	}
	if metrics2.TokensPerIP10M != 2 {
		t.Fatalf("expected second token to see tokens_per_ip_10m=2, got %#v", metrics2)
	}

	userMetrics, err := store.RecordStart(RiskSubjectTypeUser, 2001, "ip-shared", "", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("RecordStart user returned error: %v", err)
	}
	if userMetrics.TokensPerIP10M != 0 {
		t.Fatalf("expected user scope not to populate tokens_per_ip_10m, got %#v", userMetrics)
	}
}
