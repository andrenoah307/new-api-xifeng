package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestCompareValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		actual float64
		op     string
		target float64
		want   bool
	}{
		{"gte true equal", 100, ">=", 100, true},
		{"gte true greater", 101, ">=", 100, true},
		{"gte false", 99, ">=", 100, false},

		{"gt true", 101, ">", 100, true},
		{"gt false equal", 100, ">", 100, false},
		{"gt false less", 99, ">", 100, false},

		{"lte true equal", 100, "<=", 100, true},
		{"lte true less", 99, "<=", 100, true},
		{"lte false", 101, "<=", 100, false},

		{"lt true", 99, "<", 100, true},
		{"lt false equal", 100, "<", 100, false},
		{"lt false greater", 101, "<", 100, false},

		{"eq true", 100, "==", 100, true},
		{"eq false", 99, "==", 100, false},

		{"neq true", 99, "!=", 100, true},
		{"neq false", 100, "!=", 100, false},

		{"zero actual gte zero", 0, ">=", 0, true},
		{"negative vs zero", -1, ">=", 0, false},
		{"float precision", 0.1+0.2, "==", 0.3, true},

		{"unknown op", 100, "~=", 100, false},
		{"empty op", 100, "", 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := compareValue(tt.actual, tt.op, tt.target)
			require.Equal(t, tt.want, got, "compareValue(%v, %q, %v)", tt.actual, tt.op, tt.target)
		})
	}
}

func TestMatchConditions(t *testing.T) {
	t.Parallel()

	engine := &autoGroupEngine{}

	makeRule := func(matchMode string, conds []model.AutoGroupCondition) *compiledAutoGroupRule {
		return &compiledAutoGroupRule{
			Raw: &model.AutoGroupRule{
				MatchMode: matchMode,
			},
			Conditions: conds,
		}
	}

	t.Run("all mode all match", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("all", []model.AutoGroupCondition{
			{Metric: "total_spend", Op: ">=", Value: 50},
			{Metric: "total_spend", Op: "<=", Value: 200},
		})
		cache := map[string]float64{"total_spend": 100}
		got := engine.matchConditions(0, rule, cache, map[string]string{})
		require.True(t, got)
	})

	t.Run("all mode one fails", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("all", []model.AutoGroupCondition{
			{Metric: "total_spend", Op: ">=", Value: 50},
			{Metric: "total_spend", Op: ">=", Value: 200},
		})
		cache := map[string]float64{"total_spend": 100}
		got := engine.matchConditions(0, rule, cache, map[string]string{})
		require.False(t, got)
	})

	t.Run("any mode one matches", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("any", []model.AutoGroupCondition{
			{Metric: "total_spend", Op: ">=", Value: 200},
			{Metric: "total_spend", Op: "<=", Value: 50},
		})
		cache := map[string]float64{"total_spend": 30}
		got := engine.matchConditions(0, rule, cache, map[string]string{})
		require.True(t, got)
	})

	t.Run("any mode none matches", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("any", []model.AutoGroupCondition{
			{Metric: "total_spend", Op: ">=", Value: 200},
			{Metric: "total_spend", Op: "<=", Value: 50},
		})
		cache := map[string]float64{"total_spend": 100}
		got := engine.matchConditions(0, rule, cache, map[string]string{})
		require.False(t, got)
	})

	t.Run("empty conditions all mode returns true", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("all", nil)
		cache := map[string]float64{}
		got := engine.matchConditions(0, rule, cache, map[string]string{})
		require.True(t, got)
	})

	t.Run("empty conditions any mode returns false", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("any", nil)
		cache := map[string]float64{}
		got := engine.matchConditions(0, rule, cache, map[string]string{})
		require.False(t, got)
	})

	t.Run("metric cache keying with param", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("all", []model.AutoGroupCondition{
			{Metric: "recent_spend", Op: ">=", Value: 10, Param: 24},
		})
		cache := map[string]float64{"recent_spend_24": 50}
		got := engine.matchConditions(0, rule, cache, map[string]string{})
		require.True(t, got)
	})

	t.Run("metric cache keying without param", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("all", []model.AutoGroupCondition{
			{Metric: "total_spend", Op: ">=", Value: 10, Param: 0},
		})
		cache := map[string]float64{"total_spend": 50}
		got := engine.matchConditions(0, rule, cache, map[string]string{})
		require.True(t, got)
	})

	t.Run("default match mode is all", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("", []model.AutoGroupCondition{
			{Metric: "total_spend", Op: ">=", Value: 50},
			{Metric: "total_spend", Op: ">=", Value: 200},
		})
		cache := map[string]float64{"total_spend": 100}
		got := engine.matchConditions(0, rule, cache, map[string]string{})
		require.False(t, got, "empty match_mode should default to 'all' (AND)")
	})
}

func TestIsUserEnrolled(t *testing.T) {
	t.Parallel()

	t.Run("nil atomic returns false", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		require.False(t, engine.isUserEnrolled(1))
	})

	t.Run("empty map returns false", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		engine.enrolledUsers.Store(map[int]struct{}{})
		require.False(t, engine.isUserEnrolled(1))
	})

	t.Run("enrolled user returns true", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		engine.enrolledUsers.Store(map[int]struct{}{42: {}, 99: {}})
		require.True(t, engine.isUserEnrolled(42))
		require.True(t, engine.isUserEnrolled(99))
	})

	t.Run("non-enrolled user returns false", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		engine.enrolledUsers.Store(map[int]struct{}{42: {}})
		require.False(t, engine.isUserEnrolled(100))
	})

	t.Run("wrong type stored returns false", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		engine.enrolledUsers.Store("wrong type")
		require.False(t, engine.isUserEnrolled(1))
	})
}

func TestCurrentRules(t *testing.T) {
	t.Parallel()

	t.Run("nil atomic returns nil", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		require.Nil(t, engine.currentRules())
	})

	t.Run("returns stored rules", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		rules := []*compiledAutoGroupRule{
			{Raw: &model.AutoGroupRule{Name: "r1"}},
		}
		engine.rules.Store(rules)
		got := engine.currentRules()
		require.Len(t, got, 1)
		require.Equal(t, "r1", got[0].Raw.Name)
	})

	t.Run("wrong type returns nil", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		engine.rules.Store("wrong")
		require.Nil(t, engine.currentRules())
	})
}

func TestNotifyAutoGroupEvaluation(t *testing.T) {
	t.Parallel()

	t.Run("not started does nothing", func(t *testing.T) {
		t.Parallel()
		saved := globalAutoGroupEngine
		defer func() { globalAutoGroupEngine = saved }()

		engine := &autoGroupEngine{}
		engine.enrolledUsers.Store(map[int]struct{}{1: {}})
		globalAutoGroupEngine = engine
		NotifyAutoGroupEvaluation(1)
	})

	t.Run("started but not enrolled does nothing", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		engine.started.Store(true)
		engine.enrolledUsers.Store(map[int]struct{}{})
		engine.evalCh = make(chan int, 10)

		saved := globalAutoGroupEngine
		defer func() { globalAutoGroupEngine = saved }()
		globalAutoGroupEngine = engine

		NotifyAutoGroupEvaluation(99)
		require.Len(t, engine.evalCh, 0)
	})

	t.Run("started and enrolled sends to channel", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		engine.started.Store(true)
		engine.enrolledUsers.Store(map[int]struct{}{42: {}})
		engine.evalCh = make(chan int, 10)

		saved := globalAutoGroupEngine
		defer func() { globalAutoGroupEngine = saved }()
		globalAutoGroupEngine = engine

		NotifyAutoGroupEvaluation(42)
		require.Len(t, engine.evalCh, 1)
		got := <-engine.evalCh
		require.Equal(t, 42, got)
	})

	t.Run("full channel does not block", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		engine.started.Store(true)
		engine.enrolledUsers.Store(map[int]struct{}{1: {}})
		engine.evalCh = make(chan int, 1)
		engine.evalCh <- 999

		saved := globalAutoGroupEngine
		defer func() { globalAutoGroupEngine = saved }()
		globalAutoGroupEngine = engine

		NotifyAutoGroupEvaluation(1)
		require.Len(t, engine.evalCh, 1)
	})
}

func TestEvaluateBatch(t *testing.T) {
	t.Parallel()

	t.Run("no rules skips evaluation", func(t *testing.T) {
		t.Parallel()
		engine := &autoGroupEngine{}
		engine.rules.Store([]*compiledAutoGroupRule{})
		engine.evaluateBatch(map[int]struct{}{1: {}, 2: {}})
	})
}

func TestParsedConditions(t *testing.T) {
	t.Parallel()

	t.Run("valid json", func(t *testing.T) {
		t.Parallel()
		rule := &model.AutoGroupRule{
			Conditions: `[{"metric":"total_spend","op":">=","value":100},{"metric":"recent_spend","op":">=","value":50,"param":24}]`,
		}
		conds := rule.ParsedConditions()
		require.Len(t, conds, 2)
		require.Equal(t, "total_spend", conds[0].Metric)
		require.Equal(t, ">=", conds[0].Op)
		require.Equal(t, float64(100), conds[0].Value)
		require.Equal(t, 0, conds[0].Param)
		require.Equal(t, "recent_spend", conds[1].Metric)
		require.Equal(t, 24, conds[1].Param)
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		rule := &model.AutoGroupRule{Conditions: ""}
		require.Nil(t, rule.ParsedConditions())
	})

	t.Run("whitespace only", func(t *testing.T) {
		t.Parallel()
		rule := &model.AutoGroupRule{Conditions: "   "}
		require.Nil(t, rule.ParsedConditions())
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()
		rule := &model.AutoGroupRule{Conditions: "not json"}
		require.Nil(t, rule.ParsedConditions())
	})

	t.Run("nil rule", func(t *testing.T) {
		t.Parallel()
		var rule *model.AutoGroupRule
		require.Nil(t, rule.ParsedConditions())
	})
}

func TestIsStringMetric(t *testing.T) {
	t.Parallel()

	require.True(t, isStringMetric("current_group"))
	require.False(t, isStringMetric("total_spend"))
	require.False(t, isStringMetric("recent_spend"))
	require.False(t, isStringMetric(""))
}

func TestCompareString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		actual string
		op     string
		target string
		want   bool
	}{
		{"eq match", "vip", "==", "vip", true},
		{"eq mismatch", "vip", "==", "default", false},
		{"neq match", "vip", "!=", "default", true},
		{"neq mismatch", "vip", "!=", "vip", false},
		{"empty actual eq empty", "", "==", "", true},
		{"empty actual neq nonempty", "", "!=", "vip", true},
		{"unknown op", "vip", ">=", "vip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := compareString(tt.actual, tt.op, tt.target)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMatchConditionsWithStringMetric(t *testing.T) {
	t.Parallel()

	engine := &autoGroupEngine{}

	makeRule := func(matchMode string, conds []model.AutoGroupCondition) *compiledAutoGroupRule {
		return &compiledAutoGroupRule{
			Raw: &model.AutoGroupRule{
				MatchMode: matchMode,
			},
			Conditions: conds,
		}
	}

	t.Run("current_group eq match", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("all", []model.AutoGroupCondition{
			{Metric: "current_group", Op: "==", ValueStr: "default"},
		})
		cache := map[string]float64{}
		strCache := map[string]string{"current_group": "default"}
		got := engine.matchConditions(0, rule, cache, strCache)
		require.True(t, got)
	})

	t.Run("current_group eq mismatch", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("all", []model.AutoGroupCondition{
			{Metric: "current_group", Op: "==", ValueStr: "vip"},
		})
		cache := map[string]float64{}
		strCache := map[string]string{"current_group": "default"}
		got := engine.matchConditions(0, rule, cache, strCache)
		require.False(t, got)
	})

	t.Run("mixed numeric and string conditions all mode", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("all", []model.AutoGroupCondition{
			{Metric: "current_group", Op: "==", ValueStr: "default"},
			{Metric: "total_spend", Op: ">=", Value: 100},
		})
		cache := map[string]float64{"total_spend": 200}
		strCache := map[string]string{"current_group": "default"}
		got := engine.matchConditions(0, rule, cache, strCache)
		require.True(t, got)
	})

	t.Run("mixed conditions all mode one fails", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("all", []model.AutoGroupCondition{
			{Metric: "current_group", Op: "==", ValueStr: "vip"},
			{Metric: "total_spend", Op: ">=", Value: 100},
		})
		cache := map[string]float64{"total_spend": 200}
		strCache := map[string]string{"current_group": "default"}
		got := engine.matchConditions(0, rule, cache, strCache)
		require.False(t, got)
	})

	t.Run("current_group neq in any mode", func(t *testing.T) {
		t.Parallel()
		rule := makeRule("any", []model.AutoGroupCondition{
			{Metric: "current_group", Op: "!=", ValueStr: "svip"},
			{Metric: "total_spend", Op: ">=", Value: 9999},
		})
		cache := map[string]float64{"total_spend": 100}
		strCache := map[string]string{"current_group": "default"}
		got := engine.matchConditions(0, rule, cache, strCache)
		require.True(t, got)
	})
}
