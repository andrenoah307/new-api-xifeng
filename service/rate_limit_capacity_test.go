package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capacityInspectorStub struct {
	mu        sync.Mutex
	counts    []int
	err       error
	responses []capacityInspectResponse
	keys      [][]string
	calls     int
	started   chan struct{}
	release   chan struct{}
}

type capacityInspectResponse struct {
	counts []int
	err    error
}

func (s *capacityInspectorStub) Inspect(_ context.Context, keys []string) ([]int, error) {
	s.mu.Lock()
	s.calls++
	counts, err := append([]int(nil), s.counts...), s.err
	if len(s.responses) >= s.calls {
		response := s.responses[s.calls-1]
		counts, err = append([]int(nil), response.counts...), response.err
	}
	s.keys = append(s.keys, append([]string(nil), keys...))
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

func (s *capacityInspectorStub) Keys() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([][]string, len(s.keys))
	for i := range s.keys {
		keys[i] = append([]string(nil), s.keys[i]...)
	}
	return keys
}

func (s *capacityInspectorStub) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestRateLimitCapacitySnapshotCacheAndVersionInvalidation(t *testing.T) {
	previous := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":4,"group_rpm":{"free":5}}}}`))

	stub := &capacityInspectorStub{counts: []int{3, 2}}
	now := time.Unix(100, 0)
	svc := NewRateLimitCapacityService(stub)
	svc.SetClock(func() time.Time { return now })

	first, err := svc.SiteSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, stub.Calls())
	require.Len(t, first.UserLimits, 1)
	first.UserLimits[0].Limit = 999
	second, err := svc.SiteSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, first.ObservedAt, second.ObservedAt)
	assert.Equal(t, 4, second.UserLimits[0].Limit)
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

func TestRateLimitCapacityOmitsAllSectionsWhenA2IsDisabledOrEmpty(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)) }()

	for _, test := range []struct {
		name   string
		config string
	}{
		{name: "disabled", config: `{"enabled":false,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":2}}}`},
		{name: "zero models", config: `{"enabled":true,"models":{}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(test.config))
			stub := &capacityInspectorStub{}
			response := NewRateLimitCapacityService(stub).Get(context.Background(), CapacityRequest{UserID: 9})
			assert.Nil(t, response.Site)
			assert.Nil(t, response.Personal)
			assert.False(t, setting.IsRateLimitCapacityEnabled())
			assert.Equal(t, 0, stub.Calls())
		})
	}
}

func TestRateLimitCapacityGlobalOnlyRuleDoesNotCreatePersonalSection(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10}}}`))

	stub := &capacityInspectorStub{counts: []int{0}}
	response := NewRateLimitCapacityService(stub).Get(context.Background(), CapacityRequest{UserID: 9})
	require.NotNil(t, response.Site)
	assert.Nil(t, response.Personal)
	assert.Equal(t, 1, stub.Calls())
	assert.Equal(t, [][]string{{model_name_limiter.ModelKey("gpt-4o")}}, stub.Keys())
}

func TestRateLimitCapacityUsesNormalizedRuleModelForAdmissionKeys(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o-gizmo-private":{"global_rpm":10,"user_rpm":2}}}`))

	stub := &capacityInspectorStub{responses: []capacityInspectResponse{{counts: []int{0}}, {counts: []int{0}}}}
	response := NewRateLimitCapacityService(stub).Get(context.Background(), CapacityRequest{UserID: 9})
	require.NotNil(t, response.Site)
	require.NotNil(t, response.Personal)
	assert.Equal(t, [][]string{
		{model_name_limiter.ModelKey("gpt-4o-gizmo-*")},
		{model_name_limiter.UserKey("gpt-4o-gizmo-*", 9)},
	}, stub.Keys())
	assert.Equal(t, "gpt-4o-gizmo-*", response.Site.Global.Items[0].Model)
	assert.Equal(t, "gpt-4o-gizmo-*", response.Personal.Items[0].Model)
}

func TestRateLimitCapacityPersonalUsesAdmissionBucketsWithoutTopTruncation(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"a":{"global_rpm":10,"user_rpm":4},"b":{"global_rpm":20,"user_rpm":5},"c":{"global_rpm":30,"user_rpm":6},"d":{"global_rpm":40,"user_rpm":7},"site-only":{"global_rpm":50}}}`))

	stub := &capacityInspectorStub{responses: []capacityInspectResponse{
		{counts: []int{0, 0, 0, 0, 0}},
		{counts: []int{0, 1, 2, 3}},
		{counts: []int{0, 1, 2, 3}},
	}}
	svc := NewRateLimitCapacityService(stub)
	svc.SetClock(func() time.Time { return time.Unix(123, 456000000).UTC() })
	response := svc.Get(context.Background(), CapacityRequest{UserID: 9, Scope: "top"})
	require.NotNil(t, response.Personal)
	assert.Equal(t, "ok", response.Personal.Status)
	assert.Equal(t, 60, response.Personal.WindowSeconds)
	assert.Equal(t, time.Unix(123, 456000000).UTC(), response.Personal.ObservedAt)
	assert.Equal(t, 4, response.Personal.Total)
	require.Len(t, response.Personal.Items, 4)
	assert.Equal(t, 2, stub.Calls())
	assert.Equal(t, []string{
		model_name_limiter.UserKey("a", 9),
		model_name_limiter.UserKey("b", 9),
		model_name_limiter.UserKey("c", 9),
		model_name_limiter.UserKey("d", 9),
	}, stub.Keys()[1])
	var zeroItem *CapacityItem
	for i := range response.Personal.Items {
		if response.Personal.Items[i].Model == "a" {
			zeroItem = &response.Personal.Items[i]
		}
		assert.NotEqual(t, "site-only", response.Personal.Items[i].Model)
	}
	require.NotNil(t, zeroItem)
	require.NotNil(t, zeroItem.Current)
	assert.Equal(t, 0, *zeroItem.Current)
	assert.Equal(t, 4, zeroItem.Limit)
	require.NotNil(t, zeroItem.Utilization)
	assert.Equal(t, 0.0, *zeroItem.Utilization)
	assert.Empty(t, zeroItem.Group)
	assert.True(t, zeroItem.Available)
	assert.False(t, zeroItem.Unlimited)
	assert.False(t, zeroItem.OverLimit)

	second := svc.Get(context.Background(), CapacityRequest{UserID: 9, Scope: "all"})
	require.NotNil(t, second.Personal)
	assert.Equal(t, 3, stub.Calls(), "cached site data must leave exactly one personal Inspect per request")
}

func TestRateLimitCapacityPersonalInspectFailureIsUnavailableAndRedacted(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"user_rpm":2}}}`))

	rawError := "redis://private-host:6379 unavailable"
	stub := &capacityInspectorStub{responses: []capacityInspectResponse{
		{counts: []int{1}},
		{err: errors.New(rawError)},
	}}
	response := NewRateLimitCapacityService(stub).Get(context.Background(), CapacityRequest{UserID: 9})
	require.NotNil(t, response.Personal)
	assert.Equal(t, "unavailable", response.Personal.Status)
	require.Len(t, response.Personal.Items, 1)
	assert.Nil(t, response.Personal.Items[0].Current)
	assert.False(t, response.Personal.Items[0].Available)
	assert.True(t, response.Degraded)
	assert.NotContains(t, response.Warning, rawError)
}

func TestRateLimitCapacitySiteWarningDoesNotExposeBackendError(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10}}}`))

	rawError := "dial tcp redis.internal:6379: connection refused"
	response := NewRateLimitCapacityService(&capacityInspectorStub{err: errors.New(rawError)}).Get(context.Background(), CapacityRequest{UserID: 9})
	assert.True(t, response.Degraded)
	assert.Equal(t, "A2 backend is unavailable", response.Warning)
	assert.NotContains(t, response.Warning, rawError)
}

func TestRateLimitCapacityMarksProcessLocalBackend(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	defer func() {
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
	}()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":2,"user_rpm":1}}}`))
	svc := NewRateLimitCapacityService(&capacityInspectorStub{responses: []capacityInspectResponse{{counts: []int{0}}, {counts: []int{0}}}})
	svc.SetInstanceOnlyDetector(func() bool { return true })
	response := svc.Get(context.Background(), CapacityRequest{UserID: 9})
	assert.True(t, response.InstanceOnly)
	assert.Equal(t, "instance", response.BackendScope)
	assert.True(t, response.Degraded)
	require.NotNil(t, response.Personal)
	assert.True(t, response.Personal.InstanceOnly)
}

func intPtr(value int) *int { return &value }
