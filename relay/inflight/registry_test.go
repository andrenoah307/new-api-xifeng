/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package inflight

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testChannel  = 7
	otherChannel = 8
)

var liveThresholds = Thresholds{
	StreamResponseHeader: 300 * time.Second,
	Streaming:            300 * time.Second,
	NonStream:            900 * time.Second,
}

func newTestRegistry() *registry {
	return &registry{entries: make(map[uint64]*Entry)}
}

// countingCloser records how often the body was closed so tests can tell an admin
// tear-down apart from the consumer's own Close.
type countingCloser struct {
	closes atomic.Int32
	onced  func()
}

func (c *countingCloser) Close() error {
	c.closes.Add(1)
	if c.onced != nil {
		c.onced()
	}
	return nil
}

func TestRegisterAndReleaseKeepsRegistryBalanced(t *testing.T) {
	reg := newTestRegistry()

	first := reg.register(testChannel, true, nil, nil)
	second := reg.register(testChannel, false, nil, nil)
	require.NotNil(t, first)
	require.NotNil(t, second)
	assert.NotEqual(t, first.id, second.id, "entry ids must never be reused")

	snapshot, _ := reg.collect(testChannel, time.Now(), liveThresholds)
	assert.Equal(t, 2, snapshot.InFlight)

	first.Release()
	// A second Release must not double-delete or resurrect the degraded flag.
	first.Release()
	snapshot, _ = reg.collect(testChannel, time.Now(), liveThresholds)
	assert.Equal(t, 1, snapshot.InFlight)

	second.Release()
	reg.mu.RLock()
	remaining := len(reg.entries)
	reg.mu.RUnlock()
	assert.Zero(t, remaining, "released entries must be deleted, not merely marked")
}

func TestNilEntryMethodsAreSafe(t *testing.T) {
	var entry *Entry
	assert.NotPanics(t, func() {
		entry.MarkHeadersReceived(nil)
		entry.MarkChunk()
		entry.Release()
	})
}

func TestSnapshotCountsStagesAndOldestAge(t *testing.T) {
	reg := newTestRegistry()
	now := time.Now()

	waitingHeaders := reg.register(testChannel, true, func() {}, nil)
	waitingFirstChunk := reg.register(testChannel, true, func() {}, nil)
	waitingFirstChunk.MarkHeadersReceived(&countingCloser{})
	streaming := reg.register(testChannel, true, func() {}, nil)
	streaming.MarkHeadersReceived(&countingCloser{})
	streaming.MarkChunk()
	otherChannelEntry := reg.register(otherChannel, true, func() {}, nil)
	require.NotNil(t, otherChannelEntry)

	waitingHeaders.startedAt = now.Add(-90 * time.Second)

	snapshot, victims := reg.collect(testChannel, now, liveThresholds)
	assert.Equal(t, testChannel, snapshot.ChannelID)
	assert.Equal(t, 3, snapshot.InFlight, "other channels must not leak into the count")
	assert.Equal(t, 1, snapshot.AwaitingHeaders)
	assert.Equal(t, 1, snapshot.AwaitingFirstChunk)
	assert.Equal(t, int64(90_000), snapshot.OldestAgeMs)
	assert.Zero(t, snapshot.Cancellable, "healthy connections are never cancellable")
	assert.Empty(t, victims)
	assert.True(t, snapshot.PreviewOnly)
	assert.Equal(t, "local_instance", snapshot.Scope)
	assert.Len(t, snapshot.ByReason, 4, "every reason key must be present so the UI never sees undefined")
}

func TestOverdueReasonPerTimeoutContract(t *testing.T) {
	now := time.Now()
	body := &countingCloser{}

	testCases := []struct {
		name       string
		isStream   bool
		cancel     context.CancelFunc
		headersAgo time.Duration // 0 means headers never arrived
		chunkAgo   time.Duration // 0 means no chunk seen
		startedAgo time.Duration
		thresholds Thresholds
		want       string
	}{
		{
			name: "stream waiting for headers past the header timeout", isStream: true,
			cancel: func() {}, startedAgo: 301 * time.Second,
			thresholds: liveThresholds, want: ReasonWaitingResponseHeader,
		},
		{
			name: "stream waiting for headers inside the header timeout", isStream: true,
			cancel: func() {}, startedAgo: 299 * time.Second,
			thresholds: liveThresholds, want: "",
		},
		{
			name: "stream headers arrived but first chunk never came", isStream: true,
			cancel: func() {}, startedAgo: 400 * time.Second, headersAgo: 301 * time.Second,
			thresholds: liveThresholds, want: ReasonWaitingFirstChunk,
		},
		{
			name: "stream went silent after chunking", isStream: true,
			cancel: func() {}, startedAgo: 900 * time.Second, headersAgo: 800 * time.Second,
			chunkAgo: 301 * time.Second, thresholds: liveThresholds, want: ReasonStalledAfterChunk,
		},
		{
			name: "stream still chunking", isStream: true,
			cancel: func() {}, startedAgo: 900 * time.Second, headersAgo: 800 * time.Second,
			chunkAgo: 2 * time.Second, thresholds: liveThresholds, want: "",
		},
		{
			name: "non stream past the overall deadline", isStream: false,
			cancel: func() {}, startedAgo: 901 * time.Second,
			thresholds: liveThresholds, want: ReasonNonStreamTimeout,
		},
		{
			name: "non stream inside the overall deadline", isStream: false,
			cancel: func() {}, startedAgo: 899 * time.Second,
			thresholds: liveThresholds, want: "",
		},
		// "No contract, no overrun": a threshold of 0 means the operator turned
		// that class off, so nothing in it can ever be declared overdue.
		{
			name: "header timeout disabled", isStream: true,
			cancel: func() {}, startedAgo: 10 * time.Hour,
			thresholds: Thresholds{Streaming: 300 * time.Second, NonStream: 900 * time.Second}, want: "",
		},
		{
			name: "streaming timeout disabled", isStream: true,
			cancel: func() {}, startedAgo: 10 * time.Hour, headersAgo: 9 * time.Hour,
			thresholds: Thresholds{StreamResponseHeader: 300 * time.Second, NonStream: 900 * time.Second}, want: "",
		},
		{
			name: "non stream timeout disabled", isStream: false,
			cancel: func() {}, startedAgo: 10 * time.Hour,
			thresholds: Thresholds{StreamResponseHeader: 300 * time.Second, Streaming: 300 * time.Second}, want: "",
		},
		// Without a kill handle there is nothing to pull forward, so the entry must
		// not be advertised as cancellable even though it is past its threshold.
		{
			name: "stream waiting for headers with no cancel handle", isStream: true,
			startedAgo: 10 * time.Hour, thresholds: liveThresholds, want: "",
		},
		{
			name: "non stream with no cancel handle", isStream: false,
			startedAgo: 10 * time.Hour, thresholds: liveThresholds, want: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			reg := newTestRegistry()
			entry := reg.register(testChannel, testCase.isStream, testCase.cancel, nil)
			require.NotNil(t, entry)
			entry.startedAt = now.Add(-testCase.startedAgo)
			if testCase.headersAgo > 0 {
				entry.headersAt.Store(now.Add(-testCase.headersAgo).UnixNano())
				entry.body = body
			}
			if testCase.chunkAgo > 0 {
				stamp := now.Add(-testCase.chunkAgo).UnixNano()
				entry.firstChunk.Store(stamp)
				entry.lastChunk.Store(stamp)
			}

			assert.Equal(t, testCase.want, entry.overdueReason(now, testCase.thresholds))
		})
	}
}

func TestPostHeaderStageNeedsBodyHandle(t *testing.T) {
	reg := newTestRegistry()
	now := time.Now()
	entry := reg.register(testChannel, true, func() {}, nil)
	require.NotNil(t, entry)
	entry.startedAt = now.Add(-time.Hour)
	entry.headersAt.Store(now.Add(-time.Hour).UnixNano())

	assert.Empty(t, entry.overdueReason(now, liveThresholds),
		"after headers the body is the only kill handle; without it nothing is cancellable")

	entry.MarkHeadersReceived(&countingCloser{})
	assert.Equal(t, ReasonWaitingFirstChunk, entry.overdueReason(now, liveThresholds))
}

func TestCleanupTearsDownOnlyOverdueConnections(t *testing.T) {
	reg := newTestRegistry()
	now := time.Now()

	healthyCtx, healthyCancel := context.WithCancel(context.Background())
	defer healthyCancel()
	healthy := reg.register(testChannel, true, healthyCancel, nil)
	healthy.MarkHeadersReceived(&countingCloser{})
	healthy.MarkChunk()

	overdueCtx, overdueCancel := context.WithCancel(context.Background())
	defer overdueCancel()
	var killMarked atomic.Bool
	overdue := reg.register(testChannel, true, overdueCancel, func() { killMarked.Store(true) })
	overdue.startedAt = now.Add(-time.Hour)

	snapshot := reg.cleanup(testChannel, now, liveThresholds)
	assert.Equal(t, 2, snapshot.InFlight)
	assert.Equal(t, 1, snapshot.Cancellable)
	assert.Equal(t, 1, snapshot.Cancelled)
	assert.Equal(t, 1, snapshot.ByReason[ReasonWaitingResponseHeader])
	assert.False(t, snapshot.PreviewOnly)

	assert.True(t, killMarked.Load(), "the relay loop must be told this was an admin kill")
	assert.ErrorIs(t, overdueCtx.Err(), context.Canceled)
	assert.NoError(t, healthyCtx.Err(), "a healthy streaming connection must survive cleanup")

	// A killed entry is no longer cancellable, so a second click cannot double-count it.
	second := reg.cleanup(testChannel, now, liveThresholds)
	assert.Zero(t, second.Cancelled)
}

func TestCleanupClosesBodyForPostHeaderStages(t *testing.T) {
	reg := newTestRegistry()
	now := time.Now()
	body := &countingCloser{}
	entry := reg.register(testChannel, true, nil, nil)
	require.NotNil(t, entry)
	entry.startedAt = now.Add(-time.Hour)
	entry.MarkHeadersReceived(body)
	entry.headersAt.Store(now.Add(-time.Hour).UnixNano())

	snapshot := reg.cleanup(testChannel, now, liveThresholds)
	assert.Equal(t, 1, snapshot.Cancelled)
	assert.Equal(t, int32(1), body.closes.Load())
}

// The scan and the tear-down are deliberately not atomic, so a request can finish
// on its own in between. The cleared count the console reports must not claim it.
func TestConnectionFinishingBetweenScanAndTearDownIsNotCounted(t *testing.T) {
	reg := newTestRegistry()
	now := time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var killMarked atomic.Bool
	entry := reg.register(testChannel, true, cancel, func() { killMarked.Store(true) })
	require.NotNil(t, entry)
	entry.startedAt = now.Add(-time.Hour)

	_, victims := reg.collect(testChannel, now, liveThresholds)
	require.Len(t, victims, 1)

	entry.Release()
	assert.False(t, victims[0].kill(), "a connection that already finished was not cleared by the admin")
	assert.False(t, killMarked.Load(), "a finished request must not be told it was killed")
	assert.NoError(t, ctx.Err())
}

func TestCleanupDoesNotHoldRegistryLockWhileTearingDown(t *testing.T) {
	reg := newTestRegistry()
	now := time.Now()
	// Closing the body re-enters the registry. If cleanup tore connections down
	// under the registry lock this would deadlock instead of returning.
	body := &countingCloser{onced: func() { reg.collect(testChannel, time.Now(), liveThresholds) }}
	entry := reg.register(testChannel, true, nil, nil)
	require.NotNil(t, entry)
	entry.startedAt = now.Add(-time.Hour)
	entry.MarkHeadersReceived(body)
	entry.headersAt.Store(now.Add(-time.Hour).UnixNano())

	done := make(chan Snapshot, 1)
	go func() { done <- reg.cleanup(testChannel, now, liveThresholds) }()
	select {
	case snapshot := <-done:
		assert.Equal(t, 1, snapshot.Cancelled)
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup deadlocked: connections must be torn down outside the registry lock")
	}
}

func TestThresholdsAreReportedForTheOperator(t *testing.T) {
	reg := newTestRegistry()
	snapshot, _ := reg.collect(testChannel, time.Now(), Thresholds{
		StreamResponseHeader: 300 * time.Second,
		NonStream:            900 * time.Second,
	})

	assert.Equal(t, ThresholdSeconds{StreamResponseHeader: 300, Streaming: 0, NonStream: 900}, snapshot.Thresholds)
	assert.Equal(t, ThresholdEnabled{StreamResponseHeader: true, Streaming: false, NonStream: true},
		snapshot.ThresholdEnabled,
		"the UI must be able to explain that a class is uncleanable because no threshold is set")
}

func TestTrackingDegradesWithoutFailingRequests(t *testing.T) {
	reg := newTestRegistry()
	entries := make([]*Entry, 0, maxTrackedEntries)
	for i := 0; i < maxTrackedEntries; i++ {
		entry := reg.register(testChannel, true, nil, nil)
		require.NotNil(t, entry)
		entries = append(entries, entry)
	}

	assert.Nil(t, reg.register(testChannel, true, nil, nil), "registration past the cap is skipped, not fatal")
	snapshot, _ := reg.collect(testChannel, time.Now(), liveThresholds)
	assert.True(t, snapshot.TrackingDegraded)
	assert.Equal(t, maxTrackedEntries, snapshot.InFlight)

	for _, entry := range entries {
		entry.Release()
	}
	snapshot, _ = reg.collect(testChannel, time.Now(), liveThresholds)
	assert.False(t, snapshot.TrackingDegraded, "once nothing dropped is still running the counts are trustworthy again")
}

func TestConcurrentTrafficAndCleanup(t *testing.T) {
	reg := newTestRegistry()
	var waitGroup sync.WaitGroup

	for i := 0; i < 64; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			entry := reg.register(testChannel, true, func() {}, func() {})
			entry.MarkHeadersReceived(&countingCloser{})
			for chunk := 0; chunk < 16; chunk++ {
				entry.MarkChunk()
			}
			entry.Release()
		}()
	}
	for i := 0; i < 8; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			reg.cleanup(testChannel, time.Now().Add(time.Hour), liveThresholds)
		}()
	}
	waitGroup.Wait()

	reg.mu.RLock()
	remaining := len(reg.entries)
	reg.mu.RUnlock()
	assert.Zero(t, remaining, "every tracked request must be released even when cleanup runs concurrently")
}

func TestPackageLevelAPIUsesTheGlobalRegistry(t *testing.T) {
	previousHeaderTimeout := common.RelayStreamResponseHeaderTimeout
	previousStreaming := constant.StreamingTimeout
	previousNonStream := common.RelayNonStreamTimeout
	t.Cleanup(func() {
		common.RelayStreamResponseHeaderTimeout = previousHeaderTimeout
		constant.StreamingTimeout = previousStreaming
		common.RelayNonStreamTimeout = previousNonStream
	})
	common.RelayStreamResponseHeaderTimeout = 300
	constant.StreamingTimeout = 300
	common.RelayNonStreamTimeout = 900
	assert.Equal(t, liveThresholds, CurrentThresholds())

	entry := Register(testChannel, true, func() {}, nil)
	require.NotNil(t, entry)
	t.Cleanup(entry.Release)

	assert.Equal(t, 1, Preview(testChannel).InFlight)
	// The connection has just started, so it is inside every contract and the
	// cleanup button must find nothing to do.
	cleanup := Cleanup(testChannel)
	assert.Zero(t, cleanup.Cancelled)
	assert.False(t, cleanup.PreviewOnly)

	entry.Release()
	assert.Zero(t, Preview(testChannel).InFlight)
}

// The channel table renders one row per channel from a single poll, so the
// all-channel walk must group by channel and classify exactly like the
// single-channel preview does.
func TestRuntimeGroupsEveryChannelInOnePass(t *testing.T) {
	reg := newTestRegistry()
	now := time.Now()

	overdue := reg.register(otherChannel, true, func() {}, nil)
	overdue.startedAt = now.Add(-301 * time.Second)
	healthy := reg.register(testChannel, true, func() {}, nil)
	healthy.startedAt = now.Add(-30 * time.Second)
	second := reg.register(testChannel, false, func() {}, nil)
	second.startedAt = now.Add(-10 * time.Second)
	require.NotNil(t, second)

	runtime := reg.runtime(now, liveThresholds)

	require.Len(t, runtime.Channels, 2)
	assert.Equal(t, testChannel, runtime.Channels[0].ChannelID, "channels must be sorted so polls are stable")
	assert.Equal(t, otherChannel, runtime.Channels[1].ChannelID)
	assert.Equal(t, 2, runtime.Channels[0].InFlight)
	assert.Zero(t, runtime.Channels[0].Cancellable)
	assert.Equal(t, int64(30_000), runtime.Channels[0].OldestAgeMs)
	assert.Equal(t, 1, runtime.Channels[1].Cancellable)
	assert.Equal(t, 1, runtime.Channels[1].ByReason[ReasonWaitingResponseHeader])

	preview, _ := reg.collect(otherChannel, now, liveThresholds)
	assert.Equal(t, preview.ChannelRuntime, runtime.Channels[1], "both walks must agree on one channel")

	assert.Equal(t, liveThresholds.seconds(), runtime.Thresholds)
	assert.Equal(t, liveThresholds.enabled(), runtime.ThresholdEnabled)
	assert.Equal(t, "local_instance", runtime.Scope)
	assert.False(t, runtime.TrackingDegraded)
}

// An idle gateway must answer with an empty list rather than a null, or the table
// has to special-case the shape.
func TestRuntimeReportsIdleAndDegradedHonestly(t *testing.T) {
	reg := newTestRegistry()

	idle := reg.runtime(time.Now(), liveThresholds)
	assert.NotNil(t, idle.Channels)
	assert.Empty(t, idle.Channels)

	reg.degraded = true
	assert.True(t, reg.runtime(time.Now(), liveThresholds).TrackingDegraded)
}

func TestCurrentRuntimeUsesTheGlobalRegistry(t *testing.T) {
	entry := Register(testChannel, true, func() {}, nil)
	require.NotNil(t, entry)
	t.Cleanup(entry.Release)

	runtime := CurrentRuntime()

	var found *ChannelRuntime
	for i := range runtime.Channels {
		if runtime.Channels[i].ChannelID == testChannel {
			found = &runtime.Channels[i]
		}
	}
	require.NotNil(t, found)
	assert.GreaterOrEqual(t, found.InFlight, 1)
	assert.Equal(t, CurrentThresholds().seconds(), runtime.Thresholds)
}
