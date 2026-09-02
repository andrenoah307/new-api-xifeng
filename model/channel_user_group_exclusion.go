package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// channel2excludedUserGroups caches each channel's parsed "excluded user groups"
// so relay selection never re-parses channels.setting JSON per request.
// Channels excluding nobody have no entry at all, so the common case is a single
// miss on a small map. Refreshed on full sync (InitChannelCache) and on
// CacheUpdateChannel; guarded by channelSyncLock together with
// group2model2channels / channelsIDM / channel2advancedCustomConfig.
var channel2excludedUserGroups map[int]map[string]struct{}

// NormalizeExcludedUserGroups trims whitespace, drops empty entries and removes
// duplicates while preserving the order the administrator entered. It returns nil
// when nothing remains so the field is omitted from channels.setting entirely.
func NormalizeExcludedUserGroups(groups []string) []string {
	if len(groups) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(groups))
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		normalized = append(normalized, group)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// buildChannelExcludedUserGroupSet returns nil when the channel excludes nobody,
// which keeps the index sparse.
func buildChannelExcludedUserGroupSet(channel *Channel) map[string]struct{} {
	if channel == nil {
		return nil
	}
	excluded := NormalizeExcludedUserGroups(channel.GetSetting().ExcludedUserGroups)
	if len(excluded) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(excluded))
	for _, group := range excluded {
		set[group] = struct{}{}
	}
	return set
}

// isChannelExcludedForUserGroup reports whether the channel refuses this user
// group. Caller must hold channelSyncLock (read lock).
func isChannelExcludedForUserGroup(channelID int, userGroup string) bool {
	if userGroup == "" || len(channel2excludedUserGroups) == 0 {
		return false
	}
	excluded, ok := channel2excludedUserGroups[channelID]
	if !ok {
		return false
	}
	_, hit := excluded[userGroup]
	return hit
}

// IsChannelExcludedForUserGroup is the lock-taking variant used outside the
// selection hot path (console model lists, affinity checks).
func IsChannelExcludedForUserGroup(channelID int, userGroup string) bool {
	if userGroup == "" || channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(channelID, true)
		if err != nil || channel == nil {
			// Treat an unreadable channel as excluded. Every caller falls back to
			// ordinary selection, which re-applies the filter, so failing closed
			// costs one normal lookup instead of admitting a loss-making route.
			return true
		}
		_, hit := buildChannelExcludedUserGroupSet(channel)[userGroup]
		return hit
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	return isChannelExcludedForUserGroup(channelID, userGroup)
}

// FilterModelsAvailableForUserGroup drops models whose every channel in
// routingGroup excludes this user group, so the console and /v1/models stop
// advertising models the caller can never actually reach.
//
// It is deliberately cache-only: issuing one query per model would turn a console
// page load into a query storm. Without the memory cache the list is returned
// unchanged; relay selection stays authoritative and fail-closed either way.
func FilterModelsAvailableForUserGroup(routingGroup string, models []string, userGroup string) []string {
	if userGroup == "" || routingGroup == "" || len(models) == 0 || !common.MemoryCacheEnabled {
		return models
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if len(channel2excludedUserGroups) == 0 {
		return models
	}
	filtered := make([]string, 0, len(models))
	for _, modelName := range models {
		channels := group2model2channels[routingGroup][modelName]
		if len(channels) == 0 {
			// Not represented in the routing cache; leave existing behaviour alone.
			filtered = append(filtered, modelName)
			continue
		}
		for _, channelID := range channels {
			if !isChannelExcludedForUserGroup(channelID, userGroup) {
				filtered = append(filtered, modelName)
				break
			}
		}
	}
	return filtered
}

// filterExcludedUserGroupChannelIDs drops channels that exclude this user group.
// Caller must hold channelSyncLock (read lock). The cached slice is never mutated.
func filterExcludedUserGroupChannelIDs(channelIDs []int, userGroup string) []int {
	if userGroup == "" || len(channelIDs) == 0 || len(channel2excludedUserGroups) == 0 {
		return channelIDs
	}
	filtered := make([]int, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if isChannelExcludedForUserGroup(channelID, userGroup) {
			continue
		}
		filtered = append(filtered, channelID)
	}
	return filtered
}

// filterExcludedUserGroupChannels is the non-cached counterpart used by the
// database selection branch, where no channel index exists. It reads the setting
// off the already loaded channel, so each channel is parsed at most once.
func filterExcludedUserGroupChannels(channels []*Channel, userGroup string) []*Channel {
	if userGroup == "" || len(channels) == 0 {
		return channels
	}
	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			filtered = append(filtered, channel)
			continue
		}
		if _, hit := buildChannelExcludedUserGroupSet(channel)[userGroup]; hit {
			continue
		}
		filtered = append(filtered, channel)
	}
	return filtered
}
