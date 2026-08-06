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

func scopeForIndex(index int) string {
	if index == 1 {
		return "global"
	}
	if index == 2 {
		return "group"
	}
	return ""
}
