// Package model_name_limiter provides an atomic sliding-window RPM limiter for
// arbitrary string keys.  The package deliberately does not know anything
// about models or groups; callers choose the keys and their limits.
package model_name_limiter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	rpmWindowSeconds        = 60
	windowSeconds           = rpmWindowSeconds
	backendOperationTimeout = 200 * time.Millisecond
)

// Result is the outcome of an acquire attempt.  Scope, Limit, and Current are
// populated only when a request is rejected by a limit.
type Result struct {
	Allowed bool
	Scope   string // "global" or "group"
	Limit   int
	Current int
}

type backend interface {
	Acquire(context.Context, []string, []int) Result
	Inspect(context.Context, []string) ([]int, error)
}

var (
	backendOnce sync.Once
	backendImpl backend
)

// InitModelNameLimiter selects and initializes the model-name RPM backend.
// Its state is intentionally independent from service/channel_limiter.
func InitModelNameLimiter() {
	backendOnce.Do(func() {
		if common.RedisEnabled && common.RDB != nil {
			redisImpl, err := newRedisBackend()
			if err == nil {
				backendImpl = redisImpl
				common.SysLog("model_name_limiter: using redis backend")
				return
			}
			common.SysError(fmt.Sprintf("model_name_limiter: redis backend init failed: %v", err))
		}

		backendImpl = newMemoryBackend()
		common.SysLog("model_name_limiter: redis unavailable, using in-memory backend (single-instance only)")
	})
}

func getBackend() backend {
	InitModelNameLimiter()
	return backendImpl
}

// Acquire atomically checks and, when allowed, records all supplied keys.
// Invalid key/limit shapes fail open because they cannot safely be represented
// by the v1 backend contract.
func Acquire(ctx context.Context, keys []string, limits []int) Result {
	if len(keys) == 0 {
		return Result{Allowed: true}
	}
	if len(keys) != len(limits) || (len(keys) != 1 && len(keys) != 2) {
		common.SysError(fmt.Sprintf("model_name_limiter: invalid acquire arguments keys=%d limits=%d", len(keys), len(limits)))
		return Result{Allowed: true}
	}
	return getBackend().Acquire(ctx, keys, limits)
}

// Inspect returns current sliding-window counts without changing any bucket.
// Unlike Acquire, inspection errors are returned to the caller so dashboards
// can distinguish an unavailable backend from a genuine zero count.
func Inspect(ctx context.Context, keys []string) ([]int, error) {
	if len(keys) == 0 {
		return []int{}, nil
	}
	return getBackend().Inspect(ctx, keys)
}

// UsingMemoryBackend reports whether the active backend is the process-local
// fallback. Counts from that backend are valid only for this instance.
func UsingMemoryBackend() bool {
	backendInstance := getBackend()
	memoryBackend, ok := backendInstance.(interface{ IsMemory() bool })
	return ok && memoryBackend.IsMemory()
}

// IsMemory is intentionally kept on the private backend contract so the
// capacity service can label fallback data without type assertions.
func (b *memoryBackend) IsMemory() bool { return true }
func (b *redisBackend) IsMemory() bool  { return false }

// ModelKey and GroupKey are the single key builders shared by the admission
// middleware and read-only capacity paths.
func ModelKey(model string) string { return "mdrl:v1:rpm:model:" + model }
func GroupKey(model, group string) string {
	return "mdrl:v1:rpm:group:" + model + ":" + group
}

func scopeForIndex(index int) string {
	if index == 1 {
		return "global"
	}
	if index == 2 {
		return "group"
	}
	return ""
}
