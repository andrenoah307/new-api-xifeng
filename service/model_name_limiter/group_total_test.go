package model_name_limiter

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupKeysUseIndependentNamespaces(t *testing.T) {
	model := "vip"
	group := "vip"
	userID := 7
	keys := map[string]string{
		"model":       ModelKey(model),
		"group":       GroupKey(model, group),
		"user":        UserKey(model, userID),
		"group_total": GroupTotalKey(group),
		"group_user":  GroupUserKey(group, userID),
	}
	assert.Equal(t, "mdrl:v1:rpm:guser:vip:7", keys["group_user"])

	pairs := []struct {
		name  string
		first string
		other string
	}{
		{name: "group and group total", first: keys["group"], other: keys["group_total"]},
		{name: "group and group user", first: keys["group"], other: keys["group_user"]},
		{name: "group total and group user", first: keys["group_total"], other: keys["group_user"]},
	}
	for _, tt := range pairs {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, strings.HasPrefix(tt.first, tt.other))
			assert.False(t, strings.HasPrefix(tt.other, tt.first))
		})
	}
}

func TestFiveBucketsAreValidAndSixBucketsFailOpen(t *testing.T) {
	four := []Bucket{
		{Key: "global", Limit: 10, Scope: "global"},
		{Key: "group", Limit: 10, Scope: "group"},
		{Key: "user", Limit: 10, Scope: "user"},
		{Key: "total", Limit: 1, Scope: "group_total"},
	}
	five := append(append([]Bucket(nil), four...), Bucket{Key: "group-user", Limit: 10, Scope: "group_user"})
	six := append(append([]Bucket(nil), five...), Bucket{Key: "sixth", Limit: 1, Scope: "global"})
	assert.True(t, validAcquireBuckets(five))
	assert.False(t, validAcquireBuckets(six))

	previousBackend := backendImpl
	previousInitialized := previousBackend != nil
	backendImpl = newMemoryBackend()
	backendOnce = sync.Once{}
	backendOnce.Do(func() {})
	t.Cleanup(func() {
		backendImpl = previousBackend
		backendOnce = sync.Once{}
		if previousInitialized {
			backendOnce.Do(func() {})
		}
	})

	assert.Equal(t, Result{Allowed: true}, Acquire(context.Background(), five))
	assert.Equal(t, Result{Allowed: false, Scope: "group_total", Limit: 1, Current: 1}, Acquire(context.Background(), five))
	assert.Equal(t, Result{Allowed: true}, Acquire(context.Background(), six))
	b := backendImpl.(*memoryBackend)
	assert.Equal(t, 1, b.count("total"), "invalid six-bucket shape must not reach the backend")
	assert.Equal(t, 1, b.count("group-user"))
	assert.Equal(t, 0, b.count("sixth"))
}

func TestAcquireBucketScopeValidation(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		valid bool
	}{
		{name: "group user", scope: "group_user", valid: true},
		{name: "unknown", scope: "unknown", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, validAcquireBuckets([]Bucket{{Key: "key", Limit: 1, Scope: tt.scope}}))
		})
	}
}

func TestBackendsRejectGroupTotalAtomicallyAndReturnScope(t *testing.T) {
	for name, newFixture := range map[string]func(*testing.T) limiterFixture{
		"memory": newMemoryFixture,
		"redis":  newRedisFixture,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			defer fixture.close()
			buckets := []Bucket{
				{Key: "global", Limit: 10, Scope: "global"},
				{Key: "group", Limit: 10, Scope: "group"},
				{Key: "user", Limit: 10, Scope: "user"},
				{Key: "total", Limit: 1, Scope: "group_total"},
			}
			require.True(t, fixture.backend.Acquire(context.Background(), buckets).Allowed)
			result := fixture.backend.Acquire(context.Background(), buckets)
			assert.Equal(t, Result{Allowed: false, Scope: "group_total", Limit: 1, Current: 1}, result)
			assert.Equal(t, 1, fixture.count("global"))
			assert.Equal(t, 1, fixture.count("group"))
			assert.Equal(t, 1, fixture.count("user"))
			assert.Equal(t, 1, fixture.count("total"))
		})
	}
}

func TestBackendsCountOnlyGlobalCoexistsWithGroupTotal(t *testing.T) {
	for name, newFixture := range map[string]func(*testing.T) limiterFixture{
		"memory": newMemoryFixture,
		"redis":  newRedisFixture,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			defer fixture.close()
			buckets := []Bucket{
				{Key: "global", Limit: 0, Scope: "global"},
				{Key: "total", Limit: 2, Scope: "group_total"},
			}
			for i := 0; i < 2; i++ {
				require.Equal(t, Result{Allowed: true}, fixture.backend.Acquire(context.Background(), buckets))
			}
			assert.Equal(t, Result{Allowed: false, Scope: "group_total", Limit: 2, Current: 2}, fixture.backend.Acquire(context.Background(), buckets))
			assert.Equal(t, 2, fixture.count("global"))
			assert.Equal(t, 2, fixture.count("total"))
		})
	}
}
