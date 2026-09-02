package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeExcludedUserGroups(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{name: "nil", input: nil, want: nil},
		{name: "all empty after trim", input: []string{"", "   ", "\t"}, want: nil},
		{name: "trims and drops empty", input: []string{" vip ", "", "pro"}, want: []string{"vip", "pro"}},
		{name: "dedupes preserving order", input: []string{"vip", "pro", "vip", " pro "}, want: []string{"vip", "pro"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeExcludedUserGroups(tt.input))
		})
	}
}

// seedExclusionChannels creates one high-priority channel excluding excludedGroup
// and one low-priority channel open to everyone, both in group/model.
func seedExclusionChannels(t *testing.T, group string, modelName string, excludedGroup string) (highID int, lowID int) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	highPriority := int64(20)
	lowPriority := int64(10)
	weight := uint(1)
	high := &Channel{Id: 9201, Name: "excluded-high", Key: "k1", Status: common.ChannelStatusEnabled, Group: group, Models: modelName, Priority: &highPriority, Weight: &weight}
	low := &Channel{Id: 9202, Name: "open-low", Key: "k2", Status: common.ChannelStatusEnabled, Group: group, Models: modelName, Priority: &lowPriority, Weight: &weight}
	high.SetSetting(dto.ChannelSettings{ExcludedUserGroups: []string{excludedGroup}})
	for _, channel := range []*Channel{high, low} {
		require.NoError(t, DB.Create(channel).Error)
		require.NoError(t, DB.Create(&Ability{
			Group: group, Model: modelName, ChannelId: channel.Id, Enabled: true,
			Priority: channel.Priority, Weight: *channel.Weight,
		}).Error)
	}
	return high.Id, low.Id
}

func withMemoryCache(t *testing.T, enabled bool) {
	t.Helper()
	original := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = enabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = original
		if original {
			InitChannelCache()
		}
	})
}

// The memory branch must drop excluded channels BEFORE the priority tier is
// picked, so an excluded top-priority channel does not swallow the first attempt.
func TestGetSatisfiedChannelCandidatesExcludesUserGroupBeforePriority(t *testing.T) {
	withMemoryCache(t, true)
	truncateTables(t)
	highID, lowID := seedExclusionChannels(t, "default", "excl-model", "vip")
	InitChannelCache()

	excluded, err := GetSatisfiedChannelCandidates("default", "excl-model", 0, "", "vip")
	require.NoError(t, err)
	require.Len(t, excluded, 1)
	assert.Equal(t, lowID, excluded[0].Id, "an excluded top-priority channel must not cost a retry")

	allowed, err := GetSatisfiedChannelCandidates("default", "excl-model", 0, "", "normal")
	require.NoError(t, err)
	require.Len(t, allowed, 1)
	assert.Equal(t, highID, allowed[0].Id)

	anonymous, err := GetSatisfiedChannelCandidates("default", "excl-model", 0, "", "")
	require.NoError(t, err)
	require.Len(t, anonymous, 1)
	assert.Equal(t, highID, anonymous[0].Id, "an empty user group must never be treated as excluded")
}

// The database branch keeps the pre-existing post-priority filter shape shared
// with group cooling: it is fail-closed, at the cost of not descending a tier.
func TestGetSatisfiedChannelCandidatesExcludesUserGroupOnDatabaseBranch(t *testing.T) {
	withMemoryCache(t, false)
	truncateTables(t)
	_, lowID := seedExclusionChannels(t, "default", "excl-model", "vip")

	excluded, err := GetSatisfiedChannelCandidates("default", "excl-model", 0, "", "vip")
	require.NoError(t, err)
	assert.Empty(t, excluded, "the excluded channel must never be admitted")

	next, err := GetSatisfiedChannelCandidates("default", "excl-model", 1, "", "vip")
	require.NoError(t, err)
	require.Len(t, next, 1)
	assert.Equal(t, lowID, next[0].Id)
}

func TestIsChannelAvailableForUserGroup(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			withMemoryCache(t, memoryCacheEnabled)
			truncateTables(t)
			highID, lowID := seedExclusionChannels(t, "default", "excl-model", "vip")
			if memoryCacheEnabled {
				InitChannelCache()
			}

			assert.False(t, IsChannelAvailableForUserGroup("default", "excl-model", highID, "vip"))
			assert.True(t, IsChannelAvailableForUserGroup("default", "excl-model", highID, "normal"))
			assert.True(t, IsChannelAvailableForUserGroup("default", "excl-model", highID, ""))
			assert.True(t, IsChannelAvailableForUserGroup("default", "excl-model", lowID, "vip"))
			assert.False(t, IsChannelAvailableForUserGroup("other-group", "excl-model", highID, "normal"),
				"the wrapper must keep the underlying group/model gate")
			assert.False(t, IsChannelAvailableForUserGroup("default", "excl-model", 999999, "vip"),
				"an unknown channel must not be admitted through the affinity shortcut")
		})
	}
}

// A channel edited through the incremental cache-update path must not keep a
// stale exclusion set in either direction.
func TestCacheUpdateChannelRefreshesExclusionIndex(t *testing.T) {
	withMemoryCache(t, true)
	truncateTables(t)
	highID, _ := seedExclusionChannels(t, "default", "excl-model", "vip")
	InitChannelCache()
	require.True(t, IsChannelExcludedForUserGroup(highID, "vip"))

	updated, err := GetChannelById(highID, true)
	require.NoError(t, err)
	updated.SetSetting(dto.ChannelSettings{})
	CacheUpdateChannel(updated)
	assert.False(t, IsChannelExcludedForUserGroup(highID, "vip"), "clearing the list must clear the index")

	updated.SetSetting(dto.ChannelSettings{ExcludedUserGroups: []string{"pro"}})
	CacheUpdateChannel(updated)
	assert.False(t, IsChannelExcludedForUserGroup(highID, "vip"))
	assert.True(t, IsChannelExcludedForUserGroup(highID, "pro"))
}

func TestFilterModelsAvailableForUserGroup(t *testing.T) {
	withMemoryCache(t, true)
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	priority := int64(10)
	weight := uint(1)
	// blocked-model is served only by a channel that excludes vip;
	// mixed-model is served by that channel and an open one.
	blocking := &Channel{Id: 9301, Name: "blocking", Key: "k1", Status: common.ChannelStatusEnabled, Group: "default", Models: "blocked-model,mixed-model", Priority: &priority, Weight: &weight}
	blocking.SetSetting(dto.ChannelSettings{ExcludedUserGroups: []string{"vip"}})
	open := &Channel{Id: 9302, Name: "open", Key: "k2", Status: common.ChannelStatusEnabled, Group: "default", Models: "mixed-model", Priority: &priority, Weight: &weight}
	for _, channel := range []*Channel{blocking, open} {
		require.NoError(t, DB.Create(channel).Error)
		for _, modelName := range strings.Split(channel.Models, ",") {
			require.NoError(t, DB.Create(&Ability{
				Group: "default", Model: modelName, ChannelId: channel.Id, Enabled: true,
				Priority: channel.Priority, Weight: *channel.Weight,
			}).Error)
		}
	}
	InitChannelCache()

	models := []string{"blocked-model", "mixed-model", "unknown-model"}
	assert.Equal(t, []string{"mixed-model", "unknown-model"},
		FilterModelsAvailableForUserGroup("default", models, "vip"))
	assert.Equal(t, models, FilterModelsAvailableForUserGroup("default", models, "normal"))
	assert.Equal(t, models, FilterModelsAvailableForUserGroup("default", models, ""))
	assert.Equal(t, models, FilterModelsAvailableForUserGroup("", models, "vip"))
}
