package model

import "sync/atomic"

var groupCoolingActive atomic.Bool
var groupCoolingOverlay atomic.Pointer[map[int]map[string]struct{}]

// SetChannelGroupCooling replaces the complete group cooling snapshot.
// The caller may safely reuse or mutate m after this function returns.
func SetChannelGroupCooling(m map[int]map[string]struct{}) {
	if len(m) == 0 {
		groupCoolingOverlay.Store(nil)
		groupCoolingActive.Store(false)
		return
	}

	snapshot := make(map[int]map[string]struct{}, len(m))
	for channelID, groups := range m {
		if len(groups) == 0 {
			continue
		}
		groupSnapshot := make(map[string]struct{}, len(groups))
		for group := range groups {
			groupSnapshot[group] = struct{}{}
		}
		snapshot[channelID] = groupSnapshot
	}
	if len(snapshot) == 0 {
		groupCoolingOverlay.Store(nil)
		groupCoolingActive.Store(false)
		return
	}

	groupCoolingOverlay.Store(&snapshot)
	groupCoolingActive.Store(true)
}

// IsChannelGroupCooled checks the immutable runtime snapshot. Keep the empty
// snapshot path to one atomic load because this function is used per request.
func IsChannelGroupCooled(channelId int, group string) bool {
	if !groupCoolingActive.Load() {
		return false
	}
	snapshot := groupCoolingOverlay.Load()
	if snapshot == nil {
		return false
	}
	groups, ok := (*snapshot)[channelId]
	if !ok {
		return false
	}
	_, ok = groups[group]
	return ok
}

func filterCooledChannelIDs(channelIDs []int, group string) []int {
	if !groupCoolingActive.Load() || len(channelIDs) == 0 {
		return channelIDs
	}
	snapshot := groupCoolingOverlay.Load()
	if snapshot == nil {
		return channelIDs
	}
	filtered := make([]int, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		groups, ok := (*snapshot)[channelID]
		if !ok {
			filtered = append(filtered, channelID)
			continue
		}
		if _, cooled := groups[group]; !cooled {
			filtered = append(filtered, channelID)
		}
	}
	return filtered
}

func filterCooledChannels(channels []*Channel, group string) []*Channel {
	if !groupCoolingActive.Load() || len(channels) == 0 {
		return channels
	}
	snapshot := groupCoolingOverlay.Load()
	if snapshot == nil {
		return channels
	}
	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			filtered = append(filtered, channel)
			continue
		}
		groups, ok := (*snapshot)[channel.Id]
		if !ok {
			filtered = append(filtered, channel)
			continue
		}
		if _, cooled := groups[group]; !cooled {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

func cooledChannelIDsForGroup(group string) []int {
	if !groupCoolingActive.Load() {
		return nil
	}
	snapshot := groupCoolingOverlay.Load()
	if snapshot == nil {
		return nil
	}
	channelIDs := make([]int, 0)
	for channelID, groups := range *snapshot {
		if _, cooled := groups[group]; cooled {
			channelIDs = append(channelIDs, channelID)
		}
	}
	return channelIDs
}
