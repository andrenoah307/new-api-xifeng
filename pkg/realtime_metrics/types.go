package realtimemetrics

// InstanceGauges is the point-in-time load of one process. The collector never
// samples these itself: they live in middleware/common, which import this
// package, so main.go registers a provider closure at startup instead.
type InstanceGauges struct {
	ActiveRequests   int64 `json:"active_requests"`
	ActiveBodyBytes  int64 `json:"active_body_bytes"`
	MaxConcurrent    int64 `json:"max_concurrent"`
	MaxBodyBytes     int64 `json:"max_body_bytes"`
	CgroupPermille   int64 `json:"cgroup_permille"`
	CgroupTripped    bool  `json:"cgroup_tripped"`
	TripCount        int64 `json:"trip_count"`
	ForcedResetCount int64 `json:"forced_reset_count"`
	Goroutines       int64 `json:"goroutines"`
}

// InstanceSnapshot is one live process as seen by the dashboard.
type InstanceSnapshot struct {
	Node           string `json:"node"`
	LastSeenUnix   int64  `json:"last_seen_unix"`
	StaleSeconds   int64  `json:"stale_seconds"`
	InstanceGauges        // gauges are flattened into the instance object
}

// MinutePoint is one aggregated minute across every live instance.
type MinutePoint struct {
	MinuteUnix       int64 `json:"minute_unix"`
	Requests         int64 `json:"requests"`
	Success          int64 `json:"success"`
	Errors           int64 `json:"errors"`
	ClientGone       int64 `json:"client_gone"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	Quota            int64 `json:"quota"`
	RejGate          int64 `json:"rej_gate"`
	RejConcurrency   int64 `json:"rej_concurrency"`
	RejBody          int64 `json:"rej_body"`
	RejMemory        int64 `json:"rej_memory"`
	RejModelRPM      int64 `json:"rej_model_rpm"`
	RejUserRPM       int64 `json:"rej_user_rpm"`
}

// ChannelPoint is one channel's live load, summed across instances.
type ChannelPoint struct {
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Concurrency int64  `json:"concurrency"`
	Requests    int64  `json:"requests"`
	Errors      int64  `json:"errors"`
	WindowSecs  int64  `json:"window_secs"`
}

// Snapshot is the whole dashboard payload.
type Snapshot struct {
	RedisEnabled bool               `json:"redis_enabled"`
	NowUnix      int64              `json:"now_unix"`
	Instances    []InstanceSnapshot `json:"instances"`
	Totals       InstanceGauges     `json:"totals"`
	Series       []MinutePoint      `json:"series"`
	Channels     []ChannelPoint     `json:"channels"`
	// Degraded marks a payload that could not be read from Redis and therefore
	// describes this process only. The HTTP status stays 200 so the polling
	// dashboard does not raise a toast every refresh.
	Degraded bool   `json:"degraded"`
	Warning  string `json:"warning,omitempty"`
}

// Rejection kinds. These are the pre-relay gates that abort a request before it
// ever reaches a channel, so nothing else in the system records them.
const (
	RejectionGate        = "rej_gate"
	RejectionConcurrency = "rej_concurrency"
	RejectionBody        = "rej_body"
	RejectionMemory      = "rej_memory"
	RejectionModelRPM    = "rej_model_rpm"
	RejectionUserRPM     = "rej_user_rpm"
)
