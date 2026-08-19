// Package model_name_limiter provides an atomic sliding-window RPM limiter for
// arbitrary string keys.  The package deliberately does not know anything
// about models or groups; callers choose the keys and their limits.
package model_name_limiter

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	rpmWindowSeconds        = 60
	windowSeconds           = rpmWindowSeconds
	backendOperationTimeout = 200 * time.Millisecond
)

// WindowSeconds is the shared admission and capacity observation window.
const WindowSeconds = rpmWindowSeconds

// Bucket keeps the key, limit, and rejection scope for one atomic counter.
type Bucket struct {
	Key   string
	Limit int
	Scope string // "global", "group", "user", "group_total", or "group_user"
}

// Result is the outcome of an acquire attempt. Scope, Limit, and Current are
// populated only when a request is rejected by a limit.
type Result struct {
	Allowed bool
	Scope   string // "global", "group", "user", "group_total", or "group_user"
	Limit   int
	Current int
}

type backend interface {
	Acquire(context.Context, []Bucket) Result
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
// Invalid bucket shapes fail open because they cannot safely be represented
// by the v1 backend contract.
func Acquire(ctx context.Context, buckets []Bucket) Result {
	if len(buckets) == 0 {
		return Result{Allowed: true}
	}
	if !validAcquireBuckets(buckets) {
		common.SysError(fmt.Sprintf("model_name_limiter: invalid acquire buckets=%d", len(buckets)))
		return Result{Allowed: true}
	}
	return getBackend().Acquire(ctx, buckets)
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

// ModelKey, GroupKey, UserKey, GroupTotalKey, and GroupUserKey are shared by
// admission and capacity reads. GroupTotalKey and GroupUserKey aggregate all
// models in a group, while GroupKey limits one model in a group.
func ModelKey(model string) string { return "mdrl:v1:rpm:model:" + model }
func GroupKey(model, group string) string {
	return "mdrl:v1:rpm:group:" + model + ":" + group
}
func UserKey(model string, userID int) string {
	return "mdrl:v1:rpm:user:" + model + ":" + strconv.Itoa(userID)
}
func GroupTotalKey(group string) string { return "mdrl:v1:rpm:gtotal:" + group }
func GroupUserKey(group string, userID int) string {
	return "mdrl:v1:rpm:guser:" + group + ":" + strconv.Itoa(userID)
}

func validAcquireBuckets(buckets []Bucket) bool {
	if len(buckets) < 1 || len(buckets) > 5 {
		return false
	}
	for _, bucket := range buckets {
		// Limit == 0 is a count-only bucket: record hits without rejecting.
		if bucket.Key == "" || bucket.Limit < 0 {
			return false
		}
		if bucket.Scope != "global" && bucket.Scope != "group" && bucket.Scope != "user" && bucket.Scope != "group_total" && bucket.Scope != "group_user" {
			return false
		}
	}
	return true
}
