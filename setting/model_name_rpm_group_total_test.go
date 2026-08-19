package setting

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupTotalRPMOldConfigRoundTripAndGroupsOnlyMatch(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	oldConfig := `{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":2}}}`
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(oldConfig))
	var decoded ModelNameRPMConfig
	require.NoError(t, common.UnmarshalJsonStr(ModelNameRPMRateLimit2JSONString(), &decoded))
	assert.Empty(t, decoded.Groups)
	assert.NotContains(t, ModelNameRPMRateLimit2JSONString(), `"groups"`)

	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{},"groups":{"vip_2_cheap":{"total_rpm":30}}}`))
	decision := MatchModelNameRPM("not-configured", "vip_2_cheap")
	assert.False(t, decision.Matched)
	assert.Equal(t, 30, decision.GroupTotalRPM)
	assert.Zero(t, decision.GlobalRPM)
	assert.Zero(t, MatchModelNameRPM("not-configured", "").GroupTotalRPM)
}

func TestGroupTotalRPMDeepCopyAndVersion(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	before := ModelNameRPMConfigVersion()
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":30}}}`))
	assert.Greater(t, ModelNameRPMConfigVersion(), before)

	rules := ListModelNameRPMRules()
	rules.Groups["vip"] = GroupTotalRPMRule{TotalRPM: 99}
	latest := ListModelNameRPMRules()
	assert.Equal(t, 30, latest.Groups["vip"].TotalRPM)
}

func TestGroupTotalRPMValidationBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
		errorPart string
	}{
		{name: "groups null is treated as empty", value: `{"enabled":true,"models":{},"groups":null}`},
		{name: "groups must be an object", value: `{"enabled":true,"models":{},"groups":[]}`, wantError: true},
		{name: "neither limit configured", value: `{"enabled":true,"models":{},"groups":{"vip":{}}}`, errorPart: `group "vip" has neither total_rpm nor user_rpm configured; remove the group entry to disable it`},
		{name: "explicit zero limits", value: `{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":0,"user_rpm":0}}}`, errorPart: "has neither total_rpm nor user_rpm configured"},
		{name: "negative total", value: `{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":-1}}}`, errorPart: "total_rpm must not be negative"},
		{name: "user only", value: `{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":0,"user_rpm":20}}}`},
		{name: "legacy total only", value: `{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":1}}}`},
		{name: "user equals total", value: `{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":20,"user_rpm":20}}}`},
		{name: "user exceeds total", value: `{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":20,"user_rpm":21}}}`, errorPart: "user_rpm must not exceed total_rpm"},
		{name: "negative user", value: `{"enabled":true,"models":{},"groups":{"vip":{"user_rpm":-1}}}`, errorPart: "user_rpm must not be negative"},
		{name: "maximum", value: `{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":1000000}}}`},
		{name: "total over maximum", value: `{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":1000001}}}`, errorPart: "total_rpm must not exceed 1000000"},
		{name: "user over maximum", value: `{"enabled":true,"models":{},"groups":{"vip":{"user_rpm":1000001}}}`, errorPart: "user_rpm must not exceed 1000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckModelNameRPMRateLimit(tt.value)
			if tt.wantError || tt.errorPart != "" {
				require.Error(t, err)
				if tt.errorPart != "" {
					assert.ErrorContains(t, err, tt.errorPart)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGroupTotalRPMGroupNameValidation(t *testing.T) {
	longName := strings.Repeat("界", 65)
	tests := []struct {
		name      string
		group     string
		wantError bool
	}{
		{name: "empty", group: "", wantError: true},
		{name: "whitespace", group: "   ", wantError: true},
		{name: "too long", group: longName, wantError: true},
		{name: "control", group: "vip\nprod", wantError: true},
		{name: "colon is valid", group: "tenant:vip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := common.Marshal(struct {
				Enabled bool                         `json:"enabled"`
				Models  map[string]ModelNameRPMRule  `json:"models"`
				Groups  map[string]GroupTotalRPMRule `json:"groups"`
			}{
				Enabled: true,
				Models:  map[string]ModelNameRPMRule{},
				Groups:  map[string]GroupTotalRPMRule{tt.group: {TotalRPM: 1}},
			})
			require.NoError(t, err)
			err = CheckModelNameRPMRateLimit(string(payload))
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGroupTotalRPMMatcherFourQuadrantsAndNormalizedModel(t *testing.T) {
	t1PreserveModelNameRPMSnapshot(t)
	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gemini-2.5-pro":{"global_rpm":10}},"groups":{"vip":{"total_rpm":20,"user_rpm":7},"personal":{"user_rpm":9}}}`))
	tests := []struct {
		name          string
		model         string
		group         string
		wantMatched   bool
		wantRuleModel string
		wantTotal     int
		wantUser      int
	}{
		{name: "model and group", model: "gemini-2.5-pro", group: "vip", wantMatched: true, wantRuleModel: "gemini-2.5-pro", wantTotal: 20, wantUser: 7},
		{name: "model only", model: "gemini-2.5-pro", group: "free", wantMatched: true, wantRuleModel: "gemini-2.5-pro"},
		{name: "group only", model: "unknown", group: "vip", wantTotal: 20, wantUser: 7},
		{name: "group user only", model: "unknown", group: "personal", wantUser: 9},
		{name: "neither", model: "unknown", group: "free"},
		{name: "normalized model and group", model: "gemini-2.5-pro[1m]", group: "vip", wantMatched: true, wantRuleModel: "gemini-2.5-pro", wantTotal: 20, wantUser: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchModelNameRPM(tt.model, tt.group)
			assert.Equal(t, tt.wantMatched, got.Matched)
			assert.Equal(t, tt.wantRuleModel, got.RuleModel)
			assert.Equal(t, tt.wantTotal, got.GroupTotalRPM)
			assert.Equal(t, tt.wantUser, got.GroupUserRPM)
		})
	}

	require.NoError(t, UpdateModelNameRPMRateLimitByJSONString(`{"enabled":false,"models":{},"groups":{"vip":{"total_rpm":20,"user_rpm":7}}}`))
	disabled := MatchModelNameRPM("unknown", "vip")
	assert.False(t, disabled.Matched)
	assert.Zero(t, disabled.GroupTotalRPM)
	assert.Zero(t, disabled.GroupUserRPM)
}
