package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitCapacityGroupsOnlyBuildsSortedTotalKeysAndAlignsCounts(t *testing.T) {
	previous := setting.ModelNameRPMRateLimit2JSONString()
	t.Cleanup(func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) })
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{},"groups":{"zeta":{"total_rpm":7},"alpha":{"total_rpm":3}}}`))
	stub := &capacityInspectorStub{counts: []int{9, 4}}
	snapshot, err := NewRateLimitCapacityService(stub).SiteSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, [][]string{{
		model_name_limiter.GroupTotalKey("alpha"),
		model_name_limiter.GroupTotalKey("zeta"),
	}}, stub.Keys())
	require.Len(t, snapshot.GroupTotals, 2)
	assert.Equal(t, "alpha", snapshot.GroupTotals[0].Group)
	assert.Equal(t, 9, *snapshot.GroupTotals[0].Current)
	assert.Equal(t, 3, snapshot.GroupTotals[0].Limit)
	assert.Equal(t, "zeta", snapshot.GroupTotals[1].Group)
	assert.Equal(t, 4, *snapshot.GroupTotals[1].Current)
	assert.Equal(t, 7, snapshot.GroupTotals[1].Limit)
}

func TestRateLimitCapacityGroupTotalKeysFollowModelAndGroupKeys(t *testing.T) {
	previous := setting.ModelNameRPMRateLimit2JSONString()
	t.Cleanup(func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) })
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt":{"global_rpm":10,"group_rpm":{"vip":2}}},"groups":{"zeta":{"total_rpm":7},"alpha":{"total_rpm":3}}}`))
	stub := &capacityInspectorStub{counts: []int{1, 2, 3, 4}}
	_, err := NewRateLimitCapacityService(stub).SiteSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, [][]string{{
		model_name_limiter.ModelKey("gpt"),
		model_name_limiter.GroupKey("gpt", "vip"),
		model_name_limiter.GroupTotalKey("alpha"),
		model_name_limiter.GroupTotalKey("zeta"),
	}}, stub.Keys())
}

func TestRateLimitCapacityGroupTotalsMarkInspectorFailureUnavailable(t *testing.T) {
	previous := setting.ModelNameRPMRateLimit2JSONString()
	t.Cleanup(func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) })
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{},"groups":{"vip":{"total_rpm":30}}}`))
	snapshot, err := NewRateLimitCapacityService(&capacityInspectorStub{err: assert.AnError}).SiteSnapshot(context.Background())
	require.Error(t, err)
	require.Len(t, snapshot.GroupTotals, 1)
	assert.Nil(t, snapshot.GroupTotals[0].Current)
	assert.False(t, snapshot.GroupTotals[0].Available)
}

func TestRateLimitCapacityPersonalAlignsModelAndSortedGroupUserBuckets(t *testing.T) {
	previous := setting.ModelNameRPMRateLimit2JSONString()
	t.Cleanup(func() { require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous)) })
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{"gpt":{"global_rpm":10,"user_rpm":4}},"groups":{"zeta":{"user_rpm":7},"alpha":{"total_rpm":3,"user_rpm":2}}}`))
	stub := &capacityInspectorStub{responses: []capacityInspectResponse{
		{counts: []int{11, 12}},
		{counts: []int{1, 2, 3}},
	}}
	svc := NewRateLimitCapacityService(stub)
	snapshot, err := svc.SiteSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []UserRPMCapacityLimit{
		{Model: "gpt", Limit: 4},
		{Group: "alpha", Limit: 2},
		{Group: "zeta", Limit: 7},
	}, snapshot.UserLimits)

	response := svc.Get(context.Background(), CapacityRequest{UserID: 9, IsAdmin: true, Scope: "all"})
	require.NotNil(t, response.Personal)
	assert.Equal(t, [][]string{
		{model_name_limiter.ModelKey("gpt"), model_name_limiter.GroupTotalKey("alpha")},
		{
			model_name_limiter.UserKey("gpt", 9),
			model_name_limiter.GroupUserKey("alpha", 9),
			model_name_limiter.GroupUserKey("zeta", 9),
		},
	}, stub.Keys())
	require.Len(t, response.Personal.Items, 3)

	type itemIdentity struct {
		model string
		group string
	}
	want := map[itemIdentity]struct {
		current int
		limit   int
	}{
		{model: "gpt"}:   {current: 1, limit: 4},
		{group: "alpha"}: {current: 2, limit: 2},
		{group: "zeta"}:  {current: 3, limit: 7},
	}
	for _, item := range response.Personal.Items {
		identity := itemIdentity{model: item.Model, group: item.Group}
		expected, ok := want[identity]
		require.True(t, ok, "unexpected personal item: %+v", item)
		require.NotNil(t, item.Current)
		assert.Equal(t, expected.current, *item.Current)
		assert.Equal(t, expected.limit, item.Limit)
		if item.Group != "" {
			assert.Empty(t, item.Model)
		}
	}
}

func TestRateLimitCapacityGroupTotalsSortByGroupAndRespectVisibilityAndScope(t *testing.T) {
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousUsable := setting.UserUsableGroups2JSONString()
	previousRPM := setting.ModelNameRPMRateLimit2JSONString()
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(previousRatios)
		_ = setting.UpdateUserUsableGroupsByJSONString(previousUsable)
		_ = setting.UpdateModelNameRPMRateLimitByJSONString(previousRPM)
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"public":1,"hidden":1,"third":1,"zeta":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"public":"Public"}`))
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(`{"enabled":true,"models":{},"groups":{"public":{"total_rpm":7},"alpha":{"total_rpm":3},"hidden":{"total_rpm":5},"third":{"total_rpm":6}}}`))

	snapshot := SiteCapacitySnapshot{
		ObservedAt:            time.Unix(123, 0).UTC(),
		ModelRPMVersion:       setting.ModelNameRPMConfigVersion(),
		GroupRateLimitVersion: setting.ModelRequestRateLimitConfigVersion(),
		GroupTotals: []CapacityItem{
			{Group: "public", Current: intPtr(1), Limit: 10, Available: true},
			{Group: "alpha", Current: intPtr(1), Limit: 10, Available: true},
			{Group: "hidden", Current: intPtr(1), Limit: 10, Available: true},
			{Group: "third", Current: intPtr(1), Limit: 10, Available: true},
		},
	}
	svc := NewRateLimitCapacityService(&capacityInspectorStub{})
	svc.cache = &cachedSiteCapacity{snapshot: snapshot}
	nonAdminTop := svc.Get(context.Background(), CapacityRequest{UserGroup: "member", Scope: "top"})
	require.NotNil(t, nonAdminTop.Site)
	require.Len(t, nonAdminTop.Site.GroupTotals.Items, 1)
	assert.Equal(t, "public", nonAdminTop.Site.GroupTotals.Items[0].Group)
	assert.Equal(t, 1, nonAdminTop.Site.GroupTotals.Total)
	assert.Equal(t, 1, nonAdminTop.Total)

	adminAll := svc.Get(context.Background(), CapacityRequest{IsAdmin: true, Scope: "all"})
	require.NotNil(t, adminAll.Site)
	assert.Equal(t, 4, adminAll.Site.GroupTotals.Total)
	assert.Equal(t, []string{"alpha", "hidden", "public", "third"}, []string{
		adminAll.Site.GroupTotals.Items[0].Group,
		adminAll.Site.GroupTotals.Items[1].Group,
		adminAll.Site.GroupTotals.Items[2].Group,
		adminAll.Site.GroupTotals.Items[3].Group,
	})
	assert.Equal(t, 4, adminAll.Total)
	adminTop := svc.Get(context.Background(), CapacityRequest{IsAdmin: true, Scope: "top"})
	require.NotNil(t, adminTop.Site)
	assert.Equal(t, 4, adminTop.Site.GroupTotals.Total)
	assert.Len(t, adminTop.Site.GroupTotals.Items, 3)
	assert.Equal(t, 4, adminTop.Total)
}
