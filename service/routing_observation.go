package service

import (
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const routingObservationKey = "routing_observation_v1"

// RoutingObservation is a point-in-time snapshot, never shared with later writes.
// Events describe decisions already made by the relay; they cannot affect routing.
type RoutingObservation struct {
	Version       int               `json:"version"`
	Model         string            `json:"model"`
	UserID        int               `json:"user_id"`
	TokenID       int               `json:"token_id"`
	Group         string            `json:"group,omitempty"`
	Signals       map[string]string `json:"signals,omitempty"`
	Lookup        string            `json:"lookup"`
	Rule          string            `json:"rule,omitempty"`
	BindingSource string            `json:"binding_source,omitempty"`
	BindingFP     string            `json:"binding_fp,omitempty"`
	TTL           int               `json:"ttl_seconds,omitempty"`
	SkipRetry     bool              `json:"skip_retry"`
	SwitchSuccess bool              `json:"switch_on_success"`
	KeepDisabled  bool              `json:"keep_on_disabled"`
	Preferred     int               `json:"preferred_channel,omitempty"`
	FinalChannel  int               `json:"final_channel,omitempty"`
	Outcome       string            `json:"outcome"`
	HTTPStatus    int               `json:"http_status,omitempty"`
	WriteChannel  int               `json:"write_channel,omitempty"`
	WriteResult   string            `json:"write_result"`
	Events        []RoutingEvent    `json:"events,omitempty"`
	Dropped       int               `json:"dropped_events,omitempty"`
}

type RoutingEvent struct {
	Kind      string `json:"kind"`
	Channel   int    `json:"channel,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Status    int    `json:"status,omitempty"`
	Attempt   int    `json:"attempt,omitempty"`
	RuleIndex int    `json:"rule_index,omitempty"`
}

type routingObservation struct {
	mu   sync.Mutex
	data RoutingObservation
}

func getRoutingObservation(c *gin.Context) *routingObservation {
	if c == nil {
		return nil
	}
	v, _ := c.Get(routingObservationKey)
	o, _ := v.(*routingObservation)
	return o
}

// Fingerprints are domain separated and scoped to the authenticated user/token.
// Matching across nodes/restarts requires the same configured CRYPTO_SECRET.
func routingFingerprint(c *gin.Context, domain, value string) string {
	value = fmt.Sprintf("routing-observation-v1\x00%d\x00%d\x00%s\x00%s", c.GetInt("id"), c.GetInt("token_id"), domain, value)
	return common.GenerateHMAC(value)[:32]
}

func BeginRoutingObservation(c *gin.Context, model string) {
	if c == nil || len(model) > 256 || !(strings.HasPrefix(model, "claude-") || (strings.HasPrefix(model, "gpt-") && !strings.HasPrefix(model, "gpt-image"))) {
		return
	}
	o := &routingObservation{data: RoutingObservation{Version: 1, Model: model, Lookup: "not_evaluated", Outcome: "not_completed", WriteResult: "not_attempted"}}
	c.Set(routingObservationKey, o)
	if c.Request != nil {
		for _, header := range []string{"Session_id", "Session-Id", "Thread_id", "Thread-Id"} {
			ObserveRoutingSignal(c, "header:"+strings.ToLower(header), c.GetHeader(header))
		}
	}
}

func ObserveRoutingSignal(c *gin.Context, source, value string) {
	o := getRoutingObservation(c)
	if o == nil || value == "" || len(value) > 4096 {
		return
	}
	fp := routingFingerprint(c, source, value)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.data.Signals == nil {
		o.data.Signals = make(map[string]string)
	}
	if len(o.data.Signals) < 8 {
		o.data.Signals[source] = fp
	}
}

// Read only already-parsed request fields; never reread the body or alter it.
func ObserveRoutingRequest(c *gin.Context, request any) {
	if getRoutingObservation(c) == nil {
		return
	}
	switch r := request.(type) {
	case *dto.OpenAIResponsesRequest:
		if r != nil && len(r.PromptCacheKey) <= 4096 {
			var key string
			if common.Unmarshal(r.PromptCacheKey, &key) == nil {
				ObserveRoutingSignal(c, "body:prompt_cache_key", key)
			}
		}
	case *dto.ClaudeRequest:
		if r != nil && len(r.Metadata) <= 8192 {
			value := gjson.GetBytes(r.Metadata, "user_id")
			if value.Type == gjson.String {
				ObserveRoutingSignal(c, "body:metadata.user_id", value.Str)
			}
		}
	}
}

func ObserveRoutingEvent(c *gin.Context, event RoutingEvent) {
	o := getRoutingObservation(c)
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.data.Events) >= 16 {
		o.data.Dropped++
		return
	}
	// Callers supply fixed reason codes, never upstream messages or headers.
	if len(event.Kind) > 48 || len(event.Reason) > 64 {
		o.data.Dropped++
		return
	}
	o.data.Events = append(o.data.Events, event)
}

func observeAffinityLookup(c *gin.Context, result string, channel int) {
	o := getRoutingObservation(c)
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.data.Lookup, o.data.Preferred = result, channel
	if meta, ok := getChannelAffinityMeta(c); ok {
		o.data.Rule = meta.RuleName
		if len(o.data.Rule) > 128 {
			o.data.Rule = o.data.Rule[:128]
		}
		o.data.BindingSource = meta.KeySourceType + ":" + meta.KeySourceKey + meta.KeySourcePath
		if len(o.data.BindingSource) > 128 {
			o.data.BindingSource = o.data.BindingSource[:128]
		}
		if len(meta.CacheKey) <= 8192 {
			o.data.BindingFP = routingFingerprint(c, "binding", meta.CacheKey)
		}
		o.data.TTL, o.data.SkipRetry = meta.TTLSeconds, meta.SkipRetry
	}
}

func ObserveRoutingOutcome(c *gin.Context, outcome string) {
	o := getRoutingObservation(c)
	if o == nil {
		return
	}
	o.mu.Lock()
	o.data.Outcome = outcome
	o.mu.Unlock()
}

func observeAffinityWrite(c *gin.Context, channel, ttl int, result string) {
	o := getRoutingObservation(c)
	if o == nil {
		return
	}
	o.mu.Lock()
	o.data.WriteChannel, o.data.WriteResult = channel, result
	o.data.TTL = ttl
	o.mu.Unlock()
}

func RoutingObservationSnapshot(c *gin.Context) *RoutingObservation {
	o := getRoutingObservation(c)
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	copy := o.data
	copy.UserID, copy.TokenID = c.GetInt("id"), c.GetInt("token_id")
	copy.Group = RequestAccountingGroup(c)
	if len(copy.Group) > 128 {
		copy.Group = copy.Group[:128]
	}
	copy.FinalChannel = c.GetInt("channel_id")
	copy.Signals = make(map[string]string, len(o.data.Signals))
	for k, v := range o.data.Signals {
		copy.Signals[k] = v
	}
	copy.Events = append([]RoutingEvent(nil), o.data.Events...)
	return &copy
}

// Consume/error rows precede the actual affinity write. This final, request-ID
// correlated line records the write result without rewriting those database rows.
func FinishRoutingObservation(c *gin.Context) {
	snapshot := RoutingObservationSnapshot(c)
	if snapshot == nil {
		return
	}
	if c.Writer != nil {
		snapshot.HTTPStatus = c.Writer.Status()
	}
	if raw, err := common.Marshal(snapshot); err == nil {
		logger.LogInfo(c, "routing_observation "+string(raw))
	}
}
