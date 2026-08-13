package realtimemetrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// splitChannelField decodes the field names writeToRedis produces. The two sides
// are in different files, so this is the only place the encoding contract is
// pinned down.
func TestSplitChannelField(t *testing.T) {
	cases := []struct {
		field string
		id    int
		kind  string
		ok    bool
	}{
		{field: "122:r", id: 122, kind: "r", ok: true},
		{field: "122:e", id: 122, kind: "e", ok: true},
		{field: "122", ok: false},
		{field: ":r", ok: false},
		{field: "122:", ok: false},
		{field: "abc:r", ok: false},
		{field: "122:x", ok: false},
		{field: "", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			id, kind, ok := splitChannelField(tc.field)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.id, id)
			assert.Equal(t, tc.kind, kind)
		})
	}
}

func TestParseIntTreatsMissingFieldsAsZero(t *testing.T) {
	assert.Equal(t, int64(42), parseInt("42"))
	assert.Equal(t, int64(-7), parseInt("-7"))
	// HGetAll returns no key at all for a counter that was never incremented.
	assert.Zero(t, parseInt(""))
	assert.Zero(t, parseInt("not a number"))
}

// Memory pressure is the one gauge that must not be summed: two instances at 40%
// are not one instance at 80%, and the dashboard uses this number to decide
// whether traffic is being shed.
func TestAddGaugesSumsLoadButTakesWorstMemory(t *testing.T) {
	var total InstanceGauges
	addGauges(&total, InstanceGauges{
		ActiveRequests: 10, ActiveBodyBytes: 100, MaxConcurrent: 5000, MaxBodyBytes: 1000,
		CgroupPermille: 400, CgroupTripped: false, TripCount: 1, ForcedResetCount: 0, Goroutines: 300,
	})
	addGauges(&total, InstanceGauges{
		ActiveRequests: 20, ActiveBodyBytes: 200, MaxConcurrent: 5000, MaxBodyBytes: 1000,
		CgroupPermille: 810, CgroupTripped: true, TripCount: 2, ForcedResetCount: 1, Goroutines: 500,
	})

	assert.Equal(t, InstanceGauges{
		ActiveRequests: 30, ActiveBodyBytes: 300, MaxConcurrent: 10000, MaxBodyBytes: 2000,
		CgroupPermille: 810, CgroupTripped: true, TripCount: 3, ForcedResetCount: 1, Goroutines: 800,
	}, total)
}

func TestBuildChannelsUnionsSourcesAndOrdersByLoad(t *testing.T) {
	points := buildChannels(
		map[int]int64{122: 3, 133: 12},
		map[int]int64{122: 500, 276: 40},
		map[int]int64{276: 40},
		func(channelID int) string {
			if channelID == 276 {
				return "unresolved-by-cache"
			}
			return ""
		},
	)

	// A channel that finished every attempt still has traffic in the window, and a
	// channel that is busy right now may have no completed request yet: the panel
	// has to show both.
	require.Len(t, points, 3)
	assert.Equal(t, []int{133, 122, 276}, []int{points[0].ChannelID, points[1].ChannelID, points[2].ChannelID})
	assert.Equal(t, int64(12), points[0].Concurrency)
	assert.Equal(t, int64(500), points[1].Requests)
	assert.Equal(t, int64(40), points[2].Errors)
	assert.Equal(t, "unresolved-by-cache", points[2].ChannelName)
	assert.Equal(t, int64(channelWindowMins*60), points[0].WindowSecs)
}

// The console polls this every ten seconds. Read must degrade to local numbers
// instead of failing, and it must say so, so the page can label itself as showing
// one instance rather than claiming the cluster went quiet.
func TestReadDegradesToLocalWhenRedisIsUnavailable(t *testing.T) {
	resetCollector(t)
	const channelID = 90101
	release := ChannelAttempt(channelID)
	defer release()

	snapshot := Read(context.Background(), func(int) string { return "local-channel" })

	require.False(t, snapshot.RedisEnabled)
	assert.True(t, snapshot.Degraded)
	assert.Equal(t, "redis_disabled", snapshot.Warning)
	assert.NotZero(t, snapshot.NowUnix)
	require.Len(t, snapshot.Instances, 1, "the degraded payload describes this process only")
	assert.Zero(t, snapshot.Instances[0].StaleSeconds)
	// Inventing a single history point would look like a flat line rather than a
	// gap, so the series stays empty.
	assert.Empty(t, snapshot.Series)

	require.NotEmpty(t, snapshot.Channels)
	found := false
	for _, point := range snapshot.Channels {
		if point.ChannelID == channelID {
			found = true
			assert.Equal(t, int64(1), point.Concurrency)
			assert.Equal(t, "local-channel", point.ChannelName)
		}
	}
	assert.True(t, found, "live local concurrency must survive the degraded path")
}

func TestBuildChannelsToleratesMissingNameResolver(t *testing.T) {
	points := buildChannels(map[int]int64{7: 1}, nil, nil, nil)
	require.Len(t, points, 1)
	assert.Empty(t, points[0].ChannelName)
}
