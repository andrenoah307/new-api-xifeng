package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// ModerationResult is the structured result returned to debug callers and
// (in summarised form) persisted to moderation_incidents. As of v3 the
// engine attaches the rule decision so the debug card can show admins which
// rules fired without re-running the upstream model.
type ModerationResult struct {
	RequestID         string                  `json:"request_id"`
	Flagged           bool                    `json:"flagged"`
	MaxScore          float64                 `json:"max_score"`
	MaxCategory       string                  `json:"max_category"`
	Categories        map[string]float64      `json:"categories"`
	AppliedTypes      map[string][]string     `json:"applied_types"`
	UpstreamLatencyMS int                     `json:"upstream_latency_ms"`
	UsedKeySuffix     string                  `json:"used_key_suffix"`
	Error             string                  `json:"error,omitempty"`
	CompletedAt       int64                   `json:"completed_at"`
	Decision          *types.ModerationDecision `json:"decision,omitempty"`
}

type moderationEvent struct {
	OccurAt        int64
	Source         string // "relay" / "debug"
	UserID         int
	TokenID        int
	Username       string
	TokenName      string
	TokenMaskedKey string
	Group          string
	RequestID      string
	TextItems      []string
	Images         []string // already https URL or data URI
	Done           chan *ModerationResult
}

type moderationCenter struct {
	started    atomic.Bool
	mu         sync.Mutex
	queue      chan *moderationEvent
	keyRing    *ModerationKeyRing
	dropCount  atomic.Int64
	httpClient *http.Client
	debugStore sync.Map // requestID -> *moderationDebugEntry
	stopOnce   sync.Once
	stopCh     chan struct{}
}

type moderationDebugEntry struct {
	Result    *ModerationResult
	ExpiresAt int64
}

var globalModerationCenter = &moderationCenter{}

// StartModerationCenter is wired from main.go and reloads when config changes.
func StartModerationCenter() {
	globalModerationCenter.start()
}

// ReloadModerationConfig replaces the in-memory key ring with whatever is
// currently in operation_setting. Called from controller.UpdateModerationConfig.
func ReloadModerationConfig() {
	cfg := operation_setting.GetModerationSetting()
	globalModerationCenter.applyConfig(cfg)
}

func (m *moderationCenter) start() {
	if m.started.Load() {
		return
	}
	cfg := operation_setting.GetModerationSetting()
	m.queue = make(chan *moderationEvent, cfg.EventQueueSize)
	m.stopCh = make(chan struct{})
	m.httpClient = &http.Client{Timeout: time.Duration(cfg.HTTPTimeoutMS) * time.Millisecond}
	m.keyRing = NewModerationKeyRing(cfg.APIKeys)
	if err := ReloadModerationRules(); err != nil {
		common.SysError("moderation reload rules failed: " + err.Error())
	}
	if common.IsMasterNode {
		if err := SeedDefaultModerationRules(); err != nil {
			common.SysError("moderation seed default rules failed: " + err.Error())
		}
		// Reload after seed in case this was a fresh install.
		if err := ReloadModerationRules(); err != nil {
			common.SysError("moderation reload rules after seed failed: " + err.Error())
		}
	}
	for i := 0; i < cfg.WorkerCount; i++ {
		go m.runWorker()
	}
	if common.IsMasterNode {
		go m.runRetentionLoop()
		go m.runDebugSweeper()
	}
	m.started.Store(true)
}

func (m *moderationCenter) applyConfig(cfg *operation_setting.ModerationSetting) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keyRing != nil {
		m.keyRing.Reset(cfg.APIKeys)
	}
	if m.httpClient != nil {
		m.httpClient.Timeout = time.Duration(cfg.HTTPTimeoutMS) * time.Millisecond
	}
}

// EnqueueModerationFromRelay is the relay-side hook; it copies the data it
// needs out of gin context and gopool.Go's the rest. Designed to be called
// from a defer block; never blocks the relay path.
func EnqueueModerationFromRelay(c *gin.Context, info *relaycommon.RelayInfo, meta *types.TokenCountMeta) {
	if info == nil || meta == nil {
		return
	}
	cfg := operation_setting.GetModerationSetting()
	if !operation_setting.IsModerationEnabledForGroup(cfg, info.RiskGroup) {
		return
	}
	if cfg.SamplingRatePercent <= 0 {
		return
	}
	if cfg.SamplingRatePercent < 100 {
		// random sampling — mathrand is fine here, it's a sampling decision
		// not a security gate.
		if mathrand.Intn(100) >= cfg.SamplingRatePercent {
			return
		}
	}
	text := strings.TrimSpace(meta.CombineText)
	images := make([]string, 0, len(meta.Files))
	for _, f := range meta.Files {
		if f == nil || f.FileType != types.FileTypeImage {
			continue
		}
		raw := f.GetRawData()
		if strings.TrimSpace(raw) == "" {
			continue
		}
		images = append(images, raw)
	}
	if text == "" && len(images) == 0 {
		return
	}
	username := ""
	tokenName := ""
	if c != nil && c.Request != nil {
		username = c.GetString("username")
		tokenName = c.GetString("token_name")
	}
	tokenMasked := model.MaskTokenKey(info.TokenKey)
	requestID := info.RequestId
	group := info.RiskGroup
	userID := info.UserId
	tokenID := info.TokenId
	occurAt := common.GetTimestamp()
	gopool.Go(func() {
		globalModerationCenter.enqueue(&moderationEvent{
			OccurAt:        occurAt,
			Source:         "relay",
			UserID:         userID,
			TokenID:        tokenID,
			Username:       username,
			TokenName:      tokenName,
			TokenMaskedKey: tokenMasked,
			Group:          group,
			RequestID:      requestID,
			TextItems:      []string{text},
			Images:         images,
		})
	})
}

// SubmitModerationDebug is invoked from the admin debug card. It enqueues a
// debug event with a Done channel and returns the request_id immediately so
// the frontend can poll GetModerationDebugResult.
//
// group selects the group context for rule evaluation:
//   - non-empty:  evaluate against that group's bound rules (mirrors how the
//                 production relay path would judge the same input). The
//                 group does NOT need to be inside ModerationSetting
//                 .EnabledGroups — debug bypasses the whitelist gate so
//                 admins can rehearse before flipping it on.
//   - empty:      fall back to the legacy preview behavior, which scans
//                 every enabled rule regardless of group bindings — useful
//                 for "would any rule have fired?" rehearsals before any
//                 group is bound.
func SubmitModerationDebug(text string, images []string, group string) (string, error) {
	cfg := operation_setting.GetModerationSetting()
	if !cfg.Enabled {
		return "", errors.New("moderation is disabled in global config")
	}
	if globalModerationCenter.keyRing == nil || globalModerationCenter.keyRing.Size() == 0 {
		return "", errors.New("no moderation api key configured")
	}
	text = strings.TrimSpace(text)
	cleanedImages := make([]string, 0, len(images))
	for _, img := range images {
		img = strings.TrimSpace(img)
		if img == "" {
			continue
		}
		cleanedImages = append(cleanedImages, img)
	}
	if text == "" && len(cleanedImages) == 0 {
		return "", errors.New("text and images cannot both be empty")
	}
	requestID := "mod-debug-" + common.GetTimeString() + common.GetRandomString(6)
	occurAt := common.GetTimestamp()
	effectiveGroup := strings.TrimSpace(group)
	if effectiveGroup == "" {
		effectiveGroup = "__debug__"
	}
	event := &moderationEvent{
		OccurAt:   occurAt,
		Source:    "debug",
		Group:     effectiveGroup,
		RequestID: requestID,
		TextItems: []string{text},
		Images:    cleanedImages,
		Done:      make(chan *ModerationResult, 1),
	}
	if err := globalModerationCenter.enqueueDirect(event); err != nil {
		return "", err
	}
	return requestID, nil
}

// GetModerationDebugResult returns the result for the given debug request_id
// once the worker has produced it, or (nil, false) while still pending.
func GetModerationDebugResult(requestID string) (*ModerationResult, bool) {
	v, ok := globalModerationCenter.debugStore.Load(requestID)
	if !ok {
		return nil, false
	}
	entry, ok := v.(*moderationDebugEntry)
	if !ok || entry == nil {
		return nil, false
	}
	return entry.Result, true
}

// PreflightModerationHook is a placeholder for future enforce-mode support.
// In v2 it always allows; future versions may wait briefly on a debug-style
// Done channel before deciding. The signature is locked in now so callers in
// controller/relay.go don't need to change later.
func PreflightModerationHook(ctx context.Context, info *relaycommon.RelayInfo, meta *types.TokenCountMeta) (allow bool, reason string) {
	_ = ctx
	_ = info
	_ = meta
	return true, ""
}

func (m *moderationCenter) enqueue(event *moderationEvent) {
	if event == nil || m.queue == nil {
		return
	}
	select {
	case m.queue <- event:
	default:
		m.dropCount.Add(1)
		if m.dropCount.Load()%100 == 1 {
			common.SysLog("moderation event queue is full, dropping events")
		}
	}
}

// enqueueDirect blocks up to a short window for debug events so admins don't
// hit "queue full" in the debug UI. Production traffic still uses enqueue().
func (m *moderationCenter) enqueueDirect(event *moderationEvent) error {
	if event == nil || m.queue == nil {
		return errors.New("moderation center not started")
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case m.queue <- event:
		return nil
	case <-timer.C:
		return errors.New("moderation queue is full, please retry")
	}
}

func (m *moderationCenter) runWorker() {
	for event := range m.queue {
		if event == nil {
			continue
		}
		result := m.processEvent(event)
		m.recordResult(event, result)
	}
}

func (m *moderationCenter) recordResult(event *moderationEvent, result *ModerationResult) {
	cfg := operation_setting.GetModerationSetting()
	// Run the rule engine against the API response (no-op for debug events
	// that hit a non-existent group). Decision is attached to the result
	// before notifying any waiter so the debug card can render it.
	if result != nil && result.Error == "" && event.Group != "" && event.Group != "__debug__" {
		matched := EvaluateModerationRules(event.Group, result)
		decision := BuildModerationDecision(matched)
		result.Decision = &decision
	} else if result != nil && event.Source == "debug" {
		// Debug runs evaluate against EVERY enabled rule by default (group
		// is "__debug__", which never matches a configured rule). Provide a
		// "preview" view by re-running against all whitelisted groups so
		// the admin sees what would have happened in production.
		preview := previewModerationDecision(result, cfg)
		result.Decision = &preview
	}
	if event.Done != nil {
		select {
		case event.Done <- result:
		default:
		}
	}
	if event.Source == "debug" {
		ttl := time.Duration(cfg.DebugResultRetainMin) * time.Minute
		m.debugStore.Store(event.RequestID, &moderationDebugEntry{
			Result:    result,
			ExpiresAt: time.Now().Add(ttl).Unix(),
		})
	}
	if result == nil || result.Error != "" {
		// Failures are not persisted for relay traffic; debug callers see
		// the error via the polled Done channel.
		return
	}
	// v3: no fallback threshold — only persist when a rule fired. Debug
	// events always persist so admins can audit threshold-tuning sessions.
	if result.Decision == nil || result.Decision.Decision == ModerationDecisionAllow {
		if event.Source != "debug" {
			return
		}
	}
	matchedJSON := ""
	primaryRule := ""
	decision := ModerationDecisionAllow
	flagged := false
	if result.Decision != nil {
		matchedJSON = encodeRiskJSON(result.Decision.MatchedRules)
		primaryRule = result.Decision.PrimaryRuleName
		decision = result.Decision.Decision
		flagged = decision != ModerationDecisionAllow
	}
	incident := &model.ModerationIncident{
		CreatedAt:         event.OccurAt,
		UserID:            event.UserID,
		TokenID:           event.TokenID,
		Username:          event.Username,
		TokenName:         event.TokenName,
		TokenMaskedKey:    event.TokenMaskedKey,
		Group:             event.Group,
		RequestID:         event.RequestID,
		Model:             cfg.Model,
		Flagged:           flagged,
		MaxScore:          result.MaxScore,
		MaxCategory:       result.MaxCategory,
		Categories:        encodeRiskJSON(result.Categories),
		AppliedTypes:      encodeRiskJSON(result.AppliedTypes),
		InputSummary:      buildModerationInputSummary(event),
		UpstreamLatencyMS: result.UpstreamLatencyMS,
		Source:            event.Source,
		Decision:          decision,
		PrimaryRule:       primaryRule,
		MatchedRules:      matchedJSON,
	}
	if err := model.CreateModerationIncident(incident); err != nil {
		common.SysError("moderation create incident failed: " + err.Error())
	}
}

// previewModerationDecision powers the debug card: try evaluating the
// current API response against every enabled rule (regardless of group)
// and return the worst decision. The admin gets to see what would have
// fired, even though no real production group is involved.
func previewModerationDecision(result *ModerationResult, cfg *operation_setting.ModerationSetting) types.ModerationDecision {
	if result == nil {
		return types.ModerationDecision{Decision: ModerationDecisionAllow}
	}
	rules := currentModerationRules()
	matched := make([]types.ModerationMatchedRule, 0)
	for _, rule := range rules {
		if !rule.Raw.Enabled {
			continue
		}
		hit, conds := matchesModerationRule(rule, result)
		if !hit {
			continue
		}
		matched = append(matched, types.ModerationMatchedRule{
			RuleID:            rule.Raw.Id,
			Name:              rule.Raw.Name,
			Action:            rule.Raw.Action,
			ScoreWeight:       rule.Raw.ScoreWeight,
			MatchedConditions: conds,
		})
	}
	return BuildModerationDecision(matched)
}

func buildModerationInputSummary(event *moderationEvent) string {
	if event == nil {
		return ""
	}
	var sb strings.Builder
	for _, t := range event.TextItems {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if len(t) > 200 {
			t = t[:200] + "..."
		}
		sb.WriteString(t)
		sb.WriteString(" ")
	}
	if len(event.Images) > 0 {
		sb.WriteString(fmt.Sprintf("[+%d image(s)]", len(event.Images)))
	}
	return strings.TrimSpace(sb.String())
}

// processEvent issues the actual OpenAI call with retry/backoff. It always
// returns a non-nil *ModerationResult, populating Error on failure rather
// than nil so callers can distinguish "queue dropped" from "API failed".
func (m *moderationCenter) processEvent(event *moderationEvent) *ModerationResult {
	cfg := operation_setting.GetModerationSetting()
	result := &ModerationResult{
		RequestID:   event.RequestID,
		Categories:  map[string]float64{},
		AppliedTypes: map[string][]string{},
		CompletedAt: time.Now().Unix(),
	}
	if m.keyRing == nil || m.keyRing.Size() == 0 {
		result.Error = "no api key configured"
		return result
	}
	body, err := buildModerationRequest(cfg.Model, event.TextItems, event.Images)
	if err != nil {
		result.Error = "build request failed: " + err.Error()
		return result
	}
	maxAttempts := cfg.MaxRetries + 1
	for attempt := 0; attempt < maxAttempts; attempt++ {
		key, idx, ok := m.keyRing.NextKey()
		if !ok {
			result.Error = "all api keys are cooling down"
			return result
		}
		started := time.Now()
		resp, callErr := m.callOpenAI(cfg.BaseURL, key, body)
		latencyMS := int(time.Since(started).Milliseconds())
		result.UpstreamLatencyMS = latencyMS
		result.UsedKeySuffix = operation_setting.MaskModerationKey(key)
		if callErr != nil {
			// network / timeout — cool the key briefly and retry next.
			m.keyRing.MarkFailed(idx, time.Second*time.Duration(1<<attempt))
			result.Error = callErr.Error()
			continue
		}
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			parseErr := parseModerationResponse(resp.Body, result)
			_ = resp.Body.Close()
			if parseErr != nil {
				result.Error = "parse response failed: " + parseErr.Error()
				return result
			}
			result.Error = ""
			return result
		case resp.StatusCode == 429:
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			if retryAfter <= 0 {
				retryAfter = time.Second * time.Duration(1<<attempt)
			}
			m.keyRing.MarkFailed(idx, retryAfter)
			_ = resp.Body.Close()
			result.Error = "upstream 429"
			continue
		case resp.StatusCode >= 500:
			_ = resp.Body.Close()
			m.keyRing.MarkFailed(idx, time.Second*time.Duration(1<<attempt))
			result.Error = "upstream " + strconv.Itoa(resp.StatusCode)
			continue
		default:
			// 4xx other than 429 — input issue, do not retry.
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			result.Error = fmt.Sprintf("upstream %d: %s", resp.StatusCode, truncateModerationBody(string(respBody), 240))
			return result
		}
	}
	if result.Error == "" {
		result.Error = "max retries exhausted"
	}
	return result
}

func (m *moderationCenter) callOpenAI(baseURL, key string, body []byte) (*http.Response, error) {
	url := baseURL + "/v1/moderations"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	if m.httpClient == nil {
		m.httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return m.httpClient.Do(req)
}

func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(h); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

func truncateModerationBody(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// buildModerationRequest builds the OpenAI /v1/moderations payload. The
// official schema accepts:
//
//	{ "model": "...", "input": [
//	    {"type":"text", "text":"..."},
//	    {"type":"image_url", "image_url": {"url": "..."}}
//	]}
//
// We always emit the multi-modal array form (even for text-only) so the
// upstream parser is consistent.
func buildModerationRequest(model string, texts, images []string) ([]byte, error) {
	type imageURL struct {
		URL string `json:"url"`
	}
	type inputItem struct {
		Type     string    `json:"type"`
		Text     string    `json:"text,omitempty"`
		ImageURL *imageURL `json:"image_url,omitempty"`
	}
	items := make([]inputItem, 0, len(texts)+len(images))
	for _, t := range texts {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		items = append(items, inputItem{Type: "text", Text: t})
	}
	for _, img := range images {
		img = strings.TrimSpace(img)
		if img == "" {
			continue
		}
		items = append(items, inputItem{Type: "image_url", ImageURL: &imageURL{URL: img}})
	}
	if len(items) == 0 {
		return nil, errors.New("empty input")
	}
	payload := map[string]any{
		"model": model,
		"input": items,
	}
	return common.Marshal(payload)
}

// parseModerationResponse unwraps the official response schema:
//
//	{ "id": "...", "model": "...", "results": [
//	    { "flagged": bool,
//	      "categories": { "harassment": bool, ... },
//	      "category_scores": { "harassment": 0.123, ... },
//	      "category_applied_input_types": { "harassment": ["text"], ... } } ] }
//
// Multiple input items collapse into a single results entry; we fold the
// scores by max and bubble up the worst category so admins see the most
// alarming signal.
func parseModerationResponse(body io.Reader, out *ModerationResult) error {
	raw, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	type respShape struct {
		Results []struct {
			Flagged                   bool                `json:"flagged"`
			CategoryScores            map[string]float64  `json:"category_scores"`
			CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
		} `json:"results"`
	}
	var resp respShape
	if err := common.UnmarshalJsonStr(string(raw), &resp); err != nil {
		return err
	}
	if len(resp.Results) == 0 {
		return errors.New("empty results array")
	}
	maxScore := 0.0
	maxCategory := ""
	flagged := false
	categories := map[string]float64{}
	for _, result := range resp.Results {
		if result.Flagged {
			flagged = true
		}
		for cat, score := range result.CategoryScores {
			if score > categories[cat] {
				categories[cat] = score
			}
			if score > maxScore {
				maxScore = score
				maxCategory = cat
			}
		}
		for cat, types_ := range result.CategoryAppliedInputTypes {
			out.AppliedTypes[cat] = types_
		}
	}
	out.Flagged = flagged
	out.MaxScore = maxScore
	out.MaxCategory = maxCategory
	out.Categories = categories
	return nil
}

func (m *moderationCenter) runRetentionLoop() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cfg := operation_setting.GetModerationSetting()
		now := common.GetTimestamp()
		flaggedCutoff := now - int64(cfg.FlaggedRetentionHours)*3600
		benignCutoff := now - int64(cfg.BenignRetentionHours)*3600
		if err := model.DeleteExpiredModerationIncidents(flaggedCutoff, benignCutoff); err != nil {
			common.SysError("moderation cleanup failed: " + err.Error())
		}
	}
}

func (m *moderationCenter) runDebugSweeper() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now().Unix()
		m.debugStore.Range(func(key, value any) bool {
			entry, ok := value.(*moderationDebugEntry)
			if !ok || entry == nil || entry.ExpiresAt <= now {
				m.debugStore.Delete(key)
			}
			return true
		})
	}
}

// ModerationOverview powers the admin overview card in the new tab.
func ModerationOverview() (map[string]any, error) {
	cfg := operation_setting.GetModerationSetting()
	since := time.Now().Add(-24 * time.Hour).Unix()
	flagged24h, err := model.CountFlaggedModerationIncidentsSince(since)
	if err != nil {
		return nil, err
	}
	ruleCount, err := model.CountModerationRules()
	if err != nil {
		return nil, err
	}
	unconfigured, err := model.CountEnabledModerationRulesWithoutGroups()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"enabled":                 cfg.Enabled,
		"mode":                    cfg.Mode,
		"key_count":               len(cfg.APIKeys),
		"queue_dropped":           globalModerationCenter.dropCount.Load(),
		"flagged_24h":             flagged24h,
		"sampling_rate_percent":   cfg.SamplingRatePercent,
		"enabled_group_count":     len(cfg.EnabledGroups),
		"rule_count":              ruleCount,
		"unconfigured_rule_count": unconfigured,
	}, nil
}
