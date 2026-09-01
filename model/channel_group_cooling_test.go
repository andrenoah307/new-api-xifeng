package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelGroupCoolingOverlay(t *testing.T) {
	SetChannelGroupCooling(nil)
	t.Cleanup(func() { SetChannelGroupCooling(nil) })

	tests := []struct {
		name    string
		overlay map[int]map[string]struct{}
		id      int
		group   string
		want    bool
		active  bool
	}{
		{name: "nil overlay", overlay: nil, id: 1, group: "pro", want: false, active: false},
		{name: "empty overlay", overlay: map[int]map[string]struct{}{}, id: 1, group: "pro", want: false, active: false},
		{name: "pro hit", overlay: map[int]map[string]struct{}{1: {"pro": {}}}, id: 1, group: "pro", want: true, active: true},
		{name: "cheap unaffected", overlay: map[int]map[string]struct{}{1: {"pro": {}}}, id: 1, group: "cheap", want: false, active: true},
		{name: "other channel unaffected", overlay: map[int]map[string]struct{}{1: {"pro": {}}}, id: 2, group: "pro", want: false, active: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			SetChannelGroupCooling(test.overlay)
			assert.Equal(t, test.want, IsChannelGroupCooled(test.id, test.group))
			assert.Equal(t, test.active, groupCoolingActive.Load())
		})
	}

	SetChannelGroupCooling(nil)
	assert.False(t, IsChannelGroupCooled(1, "cheap"))
	assert.False(t, groupCoolingActive.Load())
}

func TestChannelGroupCoolingOverlayIsGroupScopedAndSnapshotIsolated(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[int]map[string]struct{})
	}{
		{
			name: "mutate outer map",
			mutate: func(cooling map[int]map[string]struct{}) {
				delete(cooling, 7)
			},
		},
		{
			name: "mutate nested group map",
			mutate: func(cooling map[int]map[string]struct{}) {
				delete(cooling[7], "pro")
				cooling[7]["cheap"] = struct{}{}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cooling := map[int]map[string]struct{}{7: {"pro": {}}}
			SetChannelGroupCooling(cooling)
			t.Cleanup(func() { SetChannelGroupCooling(nil) })

			test.mutate(cooling)
			assert.True(t, IsChannelGroupCooled(7, "pro"), "the stored snapshot must not alias caller-owned maps")
			assert.False(t, IsChannelGroupCooled(7, "cheap"))
			require.True(t, groupCoolingActive.Load())
		})
	}
}

func TestSetChannelGroupCoolingDropsEmptyGroupEntries(t *testing.T) {
	SetChannelGroupCooling(nil)
	t.Cleanup(func() { SetChannelGroupCooling(nil) })

	SetChannelGroupCooling(map[int]map[string]struct{}{
		1001: {},
		1002: {"pro": {}},
	})

	assert.False(t, IsChannelGroupCooled(1001, "pro"), "an empty group entry must be discarded")
	assert.True(t, IsChannelGroupCooled(1002, "pro"))
	assert.True(t, groupCoolingActive.Load())

	SetChannelGroupCooling(map[int]map[string]struct{}{
		1001: {},
		1002: {},
	})

	assert.False(t, groupCoolingActive.Load(), "an overlay with only empty entries must be disabled")
	assert.False(t, IsChannelGroupCooled(1001, "pro"))
	assert.Equal(t, []int{1001, 1002}, filterCooledChannelIDs([]int{1001, 1002}, "pro"))
	assert.Nil(t, cooledChannelIDsForGroup("pro"))
}

func TestChannelGroupCoolingFiltersHandleEmptySnapshotAndNilChannels(t *testing.T) {
	SetChannelGroupCooling(nil)
	t.Cleanup(func() { SetChannelGroupCooling(nil) })

	assert.Nil(t, filterCooledChannelIDs(nil, "pro"))
	assert.Nil(t, filterCooledChannels(nil, "pro"))

	ids := []int{1101, 1102}
	channels := []*Channel{{Id: 1101}, {Id: 1102}}
	SetChannelGroupCooling(map[int]map[string]struct{}{1101: {"pro": {}}})
	assert.Empty(t, filterCooledChannelIDs([]int{}, "pro"))
	assert.Empty(t, filterCooledChannels([]*Channel{}, "pro"))

	// Exercise the defensive path where the active flag and pointer are observed
	// in an inconsistent state during a concurrent snapshot replacement.
	groupCoolingActive.Store(true)
	groupCoolingOverlay.Store(nil)
	assert.False(t, IsChannelGroupCooled(1101, "pro"))
	assert.Equal(t, ids, filterCooledChannelIDs(ids, "pro"))
	assert.Equal(t, channels, filterCooledChannels(channels, "pro"))
	assert.Nil(t, cooledChannelIDsForGroup("pro"))

	SetChannelGroupCooling(map[int]map[string]struct{}{
		1101: {"pro": {}},
		1102: {"cheap": {}},
		1103: {"pro": {}, "cheap": {}},
	})
	assert.Equal(t, []int{1102}, filterCooledChannelIDs([]int{1101, 1102, 1103}, "pro"))

	available := &Channel{Id: 1102}
	unknown := &Channel{Id: 1104}
	filtered := filterCooledChannels([]*Channel{nil, {Id: 1101}, available, unknown}, "pro")
	require.Equal(t, []*Channel{nil, available, unknown}, filtered)
}

func TestCooledChannelIDsForGroupReturnsOnlyMatchingGroup(t *testing.T) {
	SetChannelGroupCooling(map[int]map[string]struct{}{
		1201: {"pro": {}},
		1202: {"cheap": {}},
		1203: {"pro": {}, "cheap": {}},
	})
	t.Cleanup(func() { SetChannelGroupCooling(nil) })

	assert.ElementsMatch(t, []int{1201, 1203}, cooledChannelIDsForGroup("pro"))
	assert.ElementsMatch(t, []int{1202, 1203}, cooledChannelIDsForGroup("cheap"))
}
