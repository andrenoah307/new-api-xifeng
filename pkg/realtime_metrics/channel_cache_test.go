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

func TestChannelHourlyCache(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldClient, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = oldClient, oldEnabled
		_ = client.Close()
	})
	ctx := context.Background()
	RecordChannelCacheUsage(11, 100, 90)
	RecordChannelCacheUsage(11, 900, 0)
	RecordChannelCacheUsage(12, 100, 0)
	RecordChannelCacheUsage(13, 0, 0)
	flushOnce()
	// A second flush/instance adds to the same minute, rather than replacing it.
	RecordChannelCacheUsage(11, 1000, 110)
	flushOnce()
	rates, err := ReadChannelCacheRates(ctx)
	require.NoError(t, err)
	assert.Equal(t, 10.0, rates[11])
	rate, ok := rates[12]
	assert.True(t, ok)
	assert.Zero(t, rate)
	assert.NotContains(t, rates, 13)
	minute := time.Now().Unix() / 60 * 60
	for _, stamp := range []int64{minute - 3600, minute + 3600} {
		require.NoError(t, client.HSet(ctx, fmt.Sprintf(keyChannelCacheMinFmt, stamp), "11:p", 10000, "11:c", 10000).Err())
	}
	rates, err = ReadChannelCacheRates(ctx)
	require.NoError(t, err)
	assert.Equal(t, 10.0, rates[11], "expired and future buckets must not enter the hour")

	// Failed writes preserve the original event minute and can be retried.
	RecordChannelCacheUsage(14, 100, 20)
	channelCachePending.Lock()
	channelCachePending.values[channelCacheKey{minute: minute - 120, channelID: 15}] = cacheUsage{prompt: 100, cached: 30}
	channelCachePending.values[channelCacheKey{minute: minute - 3600, channelID: 16}] = cacheUsage{prompt: 100, cached: 40}
	channelCachePending.Unlock()
	server.SetError("ERR unavailable")
	flushOnce()
	server.SetError("")
	flushOnce()
	rates, err = ReadChannelCacheRates(ctx)
	require.NoError(t, err)
	assert.Equal(t, 20.0, rates[14])
	assert.Equal(t, 30.0, rates[15])
	assert.NotContains(t, rates, 16)
	assert.Equal(t, "100", server.HGet(fmt.Sprintf(keyChannelCacheMinFmt, minute-120), "15:p"))
	assert.Empty(t, server.HGet(fmt.Sprintf(keyChannelCacheMinFmt, minute), "15:p"))
	server.FastForward(66 * time.Minute)
	rates, err = ReadChannelCacheRates(ctx)
	require.NoError(t, err)
	assert.Empty(t, rates)
	common.RedisEnabled = false
	rates, err = ReadChannelCacheRates(ctx)
	require.NoError(t, err)
	assert.Empty(t, rates, "missing Redis must not be reported as a measured 0%")
}
