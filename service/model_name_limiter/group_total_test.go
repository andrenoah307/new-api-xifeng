package model_name_limiter

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupTotalKeyUsesAnIndependentNamespace(t *testing.T) {
	model := "vip"
	group := "vip"
	userID := 7
	keys := []string{
		ModelKey(model),
		GroupKey(model, group),
		UserKey(model, userID),
		GroupTotalKey(group),
	}
	assert.Equal(t, []string{
		"mdrl:v1:rpm:model:vip",
		"mdrl:v1:rpm:group:vip:vip",
		"mdrl:v1:rpm:user:vip:7",
		"mdrl:v1:rpm:gtotal:vip",
	}, keys)
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			assert.NotEqual(t, keys[i], keys[j])
		}
	}
}

func TestFourBucketsAreValidAndFiveBucketsFailOpen(t *testing.T) {
	four := []Bucket{
		{Key: "global", Limit: 10, Scope: "global"},
		{Key: "group", Limit: 10, Scope: "group"},
		{Key: "user", Limit: 10, Scope: "user"},
		{Key: "total", Limit: 1, Scope: "group_total"},
	}
	five := append(append([]Bucket(nil), four...), Bucket{Key: "fifth", Limit: 1, Scope: "global"})
	assert.True(t, validAcquireBuckets(four))
	assert.False(t, validAcquireBuckets(five))

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

	assert.Equal(t, Result{Allowed: true}, Acquire(context.Background(), four))
	assert.Equal(t, Result{Allowed: false, Scope: "group_total", Limit: 1, Current: 1}, Acquire(context.Background(), four))
	assert.Equal(t, Result{Allowed: true}, Acquire(context.Background(), five))
	b := backendImpl.(*memoryBackend)
	assert.Equal(t, 1, b.count("total"), "invalid five-bucket shape must not reach the backend")
	assert.Equal(t, 0, b.count("fifth"))
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
