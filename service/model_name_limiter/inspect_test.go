package model_name_limiter

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryInspectTrimsTheClosedWindowBoundary(t *testing.T) {
	now := time.UnixMilli(10_000_000)
	b := newMemoryBackend()
	b.now = func() time.Time { return now }
	b.entries["model"] = memoryEntry{
		hits: []int64{
			now.Add(-60 * time.Second).UnixMilli(),
			now.Add(-60*time.Second + time.Millisecond).UnixMilli(),
			now.Add(-time.Second).UnixMilli(),
		},
		expiresAt: now.Add(5 * time.Second).UnixMilli(),
	}

	counts, err := b.Inspect(context.Background(), []string{"model", "missing"})
	require.NoError(t, err)
	assert.Equal(t, []int{2, 0}, counts)
}

func TestRPMInspectScriptIsReadOnlyAndUsesOpenLowerBound(t *testing.T) {
	for _, forbidden := range []string{"ZREMRANGEBYSCORE", "ZADD", "PEXPIRE", "EXPIRE", "HMSET"} {
		assert.NotContains(t, strings.ToUpper(redisInspectScript), forbidden)
	}
	assert.Contains(t, redisInspectScript, "ZCOUNT")
	assert.Contains(t, redisInspectScript, "TIME")
	assert.Contains(t, redisInspectScript, "(")
}

func TestRPMKeyBuildersAreSharedByModelAndGroupScopes(t *testing.T) {
	assert.Equal(t, "mdrl:v1:rpm:model:gpt-4o", ModelKey("gpt-4o"))
	assert.Equal(t, "mdrl:v1:rpm:group:gpt-4o:free", GroupKey("gpt-4o", "free"))
	assert.Equal(t, "mdrl:v1:rpm:user:gpt-4o:42", UserKey("gpt-4o", 42))
}

func TestMemoryInspectMissingKeyDoesNotCreateEntry(t *testing.T) {
	b := newMemoryBackend()

	counts, err := b.Inspect(context.Background(), []string{"missing"})
	require.NoError(t, err)
	assert.Equal(t, []int{0}, counts)
	assert.Empty(t, b.entries)
}

func TestMemoryBackendSweepsEntriesAfterTTLWithoutDeletingActiveKeys(t *testing.T) {
	now := time.UnixMilli(30_000_000)
	b := newMemoryBackend()
	b.now = func() time.Time { return now }
	require.True(t, b.Acquire(context.Background(), []Bucket{{Key: "expired", Limit: 10, Scope: "user"}}).Allowed)
	require.True(t, b.Acquire(context.Background(), []Bucket{{Key: "active", Limit: 10, Scope: "user"}}).Allowed)

	now = now.Add(10 * time.Second)
	require.True(t, b.Acquire(context.Background(), []Bucket{{Key: "active", Limit: 10, Scope: "user"}}).Allowed)
	now = now.Add(55 * time.Second)
	_, err := b.Inspect(context.Background(), []string{"missing"})
	require.NoError(t, err)

	assert.NotContains(t, b.entries, "expired")
	assert.Contains(t, b.entries, "active")
	counts, err := b.Inspect(context.Background(), []string{"active"})
	require.NoError(t, err)
	assert.Equal(t, []int{1}, counts)
}

func TestInspectUsesTheActiveMemoryBackendWithoutInitializingRedis(t *testing.T) {
	previousBackend := backendImpl
	previousInitialized := previousBackend != nil
	b := newMemoryBackend()
	now := time.UnixMilli(20_000_000)
	b.now = func() time.Time { return now }
	b.entries["model"] = memoryEntry{
		hits:      []int64{now.Add(-time.Second).UnixMilli()},
		expiresAt: now.Add(5 * time.Second).UnixMilli(),
	}
	backendImpl = b
	backendOnce = sync.Once{}
	backendOnce.Do(func() {})
	t.Cleanup(func() {
		backendImpl = previousBackend
		backendOnce = sync.Once{}
		if previousInitialized {
			backendOnce.Do(func() {})
		}
	})

	counts, err := Inspect(context.Background(), []string{"model"})
	require.NoError(t, err)
	assert.Equal(t, []int{1}, counts)
	assert.True(t, UsingMemoryBackend())
}
