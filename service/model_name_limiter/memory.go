package model_name_limiter

import (
	"context"
	"sync"
	"time"
)

// memoryBackend is deliberately unbounded in key count.  Entries are kept for
// the process lifetime, while old timestamps are trimmed whenever a key is
// touched, matching the channel limiter's v1 memory behaviour.
type memoryBackend struct {
	mu      sync.Mutex
	entries map[string][]int64
}

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{entries: make(map[string][]int64)}
}

func (b *memoryBackend) Acquire(_ context.Context, keys []string, limits []int) Result {
	if len(keys) == 0 {
		return Result{Allowed: true}
	}
	if b == nil || len(keys) != len(limits) || (len(keys) != 1 && len(keys) != 2) {
		return Result{Allowed: true}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries == nil {
		b.entries = make(map[string][]int64)
	}

	now := time.Now().UnixMilli()
	windowStart := now - int64(windowSeconds*1000)
	trimmed := make(map[string][]int64, len(keys))
	for i, key := range keys {
		hits, ok := trimmed[key]
		if !ok {
			hits = trimHits(b.entries[key], windowStart)
			trimmed[key] = hits
		}
		if len(hits) >= limits[i] {
			return Result{
				Allowed: false,
				Scope:   scopeForIndex(i + 1),
				Limit:   limits[i],
				Current: len(hits),
			}
		}
	}

	// Commit all buckets only after every bucket passed its check.  The lock
	// also makes concurrent two-key acquires linearizable within this process.
	for _, key := range keys {
		trimmed[key] = append(trimmed[key], now)
	}
	for key, hits := range trimmed {
		b.entries[key] = hits
	}
	return Result{Allowed: true}
}

func trimHits(hits []int64, windowStart int64) []int64 {
	idx := 0
	for idx < len(hits) && hits[idx] <= windowStart {
		idx++
	}
	if idx == 0 {
		return hits
	}
	if idx == len(hits) {
		return hits[:0]
	}
	return hits[idx:]
}

// count is useful for package-local tests and keeps direct counter assertions
// independent of the representation used by callers.
func (b *memoryBackend) count(key string) int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries == nil {
		return 0
	}
	hits := trimHits(b.entries[key], time.Now().UnixMilli()-int64(windowSeconds*1000))
	b.entries[key] = hits
	return len(hits)
}
