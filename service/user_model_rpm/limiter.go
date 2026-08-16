// Package user_model_rpm records per-user, per-model request observations in
// a short sliding window. It is deliberately observational: a backend error
// never decides whether a request may proceed.
package user_model_rpm

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

const (
	WindowSeconds = 60
	windowMillis  = int64(WindowSeconds * 1000)
	ttlMillis     = 65_000

	// maxScan bounds the amount of member data an inspection can ask Redis to
	// materialize. Above this rate the detail has no useful dashboard value and
	// returning overflow is preferable to blocking Redis's single thread.
	maxScan = 5000

	// A full sweep is rare and only runs when the process-local map grows past
	// this bound. Normal reads and writes still trim their touched user.
	memorySweepThreshold    = 1024
	backendOperationTimeout = 200 * time.Millisecond

	// memberSeparator joins the request ID and the model name into a unique
	// sorted-set member. ASCII unit separator cannot appear in either half.
	memberSeparator = "\x1f"
)

// ModelRPM is one model's request count in the most recent 60-second window.
type ModelRPM struct {
	Model string `json:"model"`
	RPM   int    `json:"rpm"`
}

type backend interface {
	Record(context.Context, int, string, string) error
	Inspect(context.Context, int) ([]ModelRPM, string, error)
	IsMemory() bool
}

var (
	backendOnce sync.Once
	backendImpl backend

	recordLogMu   sync.Mutex
	lastRecordLog time.Time
)

// Init selects the backend once for the lifetime of the process. Runtime
// Redis changes intentionally do not cause a shadow write or a backend swap.
func Init() {
	backendOnce.Do(func() {
		if common.RedisEnabled && common.RDB != nil {
			redisImpl, err := newRedisBackend()
			if err == nil {
				backendImpl = redisImpl
				common.SysLog("user_model_rpm: using redis backend")
				return
			}
			common.SysError(fmt.Sprintf("user_model_rpm: redis backend init failed: %v", err))
		}
		backendImpl = newMemoryBackend()
		common.SysLog("user_model_rpm: redis unavailable, using in-memory backend (single-instance only)")
	})
}

func getBackend() backend {
	Init()
	return backendImpl
}

// Enabled reads the startup-level feature flag. The environment is expected
// to remain fixed after process startup; reading through setting keeps the
// capacity pre-gate and the collector on the same source of truth.
func Enabled() bool { return setting.UserModelRPMEnabled() }

// Record stores one completed request observation. Callers may ignore the
// returned error to preserve fail-open request handling; this function also
// emits a throttled diagnostic for backend failures.
func Record(ctx context.Context, userID int, requestID, model string) error {
	if !Enabled() || requestID == "" || model == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	implementation := getBackend()
	if implementation == nil {
		err := fmt.Errorf("user model RPM backend is not initialized")
		logRecordError(err)
		return err
	}
	if err := implementation.Record(ctx, userID, requestID, model); err != nil {
		logRecordError(err)
		return err
	}
	return nil
}

// Inspect returns the current per-model observations for one user. Backend
// failures are distinguishable from a genuine empty window through status.
func Inspect(ctx context.Context, userID int) ([]ModelRPM, string, error) {
	if !Enabled() {
		return []ModelRPM{}, "unavailable", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	implementation := getBackend()
	if implementation == nil {
		return []ModelRPM{}, "unavailable", fmt.Errorf("user model RPM backend is not initialized")
	}
	items, status, err := implementation.Inspect(ctx, userID)
	if err != nil {
		return []ModelRPM{}, "unavailable", err
	}
	if status != "available" && status != "empty" && status != "overflow" && status != "unavailable" {
		return []ModelRPM{}, "unavailable", fmt.Errorf("unknown user model RPM status %q", status)
	}
	if items == nil {
		items = []ModelRPM{}
	}
	if status == "overflow" || status == "empty" || status == "unavailable" {
		items = []ModelRPM{}
	}
	return items, status, nil
}

// UsingMemoryBackend reports whether the process-local fallback was selected.
func UsingMemoryBackend() bool {
	implementation := getBackend()
	return implementation != nil && implementation.IsMemory()
}

func logRecordError(err error) {
	now := time.Now()
	recordLogMu.Lock()
	if !lastRecordLog.IsZero() && now.Sub(lastRecordLog) < time.Second {
		recordLogMu.Unlock()
		return
	}
	lastRecordLog = now
	recordLogMu.Unlock()
	common.SysError(fmt.Sprintf("user_model_rpm: record failed: %v", err))
}

func modelRPMKey(userID int) string {
	return "urpm:v1:" + strconv.Itoa(userID)
}

func memberFor(requestID, model string) string {
	return requestID + memberSeparator + model
}

// SortByRPM is deliberately centralized at the package boundary so Redis's
// unordered Lua table and the memory map expose identical API ordering.
func SortByRPM(items []ModelRPM) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].RPM != items[j].RPM {
			return items[i].RPM > items[j].RPM
		}
		return items[i].Model < items[j].Model
	})
}
