package model_name_limiter

import (
	"context"
	"sync"
	"time"
)

const (
	memoryEntryTTLMillis      = int64((65 * time.Second) / time.Millisecond)
	memorySweepIntervalMillis = int64((5 * time.Second) / time.Millisecond)
)

type memoryEntry struct {
	hits      []int64
	expiresAt int64
}

type memoryBackend struct {
	mu          sync.Mutex
	entries     map[string]memoryEntry
	nextSweepAt int64
	now         func() time.Time
}

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{entries: make(map[string]memoryEntry)}
}

func (b *memoryBackend) Acquire(_ context.Context, buckets []Bucket) Result {
	if len(buckets) == 0 {
		return Result{Allowed: true}
	}
	if b == nil {
		return Result{Allowed: true}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries == nil {
		b.entries = make(map[string]memoryEntry)
	}

	now := b.nowMillis()
	b.sweepExpiredLocked(now)
	windowStart := now - int64(windowSeconds*1000)
	trimmed := make(map[string]memoryEntry, len(buckets))
	for _, bucket := range buckets {
		entry, ok := trimmed[bucket.Key]
		if !ok {
			entry = b.entries[bucket.Key]
			if now >= entry.expiresAt {
				entry = memoryEntry{}
			}
			entry.hits = trimHits(entry.hits, windowStart)
			trimmed[bucket.Key] = entry
		}
		if bucket.Limit > 0 && len(entry.hits) >= bucket.Limit {
			return Result{
				Allowed: false,
				Scope:   bucket.Scope,
				Limit:   bucket.Limit,
				Current: len(entry.hits),
			}
		}
	}

	// Commit all buckets only after every bucket passed its check.
	for _, bucket := range buckets {
		entry := trimmed[bucket.Key]
		entry.hits = append(entry.hits, now)
		entry.expiresAt = now + memoryEntryTTLMillis
		trimmed[bucket.Key] = entry
	}
	for key, entry := range trimmed {
		b.entries[key] = entry
	}
	return Result{Allowed: true}
}

func (b *memoryBackend) Inspect(_ context.Context, keys []string) ([]int, error) {
	if b == nil {
		return nil, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries == nil {
		b.entries = make(map[string]memoryEntry)
	}
	now := b.nowMillis()
	b.sweepExpiredLocked(now)
	windowStart := now - int64(windowSeconds*1000)
	counts := make([]int, len(keys))
	for i, key := range keys {
		entry, exists := b.entries[key]
		if !exists {
			continue
		}
		if now >= entry.expiresAt {
			delete(b.entries, key)
			continue
		}
		entry.hits = trimHits(entry.hits, windowStart)
		b.entries[key] = entry
		counts[i] = len(entry.hits)
	}
	return counts, nil
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
	now := b.nowMillis()
	entry, exists := b.entries[key]
	if !exists || now >= entry.expiresAt {
		delete(b.entries, key)
		return 0
	}
	entry.hits = trimHits(entry.hits, now-int64(windowSeconds*1000))
	b.entries[key] = entry
	return len(entry.hits)
}

func (b *memoryBackend) sweepExpiredLocked(now int64) {
	if now < b.nextSweepAt {
		return
	}
	b.nextSweepAt = now + memorySweepIntervalMillis
	for key, entry := range b.entries {
		if now >= entry.expiresAt {
			delete(b.entries, key)
			continue
		}
		entry.hits = trimHits(entry.hits, now-int64(windowSeconds*1000))
		b.entries[key] = entry
	}
}

func (b *memoryBackend) nowMillis() int64 {
	if b != nil && b.now != nil {
		return b.now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
