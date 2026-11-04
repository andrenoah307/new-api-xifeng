package captcha

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var ErrUnavailable = errors.New("captcha storage unavailable")

type Store interface {
	Put(context.Context, string, string, time.Duration) error
	Get(context.Context, string) (string, error)
	Consume(context.Context, string, string) (bool, error)
}
type record struct {
	value   string
	expires time.Time
}
type MemoryStore struct {
	mu       sync.Mutex
	entries  map[string]record
	capacity int
}

func NewMemoryStore(capacity int) *MemoryStore {
	return &MemoryStore{entries: make(map[string]record), capacity: capacity}
}
func (s *MemoryStore) Put(_ context.Context, key, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.entries {
		if !v.expires.After(now) {
			delete(s.entries, k)
		}
	}
	if len(s.entries) >= s.capacity {
		return ErrUnavailable
	}
	if _, exists := s.entries[key]; exists {
		return ErrUnavailable
	}
	s.entries[key] = record{value, now.Add(ttl)}
	return nil
}
func (s *MemoryStore) Consume(_ context.Context, key, value string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.entries[key]
	if !ok {
		return false, nil
	}
	if !r.expires.After(time.Now()) {
		delete(s.entries, key)
		return false, nil
	}
	if r.value != value {
		return false, nil
	}
	delete(s.entries, key)
	return true, nil
}
func (s *MemoryStore) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.entries[key]
	if !ok {
		return "", nil
	}
	if !r.expires.After(time.Now()) {
		delete(s.entries, key)
		return "", nil
	}
	return r.value, nil
}

type RedisStore struct{}

func (RedisStore) Get(ctx context.Context, key string) (string, error) {
	if common.RDB == nil {
		return "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := common.RDB.Eval(ctx, `return redis.call('GET',KEYS[1]) or ''`, []string{"captcha:{proof}:" + key}).Text()
	if err != nil {
		return "", ErrUnavailable
	}
	return result, nil
}

// Both the expiry index and values share a Redis Cluster hash slot. Admission
// and consumption are atomic; outages never fall back to process-local state.
func (RedisStore) Put(ctx context.Context, key, value string, ttl time.Duration) error {
	if common.RDB == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := common.RDB.Eval(ctx, `local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
 redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',now)
 if redis.call('ZCARD',KEYS[1])>=10000 then return 0 end
 if not redis.call('SET',KEYS[2],ARGV[1],'PX',ARGV[2],'NX') then return 0 end
 redis.call('ZADD',KEYS[1],now+ARGV[2],KEYS[2]); redis.call('PEXPIRE',KEYS[1],ARGV[2]); return 1`, []string{"captcha:{proof}:index", "captcha:{proof}:" + key}, value, ttl.Milliseconds()).Int()
	if err != nil || result != 1 {
		return ErrUnavailable
	}
	return nil
}
func (RedisStore) Consume(ctx context.Context, key, value string) (bool, error) {
	if common.RDB == nil {
		return false, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := common.RDB.Eval(ctx, `if redis.call('GET',KEYS[2])~=ARGV[1] then return 0 end
 redis.call('DEL',KEYS[2]); redis.call('ZREM',KEYS[1],KEYS[2]); return 1`, []string{"captcha:{proof}:index", "captcha:{proof}:" + key}, value).Int()
	if err != nil {
		return false, ErrUnavailable
	}
	return result == 1, nil
}

var localStore = NewMemoryStore(10000)

func ActiveStore() Store {
	if common.RedisEnabled {
		return RedisStore{}
	}
	return localStore
}
