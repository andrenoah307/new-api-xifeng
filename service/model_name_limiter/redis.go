package model_name_limiter

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

//go:embed lua/rpm_acquire.lua
var redisAcquireScript string

type redisBackend struct {
	client *redis.Client

	shaMu sync.RWMutex
	// acquireSHA mirrors the channel limiter naming and is replaced when a
	// NOSCRIPT response forces a reload.
	acquireSHA string
}

var tokenCounter uint64

func newRedisBackend() (*redisBackend, error) {
	if common.RDB == nil {
		return nil, fmt.Errorf("redis client not initialized")
	}
	b := &redisBackend{client: common.RDB}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.loadScript(ctx); err != nil {
		return nil, fmt.Errorf("load acquire script: %w", err)
	}
	return b, nil
}

func (b *redisBackend) currentSHA() string {
	b.shaMu.RLock()
	defer b.shaMu.RUnlock()
	return b.acquireSHA
}

func (b *redisBackend) loadScript(ctx context.Context) error {
	if b == nil || b.client == nil {
		return fmt.Errorf("redis client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sha, err := b.client.ScriptLoad(ctx, redisAcquireScript).Result()
	if err != nil {
		return err
	}
	if sha == "" {
		return fmt.Errorf("redis returned an empty acquire script SHA")
	}
	b.shaMu.Lock()
	b.acquireSHA = sha
	b.shaMu.Unlock()
	return nil
}

func (b *redisBackend) Acquire(ctx context.Context, keys []string, limits []int) Result {
	if len(keys) == 0 {
		return Result{Allowed: true}
	}
	if b == nil || b.client == nil {
		common.SysError("model_name_limiter: redis acquire called without a client")
		return Result{Allowed: true}
	}
	if len(keys) != len(limits) || (len(keys) != 1 && len(keys) != 2) {
		return Result{Allowed: true}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	operationCtx, cancel := context.WithTimeout(ctx, backendOperationTimeout)
	defer cancel()
	args := scriptArgs(limits)
	result, err := b.eval(operationCtx, keys, args)
	if err != nil && isNoScriptError(err) {
		if loadErr := b.loadScript(operationCtx); loadErr == nil {
			result, err = b.eval(operationCtx, keys, args)
		} else {
			err = fmt.Errorf("reload acquire script: %w", loadErr)
		}
	}
	if err != nil {
		common.SysError(fmt.Sprintf("model_name_limiter: redis acquire failed: %v", err))
		return Result{Allowed: true}
	}
	return parseAcquireResult(result)
}

func (b *redisBackend) eval(ctx context.Context, keys []string, args []interface{}) ([]string, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("redis client not initialized")
	}
	sha := b.currentSHA()
	if sha == "" {
		return nil, fmt.Errorf("acquire script SHA is empty")
	}
	raw, err := b.client.EvalSha(ctx, sha, keys, args...).Slice()
	if err != nil {
		return nil, err
	}
	values := make([]string, len(raw))
	for i, value := range raw {
		values[i], err = redisResultString(value)
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func redisResultString(value interface{}) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case []byte:
		return string(value), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("redis: unexpected acquire result type=%T", value)
	}
}

func scriptArgs(limits []int) []interface{} {
	args := make([]interface{}, 0, len(limits)+2)
	args = append(args, windowSeconds)
	for _, limit := range limits {
		args = append(args, limit)
	}
	args = append(args, uniqueToken())
	return args
}

func parseAcquireResult(values []string) Result {
	if len(values) == 1 && values[0] == "1" {
		return Result{Allowed: true}
	}
	if len(values) != 4 || values[0] != "0" {
		common.SysError(fmt.Sprintf("model_name_limiter: malformed redis acquire response: %v", values))
		return Result{Allowed: true}
	}
	index, err := strconv.Atoi(values[1])
	if err != nil {
		common.SysError(fmt.Sprintf("model_name_limiter: malformed redis acquire scope: %v", values))
		return Result{Allowed: true}
	}
	limit, err := strconv.Atoi(values[2])
	if err != nil {
		common.SysError(fmt.Sprintf("model_name_limiter: malformed redis acquire limit: %v", values))
		return Result{Allowed: true}
	}
	current, err := strconv.Atoi(values[3])
	if err != nil {
		common.SysError(fmt.Sprintf("model_name_limiter: malformed redis acquire current: %v", values))
		return Result{Allowed: true}
	}
	scope := scopeForIndex(index)
	if scope == "" {
		common.SysError(fmt.Sprintf("model_name_limiter: malformed redis acquire scope index: %v", values))
		return Result{Allowed: true}
	}
	return Result{Allowed: false, Scope: scope, Limit: limit, Current: current}
}

func isNoScriptError(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "NOSCRIPT")
}

func uniqueToken() string {
	// A nanosecond component plus a process-local counter keeps members unique
	// even when many requests share the same millisecond score.
	n := time.Now().UnixNano()
	c := atomic.AddUint64(&tokenCounter, 1)
	return strconv.FormatInt(n, 36) + ":" + strconv.FormatUint(c, 36)
}
