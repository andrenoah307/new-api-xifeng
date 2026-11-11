package realtimemetrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffinityCacheUsagePopulationWindowsAndRetry(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previous, enabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = previous, enabled; _ = client.Close() })
	ctx := context.Background()
	now := time.Now().Unix()
	bucket := now / 300 * 300
	RecordChannelAffinityCacheUsage(10, 10, 100, 80)
	RecordChannelAffinityCacheUsage(10, 10, 900, 20)
	RecordChannelAffinityCacheUsage(0, 10, 10000, 10000) // no affinity
	RecordChannelAffinityCacheUsage(10, 11, 100, 0)      // marked retry: final channel, zero cache included
	RecordChannelAffinityCacheUsage(10, 10, 0, 0)
	server.SetError("ERR unavailable")
	flushChannelAffinityCacheUsage()
	server.SetError("")
	flushChannelAffinityCacheUsage()
	usage, err := ReadChannelAffinityCacheUsage(ctx, 10, 1, 0, now)
	require.NoError(t, err)
	assert.Equal(t, ChannelAffinityCacheUsage{PromptTokens: 1000, CachedTokens: 100, Samples: 2}, usage)
	fallbackUsage, err := ReadChannelAffinityCacheUsage(ctx, 11, 1, 0, now)
	require.NoError(t, err)
	assert.Equal(t, ChannelAffinityCacheUsage{PromptTokens: 100, Samples: 1}, fallbackUsage)
	key := fmt.Sprintf("nak:rt:affcache:10:%d", bucket)
	// Replaying an already committed flush cannot inflate the sample gate.
	require.NoError(t, client.Eval(ctx, affinityCacheFlushScript, []string{key}, "w:"+affinityCacheWriter, affinityCacheFlush.sequence, 1000, 100, 2).Err())
	usage, err = ReadChannelAffinityCacheUsage(ctx, 10, 1, 0, now)
	require.NoError(t, err)
	assert.EqualValues(t, 2, usage.Samples)
	oldKey := fmt.Sprintf("nak:rt:affcache:10:%d", bucket-7200)
	require.NoError(t, client.HSet(ctx, oldKey, "p", 1000, "c", 900, "n", 3).Err())
	usage, err = ReadChannelAffinityCacheUsage(ctx, 10, 3, 0, now)
	require.NoError(t, err)
	assert.EqualValues(t, 5, usage.Samples)
	assert.EqualValues(t, 2000, usage.PromptTokens)
	usage, err = ReadChannelAffinityCacheUsage(ctx, 10, 3, bucket+1, now)
	require.NoError(t, err)
	assert.Zero(t, usage.Samples, "pre-recovery and partial boundary buckets are excluded")
	require.NoError(t, client.HSet(ctx, key, "c", "corrupt").Err())
	_, err = ReadChannelAffinityCacheUsage(ctx, 10, 1, 0, now)
	require.Error(t, err)
	common.RedisEnabled = false
	_, err = ReadChannelAffinityCacheUsage(ctx, 10, 1, 0, now)
	require.Error(t, err)
}
