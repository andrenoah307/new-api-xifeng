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

// Package inflight tracks upstream requests that are currently in flight, so an
// administrator can see per-channel load and clear connections that have already
// outlived the gateway's own timeout contract.
//
// The registry never creates a cancellation handle of its own. A connection is
// cancellable only when the relay path already built one for it: the
// context.WithCancel behind the stream response-header timer, the
// context.WithTimeout behind the non-stream deadline, or the response body once
// headers arrived. A timeout configured as 0 means the operator deliberately
// turned that class off, so no handle exists and the entry is structurally not
// cancellable — "no contract, no overrun".
package inflight

import (
	"context"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

// Reasons a connection may be cleared. They mirror the gateway's own timeout
// contracts one for one; there is no reason that exists only for the admin path.
const (
	ReasonWaitingResponseHeader = "waiting_response_header"
	ReasonWaitingFirstChunk     = "waiting_first_chunk"
	ReasonStalledAfterChunk     = "stalled_after_chunk"
	ReasonNonStreamTimeout      = "non_stream_timeout"
)

// maxTrackedEntries bounds the registry at roughly twice the default global
// admission limit (RELAY_MAX_CONCURRENT_REQUESTS). Past it, registration is
// skipped and the snapshot is flagged degraded: an observability facility must
// never become a failure point on the request path.
const maxTrackedEntries = 20000

// Entry is the handle a relay request holds for the duration of its upstream
// call. Every method tolerates a nil receiver so callers never need a nil check.
type Entry struct {
	id        uint64
	reg       *registry
	channelID int
	isStream  bool
	startedAt time.Time

	// Stamps are unix nanoseconds, 0 meaning "not yet". They are atomic rather
	// than mutex-guarded because MarkChunk runs once per streamed chunk.
	headersAt  atomic.Int64
	firstChunk atomic.Int64
	lastChunk  atomic.Int64

	mu       sync.Mutex
	cancel   context.CancelFunc
	body     io.Closer
	killed   bool
	released bool
	onKill   func()
}

type registry struct {
	mu       sync.RWMutex
	nextID   uint64
	entries  map[uint64]*Entry
	degraded bool
}

var global = &registry{entries: make(map[uint64]*Entry)}

// Register starts tracking one upstream request. cancel may be nil when no
// timeout is configured for this request class; onKill is invoked before the
// connection is torn down so the relay loop can tell an admin kill apart from an
// upstream failure. A nil return means tracking is degraded, which never affects
// the request itself.
func Register(channelID int, isStream bool, cancel context.CancelFunc, onKill func()) *Entry {
	return global.register(channelID, isStream, cancel, onKill)
}

func (r *registry) register(channelID int, isStream bool, cancel context.CancelFunc, onKill func()) *Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) >= maxTrackedEntries {
		r.degraded = true
		return nil
	}
	r.nextID++
	entry := &Entry{
		id:        r.nextID,
		reg:       r,
		channelID: channelID,
		isStream:  isStream,
		startedAt: time.Now(),
		cancel:    cancel,
		onKill:    onKill,
	}
	r.entries[entry.id] = entry
	return entry
}

// MarkHeadersReceived records that upstream response headers arrived and hands
// the registry the body closer, which becomes the kill handle for every stage
// after the headers.
func (e *Entry) MarkHeadersReceived(body io.Closer) {
	if e == nil {
		return
	}
	e.headersAt.CompareAndSwap(0, time.Now().UnixNano())
	e.mu.Lock()
	e.body = body
	e.mu.Unlock()
}

// MarkChunk records an upstream payload. It runs on the streaming hot path and
// therefore takes no lock.
func (e *Entry) MarkChunk() {
	if e == nil {
		return
	}
	now := time.Now().UnixNano()
	e.firstChunk.CompareAndSwap(0, now)
	e.lastChunk.Store(now)
}

// Release stops tracking the request. It is bound to the response body's Close
// rather than to the end of the relay call, because a streamed body is consumed
// long after the call that produced it returned.
func (e *Entry) Release() {
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.released {
		e.mu.Unlock()
		return
	}
	e.released = true
	e.mu.Unlock()

	e.reg.mu.Lock()
	delete(e.reg.entries, e.id)
	if len(e.reg.entries) == 0 {
		// Nothing dropped is still running, so the counts are trustworthy again.
		e.reg.degraded = false
	}
	e.reg.mu.Unlock()
}

// Thresholds are the gateway's configured timeout contracts. A zero value means
// the operator turned that class off.
type Thresholds struct {
	StreamResponseHeader time.Duration
	Streaming            time.Duration
	NonStream            time.Duration
}

// CurrentThresholds reads the live configuration. These are the very values the
// stream header timer, the stream scanner ticker and the non-stream deadline use.
func CurrentThresholds() Thresholds {
	return Thresholds{
		StreamResponseHeader: time.Duration(common.RelayStreamResponseHeaderTimeout) * time.Second,
		Streaming:            time.Duration(constant.StreamingTimeout) * time.Second,
		NonStream:            time.Duration(common.RelayNonStreamTimeout) * time.Second,
	}
}

type ThresholdSeconds struct {
	StreamResponseHeader int `json:"stream_response_header_s"`
	Streaming            int `json:"streaming_s"`
	NonStream            int `json:"non_stream_s"`
}

type ThresholdEnabled struct {
	StreamResponseHeader bool `json:"stream_response_header"`
	Streaming            bool `json:"streaming"`
	NonStream            bool `json:"non_stream"`
}

// scopeLocalInstance tells the console that the registry only ever sees the
// connections of the process answering the request.
const scopeLocalInstance = "local_instance"

func (t Thresholds) seconds() ThresholdSeconds {
	return ThresholdSeconds{
		StreamResponseHeader: int(t.StreamResponseHeader / time.Second),
		Streaming:            int(t.Streaming / time.Second),
		NonStream:            int(t.NonStream / time.Second),
	}
}

func (t Thresholds) enabled() ThresholdEnabled {
	return ThresholdEnabled{
		StreamResponseHeader: t.StreamResponseHeader > 0,
		Streaming:            t.Streaming > 0,
		NonStream:            t.NonStream > 0,
	}
}

// ChannelRuntime is what one channel currently has in flight. It is the unit the
// admin table renders per row and the body of the single-channel snapshot.
type ChannelRuntime struct {
	ChannelID          int            `json:"channel_id"`
	InFlight           int            `json:"in_flight"`
	AwaitingHeaders    int            `json:"awaiting_headers"`
	AwaitingFirstChunk int            `json:"awaiting_first_chunk"`
	OldestAgeMs        int64          `json:"oldest_age_ms"`
	Cancellable        int            `json:"cancellable"`
	ByReason           map[string]int `json:"by_reason"`
}

func newChannelRuntime(channelID int) ChannelRuntime {
	return ChannelRuntime{
		ChannelID: channelID,
		ByReason: map[string]int{
			ReasonWaitingResponseHeader: 0,
			ReasonWaitingFirstChunk:     0,
			ReasonStalledAfterChunk:     0,
			ReasonNonStreamTimeout:      0,
		},
	}
}

// observe folds one entry into the counters and reports which timeout contract it
// broke, so the per-channel and the all-channel walks classify identically.
func (cr *ChannelRuntime) observe(entry *Entry, now time.Time, thresholds Thresholds) string {
	cr.InFlight++
	switch {
	case entry.headersAt.Load() == 0:
		cr.AwaitingHeaders++
	case entry.firstChunk.Load() == 0:
		cr.AwaitingFirstChunk++
	}
	if age := now.Sub(entry.startedAt).Milliseconds(); age > cr.OldestAgeMs {
		cr.OldestAgeMs = age
	}
	reason := entry.overdueReason(now, thresholds)
	if reason == "" {
		return ""
	}
	cr.Cancellable++
	cr.ByReason[reason]++
	return reason
}

// Snapshot is the wire shape returned by both the preview and the cleanup endpoint.
type Snapshot struct {
	ChannelRuntime
	Cancelled        int              `json:"cancelled"`
	Thresholds       ThresholdSeconds `json:"thresholds"`
	ThresholdEnabled ThresholdEnabled `json:"threshold_enabled"`
	Scope            string           `json:"scope"`
	TrackingDegraded bool             `json:"tracking_degraded"`
	PreviewOnly      bool             `json:"preview_only"`
}

// Runtime is the all-channel shape the admin channel table polls. It exists so the
// table costs one request per refresh instead of one per visible row.
type Runtime struct {
	Channels         []ChannelRuntime `json:"channels"`
	Thresholds       ThresholdSeconds `json:"thresholds"`
	ThresholdEnabled ThresholdEnabled `json:"threshold_enabled"`
	Scope            string           `json:"scope"`
	TrackingDegraded bool             `json:"tracking_degraded"`
}

// Preview reports the current state of one channel without touching any connection.
func Preview(channelID int) Snapshot {
	snapshot, _ := global.collect(channelID, time.Now(), CurrentThresholds())
	return snapshot
}

// Cleanup tears down every connection of one channel that has outlived its own
// timeout contract, and reports what it did.
func Cleanup(channelID int) Snapshot {
	return global.cleanup(channelID, time.Now(), CurrentThresholds())
}

func (r *registry) cleanup(channelID int, now time.Time, thresholds Thresholds) Snapshot {
	snapshot, victims := r.collect(channelID, now, thresholds)
	snapshot.PreviewOnly = false
	for _, victim := range victims {
		if victim.kill() {
			snapshot.Cancelled++
		}
	}
	return snapshot
}

// collect walks the registry once, producing both the counts and the kill list so
// preview and cleanup classify connections through identical code. Connections are
// never torn down while the registry lock is held.
func (r *registry) collect(channelID int, now time.Time, thresholds Thresholds) (Snapshot, []*Entry) {
	snapshot := Snapshot{
		ChannelRuntime:   newChannelRuntime(channelID),
		Thresholds:       thresholds.seconds(),
		ThresholdEnabled: thresholds.enabled(),
		Scope:            scopeLocalInstance,
		PreviewOnly:      true,
	}

	var victims []*Entry
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot.TrackingDegraded = r.degraded
	for _, entry := range r.entries {
		if entry.channelID != channelID {
			continue
		}
		if snapshot.observe(entry, now, thresholds) != "" {
			victims = append(victims, entry)
		}
	}
	return snapshot, victims
}

// CurrentRuntime reports every channel that has something in flight right now.
// Channels absent from the result simply have nothing running.
func CurrentRuntime() Runtime {
	return global.runtime(time.Now(), CurrentThresholds())
}

func (r *registry) runtime(now time.Time, thresholds Thresholds) Runtime {
	result := Runtime{
		Channels:         []ChannelRuntime{},
		Thresholds:       thresholds.seconds(),
		ThresholdEnabled: thresholds.enabled(),
		Scope:            scopeLocalInstance,
	}

	byChannel := make(map[int]*ChannelRuntime)
	r.mu.RLock()
	result.TrackingDegraded = r.degraded
	for _, entry := range r.entries {
		channel, ok := byChannel[entry.channelID]
		if !ok {
			created := newChannelRuntime(entry.channelID)
			channel = &created
			byChannel[entry.channelID] = channel
		}
		channel.observe(entry, now, thresholds)
	}
	r.mu.RUnlock()

	for _, channel := range byChannel {
		result.Channels = append(result.Channels, *channel)
	}
	// Sorted so the response is stable across polls and assertable in tests.
	sort.Slice(result.Channels, func(i, j int) bool {
		return result.Channels[i].ChannelID < result.Channels[j].ChannelID
	})
	return result
}

// overdueReason names the timeout contract this connection has already broken, or
// "" when it is healthy. A connection is only reported when a kill handle exists
// for it: an entry we cannot tear down must never be counted as cancellable.
func (e *Entry) overdueReason(now time.Time, thresholds Thresholds) string {
	e.mu.Lock()
	killable := !e.killed && !e.released
	hasCancel := e.cancel != nil
	hasBody := e.body != nil
	e.mu.Unlock()
	if !killable {
		return ""
	}

	headersAt := e.headersAt.Load()
	if !e.isStream {
		if hasCancel && thresholds.NonStream > 0 && now.Sub(e.startedAt) > thresholds.NonStream {
			return ReasonNonStreamTimeout
		}
		return ""
	}
	if headersAt == 0 {
		if hasCancel && thresholds.StreamResponseHeader > 0 && now.Sub(e.startedAt) > thresholds.StreamResponseHeader {
			return ReasonWaitingResponseHeader
		}
		return ""
	}
	if !hasBody || thresholds.Streaming <= 0 {
		return ""
	}
	if firstChunk := e.firstChunk.Load(); firstChunk == 0 {
		if now.Sub(time.Unix(0, headersAt)) > thresholds.Streaming {
			return ReasonWaitingFirstChunk
		}
		return ""
	}
	if now.Sub(time.Unix(0, e.lastChunk.Load())) > thresholds.Streaming {
		return ReasonStalledAfterChunk
	}
	return ""
}

// kill pulls this connection's existing timeout forward: cancelling the context or
// closing the body lands on exactly the code paths the timer and the scanner ticker
// already use, which is why no billing or refund handling is needed here.
func (e *Entry) kill() bool {
	e.mu.Lock()
	if e.killed || e.released {
		e.mu.Unlock()
		return false
	}
	e.killed = true
	cancel, body, onKill := e.cancel, e.body, e.onKill
	e.mu.Unlock()

	// Marked before tear-down so the relay loop sees the flag no matter how fast
	// the upstream read unblocks.
	if onKill != nil {
		onKill()
	}
	if cancel != nil {
		cancel()
	}
	if body != nil {
		_ = body.Close()
	}
	return true
}
