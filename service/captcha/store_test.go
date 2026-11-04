package captcha

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProofIsBoundAndConsumedOnlyOnce(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()
	require.NoError(t, s.Put(ctx, "proof", "login:browser:target", time.Minute))
	ok, err := s.Consume(ctx, "proof", "register:browser:target")
	require.NoError(t, err)
	assert.False(t, ok)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, e := s.Consume(ctx, "proof", "login:browser:target")
			if e == nil && ok {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, accepted.Load())
}

func TestRedisProofAcrossNodesAndOutage(t *testing.T) {
	server := miniredis.RunT(t)
	old, enabled := common.RDB, common.RedisEnabled
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { _ = client.Close(); common.RDB, common.RedisEnabled = old, enabled })
	ctx := context.Background()
	writer, reader := RedisStore{}, RedisStore{}
	require.NoError(t, writer.Put(ctx, "test-proof", "bound", time.Minute))
	ok, err := reader.Consume(ctx, "test-proof", "wrong")
	require.NoError(t, err)
	assert.False(t, ok)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, e := reader.Consume(ctx, "test-proof", "bound")
			if e == nil && ok {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, accepted.Load())
	require.NoError(t, writer.Put(ctx, "expired-proof", "bound", time.Minute))
	server.FastForward(2 * time.Minute)
	ok, err = reader.Consume(ctx, "expired-proof", "bound")
	require.NoError(t, err)
	assert.False(t, ok)
	server.Close()
	require.ErrorIs(t, ActiveStore().Put(ctx, "outage", "bound", time.Minute), ErrUnavailable)
}

func TestExpiredProofAndCapacity(t *testing.T) {
	s := NewMemoryStore(1)
	ctx := context.Background()
	require.NoError(t, s.Put(ctx, "expired", "value", -time.Second))
	ok, err := s.Consume(ctx, "expired", "value")
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, s.Put(ctx, "live", "value", time.Minute))
	require.Error(t, s.Put(ctx, "extra", "value", time.Minute))
}
