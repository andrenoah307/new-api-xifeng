package user_model_rpm

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

//go:embed lua/record.lua
var redisRecordScript string

//go:embed lua/inspect.lua
var redisInspectScript string

type redisBackend struct {
	client *redis.Client

	shaMu      sync.RWMutex
	recordSHA  string
	inspectSHA string
}

func newRedisBackend() (*redisBackend, error) {
	if common.RDB == nil {
		return nil, fmt.Errorf("redis client not initialized")
	}
	b := &redisBackend{client: common.RDB}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.loadRecordScript(ctx); err != nil {
		return nil, fmt.Errorf("load record script: %w", err)
	}
	if err := b.loadInspectScript(ctx); err != nil {
		return nil, fmt.Errorf("load inspect script: %w", err)
	}
	return b, nil
}

func (b *redisBackend) IsMemory() bool { return false }

func (b *redisBackend) Record(ctx context.Context, userID int, requestID, model string) error {
	if b == nil || b.client == nil {
		return fmt.Errorf("redis client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	operationCtx, cancel := context.WithTimeout(ctx, backendOperationTimeout)
	defer cancel()
	key := modelRPMKey(userID)
	member := memberFor(requestID, model)
	_, err := b.evalRecord(operationCtx, key, member)
	if err != nil && isNoScriptError(err) {
		if loadErr := b.loadRecordScript(operationCtx); loadErr != nil {
			return fmt.Errorf("reload record script: %w", loadErr)
		}
		_, err = b.evalRecord(operationCtx, key, member)
	}
	return err
}

func (b *redisBackend) Inspect(ctx context.Context, userID int) ([]ModelRPM, string, error) {
	if b == nil || b.client == nil {
		return nil, "unavailable", fmt.Errorf("redis client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	operationCtx, cancel := context.WithTimeout(ctx, backendOperationTimeout)
	defer cancel()
	if b.currentInspectSHA() == "" {
		if err := b.loadInspectScript(operationCtx); err != nil {
			return nil, "unavailable", err
		}
	}
	raw, err := b.evalInspect(operationCtx, modelRPMKey(userID))
	if err != nil && isNoScriptError(err) {
		if loadErr := b.loadInspectScript(operationCtx); loadErr != nil {
			return nil, "unavailable", fmt.Errorf("reload inspect script: %w", loadErr)
		}
		raw, err = b.evalInspect(operationCtx, modelRPMKey(userID))
	}
	if err != nil {
		return nil, "unavailable", err
	}
	return parseInspectResult(raw)
}

func (b *redisBackend) evalRecord(ctx context.Context, key, member string) (int64, error) {
	sha := b.currentRecordSHA()
	if sha == "" {
		return 0, fmt.Errorf("record script SHA is empty")
	}
	return b.client.EvalSha(ctx, sha, []string{key}, member).Int64()
}

func (b *redisBackend) evalInspect(ctx context.Context, key string) ([]interface{}, error) {
	sha := b.currentInspectSHA()
	if sha == "" {
		return nil, fmt.Errorf("inspect script SHA is empty")
	}
	return b.client.EvalSha(ctx, sha, []string{key}, maxScan).Slice()
}

func (b *redisBackend) currentRecordSHA() string {
	if b == nil {
		return ""
	}
	b.shaMu.RLock()
	defer b.shaMu.RUnlock()
	return b.recordSHA
}

func (b *redisBackend) currentInspectSHA() string {
	if b == nil {
		return ""
	}
	b.shaMu.RLock()
	defer b.shaMu.RUnlock()
	return b.inspectSHA
}

func (b *redisBackend) loadRecordScript(ctx context.Context) error {
	if b == nil || b.client == nil {
		return fmt.Errorf("redis client not initialized")
	}
	sha, err := b.client.ScriptLoad(ctx, redisRecordScript).Result()
	if err != nil {
		return err
	}
	if sha == "" {
		return fmt.Errorf("redis returned an empty record script SHA")
	}
	b.shaMu.Lock()
	b.recordSHA = sha
	b.shaMu.Unlock()
	return nil
}

func (b *redisBackend) loadInspectScript(ctx context.Context) error {
	if b == nil || b.client == nil {
		return fmt.Errorf("redis client not initialized")
	}
	sha, err := b.client.ScriptLoad(ctx, redisInspectScript).Result()
	if err != nil {
		return err
	}
	if sha == "" {
		return fmt.Errorf("redis returned an empty inspect script SHA")
	}
	b.shaMu.Lock()
	b.inspectSHA = sha
	b.shaMu.Unlock()
	return nil
}

func parseInspectResult(raw []interface{}) ([]ModelRPM, string, error) {
	if len(raw) == 0 {
		return []ModelRPM{}, "empty", nil
	}
	marker, err := redisResultString(raw[0])
	if err != nil {
		return nil, "unavailable", err
	}
	if len(raw) == 1 && marker == "overflow" {
		return []ModelRPM{}, "overflow", nil
	}
	if len(raw)%2 != 0 {
		return nil, "unavailable", fmt.Errorf("malformed user model RPM response length=%d", len(raw))
	}
	items := make([]ModelRPM, 0, len(raw)/2)
	for i := 0; i < len(raw); i += 2 {
		model, modelErr := redisResultString(raw[i])
		countText, countErr := redisResultString(raw[i+1])
		if modelErr != nil || countErr != nil {
			if modelErr != nil {
				return nil, "unavailable", modelErr
			}
			return nil, "unavailable", countErr
		}
		count, parseErr := strconv.Atoi(countText)
		if parseErr != nil || count < 0 || model == "" {
			return nil, "unavailable", fmt.Errorf("malformed user model RPM item model=%q count=%q", model, countText)
		}
		items = append(items, ModelRPM{Model: model, RPM: count})
	}
	if len(items) == 0 {
		return []ModelRPM{}, "empty", nil
	}
	SortByRPM(items)
	return items, "available", nil
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
		return "", fmt.Errorf("redis: unexpected user model RPM result type=%T", value)
	}
}

func isNoScriptError(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "NOSCRIPT")
}
