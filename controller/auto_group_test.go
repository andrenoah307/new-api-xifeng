package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestValidateAutoGroupRuleRequest(t *testing.T) {
	t.Parallel()

	t.Run("empty name", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{Name: "  ", TargetGroup: "default"}
		err := validateAutoGroupRuleRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "名称")
	})

	t.Run("empty target group", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{Name: "test", TargetGroup: ""}
		err := validateAutoGroupRuleRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "分组")
	})

	t.Run("empty conditions", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{
			Name:        "test",
			TargetGroup: "default",
			Conditions:  nil,
		}
		err := validateAutoGroupRuleRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "条件")
	})

	t.Run("invalid metric", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{
			Name:        "test",
			TargetGroup: "default",
			Conditions: []model.AutoGroupCondition{
				{Metric: "invalid_metric", Op: ">=", Value: 100},
			},
		}
		err := validateAutoGroupRuleRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "不支持的指标")
	})

	t.Run("invalid op", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{
			Name:        "test",
			TargetGroup: "default",
			Conditions: []model.AutoGroupCondition{
				{Metric: "total_spend", Op: "~=", Value: 100},
			},
		}
		err := validateAutoGroupRuleRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "不支持的操作符")
	})

	t.Run("recent metric requires positive param", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{
			Name:        "test",
			TargetGroup: "default",
			Conditions: []model.AutoGroupCondition{
				{Metric: "recent_spend", Op: ">=", Value: 100, Param: 0},
			},
		}
		err := validateAutoGroupRuleRequest(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "参数")
	})

	t.Run("recent_topup with negative param", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{
			Name:        "test",
			TargetGroup: "default",
			Conditions: []model.AutoGroupCondition{
				{Metric: "recent_topup", Op: ">=", Value: 100, Param: -1},
			},
		}
		err := validateAutoGroupRuleRequest(req)
		require.Error(t, err)
	})

	t.Run("recent_request_count with zero param", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{
			Name:        "test",
			TargetGroup: "default",
			Conditions: []model.AutoGroupCondition{
				{Metric: "recent_request_count", Op: ">=", Value: 10, Param: 0},
			},
		}
		err := validateAutoGroupRuleRequest(req)
		require.Error(t, err)
	})

	t.Run("non-recent metric does not require param", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{
			Name:        "test",
			TargetGroup: "default",
			Conditions: []model.AutoGroupCondition{
				{Metric: "total_spend", Op: ">=", Value: 100, Param: 0},
			},
		}
		err := validateAutoGroupRuleRequest(req)
		if err != nil {
			require.NotContains(t, err.Error(), "参数", "total_spend should not require param")
		}
	})

	t.Run("default match mode to all", func(t *testing.T) {
		t.Parallel()
		req := &autoGroupRuleRequest{
			Name:        "test",
			TargetGroup: "default",
			MatchMode:   "invalid",
			Conditions: []model.AutoGroupCondition{
				{Metric: "total_spend", Op: ">=", Value: 100},
			},
		}
		_ = validateAutoGroupRuleRequest(req)
		require.Equal(t, "all", req.MatchMode)
	})

	t.Run("all valid metrics accepted", func(t *testing.T) {
		t.Parallel()
		metrics := []string{
			"total_spend", "recent_spend", "yesterday_spend",
			"total_topup", "recent_topup", "yesterday_topup",
			"total_request_count", "recent_request_count",
		}
		for _, m := range metrics {
			param := 0
			if m == "recent_spend" || m == "recent_topup" || m == "recent_request_count" {
				param = 24
			}
			req := &autoGroupRuleRequest{
				Name:        "test",
				TargetGroup: "default",
				Conditions: []model.AutoGroupCondition{
					{Metric: m, Op: ">=", Value: 1, Param: param},
				},
			}
			err := validateAutoGroupRuleRequest(req)
			if err != nil {
				require.NotContains(t, err.Error(), "不支持的指标", "metric %s should be valid", m)
			}
		}
	})

	t.Run("all valid ops accepted", func(t *testing.T) {
		t.Parallel()
		ops := []string{">=", ">", "<=", "<", "==", "!="}
		for _, op := range ops {
			req := &autoGroupRuleRequest{
				Name:        "test",
				TargetGroup: "default",
				Conditions: []model.AutoGroupCondition{
					{Metric: "total_spend", Op: op, Value: 1},
				},
			}
			err := validateAutoGroupRuleRequest(req)
			if err != nil {
				require.NotContains(t, err.Error(), "不支持的操作符", "op %s should be valid", op)
			}
		}
	})
}

func TestValidMetrics(t *testing.T) {
	t.Parallel()

	require.Len(t, validMetrics, 8, "should support exactly 8 metrics")

	expected := []string{
		"total_spend", "recent_spend", "yesterday_spend",
		"total_topup", "recent_topup", "yesterday_topup",
		"total_request_count", "recent_request_count",
	}
	for _, m := range expected {
		require.True(t, validMetrics[m], "missing metric: %s", m)
	}
}

func TestValidOps(t *testing.T) {
	t.Parallel()

	require.Len(t, validOps, 6, "should support exactly 6 operators")

	expected := []string{">=", ">", "<=", "<", "==", "!="}
	for _, op := range expected {
		require.True(t, validOps[op], "missing op: %s", op)
	}
}
