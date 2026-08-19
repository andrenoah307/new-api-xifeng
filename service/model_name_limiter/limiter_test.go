package model_name_limiter

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type limiterFixture struct {
	backend backend
	count   func(string) int
	oldHit  func(string)
	close   func()
}

func TestBackendsHaveTheSameAcquireSemantics(t *testing.T) {
	newFixtures := map[string]func(*testing.T) limiterFixture{
		"memory": newMemoryFixture,
		"redis":  newRedisFixture,
	}

	for name, newFixture := range newFixtures {
		name, newFixture := name, newFixture
		t.Run(name, func(t *testing.T) {
			t.Run("single key reaches limit", func(t *testing.T) {
				fixture := newFixture(t)
				defer fixture.close()
				for i := 0; i < 3; i++ {
					require.Equal(t, Result{Allowed: true}, fixture.backend.Acquire(context.Background(), []Bucket{{Key: "global", Limit: 3, Scope: "global"}}))
				}
				assert.Equal(t, Result{Allowed: false, Scope: "global", Limit: 3, Current: 3}, fixture.backend.Acquire(context.Background(), []Bucket{{Key: "global", Limit: 3, Scope: "global"}}))
			})

			t.Run("group rejection does not write global", func(t *testing.T) {
				fixture := newFixture(t)
				defer fixture.close()
				buckets := []Bucket{
					{Key: "global", Limit: 10, Scope: "global"},
					{Key: "group", Limit: 2, Scope: "group"},
				}
				for i := 0; i < 2; i++ {
					require.True(t, fixture.backend.Acquire(context.Background(), buckets).Allowed)
				}
				result := fixture.backend.Acquire(context.Background(), buckets)
				assert.Equal(t, Result{Allowed: false, Scope: "group", Limit: 2, Current: 2}, result)
				assert.Equal(t, 2, fixture.count("global"))
				assert.Equal(t, 2, fixture.count("group"))
			})

			t.Run("global rejection does not write group", func(t *testing.T) {
				fixture := newFixture(t)
				defer fixture.close()
				buckets := []Bucket{
					{Key: "global", Limit: 2, Scope: "global"},
					{Key: "group", Limit: 10, Scope: "group"},
				}
				for i := 0; i < 2; i++ {
					require.True(t, fixture.backend.Acquire(context.Background(), buckets).Allowed)
				}
				result := fixture.backend.Acquire(context.Background(), buckets)
				assert.Equal(t, Result{Allowed: false, Scope: "global", Limit: 2, Current: 2}, result)
				assert.Equal(t, 2, fixture.count("global"))
				assert.Equal(t, 2, fixture.count("group"))
			})

			t.Run("expired hits are ignored", func(t *testing.T) {
				fixture := newFixture(t)
				defer fixture.close()
				fixture.oldHit("global")
				result := fixture.backend.Acquire(context.Background(), []Bucket{{Key: "global", Limit: 1, Scope: "global"}})
				assert.Equal(t, Result{Allowed: true}, result)
				assert.Equal(t, 1, fixture.count("global"))
			})

			t.Run("concurrent acquire never exceeds limit", func(t *testing.T) {
				fixture := newFixture(t)
				defer fixture.close()
				const limit = 10
				var wg sync.WaitGroup
				for i := 0; i < 60; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_ = fixture.backend.Acquire(context.Background(), []Bucket{{Key: "global", Limit: limit, Scope: "global"}})
					}()
				}
				wg.Wait()
				// Redis failures are intentionally fail-open; the bucket count is
				// the invariant that must never overrun.
				assert.Equal(t, limit, fixture.count("global"))
			})
		})
	}
}

func TestBackendsCountOnlyGlobalBucket(t *testing.T) {
	for backendName, newFixture := range map[string]func(*testing.T) limiterFixture{
		"memory": newMemoryFixture,
		"redis":  newRedisFixture,
	} {
		t.Run(backendName, func(t *testing.T) {
			fixture := newFixture(t)
			defer fixture.close()
			bucket := []Bucket{{Key: "global", Limit: 0, Scope: "global"}}
			for i := 0; i < 3; i++ {
				assert.Equal(t, Result{Allowed: true}, fixture.backend.Acquire(context.Background(), bucket))
			}
			assert.Equal(t, 3, fixture.count("global"))
		})
	}
}

func TestBackendsCountOnlyGlobalStillHonorsUserLimitAtomically(t *testing.T) {
	for backendName, newFixture := range map[string]func(*testing.T) limiterFixture{
		"memory": newMemoryFixture,
		"redis":  newRedisFixture,
	} {
		t.Run(backendName, func(t *testing.T) {
			fixture := newFixture(t)
			defer fixture.close()
			buckets := []Bucket{
				{Key: "global", Limit: 0, Scope: "global"},
				{Key: "user", Limit: 2, Scope: "user"},
			}
			for i := 0; i < 2; i++ {
				require.Equal(t, Result{Allowed: true}, fixture.backend.Acquire(context.Background(), buckets))
			}
			assert.Equal(t, Result{Allowed: false, Scope: "user", Limit: 2, Current: 2}, fixture.backend.Acquire(context.Background(), buckets))
			assert.Equal(t, 2, fixture.count("global"))
			assert.Equal(t, 2, fixture.count("user"))
		})
	}
}

func TestBackendsKeepThreeBucketAcquireAtomicAndUseExplicitScopes(t *testing.T) {
	newFixtures := map[string]func(*testing.T) limiterFixture{
		"memory": newMemoryFixture,
		"redis":  newRedisFixture,
	}
	tests := []struct {
		name         string
		blockedKey   string
		blockedScope string
	}{
		{name: "global bucket", blockedKey: "global", blockedScope: "global"},
		{name: "group bucket", blockedKey: "group", blockedScope: "group"},
		{name: "user bucket at index three", blockedKey: "user", blockedScope: "user"},
	}

	for backendName, newFixture := range newFixtures {
		for _, test := range tests {
			t.Run(backendName+"/"+test.name, func(t *testing.T) {
				fixture := newFixture(t)
				defer fixture.close()
				require.True(t, fixture.backend.Acquire(context.Background(), []Bucket{{
					Key: test.blockedKey, Limit: 1, Scope: test.blockedScope,
				}}).Allowed)

				buckets := []Bucket{
					{Key: "global", Limit: 2, Scope: "global"},
					{Key: "group", Limit: 2, Scope: "group"},
					{Key: "user", Limit: 2, Scope: "user"},
				}
				for i := range buckets {
					if buckets[i].Key == test.blockedKey {
						buckets[i].Limit = 1
					}
				}
				result := fixture.backend.Acquire(context.Background(), buckets)
				assert.Equal(t, Result{Allowed: false, Scope: test.blockedScope, Limit: 1, Current: 1}, result)
				expectedCounts := map[string]int{"global": 0, "group": 0, "user": 0}
				expectedCounts[test.blockedKey] = 1
				assert.Equal(t, expectedCounts, map[string]int{
					"global": fixture.count("global"),
					"group":  fixture.count("group"),
					"user":   fixture.count("user"),
				})
			})
		}
	}
}

func TestBackendsDoNotInferScopeFromBucketPosition(t *testing.T) {
	for name, newFixture := range map[string]func(*testing.T) limiterFixture{
		"memory": newMemoryFixture,
		"redis":  newRedisFixture,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			defer fixture.close()
			buckets := []Bucket{
				{Key: "user", Limit: 1, Scope: "user"},
				{Key: "global", Limit: 10, Scope: "global"},
			}
			require.True(t, fixture.backend.Acquire(context.Background(), buckets).Allowed)
			assert.Equal(t, Result{Allowed: false, Scope: "user", Limit: 1, Current: 1}, fixture.backend.Acquire(context.Background(), buckets))
		})
	}
}

func TestBackendsReportTheFirstFullBucketInCallerPriorityOrder(t *testing.T) {
	for backendName, newFixture := range map[string]func(*testing.T) limiterFixture{
		"memory": newMemoryFixture,
		"redis":  newRedisFixture,
	} {
		for _, test := range []struct {
			name          string
			prefill       []Bucket
			expectedScope string
		}{
			{
				name: "global wins when every bucket is full",
				prefill: []Bucket{
					{Key: "global", Limit: 1, Scope: "global"},
					{Key: "group", Limit: 1, Scope: "group"},
					{Key: "user", Limit: 1, Scope: "user"},
				},
				expectedScope: "global",
			},
			{
				name: "group wins over user",
				prefill: []Bucket{
					{Key: "group", Limit: 1, Scope: "group"},
					{Key: "user", Limit: 1, Scope: "user"},
				},
				expectedScope: "group",
			},
		} {
			t.Run(backendName+"/"+test.name, func(t *testing.T) {
				fixture := newFixture(t)
				defer fixture.close()
				for _, bucket := range test.prefill {
					require.True(t, fixture.backend.Acquire(context.Background(), []Bucket{bucket}).Allowed)
				}
				result := fixture.backend.Acquire(context.Background(), []Bucket{
					{Key: "global", Limit: 1, Scope: "global"},
					{Key: "group", Limit: 1, Scope: "group"},
					{Key: "user", Limit: 1, Scope: "user"},
				})
				assert.False(t, result.Allowed)
				assert.Equal(t, test.expectedScope, result.Scope)
			})
		}
	}
}

func TestBackendsInspectThreeBucketsInOneRead(t *testing.T) {
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
			}
			require.True(t, fixture.backend.Acquire(context.Background(), buckets).Allowed)

			counts, err := fixture.backend.Inspect(context.Background(), []string{"global", "group", "user"})
			require.NoError(t, err)
			assert.Equal(t, []int{1, 1, 1}, counts)
		})
	}
}

func TestAcquireEmptyKeysDoesNotInitializeBackend(t *testing.T) {
	previousBackend := backendImpl
	previousInitialized := previousBackend != nil
	backendImpl = nil
	backendOnce = sync.Once{}
	t.Cleanup(func() {
		backendImpl = previousBackend
		backendOnce = sync.Once{}
		if previousInitialized {
			backendOnce.Do(func() {})
		}
	})

	assert.Equal(t, Result{Allowed: true}, Acquire(context.Background(), nil))
	assert.Nil(t, backendImpl)
}

func TestAcquireRejectsInvalidShapeOpen(t *testing.T) {
	fixture := newMemoryBackend()
	previousBackend := backendImpl
	previousInitialized := previousBackend != nil
	backendImpl = fixture
	backendOnce = sync.Once{}
	backendOnce.Do(func() {})
	t.Cleanup(func() {
		backendImpl = previousBackend
		backendOnce = sync.Once{}
		if previousInitialized {
			backendOnce.Do(func() {})
		}
	})

	assert.Equal(t, Result{Allowed: true}, Acquire(context.Background(), []Bucket{
		{Key: "a", Limit: 1, Scope: "global"},
		{Key: "b", Limit: 1, Scope: "group"},
		{Key: "c", Limit: 1, Scope: "user"},
		{Key: "d", Limit: 1, Scope: "group_total"},
	}))
	assert.Equal(t, Result{Allowed: true}, Acquire(context.Background(), []Bucket{{Key: "a", Limit: 0, Scope: "global"}}))
	assert.Equal(t, 2, fixture.count("a"))
	assert.Equal(t, Result{Allowed: true}, Acquire(context.Background(), []Bucket{{Key: "negative", Limit: -1, Scope: "global"}}))
	assert.Equal(t, 0, fixture.count("negative"))
	assert.Equal(t, Result{Allowed: true}, Acquire(context.Background(), []Bucket{
		{Key: "a", Limit: 1, Scope: "global"},
		{Key: "b", Limit: 1, Scope: "group"},
		{Key: "c", Limit: 1, Scope: "user"},
		{Key: "d", Limit: 1, Scope: "group_total"},
		{Key: "e", Limit: 1, Scope: "global"},
	}))
}

func TestAcquireAcceptsThreeBuckets(t *testing.T) {
	fixture := newMemoryBackend()
	previousBackend := backendImpl
	previousInitialized := previousBackend != nil
	backendImpl = fixture
	backendOnce = sync.Once{}
	backendOnce.Do(func() {})
	t.Cleanup(func() {
		backendImpl = previousBackend
		backendOnce = sync.Once{}
		if previousInitialized {
			backendOnce.Do(func() {})
		}
	})

	result := Acquire(context.Background(), []Bucket{
		{Key: "global", Limit: 10, Scope: "global"},
		{Key: "group", Limit: 5, Scope: "group"},
		{Key: "user", Limit: 2, Scope: "user"},
	})
	assert.Equal(t, Result{Allowed: true}, result)
	assert.Equal(t, 1, fixture.count("global"))
	assert.Equal(t, 1, fixture.count("group"))
	assert.Equal(t, 1, fixture.count("user"))
}

func TestInitModelNameLimiterSelectsMemoryWhenRedisDisabled(t *testing.T) {
	resetGlobalLimiterState(t)
	previousEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousEnabled
		common.RDB = previousRDB
	})

	InitModelNameLimiter()
	require.IsType(t, &memoryBackend{}, backendImpl)
	assert.Same(t, backendImpl, getBackend())
}

func TestInitModelNameLimiterFallsBackWhenScriptLoadFails(t *testing.T) {
	resetGlobalLimiterState(t)
	previousEnabled := common.RedisEnabled
	previousRDB := common.RDB
	fake, client := newFakeRedisClient(t)
	common.RedisEnabled = true
	common.RDB = client
	fake.mu.Lock()
	fake.scriptLoadError = true
	fake.mu.Unlock()
	t.Cleanup(func() {
		common.RedisEnabled = previousEnabled
		common.RDB = previousRDB
	})

	InitModelNameLimiter()
	require.IsType(t, &memoryBackend{}, backendImpl)
}

func TestInitModelNameLimiterSelectsRedisAndIsIdempotent(t *testing.T) {
	resetGlobalLimiterState(t)
	previousEnabled := common.RedisEnabled
	previousRDB := common.RDB
	fake, client := newFakeRedisClient(t)
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = previousEnabled
		common.RDB = previousRDB
	})

	InitModelNameLimiter()
	first := backendImpl
	require.IsType(t, &redisBackend{}, first)
	InitModelNameLimiter()
	assert.Same(t, first, backendImpl)
	fake.mu.Lock()
	assert.Equal(t, 1, fake.scriptLoads)
	fake.mu.Unlock()
}

func TestGetBackendLazilyInitializesMemory(t *testing.T) {
	resetGlobalLimiterState(t)
	previousEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousEnabled
		common.RDB = previousRDB
	})

	require.IsType(t, &memoryBackend{}, getBackend())
}

func TestRedisBackendHandlesMissingScriptAndMalformedClient(t *testing.T) {
	fake, client := newFakeRedisClient(t)
	defer client.Close()
	b := &redisBackend{client: client}
	assert.Equal(t, Result{Allowed: true}, b.Acquire(context.Background(), []Bucket{{Key: "global", Limit: 1, Scope: "global"}}))
	assert.Equal(t, Result{Allowed: true}, (*redisBackend)(nil).Acquire(context.Background(), []Bucket{{Key: "global", Limit: 1, Scope: "global"}}))

	fake.mu.Lock()
	fake.scriptLoadError = true
	fake.mu.Unlock()
	assert.Error(t, b.loadScript(context.Background()))
}

func TestMemoryTrimDropsAllAndKeepsRecentHits(t *testing.T) {
	b := newMemoryBackend()
	now := time.Now().UnixMilli()
	b.entries["bucket"] = memoryEntry{hits: []int64{now - int64(rpmWindowSeconds*1000) - 1, now - 1}, expiresAt: now + int64(5*time.Second/time.Millisecond)}
	assert.Equal(t, 1, b.count("bucket"))
	b.entries["old"] = memoryEntry{hits: []int64{now - int64(rpmWindowSeconds*1000) - 1}, expiresAt: now + int64(5*time.Second/time.Millisecond)}
	assert.Equal(t, 0, b.count("old"))
	assert.Equal(t, Result{Allowed: true}, (*memoryBackend)(nil).Acquire(context.Background(), []Bucket{{Key: "global", Limit: 1, Scope: "global"}}))
	assert.Equal(t, 0, (*memoryBackend)(nil).count("global"))
	zero := &memoryBackend{}
	assert.Equal(t, Result{Allowed: true}, zero.Acquire(context.Background(), []Bucket{{Key: "global", Limit: 1, Scope: "global"}}))
	assert.Equal(t, Result{Allowed: true}, zero.Acquire(context.Background(), []Bucket{{Key: "other-global", Limit: 1, Scope: "global"}, {Key: "group", Limit: 1, Scope: "group"}}))
}

func resetGlobalLimiterState(t *testing.T) {
	t.Helper()
	previousBackend := backendImpl
	previousInitialized := previousBackend != nil
	backendImpl = nil
	backendOnce = sync.Once{}
	t.Cleanup(func() {
		backendImpl = previousBackend
		backendOnce = sync.Once{}
		if previousInitialized {
			backendOnce.Do(func() {})
		}
	})
}

func TestRedisBackendFailOpenOnCommandError(t *testing.T) {
	fake, client := newFakeRedisClient(t)
	defer client.Close()
	backend := &redisBackend{client: client, acquireSHA: "sha"}
	fake.mu.Lock()
	fake.evalError = true
	fake.mu.Unlock()

	assert.Equal(t, Result{Allowed: true}, backend.Acquire(context.Background(), []Bucket{{Key: "global", Limit: 1, Scope: "global"}}))
}

func TestRedisBackendFailOpenOnTimeoutAndHonorsRequestContext(t *testing.T) {
	fake, client := newFakeRedisClient(t)
	defer client.Close()
	backend := &redisBackend{client: client, acquireSHA: "sha"}
	fake.mu.Lock()
	fake.evalDelay = 500 * time.Millisecond
	fake.mu.Unlock()

	started := time.Now()
	result := backend.Acquire(context.Background(), []Bucket{{Key: "global", Limit: 1, Scope: "global"}})
	assert.Equal(t, Result{Allowed: true}, result)
	assert.Less(t, time.Since(started), time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(t, Result{Allowed: true}, backend.Acquire(ctx, []Bucket{{Key: "global", Limit: 1, Scope: "global"}}))
}

func TestRedisBackendReloadsOnNoScript(t *testing.T) {
	fake, client := newFakeRedisClient(t)
	defer client.Close()
	backend := &redisBackend{client: client, acquireSHA: "old-sha"}
	fake.mu.Lock()
	fake.noScriptRemaining = 1
	fake.mu.Unlock()

	assert.Equal(t, Result{Allowed: true}, backend.Acquire(context.Background(), []Bucket{{Key: "global", Limit: 1, Scope: "global"}}))
	fake.mu.Lock()
	loadCount := fake.scriptLoads
	fake.mu.Unlock()
	assert.Equal(t, 1, loadCount)
	assert.Equal(t, "sha-1", backend.currentSHA())
}

func TestNewRedisBackendLoadsScriptAndReportsFailure(t *testing.T) {
	previousRDB := common.RDB
	t.Cleanup(func() { common.RDB = previousRDB })

	common.RDB = nil
	backend, err := newRedisBackend()
	assert.Nil(t, backend)
	require.EqualError(t, err, "redis client not initialized")

	fake, client := newFakeRedisClient(t)
	defer client.Close()
	common.RDB = client
	backend, err = newRedisBackend()
	require.NoError(t, err)
	require.NotNil(t, backend)
	assert.Equal(t, "sha-1", backend.currentSHA())
	fake.mu.Lock()
	assert.Equal(t, 1, fake.scriptLoads)
	fake.mu.Unlock()
}

func TestParseAcquireResult(t *testing.T) {
	buckets := []Bucket{
		{Key: "model", Limit: 3, Scope: "global"},
		{Key: "group", Limit: 4, Scope: "group"},
		{Key: "user", Limit: 5, Scope: "user"},
	}
	tests := []struct {
		name  string
		input []string
		want  Result
	}{
		{name: "allowed", input: []string{"1"}, want: Result{Allowed: true}},
		{name: "global denied", input: []string{"0", "1", "3", "3"}, want: Result{Scope: "global", Limit: 3, Current: 3}},
		{name: "group denied", input: []string{"0", "2", "4", "4"}, want: Result{Scope: "group", Limit: 4, Current: 4}},
		{name: "user denied", input: []string{"0", "3", "5", "5"}, want: Result{Scope: "user", Limit: 5, Current: 5}},
		{name: "malformed", input: []string{"0"}, want: Result{Allowed: true}},
		{name: "bad scope", input: []string{"0", "x", "1", "1"}, want: Result{Allowed: true}},
		{name: "bad limit", input: []string{"0", "1", "x", "1"}, want: Result{Allowed: true}},
		{name: "bad current", input: []string{"0", "1", "1", "x"}, want: Result{Allowed: true}},
		{name: "unknown index", input: []string{"0", "4", "1", "1"}, want: Result{Allowed: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, parseAcquireResult(test.input, buckets))
		})
	}
}

func TestRedisResultStringAcceptsRedisScalarTypes(t *testing.T) {
	tests := []struct {
		input interface{}
		want  string
	}{
		{input: "bulk", want: "bulk"},
		{input: []byte("bytes"), want: "bytes"},
		{input: int64(7), want: "7"},
		{input: float64(2.5), want: "2.5"},
	}
	for _, test := range tests {
		value, err := redisResultString(test.input)
		require.NoError(t, err)
		assert.Equal(t, test.want, value)
	}
	_, err := redisResultString(true)
	assert.Error(t, err)
}

func newMemoryFixture(_ *testing.T) limiterFixture {
	b := newMemoryBackend()
	return limiterFixture{
		backend: b,
		count:   b.count,
		oldHit: func(key string) {
			b.mu.Lock()
			b.entries[key] = memoryEntry{
				hits:      []int64{time.Now().Add(-61 * time.Second).UnixMilli()},
				expiresAt: time.Now().Add(4 * time.Second).UnixMilli(),
			}
			b.mu.Unlock()
		},
		close: func() {},
	}
}

func newRedisFixture(t *testing.T) limiterFixture {
	fake, client := newFakeRedisClient(t)
	b := &redisBackend{client: client, acquireSHA: "sha"}
	return limiterFixture{
		backend: b,
		count: func(key string) int {
			count, err := client.ZCard(context.Background(), key).Result()
			require.NoError(t, err)
			return int(count)
		},
		oldHit: func(key string) {
			fake.mu.Lock()
			fake.hits[key] = append(fake.hits[key], fakeHit{score: time.Now().Add(-61 * time.Second).UnixMilli(), member: "old"})
			fake.mu.Unlock()
		},
		close: func() { require.NoError(t, client.Close()) },
	}
}

type fakeHit struct {
	score  int64
	member string
}

type fakeRedis struct {
	mu sync.Mutex

	hits map[string][]fakeHit

	scriptLoads       int
	noScriptRemaining int
	evalError         bool
	evalDelay         time.Duration
	scriptLoadError   bool
}

func newFakeRedisClient(t *testing.T) (*fakeRedis, *redis.Client) {
	t.Helper()
	fake := &fakeRedis{hits: make(map[string][]fakeHit)}
	client := redis.NewClient(&redis.Options{
		Addr:         "model-name-limiter-test",
		MaxRetries:   -1,
		PoolSize:     64,
		MinIdleConns: 16,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go fake.serve(serverConn)
			return clientConn, nil
		},
	})
	t.Cleanup(func() { _ = client.Close() })
	return fake, client
}

func (f *fakeRedis) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		command, err := readRESPCommand(reader)
		if err != nil {
			return
		}
		if _, err = io.WriteString(conn, f.respond(command)); err != nil {
			return
		}
	}
}

func (f *fakeRedis) respond(command []string) string {
	if len(command) == 0 {
		return "-ERR empty command\r\n"
	}
	switch strings.ToUpper(command[0]) {
	case "SCRIPT":
		if len(command) < 2 || strings.ToUpper(command[1]) != "LOAD" {
			return "-ERR unsupported script command\r\n"
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.scriptLoadError {
			return "-ERR script load failed\r\n"
		}
		f.scriptLoads++
		return redisBulkResponse(fmt.Sprintf("sha-%d", f.scriptLoads))
	case "EVALSHA":
		return f.respondEval(command)
	case "ZCARD":
		if len(command) != 2 {
			return "-ERR malformed zcard\r\n"
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.trimLocked(command[1], time.Now().UnixMilli())
		return fmt.Sprintf(":%d\r\n", len(f.hits[command[1]]))
	case "ZADD":
		if len(command) < 4 {
			return "-ERR malformed zadd\r\n"
		}
		score, err := strconv.ParseInt(command[2], 10, 64)
		if err != nil {
			return "-ERR malformed score\r\n"
		}
		f.mu.Lock()
		f.hits[command[1]] = append(f.hits[command[1]], fakeHit{score: score, member: command[3]})
		f.mu.Unlock()
		return ":1\r\n"
	case "DEL":
		f.mu.Lock()
		for _, key := range command[1:] {
			delete(f.hits, key)
		}
		f.mu.Unlock()
		return fmt.Sprintf(":%d\r\n", len(command)-1)
	case "PEXPIRE":
		return ":1\r\n"
	default:
		return "-ERR unexpected command\r\n"
	}
}

func (f *fakeRedis) respondEval(command []string) string {
	if len(command) < 4 {
		return "-ERR malformed evalsha\r\n"
	}
	f.mu.Lock()
	if f.evalDelay > 0 {
		delay := f.evalDelay
		f.mu.Unlock()
		time.Sleep(delay)
		f.mu.Lock()
	}
	if f.evalError {
		f.mu.Unlock()
		return "-ERR injected acquire failure\r\n"
	}
	if f.noScriptRemaining > 0 {
		f.noScriptRemaining--
		f.mu.Unlock()
		return "-NOSCRIPT No matching script. Please use EVAL.\r\n"
	}
	numKeys, err := strconv.Atoi(command[2])
	if err != nil || numKeys < 1 || len(command) < 3+numKeys+1 {
		f.mu.Unlock()
		return "-ERR malformed evalsha arguments\r\n"
	}
	keys := command[3 : 3+numKeys]
	args := command[3+numKeys:]
	window, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		f.mu.Unlock()
		return "-ERR malformed evalsha args\r\n"
	}
	now := time.Now().UnixMilli()
	cutoff := now - window*1000
	if len(args) == 1 {
		counts := make([]int64, len(keys))
		for i, key := range keys {
			f.trimLocked(key, now)
			counts[i] = int64(len(f.hits[key]))
		}
		f.mu.Unlock()
		return redisIntegerArrayResponse(counts...)
	}
	if len(args) < numKeys+2 {
		f.mu.Unlock()
		return "-ERR malformed evalsha args\r\n"
	}
	for i, key := range keys {
		f.trimLocked(key, now)
		limit, parseErr := strconv.Atoi(args[i+1])
		if parseErr != nil {
			f.mu.Unlock()
			return "-ERR malformed limit\r\n"
		}
		current := len(f.hits[key])
		if limit > 0 && current >= limit {
			f.mu.Unlock()
			return redisMixedArrayResponse("0", int64(i+1), int64(limit), int64(current))
		}
	}
	token := args[len(args)-1]
	for i, key := range keys {
		f.hits[key] = append(f.hits[key], fakeHit{score: now, member: token + ":" + strconv.Itoa(i+1)})
	}
	_ = cutoff
	f.mu.Unlock()
	return redisArrayResponse("1")
}

func (f *fakeRedis) trimLocked(key string, now int64) {
	cutoff := now - int64(windowSeconds*1000)
	hits := f.hits[key]
	kept := hits[:0]
	for _, hit := range hits {
		if hit.score > cutoff {
			kept = append(kept, hit)
		}
	}
	f.hits[key] = kept
}

func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(header) < 3 || header[0] != '*' {
		return nil, fmt.Errorf("invalid RESP array header %q", header)
	}
	count, err := strconv.Atoi(strings.TrimSpace(header[1:]))
	if err != nil {
		return nil, err
	}
	command := make([]string, count)
	for i := range command {
		bulkHeader, readErr := reader.ReadString('\n')
		if readErr != nil {
			return nil, readErr
		}
		if len(bulkHeader) < 3 || bulkHeader[0] != '$' {
			return nil, fmt.Errorf("invalid RESP bulk header %q", bulkHeader)
		}
		length, parseErr := strconv.Atoi(strings.TrimSpace(bulkHeader[1:]))
		if parseErr != nil {
			return nil, parseErr
		}
		payload := make([]byte, length+2)
		if _, readErr = io.ReadFull(reader, payload); readErr != nil {
			return nil, readErr
		}
		command[i] = string(payload[:length])
	}
	return command, nil
}

func redisBulkResponse(value string) string {
	return fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)
}

func redisArrayResponse(values ...string) string {
	var response strings.Builder
	fmt.Fprintf(&response, "*%d\r\n", len(values))
	for _, value := range values {
		response.WriteString(redisBulkResponse(value))
	}
	return response.String()
}

func redisMixedArrayResponse(first string, values ...int64) string {
	var response strings.Builder
	fmt.Fprintf(&response, "*%d\r\n", len(values)+1)
	response.WriteString(redisBulkResponse(first))
	for _, value := range values {
		fmt.Fprintf(&response, ":%d\r\n", value)
	}
	return response.String()
}

func redisIntegerArrayResponse(values ...int64) string {
	var response strings.Builder
	fmt.Fprintf(&response, "*%d\r\n", len(values))
	for _, value := range values {
		fmt.Fprintf(&response, ":%d\r\n", value)
	}
	return response.String()
}
