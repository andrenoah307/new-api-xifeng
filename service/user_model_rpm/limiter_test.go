package user_model_rpm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installTestBackend(t *testing.T, implementation backend) {
	t.Helper()
	previousBackend := backendImpl
	previousInitialized := previousBackend != nil
	backendImpl = implementation
	backendOnce = sync.Once{}
	backendOnce.Do(func() {})
	t.Cleanup(func() {
		backendImpl = previousBackend
		backendOnce = sync.Once{}
		if previousInitialized {
			backendOnce.Do(func() {})
		}
	})
}

func TestRecordUsesRequestIDForIdempotencyAndPreservesModelDelimiter(t *testing.T) {
	now := time.UnixMilli(10_000_000)
	memory := newMemoryBackend()
	memory.now = func() time.Time { return now }
	installTestBackend(t, memory)
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")

	for _, request := range []struct {
		id    string
		model string
	}{
		{id: "request-a", model: "gpt-4o"},
		{id: "request-b", model: "gpt-4o"},
		{id: "request-a", model: "gpt-4o"},
		{id: "request-c", model: "claude" + string(rune(31)) + "sonnet"},
	} {
		require.NoError(t, Record(context.Background(), 7, request.id, request.model))
	}

	items, status, err := Inspect(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, "available", status)
	assert.Equal(t, []ModelRPM{
		{Model: "gpt-4o", RPM: 2},
		{Model: "claude" + string(rune(31)) + "sonnet", RPM: 1},
	}, items)
}

func TestInspectExcludesClosedWindowBoundaryWithoutSleeping(t *testing.T) {
	base := time.UnixMilli(20_000_000)
	memory := newMemoryBackend()
	clock := base.Add(-60 * time.Second)
	memory.now = func() time.Time { return clock }
	installTestBackend(t, memory)
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")

	require.NoError(t, Record(context.Background(), 8, "closed", "closed-model"))
	clock = base.Add(-59*time.Second - 999*time.Millisecond)
	require.NoError(t, Record(context.Background(), 8, "open", "open-model"))
	clock = base

	items, status, err := Inspect(context.Background(), 8)
	require.NoError(t, err)
	assert.Equal(t, "available", status)
	assert.Equal(t, []ModelRPM{{Model: "open-model", RPM: 1}}, items)
}

func TestInspectStatesAndSortsModels(t *testing.T) {
	now := time.UnixMilli(30_000_000)
	memory := newMemoryBackend()
	memory.now = func() time.Time { return now }
	installTestBackend(t, memory)
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")

	require.NoError(t, Record(context.Background(), 9, "a", "z-model"))
	require.NoError(t, Record(context.Background(), 9, "b", "a-model"))
	require.NoError(t, Record(context.Background(), 9, "c", "a-model"))
	items, status, err := Inspect(context.Background(), 9)
	require.NoError(t, err)
	assert.Equal(t, "available", status)
	assert.Equal(t, []ModelRPM{{Model: "a-model", RPM: 2}, {Model: "z-model", RPM: 1}}, items)

	items, status, err = Inspect(context.Background(), 99)
	require.NoError(t, err)
	assert.Equal(t, "empty", status)
	assert.Empty(t, items)
}

func TestInspectBackendErrorIsUnavailable(t *testing.T) {
	installTestBackend(t, testBackend{inspectErr: errors.New("redis unavailable")})
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")

	items, status, err := Inspect(context.Background(), 1)
	assert.Error(t, err)
	assert.Equal(t, "unavailable", status)
	assert.Empty(t, items)
	assert.NotEqual(t, "empty", status)
}

func TestInspectOverflowHasNoItems(t *testing.T) {
	now := time.UnixMilli(40_000_000)
	memory := newMemoryBackend()
	memory.now = func() time.Time { return now }
	memory.scanLimit = 1
	installTestBackend(t, memory)
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")

	require.NoError(t, Record(context.Background(), 10, "one", "one"))
	require.NoError(t, Record(context.Background(), 10, "two", "two"))
	items, status, err := Inspect(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, "overflow", status)
	assert.Empty(t, items)
}

func TestDisabledCollectorHasNoSideEffectsAndIsUnavailable(t *testing.T) {
	now := time.UnixMilli(50_000_000)
	memory := newMemoryBackend()
	memory.now = func() time.Time { return now }
	installTestBackend(t, memory)
	t.Setenv("USER_MODEL_RPM_ENABLED", "false")

	require.NoError(t, Record(context.Background(), 11, "request", "model"))
	assert.Empty(t, memory.users)
	items, status, err := Inspect(context.Background(), 11)
	assert.NoError(t, err)
	assert.Equal(t, "unavailable", status)
	assert.Empty(t, items)
}

func TestMemoryBackendEvictsExpiredUsersOnRead(t *testing.T) {
	now := time.UnixMilli(60_000_000)
	memory := newMemoryBackend()
	clock := now
	memory.now = func() time.Time { return clock }
	installTestBackend(t, memory)
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")

	require.NoError(t, Record(context.Background(), 12, "request", "model"))
	clock = now.Add(66 * time.Second)
	items, status, err := Inspect(context.Background(), 12)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Equal(t, "empty", status)
	assert.NotContains(t, memory.users, 12)
	assert.True(t, memory.IsMemory())
	assert.True(t, UsingMemoryBackend())
}

func TestRecordAndInspectSkipEmptyIdentityFields(t *testing.T) {
	memory := newMemoryBackend()
	installTestBackend(t, memory)
	t.Setenv("USER_MODEL_RPM_ENABLED", "true")

	require.NoError(t, Record(context.Background(), 13, "", "model"))
	require.NoError(t, Record(context.Background(), 13, "request", ""))
	assert.Empty(t, memory.users)
}

type testBackend struct {
	inspectErr error
}

func (testBackend) Record(context.Context, int, string, string) error { return nil }

func (b testBackend) Inspect(context.Context, int) ([]ModelRPM, string, error) {
	return nil, "unavailable", b.inspectErr
}

func (testBackend) IsMemory() bool { return false }
