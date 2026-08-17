package setting

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func t1ModelNameRPMTestJSON(t *testing.T, enabled bool, models map[string]modelNameRPMRule) string {
	t.Helper()
	jsonBytes, err := common.Marshal(modelNameRPMConfig{Enabled: enabled, Models: models})
	require.NoError(t, err)
	return string(jsonBytes)
}

func t1PreserveModelNameRPMSnapshot(t *testing.T) {
	t.Helper()
	previous := ModelNameRPMRateLimit2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(previous))
	})
}

func TestT1MatchModelNameRPM(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
		"gpt-4o": {
			GlobalRPM: 120,
			GroupRPM:  map[string]int{"free": 30, "vip": 100},
		},
		"gemini-2.5-pro": {GlobalRPM: 60, GroupRPM: map[string]int{"vip": 20}},
	})))

	tests := []struct {
		name      string
		model     string
		group     string
		matched   bool
		ruleModel string
		globalRPM int
		groupRPM  int
	}{
		{
			name:      "exact match with group sub-limit",
			model:     "gpt-4o",
			group:     "free",
			matched:   true,
			ruleModel: "gpt-4o",
			globalRPM: 120,
			groupRPM:  30,
		},
		{
			name:      "exact match with unknown group has no sub-limit",
			model:     "gpt-4o",
			group:     "standard",
			matched:   true,
			ruleModel: "gpt-4o",
			globalRPM: 120,
			groupRPM:  0,
		},
		{
			name:      "format matching fallback",
			model:     "gemini-2.5-pro[1m]",
			group:     "vip",
			matched:   true,
			ruleModel: "gemini-2.5-pro",
			globalRPM: 60,
			groupRPM:  20,
		},
		{
			name:    "unmatched model",
			model:   "not-configured",
			group:   "free",
			matched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := MatchModelNameRPM(tt.model, tt.group)
			assert.Equal(t, tt.matched, decision.Matched)
			assert.Equal(t, tt.ruleModel, decision.RuleModel)
			assert.Equal(t, tt.globalRPM, decision.GlobalRPM)
			assert.Equal(t, tt.groupRPM, decision.GroupRPM)
		})
	}
}

func TestT1MatchModelNameRPMDisabled(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(t1ModelNameRPMTestJSON(t, false, map[string]modelNameRPMRule{
		"gpt-4o": {GlobalRPM: 120, GroupRPM: map[string]int{"free": 30}},
	})))

	decision := MatchModelNameRPM("gpt-4o", "free")
	assert.False(t, decision.Matched)
	assert.Empty(t, decision.RuleModel)
	assert.Zero(t, decision.GlobalRPM)
	assert.Zero(t, decision.GroupRPM)
}

func TestT1ModelNameRPMRateLimitJSONStringRoundTrip(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	configJSON := t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
		"gpt-4o": {
			GlobalRPM: 120,
			GroupRPM:  map[string]int{"free": 30},
		},
		"claude-3": {GlobalRPM: 60},
	})
	require.NoError(t, CheckModelNameRPMRateLimit(configJSON))
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(configJSON))

	var decoded modelNameRPMConfig
	require.NoError(t, common.UnmarshalJsonStr(ModelNameRPMRateLimit2JSONString(), &decoded))
	assert.True(t, decoded.Enabled)
	assert.Equal(t, 120, decoded.Models["gpt-4o"].GlobalRPM)
	assert.Equal(t, 30, decoded.Models["gpt-4o"].GroupRPM["free"])
	assert.Equal(t, 60, decoded.Models["claude-3"].GlobalRPM)
	assert.Nil(t, decoded.Models["claude-3"].GroupRPM)
}

func TestT1ModelNameRPMUserRPMValidationBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
		errorPart string
	}{
		{
			name:  "missing disables user limit",
			value: `{"enabled":true,"models":{"gpt-4o":{"global_rpm":10}}}`,
		},
		{
			name:  "zero disables user limit",
			value: `{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":0}}}`,
		},
		{
			name:  "one is accepted",
			value: `{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":1}}}`,
		},
		{
			name:  "equal to global is accepted",
			value: `{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":10}}}`,
		},
		{
			name:      "over global is rejected",
			value:     `{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":11}}}`,
			wantError: true,
			errorPart: "user_rpm must not exceed global_rpm",
		},
		{
			name:      "negative is rejected",
			value:     `{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":-1}}}`,
			wantError: true,
			errorPart: "user_rpm must be at least 1 or 0 to disable",
		},
		{
			name:      "floating point is rejected",
			value:     `{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":1.5}}}`,
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := CheckModelNameRPMRateLimit(test.value)
			if !test.wantError {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if test.errorPart != "" {
				assert.ErrorContains(t, err, test.errorPart)
			}
		})
	}
}

func TestT1MatchModelNameRPMReturnsUserRPMForNormalizedRule(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gemini-2.5-pro":{"global_rpm":10,"user_rpm":3}}}`))

	decision := MatchModelNameRPM("gemini-2.5-pro[1m]", "free")
	require.True(t, decision.Matched)
	assert.Equal(t, "gemini-2.5-pro", decision.RuleModel)
	assert.Equal(t, 3, decision.UserRPM)
}

func TestT1MatchModelNameRPMNormalizesExactRuleModel(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o-gizmo-private":{"global_rpm":10,"user_rpm":3}}}`))

	decision := MatchModelNameRPM("gpt-4o-gizmo-private", "free")
	require.True(t, decision.Matched)
	assert.Equal(t, "gpt-4o-gizmo-*", decision.RuleModel)
}

func TestT1ModelNameRPMUserRPMJSONRoundTrip(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":3}}}`))

	var decoded struct {
		Models map[string]struct {
			GlobalRPM int `json:"global_rpm"`
			UserRPM   int `json:"user_rpm"`
		} `json:"models"`
	}
	require.NoError(t, common.UnmarshalJsonStr(ModelNameRPMRateLimit2JSONString(), &decoded))
	assert.Equal(t, 10, decoded.Models["gpt-4o"].GlobalRPM)
	assert.Equal(t, 3, decoded.Models["gpt-4o"].UserRPM)
}

func TestT1CloneModelNameRPMConfigPreservesUserRPM(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":3}}}`))

	var decoded struct {
		Models map[string]struct {
			UserRPM int `json:"user_rpm"`
		} `json:"models"`
	}
	require.NoError(t, common.UnmarshalJsonStr(ModelNameRPMRateLimit2JSONString(), &decoded))
	assert.Equal(t, 3, decoded.Models["gpt-4o"].UserRPM)
}

func TestT1CheckModelNameRPMRateLimitRejectsInvalidConfigurations(t *testing.T) {
	longModel := strings.Repeat("m", 256)
	longGroup := strings.Repeat("g", 65)
	tests := []struct {
		name  string
		value string
	}{
		{
			name:  "global rpm zero",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{"gpt-4o": {GlobalRPM: 0}}),
		},
		{
			name:  "global rpm negative",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{"gpt-4o": {GlobalRPM: -1}}),
		},
		{
			name:  "global rpm over maximum",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{"gpt-4o": {GlobalRPM: 1_000_001}}),
		},
		{
			name: "group rpm over global",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
				"gpt-4o": {GlobalRPM: 10, GroupRPM: map[string]int{"free": 11}},
			}),
		},
		{
			name: "group rpm below one",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
				"gpt-4o": {GlobalRPM: 10, GroupRPM: map[string]int{"free": 0}},
			}),
		},
		{
			name:  "model name too long",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{longModel: {GlobalRPM: 10}}),
		},
		{
			name: "group name too long",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
				"gpt-4o": {GlobalRPM: 10, GroupRPM: map[string]int{longGroup: 1}},
			}),
		},
		{
			name:  "model name contains whitespace",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{"gpt 4o": {GlobalRPM: 10}}),
		},
		{
			name: "group name contains whitespace",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
				"gpt-4o": {GlobalRPM: 10, GroupRPM: map[string]int{"free group": 1}},
			}),
		},
		{
			name:  "model name contains control character",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{"gpt\n4o": {GlobalRPM: 10}}),
		},
		{
			name: "group name contains control character",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
				"gpt-4o": {GlobalRPM: 10, GroupRPM: map[string]int{"free\tgroup": 1}},
			}),
		},
		{
			name:  "empty model name",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{"": {GlobalRPM: 10}}),
		},
		{
			name: "empty group name",
			value: t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
				"gpt-4o": {GlobalRPM: 10, GroupRPM: map[string]int{"": 1}},
			}),
		},
		{
			name:  "normalized model collision",
			value: `{"enabled":true,"models":{"gemini-2.5-pro-thinking-1024":{"global_rpm":10,"user_rpm":2},"gemini-2.5-pro-thinking-2048":{"global_rpm":10,"user_rpm":2}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, CheckModelNameRPMRateLimit(tt.value))
		})
	}
}

func TestT1ModelNameRPMInvalidJSONDoesNotPolluteSnapshot(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
		"gpt-4o": {GlobalRPM: 120, GroupRPM: map[string]int{"free": 30}},
	})))

	before := MatchModelNameRPM("gpt-4o", "free")
	assert.Error(t, CheckModelNameRPMRateLimit(`{"enabled":true,"models":`))
	assert.Equal(t, before, MatchModelNameRPM("gpt-4o", "free"))
	assert.Error(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":`))
	assert.Equal(t, before, MatchModelNameRPM("gpt-4o", "free"))
	assert.Error(t, UpdateModelNameRPMRateLimitByJSONString(t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{
		"gpt-4o": {GlobalRPM: 0},
	})))
	assert.Equal(t, before, MatchModelNameRPM("gpt-4o", "free"))
}

func TestT1ModelNameRPMConcurrentSnapshotAccess(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	configs := []string{
		t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{"gpt-4o": {GlobalRPM: 120, GroupRPM: map[string]int{"free": 30}}}),
		t1ModelNameRPMTestJSON(t, true, map[string]modelNameRPMRule{"claude-3": {GlobalRPM: 60}}),
	}
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(configs[0]))

	var group sync.WaitGroup
	var invalid atomic.Bool
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			for j := 0; j < 100; j++ {
				gptDecision := MatchModelNameRPM("gpt-4o", "free")
				if gptDecision.Matched && (gptDecision.RuleModel != "gpt-4o" || gptDecision.GlobalRPM != 120 || gptDecision.GroupRPM != 30) {
					invalid.Store(true)
				}
				claudeDecision := MatchModelNameRPM("claude-3", "free")
				if claudeDecision.Matched && (claudeDecision.RuleModel != "claude-3" || claudeDecision.GlobalRPM != 60 || claudeDecision.GroupRPM != 0) {
					invalid.Store(true)
				}
				if MatchModelNameRPM("not-configured", "free").Matched {
					invalid.Store(true)
				}
			}
		}()
	}
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			for j := 0; j < 50; j++ {
				if err := UpdateModelNameRPMRateLimitByJSONString(configs[index%len(configs)]); err != nil {
					// The test inputs are fixed and valid; retain the failure for
					// the main goroutine rather than calling FailNow here.
					invalid.Store(true)
				}
			}
		}(i)
	}
	close(start)
	group.Wait()

	decision := MatchModelNameRPM("gpt-4o", "free")
	assert.False(t, invalid.Load())
	if decision.Matched {
		assert.Equal(t, "gpt-4o", decision.RuleModel)
		assert.Equal(t, 120, decision.GlobalRPM)
	} else {
		claudeDecision := MatchModelNameRPM("claude-3", "free")
		assert.True(t, claudeDecision.Matched)
		assert.Equal(t, 60, claudeDecision.GlobalRPM)
	}
}

func TestT1ModelRequestRateLimitGroupConcurrentUpdateAndGet(t *testing.T) {
	previous := ModelRequestRateLimitGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(previous))
	})
	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(`{"free":[10,20]}`))
	require.Error(t, UpdateModelRequestRateLimitGroupByJSONString(`{"free":[`))
	total, success, found := GetGroupRateLimit("free")
	require.True(t, found)
	assert.Equal(t, 10, total)
	assert.Equal(t, 20, success)

	configs := []string{`{"free":[10,20]}`, `{"vip":[30,40]}`}
	var invalid atomic.Bool
	var group sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			for j := 0; j < 100; j++ {
				freeTotal, freeSuccess, freeFound := GetGroupRateLimit("free")
				vipTotal, vipSuccess, vipFound := GetGroupRateLimit("vip")
				if freeFound && (freeTotal != 10 || freeSuccess != 20) {
					invalid.Store(true)
				}
				if vipFound && (vipTotal != 30 || vipSuccess != 40) {
					invalid.Store(true)
				}
			}
		}()
	}
	for i := range configs {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			for j := 0; j < 50; j++ {
				if err := UpdateModelRequestRateLimitGroupByJSONString(configs[index]); err != nil {
					invalid.Store(true)
				}
			}
		}(i)
	}
	close(start)
	group.Wait()
	assert.False(t, invalid.Load())
}
