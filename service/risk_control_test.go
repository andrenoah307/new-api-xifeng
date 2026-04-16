package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

func TestValidateRiskRuleRejectsTokenOnlyMetricOnUserScope(t *testing.T) {
	conditions, err := common.Marshal([]types.RiskCondition{
		{Metric: "tokens_per_ip_10m", Op: ">=", Value: 3},
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	rule := &model.RiskRule{
		Name:       "user_scope_with_token_metric",
		Scope:      RiskSubjectTypeUser,
		Detector:   "distribution",
		Action:     RiskActionObserve,
		Conditions: string(conditions),
	}

	err = validateRiskRule(rule)
	if err == nil {
		t.Fatal("validateRiskRule returned nil, want scope validation error")
	}
	if !strings.Contains(err.Error(), "tokens_per_ip_10m") {
		t.Fatalf("validateRiskRule error = %q, want metric name in error", err.Error())
	}
}

func TestValidateRiskRuleRejectsUnsupportedMetric(t *testing.T) {
	conditions, err := common.Marshal([]types.RiskCondition{
		{Metric: "made_up_metric", Op: ">=", Value: 1},
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	rule := &model.RiskRule{
		Name:       "unsupported_metric_rule",
		Scope:      RiskSubjectTypeToken,
		Detector:   "distribution",
		Action:     RiskActionObserve,
		Conditions: string(conditions),
	}

	err = validateRiskRule(rule)
	if err == nil {
		t.Fatal("validateRiskRule returned nil, want unsupported metric error")
	}
	if !strings.Contains(err.Error(), "unsupported risk metric") {
		t.Fatalf("validateRiskRule error = %q, want unsupported metric error", err.Error())
	}
}
