package channel_limiter

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

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBackend struct {
	acquireFn func(context.Context, int, *dto.ChannelRateLimit) (*Token, Decision)
	peekFn    func(context.Context, int, *dto.ChannelRateLimit) Decision
	statsFn   func(context.Context, []int) map[int][2]int64
}

func (f *fakeBackend) Acquire(ctx context.Context, channelID int, cfg *dto.ChannelRateLimit) (*Token, Decision) {
	if f.acquireFn == nil {
		return nil, Decision{Allowed: true}
	}
	return f.acquireFn(ctx, channelID, cfg)
}

func (f *fakeBackend) Peek(ctx context.Context, channelID int, cfg *dto.ChannelRateLimit) Decision {
	if f.peekFn == nil {
		return Decision{Allowed: true}
	}
	return f.peekFn(ctx, channelID, cfg)
}

func (f *fakeBackend) Stats(ctx context.Context, channelIDs []int) map[int][2]int64 {
	if f.statsFn == nil {
		return map[int][2]int64{}
	}
	return f.statsFn(ctx, channelIDs)
}

func useBackend(t *testing.T, replacement backend) {
	t.Helper()
	previous := backendImpl
	previousInitialized := previous != nil
	backendImpl = replacement
	backendOnce = sync.Once{}
	backendOnce.Do(func() {})
	t.Cleanup(func() {
		backendImpl = previous
		backendOnce = sync.Once{}
		if previousInitialized {
			backendOnce.Do(func() {})
		}
	})
}

func resetQueueWaiters(t *testing.T) {
	t.Helper()
	queueWaitersMu.Lock()
	previous := queueWaiters
	queueWaiters = make(map[int]int)
	queueWaitersMu.Unlock()
	t.Cleanup(func() {
		queueWaitersMu.Lock()
		queueWaiters = previous
		queueWaitersMu.Unlock()
	})
}

func assertNoQueueWaiter(t *testing.T, channelID int) {
	t.Helper()
	queueWaitersMu.Lock()
	defer queueWaitersMu.Unlock()
	assert.NotContains(t, queueWaiters, channelID)
}

func newRedisTestClient(t *testing.T, responder func([]string) string) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr:       "channel-limiter-test",
		MaxRetries: -1,
		PoolSize:   1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go func() {
				defer serverConn.Close()
				reader := bufio.NewReader(serverConn)
				for {
					command, err := readRedisCommand(reader)
					if err != nil {
						return
					}
					if _, err = io.WriteString(serverConn, responder(command)); err != nil {
						return
					}
				}
			}()
			return clientConn, nil
		},
	})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func readRedisCommand(reader *bufio.Reader) ([]string, error) {
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

func redisBulk(value string) string {
	return fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)
}

func redisStringArray(values ...string) string {
	var response strings.Builder
	fmt.Fprintf(&response, "*%d\r\n", len(values))
	for _, value := range values {
		response.WriteString(redisBulk(value))
	}
	return response.String()
}

func redisIntArray(values ...int64) string {
	var response strings.Builder
	fmt.Fprintf(&response, "*%d\r\n", len(values))
	for _, value := range values {
		fmt.Fprintf(&response, ":%d\r\n", value)
	}
	return response.String()
}

func resetBackendInitialization(t *testing.T) {
	t.Helper()
	previous := backendImpl
	previousInitialized := previous != nil
	backendImpl = nil
	backendOnce = sync.Once{}
	t.Cleanup(func() {
		backendImpl = previous
		backendOnce = sync.Once{}
		if previousInitialized {
			backendOnce.Do(func() {})
		}
	})
}

func TestTokenReleaseIsNilSafeAndIdempotent(t *testing.T) {
	var nilToken *Token
	require.NotPanics(t, nilToken.Release)

	releaseCount := 0
	token := &Token{release: func() { releaseCount++ }}
	token.Release()
	token.Release()
	assert.Equal(t, 1, releaseCount)
	require.NotPanics(t, (&Token{}).Release)
}

func TestConfigResolution(t *testing.T) {
	assert.False(t, IsActive(nil))
	assert.False(t, IsActive(&dto.ChannelRateLimit{Enabled: true}))
	assert.False(t, IsActive(&dto.ChannelRateLimit{RPM: 1}))
	assert.True(t, IsActive(&dto.ChannelRateLimit{Enabled: true, RPM: 1}))
	assert.True(t, IsActive(&dto.ChannelRateLimit{Enabled: true, Concurrency: 1}))
	assert.Nil(t, resolvedConfig(nil))

	original := &dto.ChannelRateLimit{Enabled: true, RPM: 1, OnLimit: "invalid"}
	resolved := resolvedConfig(original)
	require.NotSame(t, original, resolved)
	assert.Equal(t, OnLimitSkip, resolved.OnLimit)
	assert.Equal(t, defaultQueueDepth, resolved.QueueDepth)
	assert.Equal(t, defaultQueueMaxWaitMs, resolved.QueueMaxWaitMs)
	assert.Equal(t, "invalid", original.OnLimit)

	for _, strategy := range []string{OnLimitSkip, OnLimitQueue, OnLimitReject} {
		resolved = resolvedConfig(&dto.ChannelRateLimit{
			OnLimit:        strategy,
			QueueDepth:     3,
			QueueMaxWaitMs: 4,
		})
		assert.Equal(t, strategy, resolved.OnLimit)
		assert.Equal(t, 3, resolved.QueueDepth)
		assert.Equal(t, 4, resolved.QueueMaxWaitMs)
	}
}

func TestCheckOnly(t *testing.T) {
	peekCalls := 0
	useBackend(t, &fakeBackend{peekFn: func(ctx context.Context, channelID int, cfg *dto.ChannelRateLimit) Decision {
		peekCalls++
		assert.Equal(t, 17, channelID)
		assert.Equal(t, 5, cfg.RPM)
		return Decision{Allowed: false, Reason: ReasonRPMExceeded}
	}})

	assert.Equal(t, Decision{Allowed: true}, CheckOnly(context.Background(), 17, nil))
	decision := CheckOnly(context.Background(), 17, &dto.ChannelRateLimit{Enabled: true, RPM: 5})
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonRPMExceeded}, decision)
	assert.Equal(t, 1, peekCalls)
}

func TestCheckOnlyBatchUsesOneStatsCallAndPrioritizesRPM(t *testing.T) {
	statsCalls := 0
	useBackend(t, &fakeBackend{statsFn: func(ctx context.Context, channelIDs []int) map[int][2]int64 {
		statsCalls++
		assert.ElementsMatch(t, []int{1, 2, 3, 6}, channelIDs)
		return map[int][2]int64{
			1: {10, 4},
			2: {9, 4},
			3: {9, 3},
		}
	}})

	decisions := CheckOnlyBatch(context.Background(), map[int]*dto.ChannelRateLimit{
		1: {Enabled: true, RPM: 10, Concurrency: 4},
		2: {Enabled: true, RPM: 10, Concurrency: 4},
		3: {Enabled: true, RPM: 10, Concurrency: 4},
		4: {Enabled: false, RPM: 1},
		5: nil,
		6: {Enabled: true, RPM: 1},
	})

	assert.Equal(t, 1, statsCalls)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonRPMExceeded}, decisions[1])
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonConcurrencyExceeded}, decisions[2])
	assert.Equal(t, Decision{Allowed: true}, decisions[3])
	assert.Equal(t, Decision{Allowed: true}, decisions[4])
	assert.Equal(t, Decision{Allowed: true}, decisions[5])
	assert.Equal(t, Decision{Allowed: true}, decisions[6])
}

func TestCheckOnlyBatchFailsOpenWhenStatsUnavailable(t *testing.T) {
	statsCalls := 0
	useBackend(t, &fakeBackend{statsFn: func(context.Context, []int) map[int][2]int64 {
		statsCalls++
		return nil
	}})

	decisions := CheckOnlyBatch(context.Background(), map[int]*dto.ChannelRateLimit{
		1: {Enabled: true, RPM: 1},
		2: {Enabled: true, Concurrency: 1},
	})

	assert.Equal(t, 1, statsCalls)
	assert.Equal(t, Decision{Allowed: true}, decisions[1])
	assert.Equal(t, Decision{Allowed: true}, decisions[2])
}

func TestCheckOnlyBatchSkipsStatsWithoutActiveChannels(t *testing.T) {
	statsCalls := 0
	useBackend(t, &fakeBackend{statsFn: func(context.Context, []int) map[int][2]int64 {
		statsCalls++
		return nil
	}})

	assert.Nil(t, CheckOnlyBatch(context.Background(), nil))
	decisions := CheckOnlyBatch(context.Background(), map[int]*dto.ChannelRateLimit{
		1: nil,
		2: {Enabled: false, RPM: 1},
	})

	assert.Equal(t, 0, statsCalls)
	assert.Equal(t, Decision{Allowed: true}, decisions[1])
	assert.Equal(t, Decision{Allowed: true}, decisions[2])
}

type contextKey string

func TestAcquireDetachesNonQueueOperationContext(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), contextKey("request"), "value"))
	cancelRequest()

	var operationCtx context.Context
	useBackend(t, &fakeBackend{acquireFn: func(ctx context.Context, channelID int, cfg *dto.ChannelRateLimit) (*Token, Decision) {
		operationCtx = ctx
		assert.NoError(t, ctx.Err())
		assert.Equal(t, "value", ctx.Value(contextKey("request")))
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline)
		return &Token{}, Decision{Allowed: true}
	}})

	token, decision := Acquire(requestCtx, 7, &dto.ChannelRateLimit{
		Enabled: true,
		RPM:     1,
		OnLimit: OnLimitSkip,
	})

	require.NotNil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)
	require.NotNil(t, operationCtx)
	assert.ErrorIs(t, operationCtx.Err(), context.Canceled)
}

func TestAcquireInactiveDoesNotCallBackend(t *testing.T) {
	useBackend(t, &fakeBackend{acquireFn: func(context.Context, int, *dto.ChannelRateLimit) (*Token, Decision) {
		require.Fail(t, "inactive limiter called backend")
		return nil, Decision{}
	}})

	token, decision := Acquire(context.Background(), 1, nil)
	assert.Nil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)
}

func TestAcquireQueueRejectsAlreadyCanceledRequest(t *testing.T) {
	resetQueueWaiters(t)
	useBackend(t, &fakeBackend{acquireFn: func(context.Context, int, *dto.ChannelRateLimit) (*Token, Decision) {
		require.Fail(t, "canceled queue request called backend")
		return nil, Decision{}
	}})
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	token, decision := Acquire(requestCtx, 21, &dto.ChannelRateLimit{
		Enabled:     true,
		Concurrency: 1,
		OnLimit:     OnLimitQueue,
	})

	assert.Nil(t, token)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonQueueTimeout}, decision)
	assertNoQueueWaiter(t, 21)
}

func TestAcquireQueueCancellationAfterAcquisitionReleasesToken(t *testing.T) {
	resetQueueWaiters(t)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	backendCalled := make(chan context.Context, 1)
	returnAcquired := make(chan struct{})
	released := make(chan struct{}, 1)
	useBackend(t, &fakeBackend{acquireFn: func(ctx context.Context, channelID int, cfg *dto.ChannelRateLimit) (*Token, Decision) {
		backendCalled <- ctx
		<-returnAcquired
		return &Token{release: func() { released <- struct{}{} }}, Decision{Allowed: true}
	}})

	type acquireResult struct {
		token    *Token
		decision Decision
	}
	resultCh := make(chan acquireResult, 1)
	go func() {
		token, decision := Acquire(requestCtx, 22, &dto.ChannelRateLimit{
			Enabled:     true,
			Concurrency: 1,
			OnLimit:     OnLimitQueue,
		})
		resultCh <- acquireResult{token: token, decision: decision}
	}()

	operationCtx := <-backendCalled
	cancelRequest()
	assert.NoError(t, operationCtx.Err())
	close(returnAcquired)
	result := <-resultCh

	assert.Nil(t, result.token)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonQueueTimeout}, result.decision)
	<-released
	assert.ErrorIs(t, operationCtx.Err(), context.Canceled)
	assertNoQueueWaiter(t, 22)
}

func TestAcquireQueueCreatesFreshOperationContextForEveryPoll(t *testing.T) {
	resetQueueWaiters(t)
	type observedContext struct {
		ctx  context.Context
		done <-chan struct{}
	}
	operationContexts := make(chan observedContext, 2)
	callCount := 0
	useBackend(t, &fakeBackend{acquireFn: func(ctx context.Context, channelID int, cfg *dto.ChannelRateLimit) (*Token, Decision) {
		callCount++
		operationContexts <- observedContext{ctx: ctx, done: ctx.Done()}
		if callCount == 1 {
			return nil, Decision{Allowed: false, Reason: ReasonConcurrencyExceeded}
		}
		return &Token{}, Decision{Allowed: true}
	}})

	type acquireResult struct {
		token    *Token
		decision Decision
	}
	resultCh := make(chan acquireResult, 1)
	go func() {
		token, decision := Acquire(context.Background(), 23, &dto.ChannelRateLimit{
			Enabled:        true,
			Concurrency:    1,
			OnLimit:        OnLimitQueue,
			QueueMaxWaitMs: 1000,
		})
		resultCh <- acquireResult{token: token, decision: decision}
	}()

	first := <-operationContexts
	second := <-operationContexts
	result := <-resultCh

	require.NotNil(t, result.token)
	assert.Equal(t, Decision{Allowed: true}, result.decision)
	assert.NotEqual(t, first.done, second.done)
	_, firstHasDeadline := first.ctx.Deadline()
	_, secondHasDeadline := second.ctx.Deadline()
	assert.True(t, firstHasDeadline)
	assert.True(t, secondHasDeadline)
	assert.ErrorIs(t, first.ctx.Err(), context.Canceled)
	assert.ErrorIs(t, second.ctx.Err(), context.Canceled)
	assertNoQueueWaiter(t, 23)
}

func TestAcquireQueueWaitRespondsToRequestCancellation(t *testing.T) {
	resetQueueWaiters(t)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	backendCalled := make(chan struct{}, 1)
	returnDenied := make(chan struct{})
	useBackend(t, &fakeBackend{acquireFn: func(ctx context.Context, channelID int, cfg *dto.ChannelRateLimit) (*Token, Decision) {
		backendCalled <- struct{}{}
		<-returnDenied
		return nil, Decision{Allowed: false, Reason: ReasonConcurrencyExceeded}
	}})

	resultCh := make(chan Decision, 1)
	go func() {
		_, decision := Acquire(requestCtx, 24, &dto.ChannelRateLimit{
			Enabled:     true,
			Concurrency: 1,
			OnLimit:     OnLimitQueue,
		})
		resultCh <- decision
	}()

	<-backendCalled
	cancelRequest()
	close(returnDenied)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonQueueTimeout}, <-resultCh)
	assertNoQueueWaiter(t, 24)
}

func TestAcquireQueueFull(t *testing.T) {
	resetQueueWaiters(t)
	useBackend(t, &fakeBackend{acquireFn: func(context.Context, int, *dto.ChannelRateLimit) (*Token, Decision) {
		require.Fail(t, "full queue called backend")
		return nil, Decision{}
	}})
	require.True(t, enterQueue(25, 1))
	t.Cleanup(func() { leaveQueue(25) })

	token, decision := Acquire(context.Background(), 25, &dto.ChannelRateLimit{
		Enabled:     true,
		Concurrency: 1,
		OnLimit:     OnLimitQueue,
		QueueDepth:  1,
	})

	assert.Nil(t, token)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonQueueFull}, decision)
	queueWaitersMu.Lock()
	assert.Equal(t, 1, queueWaiters[25])
	queueWaitersMu.Unlock()
}

func TestAcquireQueueTimesOutAndLeavesQueue(t *testing.T) {
	resetQueueWaiters(t)
	useBackend(t, &fakeBackend{acquireFn: func(context.Context, int, *dto.ChannelRateLimit) (*Token, Decision) {
		return nil, Decision{Allowed: false, Reason: ReasonRPMExceeded}
	}})

	token, decision := Acquire(context.Background(), 26, &dto.ChannelRateLimit{
		Enabled:        true,
		RPM:            1,
		OnLimit:        OnLimitQueue,
		QueueMaxWaitMs: 1,
	})

	assert.Nil(t, token)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonQueueTimeout}, decision)
	assertNoQueueWaiter(t, 26)
}

func TestAcquireQueueAllowsNilContext(t *testing.T) {
	resetQueueWaiters(t)
	useBackend(t, &fakeBackend{acquireFn: func(ctx context.Context, channelID int, cfg *dto.ChannelRateLimit) (*Token, Decision) {
		assert.NoError(t, ctx.Err())
		return &Token{}, Decision{Allowed: true}
	}})

	token, decision := Acquire(nil, 27, &dto.ChannelRateLimit{
		Enabled:     true,
		Concurrency: 1,
		OnLimit:     OnLimitQueue,
	})

	require.NotNil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)
	assertNoQueueWaiter(t, 27)
}

func TestGetStats(t *testing.T) {
	statsCalls := 0
	useBackend(t, &fakeBackend{statsFn: func(ctx context.Context, channelIDs []int) map[int][2]int64 {
		statsCalls++
		assert.ElementsMatch(t, []int{1, 2}, channelIDs)
		return map[int][2]int64{1: {3, 4}}
	}})

	assert.Nil(t, GetStats(context.Background(), nil))
	stats := GetStats(context.Background(), map[int]*dto.ChannelRateLimit{
		1: {RPM: 10, Concurrency: 5},
		2: {RPM: 20, Concurrency: 6},
	})

	assert.Equal(t, 1, statsCalls)
	assert.Equal(t, &ChannelLimitStats{RPM: 3, RPMLimit: 10, Conc: 4, ConcLimit: 5}, stats[1])
	assert.Equal(t, &ChannelLimitStats{RPM: 0, RPMLimit: 20, Conc: 0, ConcLimit: 6}, stats[2])
}

func TestGetStatsReturnsNilWhenBackendStatsUnavailable(t *testing.T) {
	useBackend(t, &fakeBackend{statsFn: func(context.Context, []int) map[int][2]int64 { return nil }})
	assert.Nil(t, GetStats(context.Background(), map[int]*dto.ChannelRateLimit{1: {RPM: 1}}))
}

func TestMemoryBackendEnforcesLimitsAndReleasesConcurrency(t *testing.T) {
	backend := newMemoryBackend()
	cfg := &dto.ChannelRateLimit{RPM: 1, Concurrency: 1}

	token, decision := backend.Acquire(context.Background(), 31, cfg)
	require.NotNil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonRPMExceeded}, backend.Peek(context.Background(), 31, cfg))
	assert.Equal(t, map[int][2]int64{31: {1, 1}, 32: {0, 0}}, backend.Stats(context.Background(), []int{31, 32}))

	secondToken, secondDecision := backend.Acquire(context.Background(), 31, cfg)
	assert.Nil(t, secondToken)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonRPMExceeded}, secondDecision)

	token.Release()
	token.Release()
	assert.Equal(t, [2]int64{1, 0}, backend.Stats(context.Background(), []int{31})[31])
}

func TestMemoryBackendEnforcesConcurrencyAndTrimsExpiredRPM(t *testing.T) {
	backend := newMemoryBackend()
	concurrencyCfg := &dto.ChannelRateLimit{Concurrency: 1}

	token, decision := backend.Acquire(context.Background(), 33, concurrencyCfg)
	require.NotNil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonConcurrencyExceeded}, backend.Peek(context.Background(), 33, concurrencyCfg))
	secondToken, secondDecision := backend.Acquire(context.Background(), 33, concurrencyCfg)
	assert.Nil(t, secondToken)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonConcurrencyExceeded}, secondDecision)
	token.Release()
	assert.Equal(t, Decision{Allowed: true}, backend.Peek(context.Background(), 33, concurrencyCfg))

	entry := backend.entry(34)
	entry.rpmHits = []int64{0}
	assert.Equal(t, Decision{Allowed: true}, backend.Peek(context.Background(), 34, &dto.ChannelRateLimit{RPM: 1}))
	assert.Empty(t, entry.rpmHits)
}

func TestOperationTimeoutIsBounded(t *testing.T) {
	useBackend(t, &fakeBackend{acquireFn: func(ctx context.Context, channelID int, cfg *dto.ChannelRateLimit) (*Token, Decision) {
		<-ctx.Done()
		assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		return nil, Decision{Allowed: true}
	}})

	token, decision := Acquire(context.Background(), 36, &dto.ChannelRateLimit{
		Enabled: true,
		RPM:     1,
		OnLimit: OnLimitSkip,
	})

	assert.Nil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)
}

func TestLeaveQueueRemovesLastWaiterAndToleratesMissingEntry(t *testing.T) {
	resetQueueWaiters(t)
	require.True(t, enterQueue(37, 2))
	require.True(t, enterQueue(37, 2))
	assert.False(t, enterQueue(37, 2))
	leaveQueue(37)
	queueWaitersMu.Lock()
	assert.Equal(t, 1, queueWaiters[37])
	queueWaitersMu.Unlock()
	leaveQueue(37)
	assertNoQueueWaiter(t, 37)
	require.NotPanics(t, func() { leaveQueue(37) })
}

func TestRedisBackendAcquireAndRelease(t *testing.T) {
	var commandsMu sync.Mutex
	var commands [][]string
	client := newRedisTestClient(t, func(command []string) string {
		commandsMu.Lock()
		commands = append(commands, append([]string(nil), command...))
		commandsMu.Unlock()
		if len(command) < 2 {
			return "-ERR malformed command\r\n"
		}
		switch command[1] {
		case "allowed", "allowed-no-concurrency", "release-error-acquire":
			return redisStringArray("1")
		case "denied":
			return redisStringArray("0", ReasonConcurrencyExceeded)
		case "denied-default":
			return redisStringArray("0")
		case "acquire-error":
			return "-ERR acquire failed\r\n"
		case "release":
			return ":0\r\n"
		case "release-error":
			return "-ERR release failed\r\n"
		default:
			return "-ERR unexpected sha\r\n"
		}
	})

	backend := &redisBackend{client: client, acquireSHA: "allowed", releaseSHA: "release"}
	token, decision := backend.Acquire(context.Background(), 41, &dto.ChannelRateLimit{RPM: 2, Concurrency: 1})
	require.NotNil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)
	token.Release()
	token.Release()

	backend.acquireSHA = "allowed-no-concurrency"
	token, decision = backend.Acquire(context.Background(), 42, &dto.ChannelRateLimit{RPM: 2})
	require.NotNil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)
	token.Release()

	backend.acquireSHA = "denied"
	token, decision = backend.Acquire(context.Background(), 43, &dto.ChannelRateLimit{RPM: 2, Concurrency: 1})
	assert.Nil(t, token)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonConcurrencyExceeded}, decision)

	backend.acquireSHA = "denied-default"
	token, decision = backend.Acquire(context.Background(), 44, &dto.ChannelRateLimit{RPM: 2})
	assert.Nil(t, token)
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonRPMExceeded}, decision)

	backend.acquireSHA = "acquire-error"
	token, decision = backend.Acquire(context.Background(), 45, &dto.ChannelRateLimit{RPM: 2})
	assert.Nil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)

	backend.acquireSHA = "release-error-acquire"
	backend.releaseSHA = "release-error"
	token, decision = backend.Acquire(context.Background(), 46, &dto.ChannelRateLimit{Concurrency: 1})
	require.NotNil(t, token)
	assert.Equal(t, Decision{Allowed: true}, decision)
	require.NotPanics(t, token.Release)

	commandsMu.Lock()
	defer commandsMu.Unlock()
	var releaseCalls int
	for _, command := range commands {
		if len(command) > 1 && (command[1] == "release" || command[1] == "release-error") {
			releaseCalls++
		}
	}
	assert.Equal(t, 2, releaseCalls)
}

func TestRedisBackendStats(t *testing.T) {
	client := newRedisTestClient(t, func(command []string) string {
		if len(command) < 2 {
			return "-ERR malformed command\r\n"
		}
		switch command[1] {
		case "stats":
			return redisIntArray(2, 3, 4, 5)
		case "stats-error":
			return "-ERR stats failed\r\n"
		case "stats-short":
			return redisIntArray(2)
		default:
			return "-ERR unexpected sha\r\n"
		}
	})
	backend := &redisBackend{client: client, statsSHA: "stats"}

	assert.Nil(t, backend.Stats(context.Background(), nil))
	assert.Equal(t, map[int][2]int64{51: {2, 3}, 52: {4, 5}}, backend.Stats(context.Background(), []int{51, 52}))

	backend.statsSHA = "stats-error"
	assert.Nil(t, backend.Stats(context.Background(), []int{51}))
	backend.statsSHA = "stats-short"
	assert.Nil(t, backend.Stats(context.Background(), []int{51}))
}

func TestRedisBackendPeek(t *testing.T) {
	client := newRedisTestClient(t, func(command []string) string {
		if len(command) < 2 {
			return "-ERR malformed command\r\n"
		}
		switch command[1] {
		case "available":
			return redisStringArray("1")
		case "concurrency":
			return redisStringArray("0", ReasonConcurrencyExceeded)
		case "default-reason":
			return redisStringArray("0")
		case "peek-error":
			return "-ERR peek failed\r\n"
		default:
			return "-ERR unexpected sha\r\n"
		}
	})
	backend := &redisBackend{client: client, peekSHA: "available"}
	cfg := &dto.ChannelRateLimit{RPM: 2, Concurrency: 1}

	assert.Equal(t, Decision{Allowed: true}, backend.Peek(context.Background(), 61, cfg))
	backend.peekSHA = "concurrency"
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonConcurrencyExceeded}, backend.Peek(context.Background(), 61, cfg))
	backend.peekSHA = "default-reason"
	assert.Equal(t, Decision{Allowed: false, Reason: ReasonRPMExceeded}, backend.Peek(context.Background(), 61, cfg))
	backend.peekSHA = "peek-error"
	assert.Equal(t, Decision{Allowed: true}, backend.Peek(context.Background(), 61, cfg))
}

func TestNewRedisBackend(t *testing.T) {
	previousRDB := common.RDB
	t.Cleanup(func() { common.RDB = previousRDB })

	common.RDB = nil
	backend, err := newRedisBackend()
	assert.Nil(t, backend)
	require.EqualError(t, err, "redis client not initialized")

	loadCount := 0
	common.RDB = newRedisTestClient(t, func(command []string) string {
		if len(command) >= 2 && strings.EqualFold(command[0], "script") && strings.EqualFold(command[1], "load") {
			loadCount++
			return redisBulk(fmt.Sprintf("sha-%d", loadCount))
		}
		return "-ERR unexpected command\r\n"
	})
	backend, err = newRedisBackend()
	require.NoError(t, err)
	require.NotNil(t, backend)
	assert.Equal(t, "sha-1", backend.acquireSHA)
	assert.Equal(t, "sha-2", backend.releaseSHA)
	assert.Equal(t, "sha-3", backend.peekSHA)
	assert.Equal(t, "sha-4", backend.statsSHA)
}

func TestNewRedisBackendReportsEveryScriptLoadFailure(t *testing.T) {
	testCases := []struct {
		name        string
		failAt      int
		errorPrefix string
	}{
		{name: "acquire", failAt: 1, errorPrefix: "load acquire script:"},
		{name: "release", failAt: 2, errorPrefix: "load release script:"},
		{name: "peek", failAt: 3, errorPrefix: "load peek script:"},
		{name: "stats", failAt: 4, errorPrefix: "load stats script:"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			previousRDB := common.RDB
			t.Cleanup(func() { common.RDB = previousRDB })
			loadCount := 0
			common.RDB = newRedisTestClient(t, func(command []string) string {
				loadCount++
				if loadCount == testCase.failAt {
					return "-ERR script load failed\r\n"
				}
				return redisBulk(fmt.Sprintf("sha-%d", loadCount))
			})

			backend, err := newRedisBackend()
			assert.Nil(t, backend)
			require.Error(t, err)
			assert.ErrorContains(t, err, testCase.errorPrefix)
			assert.Equal(t, testCase.failAt, loadCount)
		})
	}
}

func TestGetBackendSelectsConfiguredImplementation(t *testing.T) {
	t.Run("memory when Redis disabled", func(t *testing.T) {
		resetBackendInitialization(t)
		previousRedisEnabled := common.RedisEnabled
		common.RedisEnabled = false
		t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

		backend := getBackend()
		assert.IsType(t, &memoryBackend{}, backend)
		assert.Same(t, backend, getBackend())
	})

	t.Run("memory fallback when Redis initialization fails", func(t *testing.T) {
		resetBackendInitialization(t)
		previousRedisEnabled := common.RedisEnabled
		previousRDB := common.RDB
		common.RedisEnabled = true
		common.RDB = nil
		t.Cleanup(func() {
			common.RedisEnabled = previousRedisEnabled
			common.RDB = previousRDB
		})

		assert.IsType(t, &memoryBackend{}, getBackend())
	})

	t.Run("Redis when initialization succeeds", func(t *testing.T) {
		resetBackendInitialization(t)
		previousRedisEnabled := common.RedisEnabled
		previousRDB := common.RDB
		common.RedisEnabled = true
		common.RDB = newRedisTestClient(t, func(command []string) string {
			return redisBulk("sha")
		})
		t.Cleanup(func() {
			common.RedisEnabled = previousRedisEnabled
			common.RDB = previousRDB
		})

		assert.IsType(t, &redisBackend{}, getBackend())
	})
}
