package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting"
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

func TestRateLimitCapacityInspectorErrorDoesNotBecomeZero(t *testing.T) {
	previous := setting.ModelNameRPMRateLimit2JSONString()
	defer func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) }()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10}}}`))
	stub := &capacityInspectorStub{err: errors.New("redis unavailable")}
	svc := NewRateLimitCapacityService(stub)
	snapshot, err := svc.SiteSnapshot(context.Background())
	require.Error(t, err)
	require.Len(t, snapshot.Global, 1)
	assert.Nil(t, snapshot.Global[0].Current)
	assert.False(t, snapshot.Global[0].Available)
}

func TestRateLimitCapacityFiltersBeforeRankingAndScopeAllCannotBypass(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousUsable := setting.UserUsableGroups2JSONString()
	defer func() {
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
		_ = ratio_setting.UpdateGroupRatioByJSONString(previousRatios)
		_ = setting.UpdateUserUsableGroupsByJSONString(previousUsable)
	}()
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10,"group_rpm":{"hidden":9,"public":1}}}}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"public":1,"hidden":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"public":"Public"}`))

	// Counts map to global, hidden, public in the deterministic key order. The
	// hidden bucket is the highest-ranked candidate, but must be removed first.
	stub := &capacityInspectorStub{counts: []int{1, 9, 1}}
	svc := NewRateLimitCapacityService(stub)
	top := svc.Get(context.Background(), CapacityRequest{UserGroup: "member", Scope: "top"})
	require.NotNil(t, top.Site)
	assert.Equal(t, 1, top.Site.Groups.Total)
	require.Len(t, top.Site.Groups.Items, 1)
	assert.Equal(t, "public", top.Site.Groups.Items[0].Group)

	all := svc.Get(context.Background(), CapacityRequest{UserGroup: "member", Scope: "all"})
	assert.Equal(t, 1, all.Site.Groups.Total)
	require.Len(t, all.Site.Groups.Items, 1)
	assert.Equal(t, "public", all.Site.Groups.Items[0].Group)

	admin := svc.Get(context.Background(), CapacityRequest{IsAdmin: true, Scope: "all"})
	assert.Equal(t, 2, admin.Site.Groups.Total)
}

func TestRateLimitCapacityUsesInjectedPersonalReaderAndJoinsWarnings(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousA1 := setting.ModelRequestRateLimitEnabled
	defer func() {
		require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM))
		setting.ModelRequestRateLimitEnabled = previousA1
	}()
	setting.ModelRequestRateLimitEnabled = true
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":10}}}`))

	svc := NewRateLimitCapacityService(&capacityInspectorStub{err: errors.New("site unavailable")})
	svc.SetPersonalReader(func(context.Context, int, string) (*PersonalCapacity, bool, string) {
		return &PersonalCapacity{Enabled: true, Status: "unavailable"}, true, "personal unavailable"
	})
	response := svc.Get(context.Background(), CapacityRequest{UserID: 9})
	assert.True(t, response.Degraded)
	assert.Contains(t, response.Warning, "A2 backend is unavailable")
	assert.Contains(t, response.Warning, "personal unavailable")
	assert.Equal(t, "unavailable", response.Personal.Status)
}

func TestRateLimitCapacityDoesNotReadPersonalBackendWhenA1Disabled(t *testing.T) {
	previousEnabled := setting.ModelRequestRateLimitEnabled
	defer func() { setting.ModelRequestRateLimitEnabled = previousEnabled }()
	setting.ModelRequestRateLimitEnabled = false

	svc := NewRateLimitCapacityService(&capacityInspectorStub{})
	called := false
	svc.SetPersonalReader(func(context.Context, int, string) (*PersonalCapacity, bool, string) {
		called = true
		return nil, true, "unexpected"
	})
	response := svc.Get(context.Background(), CapacityRequest{})
	assert.False(t, called)
	require.NotNil(t, response.Personal)
	assert.Equal(t, "disabled", response.Personal.Status)
	assert.False(t, response.Degraded)
}

func TestRateLimitCapacityMarksProcessLocalBackend(t *testing.T) {
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	previousA1 := setting.ModelRequestRateLimitEnabled
	defer func() {
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
		setting.ModelRequestRateLimitEnabled = previousA1
	}()
	setting.ModelRequestRateLimitEnabled = false
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt-4o":{"global_rpm":1}}}`))
	svc := NewRateLimitCapacityService(&capacityInspectorStub{counts: []int{0}})
	svc.SetInstanceOnlyDetector(func() bool { return true })
	response := svc.Get(context.Background(), CapacityRequest{})
	assert.True(t, response.InstanceOnly)
	assert.Equal(t, "instance", response.BackendScope)
	assert.True(t, response.Degraded)
}

func TestPersonalCapacityDisabledAndUnconfiguredStates(t *testing.T) {
	previousEnabled := setting.ModelRequestRateLimitEnabled
	previousCount := setting.ModelRequestRateLimitCount
	previousSuccess := setting.ModelRequestRateLimitSuccessCount
	previousGroup := setting.ModelRequestRateLimitGroup2JSONString()
	defer func() {
		setting.ModelRequestRateLimitEnabled = previousEnabled
		setting.ModelRequestRateLimitCount = previousCount
		setting.ModelRequestRateLimitSuccessCount = previousSuccess
		_ = setting.UpdateModelRequestRateLimitGroupByJSONString(previousGroup)
	}()

	setting.ModelRequestRateLimitEnabled = false
	disabled, degraded, warning := readPersonalCapacity(context.Background(), 1, "default")
	require.NotNil(t, disabled)
	assert.Equal(t, "disabled", disabled.Status)
	assert.False(t, degraded)
	assert.Empty(t, warning)

	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 0
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`))
	unconfigured, degraded, warning := readPersonalCapacity(context.Background(), 1, "default")
	require.NotNil(t, unconfigured)
	assert.Equal(t, "unconfigured", unconfigured.Status)
	assert.False(t, degraded)
	assert.Empty(t, warning)
}

func TestCapacityMetricHelpersPreserveZeroAndOverLimitValues(t *testing.T) {
	zero := metricFromCurrent(0, 10, true)
	require.NotNil(t, zero.Current)
	assert.Equal(t, 0, *zero.Current)
	assert.Equal(t, float64(0), *zero.Utilization)
	assert.False(t, zero.OverLimit)

	over := metricFromCurrent(12, 10, true)
	require.NotNil(t, over.Utilization)
	assert.Equal(t, 1.2, *over.Utilization)
	assert.True(t, over.OverLimit)

	unlimited := metricFromCurrent(12, 0, true)
	assert.True(t, unlimited.Unlimited)
	assert.Nil(t, unlimited.Utilization)
	assert.False(t, unlimited.OverLimit)
	assert.False(t, unavailableMetric(10).Available)
}

func intPtr(value int) *int { return &value }
