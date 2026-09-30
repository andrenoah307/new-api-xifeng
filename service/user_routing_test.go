package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRoutingSelectionPreservesOriginalGroup(t *testing.T) {
	for _, memory := range []bool{false, true} {
		t.Run(fmt.Sprint(memory), func(t *testing.T) {
			prepareSatisfiedChannelTest(t, memory)
			require.NoError(t, model.DB.AutoMigrate(&model.UserRoutingPolicy{}))
			require.NoError(t, model.SaveUserRoutingRules(71, []model.UserRoutingRule{
				{SourceGroup: "source", Model: "routed-model", TargetGroup: "destination", Enabled: true},
			}))
			t.Cleanup(func() { require.NoError(t, model.SaveUserRoutingRules(71, nil)) })
			insertSatisfiedChannel(t, 9301, "original", "source", "routed-model", 1, nil)
			insertSatisfiedChannel(t, 9302, "routed", "destination", "routed-model", 1, nil)
			if memory {
				model.InitChannelCache()
			}
			c := newSatisfiedChannelContext()
			common.SetContextKey(c, constant.ContextKeyUserId, 71)
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "source")
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			for _, retry := range []int{0, 1} {
				ch, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: c, TokenGroup: "source", ModelName: "routed-model", Retry: &retry})
				require.NoError(t, err)
				require.NotNil(t, ch)
				assert.Equal(t, 9302, ch.Id)
				assert.Equal(t, "source", group)
				assert.Equal(t, "source", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
			}
			// An in-flight request retains its policy even when an administrator edits it.
			require.NoError(t, model.SaveUserRoutingRules(71, nil))
			assert.Equal(t, "destination", UserRoutingTargetGroup(c, "source", "routed-model"))
			assert.Equal(t, "source", UserRoutingTargetGroup(newSatisfiedChannelContext(), "source", "routed-model"))
		})
	}
}

func TestUserRoutingAutoAndUnavailableTarget(t *testing.T) {
	prepareSatisfiedChannelTest(t, false)
	require.NoError(t, model.DB.AutoMigrate(&model.UserRoutingPolicy{}))
	require.NoError(t, model.SaveUserRoutingRules(72, []model.UserRoutingRule{{SourceGroup: "default", TargetGroup: "destination", Enabled: true}}))
	t.Cleanup(func() { require.NoError(t, model.SaveUserRoutingRules(72, nil)) })
	original, err := common.Marshal(setting.GetAutoGroups())
	require.NoError(t, err)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	t.Cleanup(func() { require.NoError(t, setting.UpdateAutoGroupsByJsonString(string(original))) })
	insertSatisfiedChannel(t, 9303, "original", "default", "model-a", 1, nil)
	c := newSatisfiedChannelContext()
	c.Set("id", 72)
	c.Set("group", "auto")
	c.Set("user_group", "default")
	ch, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: c, TokenGroup: "auto", ModelName: "model-a"})
	require.NoError(t, err)
	assert.Nil(t, ch, "a matching rule must not fall back to the original group's channels")
	insertSatisfiedChannel(t, 9304, "destination", "destination", "model-a", 1, nil)
	c = newSatisfiedChannelContext()
	c.Set("id", 72)
	c.Set("group", "auto")
	c.Set("user_group", "default")
	ch, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: c, TokenGroup: "auto", ModelName: "model-a"})
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.Equal(t, 9304, ch.Id)
	assert.Equal(t, "default", group)
	assert.Equal(t, "default", c.GetString("auto_group"))
	assert.Equal(t, "default", RequestAccountingGroup(c))
}

func TestUserRoutingAffinityIsolation(t *testing.T) {
	cfg := operation_setting.GetChannelAffinitySetting()
	original := *cfg
	t.Cleanup(func() { *cfg = original; ClearChannelAffinityCacheAll() })
	cfg.Enabled = true
	cfg.Rules = []operation_setting.ChannelAffinityRule{{Name: "routing-test", ModelRegex: []string{".*"}, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Affinity"}}}}
	rules := []model.UserRoutingRule{{SourceGroup: "default", Model: "model-a", TargetGroup: "destination", Enabled: true}}
	c := newSatisfiedChannelContext()
	c.Set("id", 73)
	c.Set(userRoutingSnapshotKey, rules)
	c.Request.Header.Set("X-Affinity", "routing-isolation")
	_, found := GetPreferredChannelByAffinity(c, "model-a", "default")
	require.False(t, found)
	RecordChannelAffinity(c, 9305)
	channel, found := GetPreferredChannelByAffinity(c, "model-a", "default")
	require.True(t, found)
	assert.Equal(t, 9305, channel)
	for _, tc := range []struct {
		id    int
		rules []model.UserRoutingRule
	}{
		{74, rules}, {73, nil}, {73, []model.UserRoutingRule{{SourceGroup: "default", TargetGroup: "changed", Enabled: true}}},
	} {
		other := newSatisfiedChannelContext()
		other.Set("id", tc.id)
		other.Set(userRoutingSnapshotKey, tc.rules)
		other.Request.Header.Set("X-Affinity", "routing-isolation")
		_, found := GetPreferredChannelByAffinity(other, "model-a", "default")
		assert.False(t, found, "another user or policy must not reuse the routed channel")
	}
	// Unmatched models keep the existing affinity namespace and warm target.
	_, found = GetPreferredChannelByAffinity(c, "model-b", "default")
	require.False(t, found)
	RecordChannelAffinity(c, 9306)
	plain := newSatisfiedChannelContext()
	plain.Request.Header.Set("X-Affinity", "routing-isolation")
	channel, found = GetPreferredChannelByAffinity(plain, "model-b", "default")
	require.True(t, found)
	assert.Equal(t, 9306, channel)
}

func TestUserRoutingMatching(t *testing.T) {
	rules := []model.UserRoutingRule{
		{SourceGroup: "source", Model: "model-a", TargetGroup: "disabled", Enabled: false},
		{SourceGroup: "source", Model: "model-a", TargetGroup: "specific", Enabled: true},
		{SourceGroup: "source", TargetGroup: "group-only", Enabled: true},
		{Model: "model-a", TargetGroup: "model-only", Enabled: true},
	}
	for _, tc := range []struct{ group, model, want string }{
		{"source", "model-a", "specific"}, {"source", "model-b", "group-only"},
		{"other", "model-a", "model-only"}, {"other", "model-b", "other"},
	} {
		c := newSatisfiedChannelContext()
		c.Set(userRoutingSnapshotKey, rules)
		assert.Equal(t, tc.want, UserRoutingTargetGroup(c, tc.group, tc.model))
	}
}
