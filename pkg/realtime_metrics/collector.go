// Package realtimemetrics keeps a live, cross-instance view of relay load for the
// admin console.
//
// The request path only touches process-local atomics; a background loop batches
// them into Redis every flushInterval. Per-request Redis writes were rejected on
// purpose — at production peak that is several hundred round trips per second on
// the relay hot path, for numbers nobody reads more than once every ten seconds.
//
// Counters are cross-instance sums, so every Redis counter write is HINCRBY, never
// HSET (pitfall #128: HSET from a second instance silently drops the first one's
// delta). Gauges are per-instance keys, which is why they can be plain HSET.
package realtimemetrics

import (
	"sync"
	"sync/atomic"
)

// deltas are the counters accumulated since the last successful flush.
type deltas struct {
	requests         atomic.Int64
	success          atomic.Int64
	errors           atomic.Int64
	clientGone       atomic.Int64
	promptTokens     atomic.Int64
	completionTokens atomic.Int64
	quota            atomic.Int64
	rejGate          atomic.Int64
	rejConcurrency   atomic.Int64
	rejBody          atomic.Int64
	rejMemory        atomic.Int64
	rejModelRPM      atomic.Int64
	rejUserRPM       atomic.Int64
}

var (
	globalDeltas deltas
	// channelConcurrency holds one live counter per channel id. Entries are never
	// deleted: the id space is the channel table, so this is bounded by
	// configuration rather than by traffic, and deleting a counter that a
	// concurrent Release still holds a pointer to would lose the decrement.
	channelConcurrency sync.Map // int -> *atomic.Int64
	// channelDeltas holds per-channel request/error counts since the last flush.
	channelDeltas sync.Map // int -> *channelCounters
)

type channelCounters struct {
	requests atomic.Int64
	errors   atomic.Int64
}

// RecordRelayOutcome records one finished client request. It is called once per
// request at the end of the relay handler, after every retry has been exhausted,
// so it counts client-visible outcomes rather than upstream attempts.
func RecordRelayOutcome(success bool, clientGone bool) {
	globalDeltas.requests.Add(1)
	if success {
		globalDeltas.success.Add(1)
	} else {
		globalDeltas.errors.Add(1)
	}
	if clientGone {
		globalDeltas.clientGone.Add(1)
	}
}

// RecordUsage records billed usage. It is hooked at the consume-log write so
// every relay format and task platform is covered by one call site, and the
// numbers match what the log page shows.
func RecordUsage(promptTokens int64, completionTokens int64, quota int64) {
	if promptTokens > 0 {
		globalDeltas.promptTokens.Add(promptTokens)
	}
	if completionTokens > 0 {
		globalDeltas.completionTokens.Add(completionTokens)
	}
	if quota > 0 {
		globalDeltas.quota.Add(quota)
	}
}

// RecordRejection records a request refused by a pre-relay gate. An unknown kind
// is dropped rather than silently folded into another counter.
func RecordRejection(kind string) {
	switch kind {
	case RejectionGate:
		globalDeltas.rejGate.Add(1)
	case RejectionConcurrency:
		globalDeltas.rejConcurrency.Add(1)
	case RejectionBody:
		globalDeltas.rejBody.Add(1)
	case RejectionMemory:
		globalDeltas.rejMemory.Add(1)
	case RejectionModelRPM:
		globalDeltas.rejModelRPM.Add(1)
	case RejectionUserRPM:
		globalDeltas.rejUserRPM.Add(1)
	}
}

// ChannelAttempt marks the start of one upstream attempt on a channel. The
// returned function must be called exactly once when the attempt finishes; it is
// idempotent so a defer plus an explicit call cannot double-decrement.
//
// This is deliberately not gated on the channel rate limiter being configured:
// the point of the dashboard is to show load on channels that have no limit set,
// which is where an operator most needs to decide whether to add one.
func ChannelAttempt(channelID int) func() {
	if channelID <= 0 {
		return func() {}
	}
	gauge := channelGauge(channelID)
	gauge.Add(1)
	counters := channelCounter(channelID)
	counters.requests.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() { gauge.Add(-1) })
	}
}

// RecordChannelError attributes one failed attempt to a channel.
func RecordChannelError(channelID int) {
	if channelID <= 0 {
		return
	}
	channelCounter(channelID).errors.Add(1)
}

func channelGauge(channelID int) *atomic.Int64 {
	if value, ok := channelConcurrency.Load(channelID); ok {
		return value.(*atomic.Int64)
	}
	actual, _ := channelConcurrency.LoadOrStore(channelID, &atomic.Int64{})
	return actual.(*atomic.Int64)
}

func channelCounter(channelID int) *channelCounters {
	if value, ok := channelDeltas.Load(channelID); ok {
		return value.(*channelCounters)
	}
	actual, _ := channelDeltas.LoadOrStore(channelID, &channelCounters{})
	return actual.(*channelCounters)
}

// drainedCounters is one flush window's worth of counters, in the field order the
// Redis hash uses.
type drainedCounters map[string]int64

func (d *deltas) drain() drainedCounters {
	return drainedCounters{
		"requests":           d.requests.Swap(0),
		"success":            d.success.Swap(0),
		"errors":             d.errors.Swap(0),
		"client_gone":        d.clientGone.Swap(0),
		"prompt_tokens":      d.promptTokens.Swap(0),
		"completion_tokens":  d.completionTokens.Swap(0),
		"quota":              d.quota.Swap(0),
		RejectionGate:        d.rejGate.Swap(0),
		RejectionConcurrency: d.rejConcurrency.Swap(0),
		RejectionBody:        d.rejBody.Swap(0),
		RejectionMemory:      d.rejMemory.Swap(0),
		RejectionModelRPM:    d.rejModelRPM.Swap(0),
		RejectionUserRPM:     d.rejUserRPM.Swap(0),
	}
}

// restore puts a failed flush's counters back so the next flush retries them.
// Without it a Redis blip would silently erase a window of traffic from the
// dashboard, which is exactly when an operator is most likely to be looking.
func (d *deltas) restore(drained drainedCounters) {
	add := func(counter *atomic.Int64, field string) {
		if value := drained[field]; value != 0 {
			counter.Add(value)
		}
	}
	add(&d.requests, "requests")
	add(&d.success, "success")
	add(&d.errors, "errors")
	add(&d.clientGone, "client_gone")
	add(&d.promptTokens, "prompt_tokens")
	add(&d.completionTokens, "completion_tokens")
	add(&d.quota, "quota")
	add(&d.rejGate, RejectionGate)
	add(&d.rejConcurrency, RejectionConcurrency)
	add(&d.rejBody, RejectionBody)
	add(&d.rejMemory, RejectionMemory)
	add(&d.rejModelRPM, RejectionModelRPM)
	add(&d.rejUserRPM, RejectionUserRPM)
}

type channelDelta struct {
	channelID int
	requests  int64
	errors    int64
}

func drainChannelDeltas() []channelDelta {
	var drained []channelDelta
	channelDeltas.Range(func(key, value any) bool {
		counters := value.(*channelCounters)
		requests := counters.requests.Swap(0)
		errors := counters.errors.Swap(0)
		if requests == 0 && errors == 0 {
			return true
		}
		drained = append(drained, channelDelta{channelID: key.(int), requests: requests, errors: errors})
		return true
	})
	return drained
}

func restoreChannelDeltas(drained []channelDelta) {
	for _, item := range drained {
		counters := channelCounter(item.channelID)
		counters.requests.Add(item.requests)
		counters.errors.Add(item.errors)
	}
}

func snapshotChannelConcurrency() map[int]int64 {
	out := map[int]int64{}
	channelConcurrency.Range(func(key, value any) bool {
		if current := value.(*atomic.Int64).Load(); current > 0 {
			out[key.(int)] = current
		}
		return true
	})
	return out
}
