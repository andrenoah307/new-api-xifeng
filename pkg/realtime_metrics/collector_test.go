package realtimemetrics

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetCollector clears the package-level accumulators so one test cannot see
// another's counts. The collector deliberately exposes no reset in production
// code — draining is the only way state leaves it, and that is what a flush does.
func resetCollector(t *testing.T) {
	t.Helper()
	globalDeltas.drain()
	drainChannelDeltas()
}

func TestDrainZeroesCountersAndUsesRedisFieldNames(t *testing.T) {
	resetCollector(t)

	RecordRelayOutcome(true, false)
	RecordRelayOutcome(false, true)
	RecordUsage(11, 22, 33)
	RecordRejection(RejectionGate)
	RecordRejection(RejectionConcurrency)
	RecordRejection(RejectionBody)
	RecordRejection(RejectionMemory)
	RecordRejection(RejectionModelRPM)
	RecordRejection(RejectionUserRPM)

	drained := globalDeltas.drain()

	assert.Equal(t, drainedCounters{
		"requests":           2,
		"success":            1,
		"errors":             1,
		"client_gone":        1,
		"prompt_tokens":      11,
		"completion_tokens":  22,
		"quota":              33,
		RejectionGate:        1,
		RejectionConcurrency: 1,
		RejectionBody:        1,
		RejectionMemory:      1,
		RejectionModelRPM:    1,
		RejectionUserRPM:     1,
	}, drained)

	// A second drain must be empty: a flush that succeeded may not resend the
	// same window, or every cross-instance counter would double.
	assert.Equal(t, drainedCounters{
		"requests":           0,
		"success":            0,
		"errors":             0,
		"client_gone":        0,
		"prompt_tokens":      0,
		"completion_tokens":  0,
		"quota":              0,
		RejectionGate:        0,
		RejectionConcurrency: 0,
		RejectionBody:        0,
		RejectionMemory:      0,
		RejectionModelRPM:    0,
		RejectionUserRPM:     0,
	}, globalDeltas.drain())
}

func TestRestoreReplaysAFailedFlushWindow(t *testing.T) {
	resetCollector(t)

	RecordRelayOutcome(true, false)
	RecordRelayOutcome(true, false)
	RecordUsage(5, 7, 9)
	RecordRejection(RejectionMemory)

	first := globalDeltas.drain()
	globalDeltas.restore(first)

	assert.Equal(t, first, globalDeltas.drain(),
		"a Redis blip must not erase a window of traffic from the dashboard")
}

func TestRecordRelayOutcomeAttribution(t *testing.T) {
	cases := []struct {
		name        string
		success     bool
		clientGone  bool
		success1    int64
		errors1     int64
		clientGone1 int64
	}{
		{name: "success", success: true, success1: 1},
		{name: "failure", success: false, errors1: 1},
		{name: "client gone but relay succeeded", success: true, clientGone: true, success1: 1, clientGone1: 1},
		{name: "client gone and relay failed", success: false, clientGone: true, errors1: 1, clientGone1: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetCollector(t)
			RecordRelayOutcome(tc.success, tc.clientGone)
			drained := globalDeltas.drain()

			assert.Equal(t, int64(1), drained["requests"])
			assert.Equal(t, tc.success1, drained["success"])
			assert.Equal(t, tc.errors1, drained["errors"])
			assert.Equal(t, tc.clientGone1, drained["client_gone"])
		})
	}
}

func TestRecordUsageIgnoresNonPositiveValues(t *testing.T) {
	resetCollector(t)

	// An upstream that reports a negative or zero usage must not be able to drag
	// the dashboard's totals backwards.
	RecordUsage(-100, 0, -1)
	RecordUsage(0, -5, 0)

	drained := globalDeltas.drain()
	assert.Zero(t, drained["prompt_tokens"])
	assert.Zero(t, drained["completion_tokens"])
	assert.Zero(t, drained["quota"])
}

func TestRecordRejectionRoutesKnownKindsAndDropsUnknown(t *testing.T) {
	for _, kind := range []string{
		RejectionGate, RejectionConcurrency, RejectionBody,
		RejectionMemory, RejectionModelRPM, RejectionUserRPM,
	} {
		t.Run(kind, func(t *testing.T) {
			resetCollector(t)
			RecordRejection(kind)

			drained := globalDeltas.drain()
			assert.Equal(t, int64(1), drained[kind])

			var total int64
			for _, value := range drained {
				total += value
			}
			assert.Equal(t, int64(1), total, "a rejection must land in exactly one bucket")
		})
	}

	t.Run("unknown kind is dropped", func(t *testing.T) {
		resetCollector(t)
		RecordRejection("rej_not_a_real_gate")

		var total int64
		for _, value := range globalDeltas.drain() {
			total += value
		}
		assert.Zero(t, total, "an unknown kind must not be folded into another counter")
	})
}

func TestChannelAttemptReleaseIsIdempotent(t *testing.T) {
	resetCollector(t)
	const channelID = 90001

	release := ChannelAttempt(channelID)
	assert.Equal(t, int64(1), channelGauge(channelID).Load())

	// A defer plus an explicit call is the shape that would double-decrement and
	// leave the gauge permanently negative.
	release()
	release()
	assert.Zero(t, channelGauge(channelID).Load())

	drained := drainChannelDeltas()
	require.Len(t, drained, 1)
	assert.Equal(t, channelDelta{channelID: channelID, requests: 1}, drained[0])
}

func TestChannelHooksIgnoreInvalidChannelID(t *testing.T) {
	resetCollector(t)

	for _, channelID := range []int{0, -1} {
		release := ChannelAttempt(channelID)
		release()
		RecordChannelError(channelID)
	}

	assert.Empty(t, drainChannelDeltas(), "an unresolved channel must not create a counter")
	assert.NotContains(t, snapshotChannelConcurrency(), 0)
}

func TestDrainChannelDeltasSkipsIdleChannelsAndRestoreReplays(t *testing.T) {
	resetCollector(t)
	const busy = 90002
	const idle = 90003

	// Touch the idle channel so it has a counter entry but no traffic this window.
	ChannelAttempt(idle)()
	drainChannelDeltas()

	ChannelAttempt(busy)()
	RecordChannelError(busy)

	drained := drainChannelDeltas()
	require.Len(t, drained, 1, "a channel with no traffic this window must not be written to Redis")
	assert.Equal(t, channelDelta{channelID: busy, requests: 1, errors: 1}, drained[0])

	restoreChannelDeltas(drained)
	assert.Equal(t, drained, drainChannelDeltas())
}

func TestSnapshotChannelConcurrencyOnlyReportsActiveChannels(t *testing.T) {
	resetCollector(t)
	const active = 90004
	const finished = 90005

	holdOpen := ChannelAttempt(active)
	defer holdOpen()
	ChannelAttempt(finished)()

	snapshot := snapshotChannelConcurrency()
	assert.Equal(t, int64(1), snapshot[active])
	assert.NotContains(t, snapshot, finished, "a finished attempt must not linger as live concurrency")
}

func TestChannelGaugeIsStableUnderConcurrentAttempts(t *testing.T) {
	resetCollector(t)
	const channelID = 90006
	const attempts = 64

	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			ChannelAttempt(channelID)()
		}()
	}
	wg.Wait()

	assert.Zero(t, channelGauge(channelID).Load(), "every attempt released, so the gauge must be back at zero")

	drained := drainChannelDeltas()
	require.Len(t, drained, 1)
	assert.Equal(t, int64(attempts), drained[0].requests)
}
