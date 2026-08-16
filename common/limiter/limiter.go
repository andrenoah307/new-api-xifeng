package limiter

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

//go:embed lua/rate_limit.lua
var rateLimitScript string

//go:embed lua/rate_limit_peek.lua
var rateLimitPeekScript string

type RedisLimiter struct {
	client         *redis.Client
	limitScriptSHA string
	peekScriptSHA  string
	shaMu          sync.RWMutex
}

var (
	instance *RedisLimiter
	once     sync.Once
)

func New(ctx context.Context, r *redis.Client) *RedisLimiter {
	once.Do(func() {
		// 预加载脚本
		limitSHA, err := r.ScriptLoad(ctx, rateLimitScript).Result()
		if err != nil {
			common.SysLog(fmt.Sprintf("Failed to load rate limit script: %v", err))
		}
		peekSHA, peekErr := r.ScriptLoad(ctx, rateLimitPeekScript).Result()
		if peekErr != nil {
			common.SysLog(fmt.Sprintf("Failed to load rate limit peek script: %v", peekErr))
		}
		instance = &RedisLimiter{
			client:         r,
			limitScriptSHA: limitSHA,
			peekScriptSHA:  peekSHA,
		}
	})

	return instance
}

func (rl *RedisLimiter) Allow(ctx context.Context, key string, opts ...Option) (bool, error) {
	// 默认配置
	config := &Config{
		Capacity:  10,
		Rate:      1,
		Requested: 1,
	}

	// 应用选项模式
	for _, opt := range opts {
		opt(config)
	}

	// 执行限流
	if rl == nil || rl.client == nil {
		return false, fmt.Errorf("rate limit client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := rl.client.EvalSha(
		ctx,
		rl.currentLimitSHA(),
		[]string{key},
		config.Requested,
		config.Rate,
		config.Capacity,
	).Int()

	if err != nil {
		return false, fmt.Errorf("rate limit failed: %w", err)
	}
	return result == 1, nil
}

// Peek returns the number of tokens the next Allow call would observe without
// consuming or persisting anything. exists is false when the bucket hash has
// not been created yet; callers can then represent the current usage as zero
// (or choose another product-level semantic) without inventing a full bucket.
func (rl *RedisLimiter) Peek(ctx context.Context, key string, opts ...Option) (tokens int64, exists bool, err error) {
	config := &Config{Capacity: 10, Rate: 1, Requested: 1}
	for _, opt := range opts {
		opt(config)
	}
	if rl == nil || rl.client == nil {
		return 0, false, fmt.Errorf("rate limit client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if rl.currentPeekSHA() == "" {
		if err = rl.loadPeekScript(ctx); err != nil {
			return 0, false, fmt.Errorf("load rate limit peek script: %w", err)
		}
	}

	result, evalErr := rl.client.EvalSha(
		ctx,
		rl.currentPeekSHA(),
		[]string{key},
		config.Requested,
		config.Rate,
		config.Capacity,
	).Slice()
	if evalErr != nil && strings.Contains(strings.ToUpper(evalErr.Error()), "NOSCRIPT") {
		if loadErr := rl.loadPeekScript(ctx); loadErr != nil {
			return 0, false, fmt.Errorf("reload rate limit peek script: %w", loadErr)
		}
		result, evalErr = rl.client.EvalSha(
			ctx,
			rl.currentPeekSHA(),
			[]string{key},
			config.Requested,
			config.Rate,
			config.Capacity,
		).Slice()
	}
	if evalErr != nil {
		return 0, false, fmt.Errorf("rate limit peek failed: %w", evalErr)
	}
	return parsePeekResult(result)
}

func parsePeekResult(result []interface{}) (int64, bool, error) {
	if len(result) == 1 {
		marker, markerErr := redisScalarString(result[0])
		if markerErr != nil {
			return 0, false, markerErr
		}
		if marker == "missing" {
			return 0, false, nil
		}
	}
	if len(result) != 2 {
		return 0, false, fmt.Errorf("malformed rate limit peek response")
	}
	marker, markerErr := redisScalarString(result[0])
	if markerErr != nil {
		return 0, false, markerErr
	}
	if marker != "present" {
		return 0, false, fmt.Errorf("unknown rate limit peek marker %q", marker)
	}
	tokenString, tokenErr := redisScalarString(result[1])
	if tokenErr != nil {
		return 0, false, tokenErr
	}
	tokens, parseErr := strconv.ParseInt(tokenString, 10, 64)
	if parseErr != nil {
		// Redis may return a Lua number with a decimal point. The limiter uses
		// integer token arithmetic, so accept the exact integer representation
		// while rejecting fractional or overflowing values.
		parsedFloat, floatErr := strconv.ParseFloat(tokenString, 64)
		if floatErr != nil || parsedFloat < 0 || parsedFloat > float64(^uint64(0)>>1) || parsedFloat != float64(int64(parsedFloat)) {
			return 0, false, fmt.Errorf("invalid rate limit peek tokens %q", tokenString)
		}
		tokens = int64(parsedFloat)
	}
	return tokens, true, nil
}

func (rl *RedisLimiter) currentLimitSHA() string {
	if rl == nil {
		return ""
	}
	rl.shaMu.RLock()
	defer rl.shaMu.RUnlock()
	return rl.limitScriptSHA
}

func (rl *RedisLimiter) currentPeekSHA() string {
	if rl == nil {
		return ""
	}
	rl.shaMu.RLock()
	defer rl.shaMu.RUnlock()
	return rl.peekScriptSHA
}

func (rl *RedisLimiter) loadPeekScript(ctx context.Context) error {
	if rl == nil || rl.client == nil {
		return fmt.Errorf("rate limit client is not initialized")
	}
	sha, err := rl.client.ScriptLoad(ctx, rateLimitPeekScript).Result()
	if err != nil {
		return err
	}
	if sha == "" {
		return fmt.Errorf("redis returned an empty rate limit peek script SHA")
	}
	rl.shaMu.Lock()
	rl.peekScriptSHA = sha
	rl.shaMu.Unlock()
	return nil
}

func redisScalarString(value interface{}) (string, error) {
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
		return "", fmt.Errorf("redis: unexpected rate limit peek result type=%T", value)
	}
}

// Config 配置选项模式
type Config struct {
	Capacity  int64
	Rate      int64
	Requested int64
}

type Option func(*Config)

func WithCapacity(c int64) Option {
	return func(cfg *Config) { cfg.Capacity = c }
}

func WithRate(r int64) Option {
	return func(cfg *Config) { cfg.Rate = r }
}

func WithRequested(n int64) Option {
	return func(cfg *Config) { cfg.Requested = n }
}
