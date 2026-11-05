package service

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func routingTestContext(token int) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader("untouched body"))
	c.Set("id", 42)
	c.Set("token_id", token)
	c.Set(common.RequestIdKey, "routing-test-request")
	return c
}

func TestRoutingObservationSignalsAndSnapshots(t *testing.T) {
	c := routingTestContext(11)
	c.Request.Header.Set("Session_id", "private-session")
	BeginRoutingObservation(c, "gpt-test")
	request := &dto.OpenAIResponsesRequest{PromptCacheKey: []byte(`"private-cache-key"`)}
	before, err := common.Marshal(request)
	require.NoError(t, err)
	ObserveRoutingRequest(c, request)
	ObserveRoutingRequest(c, &dto.ClaudeRequest{Metadata: []byte(`{"user_id":"private-claude-user"}`)})
	c.Set("channel_id", 17)
	ObserveRoutingEvent(c, RoutingEvent{Kind: "attempt", Channel: 17, Attempt: 1})
	admin := map[string]interface{}{}
	AppendChannelAffinityAdminInfo(c, admin)
	snapshot, ok := admin["routing_observation"].(*RoutingObservation)
	require.True(t, ok, "a cache miss must still have an admin snapshot")
	require.Len(t, snapshot.Signals, 3)
	assert.Equal(t, "not_attempted", snapshot.WriteResult)
	assert.Equal(t, 17, snapshot.FinalChannel)
	ObserveRoutingSignal(c, "header:thread_id", "later-thread")
	observeAffinityWrite(c, 18, 300, "stored")
	c.Set("channel_id", 18)
	assert.Len(t, snapshot.Signals, 3)
	assert.Equal(t, "not_attempted", snapshot.WriteResult)
	assert.Equal(t, 17, snapshot.FinalChannel)
	assert.Equal(t, 18, RoutingObservationSnapshot(c).WriteChannel)
	after, err := common.Marshal(request)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, int64(len("untouched body")), c.Request.ContentLength)
	body, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	assert.Equal(t, "untouched body", string(body))
	raw, err := common.Marshal(snapshot)
	require.NoError(t, err)
	for _, secret := range []string{"private-session", "private-cache-key", "private-claude-user", common.CryptoSecret} {
		assert.NotContains(t, string(raw), secret)
	}
	assert.Equal(t, routingFingerprint(c, "session", "same"), routingFingerprint(routingTestContext(11), "session", "same"))
	assert.NotEqual(t, routingFingerprint(c, "session", "same"), routingFingerprint(routingTestContext(12), "session", "same"))
	assert.NotEqual(t, routingFingerprint(c, "session", "same"), routingFingerprint(c, "binding", "same"))
}

func TestRoutingObservationBoundsAndCompletion(t *testing.T) {
	c := routingTestContext(11)
	BeginRoutingObservation(c, "claude-test")
	ObserveRoutingSignal(c, "header:session_id", strings.Repeat("s", 4097))
	assert.Empty(t, RoutingObservationSnapshot(c).Signals)
	for i := 0; i < 20; i++ {
		ObserveRoutingEvent(c, RoutingEvent{Kind: "attempt", Attempt: i + 1})
	}
	ObserveRoutingOutcome(c, "relay_error")
	c.Writer.WriteHeader(502)
	var output bytes.Buffer
	old := gin.DefaultWriter
	gin.DefaultWriter = &output
	t.Cleanup(func() { gin.DefaultWriter = old })
	FinishRoutingObservation(c)
	assert.Contains(t, output.String(), "routing-test-request")
	assert.Contains(t, output.String(), `"outcome":"relay_error"`)
	assert.Contains(t, output.String(), `"http_status":502`)
	assert.Contains(t, output.String(), `"dropped_events":4`)
	assert.Len(t, RoutingObservationSnapshot(c).Events, 16)
}

func TestRoutingObservationAffinityLifecycle(t *testing.T) {
	oldConfig := *operation_setting.GetChannelAffinitySetting()
	oldEnabled := common.RedisEnabled
	oldCache := getChannelAffinityCache()
	t.Cleanup(func() {
		*operation_setting.GetChannelAffinitySetting() = oldConfig
		common.RedisEnabled = oldEnabled
		channelAffinityCache = oldCache
	})
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	common.RedisEnabled = true
	channelAffinityCache = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
		Namespace: cachex.Namespace(channelAffinityCacheNamespace), Redis: client,
		RedisEnabled: func() bool { return true }, RedisCodec: cachex.IntCodec{},
	})
	*operation_setting.GetChannelAffinitySetting() = operation_setting.ChannelAffinitySetting{
		Enabled: true, SwitchOnSuccess: true, DefaultTTLSeconds: 300,
		Rules: []operation_setting.ChannelAffinityRule{{Name: "token", ModelRegex: []string{".*"}, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "context_int", Key: "token_id"}}, IncludeModelName: true, IncludeUsingGroup: true, IncludeRuleName: true}},
	}
	c := routingTestContext(11)
	BeginRoutingObservation(c, "gpt-test")
	_, found := GetPreferredChannelByAffinity(c, "gpt-test", "group")
	assert.False(t, found)
	assert.Equal(t, "miss", RoutingObservationSnapshot(c).Lookup)
	assert.Empty(t, server.Keys(), "observation must not create cache entries")
	c.Set("channel_id", 17)
	RecordChannelAffinity(c, 17)
	keys := server.Keys()
	require.Len(t, keys, 1)
	assert.Equal(t, 300*time.Second, server.TTL(keys[0]))
	assert.Equal(t, "stored", RoutingObservationSnapshot(c).WriteResult)
	next := routingTestContext(11)
	BeginRoutingObservation(next, "gpt-test")
	id, found := GetPreferredChannelByAffinity(next, "gpt-test", "group")
	require.True(t, found)
	assert.Equal(t, 17, id)
	assert.Equal(t, "context_int:token_id", RoutingObservationSnapshot(next).BindingSource)
	assert.Equal(t, RoutingObservationSnapshot(c).BindingFP, RoutingObservationSnapshot(next).BindingFP)
	next.Set("channel_id", 18)
	RecordChannelAffinity(next, 17)
	value, err := server.Get(keys[0])
	require.NoError(t, err)
	assert.Equal(t, "18", value, "existing switch-on-success semantics must remain")
	common.SetContextKey(next, constant.ContextKeyRateLimitSkipped, true)
	next.Set("channel_id", 19)
	RecordChannelAffinity(next, 18)
	value, err = server.Get(keys[0])
	require.NoError(t, err)
	assert.Equal(t, "18", value, "capacity detours must preserve the binding")
	server.SetError("ERR simulated outage")
	RecordChannelAffinity(next, 18)
	assert.Equal(t, "cache_error", RoutingObservationSnapshot(next).WriteResult)
	_, found = GetPreferredChannelByAffinity(next, "gpt-test", "group")
	assert.False(t, found)
	assert.Equal(t, "cache_error", RoutingObservationSnapshot(next).Lookup)
	server.SetError("")
	assert.True(t, ClearCurrentChannelAffinityCache(next))
	assert.Empty(t, server.Keys())
	missing := routingTestContext(0)
	BeginRoutingObservation(missing, "gpt-test")
	_, found = GetPreferredChannelByAffinity(missing, "gpt-test", "group")
	assert.False(t, found)
	assert.Equal(t, "missing_key", RoutingObservationSnapshot(missing).Lookup)
	operation_setting.GetChannelAffinitySetting().Enabled = false
	disabled := routingTestContext(11)
	BeginRoutingObservation(disabled, "gpt-test")
	_, found = GetPreferredChannelByAffinity(disabled, "gpt-test", "group")
	assert.False(t, found)
	assert.Equal(t, "disabled", RoutingObservationSnapshot(disabled).Lookup)
	assert.Empty(t, server.Keys())
}
