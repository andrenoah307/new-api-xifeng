package user_model_rpm

import (
	"context"
	"strings"
	"sync"
	"time"
)

type memoryEvent struct {
	score  int64
	member string
}

type memoryUser struct {
	events    map[string]memoryEvent
	expiresAt int64
}

type memoryBackend struct {
	mu        sync.Mutex
	users     map[int]*memoryUser
	now       func() time.Time
	scanLimit int
}

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{
		users:     make(map[int]*memoryUser),
		scanLimit: maxScan,
	}
}

func (b *memoryBackend) IsMemory() bool { return true }

func (b *memoryBackend) Record(_ context.Context, userID int, requestID, model string) error {
	if b == nil || requestID == "" || model == "" {
		return nil
	}
	now := b.nowMillis()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.users == nil {
		b.users = make(map[int]*memoryUser)
	}
	user := b.users[userID]
	if user == nil || now >= user.expiresAt {
		user = &memoryUser{events: make(map[string]memoryEvent)}
		b.users[userID] = user
	}
	b.trimUserLocked(user, now)
	member := memberFor(requestID, model)
	if _, exists := user.events[member]; !exists {
		user.events[member] = memoryEvent{score: now, member: member}
		user.expiresAt = now + ttlMillis
	}
	if len(b.users) > memorySweepThreshold {
		b.sweepExpiredLocked(now)
	}
	return nil
}

func (b *memoryBackend) Inspect(_ context.Context, userID int) ([]ModelRPM, string, error) {
	if b == nil {
		return []ModelRPM{}, "unavailable", nil
	}
	now := b.nowMillis()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.users == nil {
		b.users = make(map[int]*memoryUser)
	}
	user := b.users[userID]
	if user == nil || now >= user.expiresAt {
		if user != nil {
			delete(b.users, userID)
		}
		return []ModelRPM{}, "empty", nil
	}
	b.trimUserLocked(user, now)
	if len(user.events) == 0 {
		delete(b.users, userID)
		return []ModelRPM{}, "empty", nil
	}
	scanLimit := b.scanLimit
	if scanLimit <= 0 {
		scanLimit = maxScan
	}
	if len(user.events) > scanLimit {
		if len(b.users) > memorySweepThreshold {
			b.sweepExpiredLocked(now)
		}
		return []ModelRPM{}, "overflow", nil
	}
	counts := make(map[string]int)
	for _, event := range user.events {
		if event.score > now {
			continue
		}
		separator := strings.Index(event.member, memberSeparator)
		if separator < 0 {
			continue
		}
		model := event.member[separator+1:]
		if model != "" {
			counts[model]++
		}
	}
	items := make([]ModelRPM, 0, len(counts))
	for model, rpm := range counts {
		items = append(items, ModelRPM{Model: model, RPM: rpm})
	}
	if len(items) == 0 {
		return []ModelRPM{}, "empty", nil
	}
	if len(b.users) > memorySweepThreshold {
		b.sweepExpiredLocked(now)
	}
	SortByRPM(items)
	return items, "available", nil
}

func (b *memoryBackend) trimUserLocked(user *memoryUser, now int64) {
	if user == nil {
		return
	}
	for member, event := range user.events {
		if event.score <= now-windowMillis {
			delete(user.events, member)
		}
	}
}

func (b *memoryBackend) sweepExpiredLocked(now int64) {
	for userID, user := range b.users {
		if user == nil || now >= user.expiresAt {
			delete(b.users, userID)
			continue
		}
		b.trimUserLocked(user, now)
		if len(user.events) == 0 {
			delete(b.users, userID)
		}
	}
}

func (b *memoryBackend) nowMillis() int64 {
	if b != nil && b.now != nil {
		return b.now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
