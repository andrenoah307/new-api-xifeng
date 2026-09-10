package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 契约：未配置白名单必须返回 nil。控制器用 `allowed == nil` 表示"该分组不做模型过滤"，
// 若这里返回空切片，管理员没配过白名单的分组会被整组过滤成空，模型性能卡片全部消失。
func TestPerfCardModelsForGroup(t *testing.T) {
	cases := []struct {
		name     string
		configed map[string][]string
		group    string
		want     []string
	}{
		{"map 为 nil", nil, "default", nil},
		{"分组未配置", map[string][]string{"vip": {"gpt-4o"}}, "default", nil},
		{"分组配置为空切片", map[string][]string{"default": {}}, "default", nil},
		{"分组配置了白名单", map[string][]string{"default": {"gpt-4o", "claude-3"}}, "default", []string{"gpt-4o", "claude-3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := GroupMonitoringSetting{PerfCardGroupModels: tc.configed}
			assert.Equal(t, tc.want, s.PerfCardModelsForGroup(tc.group))
		})
	}
}

// 契约：PerfCardGroups 是白名单，只有列进去的分组才展示模型性能卡片。
// 未配置（nil / 空切片）表示"一个都没启用"，必须返回 false —— 白名单语义下
// 空集合就是空集合，不能像黑名单那样被理解成"全部放行"。
func TestIsPerfCardGroupEnabled(t *testing.T) {
	s := GroupMonitoringSetting{PerfCardGroups: []string{"internal", "staging"}}
	assert.True(t, s.IsPerfCardGroupEnabled("internal"))
	assert.True(t, s.IsPerfCardGroupEnabled("staging"))
	assert.False(t, s.IsPerfCardGroupEnabled("default"))
	assert.False(t, s.IsPerfCardGroupEnabled(""))
	assert.False(t, GroupMonitoringSetting{}.IsPerfCardGroupEnabled("internal"))
	assert.False(t, GroupMonitoringSetting{PerfCardGroups: []string{}}.IsPerfCardGroupEnabled("internal"))
}

// 契约：TopN 必须落在 [1, 50]。0/负数来自"管理员清空输入框"，必须回落到默认值而不是
// 渲染 0 个模型；上界防止管理员填入极大值后前端一次性渲染全部模型。
func TestPerfCardTopNOrDefault(t *testing.T) {
	cases := []struct {
		configed int
		want     int
	}{
		{0, 6},
		{-1, 6},
		{1, 1},
		{6, 6},
		{50, 50},
		{51, 50},
		{100000, 50},
	}
	for _, tc := range cases {
		s := GroupMonitoringSetting{PerfCardTopN: tc.configed}
		assert.Equal(t, tc.want, s.PerfCardTopNOrDefault(), "PerfCardTopN=%d", tc.configed)
	}
}

// 默认值即生产未配置时的行为：卡片总开关开启，但白名单为空，因此实际不展示任何分组。
func TestGroupMonitoringPerfCardDefaults(t *testing.T) {
	s := GetGroupMonitoringSetting()
	assert.True(t, s.PerfCardEnabled)
	assert.False(t, s.PerfCardShowAllModels)
	assert.Equal(t, 6, s.PerfCardTopNOrDefault())
	assert.Empty(t, s.PerfCardGroups, "默认不启用任何分组的模型性能卡片，管理员必须显式勾选")
	assert.Nil(t, s.PerfCardModelsForGroup("default"))
}
