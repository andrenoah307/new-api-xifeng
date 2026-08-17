package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/user_model_rpm"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capacityInspectorStub struct {
	mu      sync.Mutex
	counts  []int
	err     error
	calls   int
	started chan struct{}
	release chan struct{}
}

func (s *capacityInspectorStub) Inspect(context.Context, []string) ([]int, error) {
	s.mu.Lock()
	s.calls++
	counts, err := append([]int(nil), s.counts...), s.err
	started, wait := s.started, s.release
	s.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if wait != nil {
		<-wait
	}
	return counts, err
}

func (s *capacityInspectorStub) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestRateLimitCapacitySnapshotCacheAndVersionInvalidation(t *testing.T) {
	previous := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"group_rpm":{"free":5}}}}`))

	stub := &capacityInspectorStub{counts: []int{3, 2}}
	now := time.Unix(100, 0)
	svc := NewRateLimitCapacityService(stub)
	svc.SetClock(func() time.Time { return now })

	first, err := svc.SiteSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, stub.Calls())
	second, err := svc.SiteSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, first.ObservedAt, second.ObservedAt)
	assert.Equal(t, 1, stub.Calls())

	now = now.Add(6 * time.Second)
	_, err = svc.SiteSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, stub.Calls())

	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":11,"group_rpm":{"free":5}}}}`))
	_, err = svc.SiteSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, stub.Calls())
}

func TestRateLimitCapacitySingleflightCollapsesConcurrentRefresh(t *testing.T) {
	previous := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10}}}`))

	started := make(chan struct{}, 2)
	continueCh := make(chan struct{})
	stub := &capacityInspectorStub{counts: []int{3}, started: started, release: continueCh}
	svc := NewRateLimitCapacityService(stub)
	results := make(chan error, 2)
	go func() { _, err := svc.SiteSnapshot(context.Background()); results <- err }()
	<-started
	go func() { _, err := svc.SiteSnapshot(context.Background()); results <- err }()
	close(continueCh)
	assert.NoError(t, <-results)
	assert.NoError(t, <-results)
	assert.Equal(t, 1, stub.Calls())
}

func TestRateLimitCapacitySortingAndUnlimitedSemantics(t *testing.T) {
	items := []CapacityItem{
		{Model: "z", Current: intPtr(1), Limit: 0},
		{Model: "b", Current: intPtr(5), Limit: 10},
		{Model: "a", Current: intPtr(5), Limit: 10},
		{Model: "c", Current: intPtr(11), Limit: 10},
		{Model: "d", Current: intPtr(5), Limit: 5},
		{Model: "same", Group: "z", Current: intPtr(1), Limit: 10},
		{Model: "same", Group: "a", Current: intPtr(1), Limit: 10},
	}
	sortCapacityItems(items)
	require.Len(t, items, 7)
	assert.Equal(t, "c", items[0].Model)
	assert.Equal(t, "d", items[1].Model)
	assert.Equal(t, "a", items[2].Model)
	assert.Equal(t, "b", items[3].Model)
	assert.Equal(t, "same", items[4].Model)
	assert.Equal(t, "a", items[4].Group)
	assert.Equal(t, "same", items[5].Model)
	assert.Equal(t, "z", items[5].Group)
	assert.Equal(t, "z", items[6].Model)
	assert.True(t, items[6].Unlimited)
	assert.Nil(t, items[6].Utilization)
	assert.Greater(t, *items[0].Utilization, 1.0)
}

func TestRateLimitCapacityGroupsAggregateAndSortByBestItem(t *testing.T) {
	svc := NewRateLimitCapacityService(&capacityInspectorStub{})
	snapshot := SiteCapacitySnapshot{
		Global: []CapacityItem{
			{Model: "global-a", Current: intPtr(1), Limit: 10, Available: true},
		},
		Groups: []CapacityItem{
			{Model: "same", Group: "same-z", Current: intPtr(5), Limit: 10, Available: true},
			{Model: "model-a", Group: "over", Current: intPtr(11), Limit: 10, Available: true},
			{Model: "model-b", Group: "same-z", Current: intPtr(4), Limit: 10, Available: true},
			{Model: "same", Group: "same-a", Current: intPtr(5), Limit: 10, Available: true},
			{Model: "model-a", Group: "unavailable", Current: nil, Limit: 10, Available: false},
			{Model: "model-a", Group: "util", Current: intPtr(8), Limit: 10, Available: true},
			{Model: "same", Group: "current-high", Current: intPtr(6), Limit: 10, Available: true},
		},
	}
	svc.cacheMu.Lock()
	snapshot.ObservedAt = time.Now().UTC()
	snapshot.ModelRPMVersion = setting.ModelNameRPMConfigVersion()
	snapshot.GroupRateLimitVersion = setting.ModelRequestRateLimitConfigVersion()
	svc.cache = &cachedSiteCapacity{snapshot: snapshot}
	svc.cacheMu.Unlock()
	svc.SetPersonalReader(func(context.Context, int) ([]user_model_rpm.ModelRPM, string, error) {
		return nil, "empty", nil
	})

	response := svc.Get(context.Background(), CapacityRequest{IsAdmin: true, Scope: "all"})
	require.NotNil(t, response.Site)
	require.Len(t, response.Site.Groups.Groups, 6)
	assert.Equal(t, []string{"over", "util", "current-high", "same-a", "same-z", "unavailable"}, []string{
		response.Site.Groups.Groups[0].Group,
		response.Site.Groups.Groups[1].Group,
		response.Site.Groups.Groups[2].Group,
		response.Site.Groups.Groups[3].Group,
		response.Site.Groups.Groups[4].Group,
		response.Site.Groups.Groups[5].Group,
	})
	assert.Equal(t, 1, response.Site.Groups.Groups[3].Total)
	require.Len(t, response.Site.Groups.Groups[4].Items, 2)
	assert.Equal(t, 2, response.Site.Groups.Groups[4].Total)
	assert.Equal(t, "same", response.Site.Groups.Groups[4].Items[0].Model)
	assert.Equal(t, "same-z", response.Site.Groups.Groups[4].Items[0].Group)
	assert.Equal(t, 8, response.Total)
}

func TestRateLimitCapacityGroupsTieBreaksByGroupName(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		groups []CapacityGroup
		want   []string
	}{
		{
			name: "group name wins over model name",
			groups: []CapacityGroup{
				{Group: "alpha", Items: []CapacityItem{{Model: "z-model", Current: intPtr(5), Limit: 10, Available: true}}},
				{Group: "zeta", Items: []CapacityItem{{Model: "a-model", Current: intPtr(5), Limit: 10, Available: true}}},
			},
			want: []string{"alpha", "zeta"},
		},
		{
			name: "second identical metric pair",
			groups: []CapacityGroup{
				{Group: "beta", Items: []CapacityItem{{Model: "z-model", Current: intPtr(5), Limit: 10, Available: true}}},
				{Group: "delta", Items: []CapacityItem{{Model: "a-model", Current: intPtr(5), Limit: 10, Available: true}}},
			},
			want: []string{"beta", "delta"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			sortCapacityGroups(testCase.groups)
			got := make([]string, len(testCase.groups))
			for i, group := range testCase.groups {
				got[i] = group.Group
			}
			require.Equal(t, testCase.want, got)
		})
	}
}

func TestRateLimitCapacityGroupsScopeAndTotals(t *testing.T) {
	groups := make([]CapacityItem, 0, 16)
	for groupIndex := 0; groupIndex < 4; groupIndex++ {
		for modelIndex := 0; modelIndex < 4; modelIndex++ {
			groups = append(groups, CapacityItem{
				Model:     fmt.Sprintf("model-%d", modelIndex),
				Group:     fmt.Sprintf("group-%d", groupIndex),
				Current:   intPtr(4 - modelIndex),
				Limit:     10,
				Available: true,
			})
		}
	}
	for _, testCase := range []struct {
		name          string
		scope         string
		groupCount    int
		itemsPerGroup int
	}{
		{name: "top", scope: "top", groupCount: 3, itemsPerGroup: 3},
		{name: "all", scope: "all", groupCount: 4, itemsPerGroup: 4},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			svc := NewRateLimitCapacityService(&capacityInspectorStub{})
			snapshot := SiteCapacitySnapshot{
				Global: []CapacityItem{
					{Model: "global-1", Current: intPtr(1), Limit: 10, Available: true},
					{Model: "global-2", Current: intPtr(2), Limit: 10, Available: true},
					{Model: "global-3", Current: intPtr(3), Limit: 10, Available: true},
					{Model: "global-4", Current: intPtr(4), Limit: 10, Available: true},
				},
				Groups: groups,
			}
			svc.cacheMu.Lock()
			snapshot.ObservedAt = time.Now().UTC()
			snapshot.ModelRPMVersion = setting.ModelNameRPMConfigVersion()
			snapshot.GroupRateLimitVersion = setting.ModelRequestRateLimitConfigVersion()
			svc.cache = &cachedSiteCapacity{snapshot: snapshot}
			svc.cacheMu.Unlock()
			svc.SetPersonalReader(func(context.Context, int) ([]user_model_rpm.ModelRPM, string, error) {
				return nil, "empty", nil
			})

			response := svc.Get(context.Background(), CapacityRequest{IsAdmin: true, Scope: testCase.scope})
			require.NotNil(t, response.Site)
			assert.Equal(t, 4, response.Site.Global.Total)
			expectedGlobalItems := 4
			if testCase.scope == "top" {
				expectedGlobalItems = 3
			}
			assert.Len(t, response.Site.Global.Items, expectedGlobalItems)
			assert.Equal(t, 4, response.Site.Groups.Total)
			require.Len(t, response.Site.Groups.Groups, testCase.groupCount)
			for _, group := range response.Site.Groups.Groups {
				assert.Equal(t, 4, group.Total)
				assert.Len(t, group.Items, testCase.itemsPerGroup)
			}
			assert.Equal(t, 20, response.Total)
		})
	}
}

func TestRateLimitCapacityInspectorErrorDoesNotBecomeZero(t *testing.T) {
	previous := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"group_rpm":{"default":5}}}}`))
	stub := &capacityInspectorStub{err: errors.New("redis unavailable")}
	svc := NewRateLimitCapacityService(stub)
	snapshot, err := svc.SiteSnapshot(context.Background())
	require.Error(t, err)
	require.Len(t, snapshot.Global, 1)
	assert.Nil(t, snapshot.Global[0].Current)
	assert.False(t, snapshot.Global[0].Available)
	require.Len(t, snapshot.Groups, 1)
	assert.Nil(t, snapshot.Groups[0].Current)
	assert.False(t, snapshot.Groups[0].Available)
	response := svc.Get(context.Background(), CapacityRequest{IsAdmin: true, Scope: "all"})
	require.NotNil(t, response.Site)
	require.Len(t, response.Site.Groups.Groups, 1)
	require.Len(t, response.Site.Groups.Groups[0].Items, 1)
	assert.Nil(t, response.Site.Groups.Groups[0].Items[0].Current)
	assert.False(t, response.Site.Groups.Groups[0].Items[0].Available)
}

func TestRateLimitCapacityFiltersBeforeRankingAndScopeAllCannotBypass(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousUsable := setting.UserUsableGroups2JSONString()
	previousRegion := operation_setting.GetRegionRestrictionSetting()
	defer func() {
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
		_ = ratio_setting.UpdateGroupRatioByJSONString(previousRatios)
		_ = setting.UpdateUserUsableGroupsByJSONString(previousUsable)
		blockedJSON, _ := common.Marshal(previousRegion.BlockedGroups)
		_ = config.GlobalConfig.LoadFromDB(map[string]string{
			"region_restriction.enabled":        fmt.Sprintf("%t", previousRegion.Enabled),
			"region_restriction.filter_console": fmt.Sprintf("%t", previousRegion.FilterConsole),
			"region_restriction.blocked_groups": string(blockedJSON),
		})
		operation_setting.RebuildRegionRestrictionIndex()
	}()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"group_rpm":{"hidden":9,"public":1,"regional":1}},"gpt-4o-mini":{"global_rpm":10,"group_rpm":{"public":2}}}}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"public":1,"hidden":1,"regional":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"public":"Public","regional":"Regional"}`))
	blockedJSON, err := common.Marshal(map[string][]string{"CN": {"regional"}})
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"region_restriction.enabled":        "true",
		"region_restriction.filter_console": "true",
		"region_restriction.blocked_groups": string(blockedJSON),
	}))
	operation_setting.RebuildRegionRestrictionIndex()

	// Counts map to global, hidden, public, regional in the deterministic key order. The
	// hidden bucket is the highest-ranked candidate, but must be removed first.
	stub := &capacityInspectorStub{counts: []int{1, 1, 9, 1, 1, 1, 2}}
	svc := NewRateLimitCapacityService(stub)
	top := svc.Get(context.Background(), CapacityRequest{UserGroup: "member", Country: "CN", Scope: "top"})
	require.NotNil(t, top.Site)
	assert.Equal(t, 1, top.Site.Groups.Total)
	require.Len(t, top.Site.Groups.Groups, 1)
	assert.Equal(t, "public", top.Site.Groups.Groups[0].Group)
	require.Len(t, top.Site.Groups.Groups[0].Items, 2)
	assert.Equal(t, 2, top.Site.Groups.Groups[0].Total)
	for _, item := range top.Site.Groups.Groups[0].Items {
		assert.NotEqual(t, "hidden", item.Group)
		assert.NotEqual(t, "regional", item.Group)
	}

	all := svc.Get(context.Background(), CapacityRequest{UserGroup: "member", Country: "CN", Scope: "all"})
	assert.Equal(t, 1, all.Site.Groups.Total)
	require.Len(t, all.Site.Groups.Groups, 1)
	assert.Equal(t, "public", all.Site.Groups.Groups[0].Group)
	require.Len(t, all.Site.Groups.Groups[0].Items, 2)
	assert.Equal(t, 4, all.Total)

	admin := svc.Get(context.Background(), CapacityRequest{IsAdmin: true, Country: "CN", Scope: "all"})
	assert.Equal(t, 3, admin.Site.Groups.Total)
	require.Len(t, admin.Site.Groups.Groups, 3)
	adminGroupNames := map[string]bool{}
	for _, group := range admin.Site.Groups.Groups {
		adminGroupNames[group.Group] = true
	}
	assert.True(t, adminGroupNames["hidden"])
	assert.True(t, adminGroupNames["public"])
	assert.True(t, adminGroupNames["regional"])
	assert.Equal(t, 6, admin.Total)
}

func TestRateLimitCapacityUsesInjectedPersonalReaderAndJoinsWarnings(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() {
		require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM))
	}()
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10}}}`))

	svc := NewRateLimitCapacityService(&capacityInspectorStub{err: errors.New("site unavailable")})
	svc.SetPersonalReader(func(context.Context, int) ([]user_model_rpm.ModelRPM, string, error) {
		return nil, "unavailable", errors.New("personal unavailable")
	})
	response := svc.Get(context.Background(), CapacityRequest{UserID: 9})
	assert.True(t, response.Degraded)
	assert.Contains(t, response.Warning, "A2 backend is unavailable")
	assert.Contains(t, response.Warning, "personal unavailable")
	assert.Equal(t, "unavailable", response.Personal.Status)
}

func TestRateLimitCapacityPersonalPayloadIsIndependentFromSiteSnapshot(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)) }()
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":false,"models":{}}`))

	svc := NewRateLimitCapacityService(&capacityInspectorStub{})
	svc.SetClock(func() time.Time { return time.Unix(123, 456000000).UTC() })
	svc.SetPersonalReader(func(context.Context, int) ([]user_model_rpm.ModelRPM, string, error) {
		return []user_model_rpm.ModelRPM{
			{Model: "z-model", RPM: 2},
			{Model: "a-model", RPM: 4},
		}, "available", nil
	})
	response := svc.Get(context.Background(), CapacityRequest{UserID: 9})
	require.NotNil(t, response.Personal)
	assert.Equal(t, "available", response.Personal.Status)
	assert.Equal(t, 60, response.Personal.WindowSeconds)
	assert.Equal(t, 2, response.Personal.Total)
	assert.Equal(t, []user_model_rpm.ModelRPM{
		{Model: "a-model", RPM: 4},
		{Model: "z-model", RPM: 2},
	}, response.Personal.Items)
	assert.Equal(t, time.Unix(123, 456000000).UTC(), response.Personal.ObservedAt)
	assert.Equal(t, 0, response.Total)
}

func TestRateLimitCapacityPersonalOverflowHasNoItems(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)) }()
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":false,"models":{}}`))

	svc := NewRateLimitCapacityService(&capacityInspectorStub{})
	svc.SetPersonalReader(func(context.Context, int) ([]user_model_rpm.ModelRPM, string, error) {
		return []user_model_rpm.ModelRPM{{Model: "should-not-render", RPM: 5001}}, "overflow", nil
	})
	response := svc.Get(context.Background(), CapacityRequest{UserID: 1})
	require.NotNil(t, response.Personal)
	assert.Equal(t, "overflow", response.Personal.Status)
	assert.Empty(t, response.Personal.Items)
	assert.Equal(t, 0, response.Personal.Total)
}

func TestRateLimitCapacityMarksProcessLocalBackend(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() {
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
	}()
	t.Setenv("USER_MODEL_RPM_ENABLED", "false")
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":1}}}`))
	svc := NewRateLimitCapacityService(&capacityInspectorStub{counts: []int{0}})
	svc.SetInstanceOnlyDetector(func() bool { return true })
	response := svc.Get(context.Background(), CapacityRequest{})
	assert.True(t, response.InstanceOnly)
	assert.Equal(t, "instance", response.BackendScope)
	assert.True(t, response.Degraded)
}

func intPtr(value int) *int { return &value }
