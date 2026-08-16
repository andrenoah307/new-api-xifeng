package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/QuantumNous/new-api/setting"
	"golang.org/x/sync/singleflight"
)

const RateLimitCapacitySnapshotTTL = 5 * time.Second

// CapacityItem is one independent limiter bucket. A group item is a
// (model, group) pair; values for the same group across models are never
// combined because each pair has its own admission gate.
type CapacityItem struct {
	Model       string   `json:"model"`
	Group       string   `json:"group,omitempty"`
	Current     *int     `json:"current"`
	Limit       int      `json:"limit"`
	Unlimited   bool     `json:"unlimited"`
	Utilization *float64 `json:"utilization"`
	OverLimit   bool     `json:"over_limit"`
	Available   bool     `json:"available"`
}

type CapacityMetric struct {
	Current     *int     `json:"current"`
	Limit       int      `json:"limit"`
	Unlimited   bool     `json:"unlimited"`
	Utilization *float64 `json:"utilization"`
	OverLimit   bool     `json:"over_limit"`
	Available   bool     `json:"available"`
}

type CapacitySection struct {
	Items []CapacityItem `json:"items"`
	Total int            `json:"total"`
}

// SiteCapacitySnapshot is the cached, user-independent A2 view. The exported
// slices make the service straightforward to test with an injected backend;
// callers receive a detached copy from SiteSnapshot.
type SiteCapacitySnapshot struct {
	ObservedAt            time.Time
	ModelRPMVersion       uint64
	GroupRateLimitVersion uint64
	Global                []CapacityItem
	Groups                []CapacityItem
	InstanceOnly          bool
}

type cachedSiteCapacity struct {
	snapshot SiteCapacitySnapshot
	err      error
}

// RPMCapacityInspector is implemented by model_name_limiter's Redis and
// memory backends. It is deliberately tiny so tests can inject a deterministic
// fake without a Redis dependency.
type RPMCapacityInspector interface {
	Inspect(context.Context, []string) ([]int, error)
}

type modelNameRPMInspector struct{}

func (modelNameRPMInspector) Inspect(ctx context.Context, keys []string) ([]int, error) {
	return model_name_limiter.Inspect(ctx, keys)
}

// RateLimitCapacityService owns the five-second site snapshot and its
// singleflight refresh. Personal A1 data is intentionally read outside this
// cache because it is keyed by the authenticated user.
type RateLimitCapacityService struct {
	inspector     RPMCapacityInspector
	instanceOnly  func() bool
	personalRead  func(context.Context, int, string) (*PersonalCapacity, bool, string)
	clockMu       sync.RWMutex
	clock         func() time.Time
	cacheMu       sync.Mutex
	cache         *cachedSiteCapacity
	refreshSingle singleflight.Group
}

func NewRateLimitCapacityService(inspector RPMCapacityInspector) *RateLimitCapacityService {
	usingDefaultInspector := inspector == nil
	if inspector == nil {
		inspector = modelNameRPMInspector{}
	}
	instanceOnlyDetector := func() bool { return false }
	if usingDefaultInspector {
		instanceOnlyDetector = model_name_limiter.UsingMemoryBackend
	}
	return &RateLimitCapacityService{
		inspector:    inspector,
		instanceOnly: instanceOnlyDetector,
		personalRead: readPersonalCapacity,
		clock:        time.Now,
	}
}

func (s *RateLimitCapacityService) SetClock(clock func() time.Time) {
	if s == nil {
		return
	}
	s.clockMu.Lock()
	if clock == nil {
		s.clock = time.Now
	} else {
		s.clock = clock
	}
	s.clockMu.Unlock()
	s.cacheMu.Lock()
	s.cache = nil
	s.cacheMu.Unlock()
}

// SetInstanceOnlyDetector is primarily useful for deterministic tests and for
// embedders that expose a custom backend. Production uses the model-name
// limiter's memory-backend detector installed by the constructor.
func (s *RateLimitCapacityService) SetInstanceOnlyDetector(detector func() bool) {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	s.instanceOnly = detector
	s.cache = nil
	s.cacheMu.Unlock()
}

func (s *RateLimitCapacityService) SetPersonalReader(reader func(context.Context, int, string) (*PersonalCapacity, bool, string)) {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	if reader == nil {
		s.personalRead = readPersonalCapacity
	} else {
		s.personalRead = reader
	}
	s.cacheMu.Unlock()
}

func (s *RateLimitCapacityService) now() time.Time {
	s.clockMu.RLock()
	clock := s.clock
	s.clockMu.RUnlock()
	if clock == nil {
		return time.Now()
	}
	return clock()
}

// SiteSnapshot returns a detached five-second snapshot. A backend error still
// produces configured items with Current=nil, allowing the HTTP layer to
// return 200 while accurately marking the section degraded.
func (s *RateLimitCapacityService) SiteSnapshot(ctx context.Context) (SiteCapacitySnapshot, error) {
	if s == nil {
		return SiteCapacitySnapshot{}, fmt.Errorf("rate limit capacity service is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	modelVersion := setting.ModelNameRPMConfigVersion()
	groupVersion := setting.ModelRequestRateLimitConfigVersion()
	now := s.now().UTC()
	s.cacheMu.Lock()
	if cached := s.cache; cached != nil && cached.snapshot.ModelRPMVersion == modelVersion &&
		cached.snapshot.GroupRateLimitVersion == groupVersion && now.Sub(cached.snapshot.ObservedAt) < RateLimitCapacitySnapshotTTL {
		snapshot, err := cloneSiteSnapshot(cached.snapshot), cached.err
		s.cacheMu.Unlock()
		return snapshot, err
	}
	s.cacheMu.Unlock()

	flightKey := fmt.Sprintf("site:%d:%d", modelVersion, groupVersion)
	value, err, _ := s.refreshSingle.Do(flightKey, func() (interface{}, error) {
		// Recheck after joining the flight: another waiter may have populated the
		// cache while this caller was acquiring the singleflight key.
		latestModelVersion := setting.ModelNameRPMConfigVersion()
		latestGroupVersion := setting.ModelRequestRateLimitConfigVersion()
		latestNow := s.now().UTC()
		s.cacheMu.Lock()
		if cached := s.cache; cached != nil && cached.snapshot.ModelRPMVersion == latestModelVersion &&
			cached.snapshot.GroupRateLimitVersion == latestGroupVersion && latestNow.Sub(cached.snapshot.ObservedAt) < RateLimitCapacitySnapshotTTL {
			result := cloneSiteSnapshot(cached.snapshot)
			cachedErr := cached.err
			s.cacheMu.Unlock()
			return siteRefreshResult{snapshot: result, err: cachedErr}, nil
		}
		s.cacheMu.Unlock()

		result, refreshErr := s.refreshSite(ctx, latestNow)
		s.cacheMu.Lock()
		s.cache = &cachedSiteCapacity{snapshot: result, err: refreshErr}
		s.cacheMu.Unlock()
		return siteRefreshResult{snapshot: result, err: refreshErr}, nil
	})
	if err != nil {
		return SiteCapacitySnapshot{}, err
	}
	result, ok := value.(siteRefreshResult)
	if !ok {
		return SiteCapacitySnapshot{}, fmt.Errorf("invalid rate limit capacity refresh result")
	}
	return cloneSiteSnapshot(result.snapshot), result.err
}

type siteRefreshResult struct {
	snapshot SiteCapacitySnapshot
	err      error
}

func (s *RateLimitCapacityService) refreshSite(ctx context.Context, now time.Time) (SiteCapacitySnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rules, actualModelVersion := setting.ListModelNameRPMRulesWithVersion()
	_, actualGroupVersion := setting.ListGroupRateLimitsWithVersion()
	snapshot := SiteCapacitySnapshot{
		ObservedAt:            now,
		ModelRPMVersion:       actualModelVersion,
		GroupRateLimitVersion: actualGroupVersion,
	}
	if !rules.Enabled || len(rules.Models) == 0 {
		return snapshot, nil
	}

	modelNames := make([]string, 0, len(rules.Models))
	for modelName := range rules.Models {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)
	keys := make([]string, 0)
	// A2 has only site model and model+group buckets. Do not manufacture a
	// per-user RPM key here: it would add write amplification without changing
	// admission semantics.
	for _, modelName := range modelNames {
		keys = append(keys, model_name_limiter.ModelKey(modelName))
	}
	for _, modelName := range modelNames {
		groups := make([]string, 0, len(rules.Models[modelName].GroupRPM))
		for groupName, limit := range rules.Models[modelName].GroupRPM {
			if limit > 0 {
				groups = append(groups, groupName)
			}
		}
		sort.Strings(groups)
		for _, groupName := range groups {
			keys = append(keys, model_name_limiter.GroupKey(modelName, groupName))
		}
	}

	var counts []int
	var err error
	if s.inspector == nil {
		err = fmt.Errorf("capacity inspector is nil")
	} else {
		counts, err = s.inspector.Inspect(ctx, keys)
	}
	if err != nil {
		counts = nil
	}
	if err == nil && len(counts) != len(keys) {
		err = fmt.Errorf("capacity inspector returned %d counts for %d keys", len(counts), len(keys))
	}
	if err == nil {
		for _, count := range counts {
			if count < 0 {
				err = fmt.Errorf("capacity inspector returned a negative count")
				break
			}
		}
	}
	available := err == nil
	countIndex := 0
	for _, modelName := range modelNames {
		rule := rules.Models[modelName]
		var current *int
		if available {
			value := counts[countIndex]
			current = &value
		}
		snapshot.Global = append(snapshot.Global, makeCapacityItem(modelName, "", current, rule.GlobalRPM, available))
		countIndex++
	}
	for _, modelName := range modelNames {
		rule := rules.Models[modelName]
		groups := make([]string, 0, len(rule.GroupRPM))
		for groupName, limit := range rule.GroupRPM {
			if limit > 0 {
				groups = append(groups, groupName)
			}
		}
		sort.Strings(groups)
		for _, groupName := range groups {
			var current *int
			if available {
				value := counts[countIndex]
				current = &value
			}
			snapshot.Groups = append(snapshot.Groups, makeCapacityItem(modelName, groupName, current, rule.GroupRPM[groupName], available))
			countIndex++
		}
	}
	s.cacheMu.Lock()
	instanceOnlyDetector := s.instanceOnly
	s.cacheMu.Unlock()
	if instanceOnlyDetector != nil {
		snapshot.InstanceOnly = instanceOnlyDetector()
	}
	if err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func makeCapacityItem(modelName, groupName string, current *int, limit int, available bool) CapacityItem {
	item := CapacityItem{Model: modelName, Group: groupName, Current: current, Limit: limit, Available: available}
	prepareCapacityItem(&item)
	return item
}

func prepareCapacityItem(item *CapacityItem) {
	if item.Current != nil {
		// A non-nil current value is the in-memory representation of a readable
		// bucket; this also keeps the sorting helper convenient for table tests.
		item.Available = true
	}
	item.Unlimited = item.Limit == 0
	item.OverLimit = false
	item.Utilization = nil
	if item.Unlimited || !item.Available || item.Current == nil || item.Limit <= 0 {
		return
	}
	ratio := float64(*item.Current) / float64(item.Limit)
	item.Utilization = &ratio
	item.OverLimit = *item.Current > item.Limit
}

// sortCapacityItems applies the four documented stable ranking keys. It is
// package-local so table tests can exercise every tie-break without an HTTP
// fixture.
func sortCapacityItems(items []CapacityItem) {
	for i := range items {
		prepareCapacityItem(&items[i])
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Available != b.Available {
			return a.Available
		}
		if a.Unlimited != b.Unlimited {
			return !a.Unlimited
		}
		if a.OverLimit != b.OverLimit {
			return a.OverLimit
		}
		if a.Utilization != nil && b.Utilization != nil && *a.Utilization != *b.Utilization {
			return *a.Utilization > *b.Utilization
		}
		if a.Current != nil && b.Current != nil && *a.Current != *b.Current {
			return *a.Current > *b.Current
		}
		if a.Model != b.Model {
			return a.Model < b.Model
		}
		return a.Group < b.Group
	})
}

func cloneSiteSnapshot(source SiteCapacitySnapshot) SiteCapacitySnapshot {
	clone := source
	clone.Global = cloneCapacityItems(source.Global)
	clone.Groups = cloneCapacityItems(source.Groups)
	return clone
}

func cloneCapacityItems(source []CapacityItem) []CapacityItem {
	if source == nil {
		return nil
	}
	clone := make([]CapacityItem, len(source))
	for i, item := range source {
		clone[i] = item
		if item.Current != nil {
			value := *item.Current
			clone[i].Current = &value
		}
		if item.Utilization != nil {
			value := *item.Utilization
			clone[i].Utilization = &value
		}
	}
	return clone
}

type CapacityRequest struct {
	UserID    int
	UserGroup string
	Country   string
	IsAdmin   bool
	Scope     string
}

type RateLimitCapacityResponse struct {
	Success               bool                  `json:"-"` // supplied by the HTTP envelope
	Scope                 string                `json:"scope"`
	ObservedAt            time.Time             `json:"observed_at"`
	ModelRPMVersion       uint64                `json:"model_name_rpm_version"`
	GroupRateLimitVersion uint64                `json:"group_rate_limit_version"`
	Degraded              bool                  `json:"degraded"`
	Warning               string                `json:"warning,omitempty"`
	InstanceOnly          bool                  `json:"instance_only"`
	BackendScope          string                `json:"backend_scope"`
	Site                  *SiteCapacityResponse `json:"site,omitempty"`
	Personal              *PersonalCapacity     `json:"personal,omitempty"`
	Total                 int                   `json:"total"`
}

type SiteCapacityResponse struct {
	Global CapacitySection `json:"global"`
	Groups CapacitySection `json:"groups"`
}

type PersonalCapacity struct {
	Enabled       bool            `json:"enabled"`
	Status        string          `json:"status"`
	Group         string          `json:"group,omitempty"`
	WindowMinutes int             `json:"window_minutes"`
	InstanceOnly  bool            `json:"instance_only"`
	Total         *CapacityMetric `json:"total,omitempty"`
	Success       *CapacityMetric `json:"success,omitempty"`
}

// Get builds the user-facing response. Site candidates are filtered before
// sorting, so non-admin top-three results and totals cannot reveal hidden
// groups.
func (s *RateLimitCapacityService) Get(ctx context.Context, request CapacityRequest) RateLimitCapacityResponse {
	if s == nil {
		return NewRateLimitCapacityService(nil).Get(ctx, request)
	}
	if request.Scope != "all" {
		request.Scope = "top"
	}
	snapshot, snapshotErr := s.SiteSnapshot(ctx)
	response := RateLimitCapacityResponse{
		Success:               true,
		Scope:                 request.Scope,
		ObservedAt:            snapshot.ObservedAt,
		ModelRPMVersion:       snapshot.ModelRPMVersion,
		GroupRateLimitVersion: snapshot.GroupRateLimitVersion,
		Degraded:              snapshotErr != nil,
		InstanceOnly:          snapshot.InstanceOnly,
	}
	response.BackendScope = "global"
	if snapshot.InstanceOnly {
		response.BackendScope = "instance"
		response.Degraded = true
		response.Warning = "A2 backend is process-local; counts cover this instance only"
	}
	if snapshotErr != nil {
		response.Warning = joinWarnings(response.Warning, "A2 backend is unavailable: "+snapshotErr.Error())
	}

	global := cloneCapacityItems(snapshot.Global)
	groups := cloneCapacityItems(snapshot.Groups)
	visibleGroups := map[string]string{}
	if !request.IsAdmin {
		visibleGroups = GetVisibleUserGroups(request.UserGroup, request.Country)
	}
	filteredGroups := groups[:0]
	for _, item := range groups {
		if request.IsAdmin {
			filteredGroups = append(filteredGroups, item)
			continue
		}
		if _, ok := visibleGroups[item.Group]; ok {
			filteredGroups = append(filteredGroups, item)
		}
	}
	sortCapacityItems(global)
	sortCapacityItems(filteredGroups)
	globalTotal, groupTotal := len(global), len(filteredGroups)
	if request.Scope == "top" {
		if len(global) > 3 {
			global = global[:3]
		}
		if len(filteredGroups) > 3 {
			filteredGroups = filteredGroups[:3]
		}
	}
	if globalTotal > 0 || groupTotal > 0 {
		if global == nil {
			global = []CapacityItem{}
		}
		if filteredGroups == nil {
			filteredGroups = []CapacityItem{}
		}
		response.Site = &SiteCapacityResponse{
			Global: CapacitySection{Items: global, Total: globalTotal},
			Groups: CapacitySection{Items: filteredGroups, Total: groupTotal},
		}
	}
	response.Total = globalTotal + groupTotal

	var personal *PersonalCapacity
	var personalDegraded bool
	var personalWarning string
	if !setting.ModelRequestRateLimitEnabled {
		// Do this gate before selecting the reader so stale A1 keys cannot make a
		// disabled personal section look like a zero-valued active limiter.
		personal = &PersonalCapacity{
			Enabled:       false,
			Status:        "disabled",
			Group:         request.UserGroup,
			WindowMinutes: setting.ModelRequestRateLimitDurationMinutes,
		}
	} else {
		s.cacheMu.Lock()
		personalReader := s.personalRead
		s.cacheMu.Unlock()
		if personalReader == nil {
			personalReader = readPersonalCapacity
		}
		personal, personalDegraded, personalWarning = personalReader(ctx, request.UserID, request.UserGroup)
	}
	response.Personal = personal
	if personal != nil && personal.Status != "disabled" && personal.Status != "unconfigured" {
		// The personal A1 section is itself a visible capacity item. Count it so
		// the public status pre-gate cannot hide a card that has no A2 rules.
		response.Total++
	}
	if personalDegraded {
		response.Degraded = true
		response.Warning = joinWarnings(response.Warning, personalWarning)
	}
	return response
}

func joinWarnings(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + "; " + second
}

func readPersonalCapacity(ctx context.Context, userID int, userGroup string) (*PersonalCapacity, bool, string) {
	personal := &PersonalCapacity{Enabled: setting.ModelRequestRateLimitEnabled, Status: "disabled", Group: userGroup, WindowMinutes: setting.ModelRequestRateLimitDurationMinutes}
	if !setting.ModelRequestRateLimitEnabled {
		return personal, false, ""
	}
	groupLimits := setting.ListGroupRateLimits()
	configuredLimits, found := groupLimits[userGroup]
	totalLimit, successLimit := configuredLimits[0], configuredLimits[1]
	if !found {
		totalLimit = setting.ModelRequestRateLimitCount
		successLimit = setting.ModelRequestRateLimitSuccessCount
	}
	if !found && totalLimit == 0 && successLimit == 0 {
		personal.Status = "unconfigured"
		return personal, false, ""
	}
	personal.Status = "available"
	if totalLimit == 0 {
		personal.Total = &CapacityMetric{Limit: 0, Unlimited: true, Available: true}
	}
	if successLimit == 0 {
		personal.Success = &CapacityMetric{Limit: 0, Unlimited: true, Available: true}
	}
	if !common.RedisEnabled || common.RDB == nil {
		personal.Status = "unavailable"
		personal.InstanceOnly = true
		markPersonalUnavailable(personal, totalLimit, successLimit)
		return personal, true, "A1 backend is unavailable"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	userKey := strconv.Itoa(userID)
	degraded := false
	warning := ""
	if successLimit > 0 {
		length, err := common.RDB.LLen(ctx, "rateLimit:MRRLS:"+userKey).Result()
		if err != nil {
			degraded = true
			warning = joinWarnings(warning, "A1 success counter is unavailable")
			personal.Success = unavailableMetric(successLimit)
		} else {
			current := int(length)
			personal.Success = metricFromCurrent(current, successLimit, true)
		}
	}
	if totalLimit > 0 {
		minutes := int64(setting.ModelRequestRateLimitDurationMinutes)
		if minutes <= 0 || minutes > math.MaxInt64/60 {
			degraded = true
			warning = joinWarnings(warning, "A1 total counter configuration is invalid")
			personal.Total = unavailableMetric(totalLimit)
			personal.Status = "unavailable"
			return personal, degraded, warning
		}
		duration := minutes * 60
		if int64(totalLimit) > math.MaxInt64/duration {
			degraded = true
			warning = joinWarnings(warning, "A1 total counter configuration is invalid")
			personal.Total = unavailableMetric(totalLimit)
			personal.Status = "unavailable"
			return personal, degraded, warning
		}
		capacity := int64(totalLimit) * duration
		tb := limiter.New(ctx, common.RDB)
		tokens, exists, err := tb.Peek(ctx, "rateLimit:"+userKey, limiter.WithCapacity(capacity), limiter.WithRate(int64(totalLimit)), limiter.WithRequested(duration))
		if err != nil {
			degraded = true
			warning = joinWarnings(warning, "A1 total counter is unavailable")
			personal.Total = unavailableMetric(totalLimit)
		} else {
			current := 0
			if exists {
				// The Lua peek deliberately returns raw tokens. A1 configures the
				// bucket as capacity=totalLimit*duration, rate=totalLimit,
				// requested=duration, so the window usage seen by Allow is
				// (capacity-tokens)/duration. Keep this conversion on the Go side.
				if tokens < 0 {
					tokens = 0
				} else if tokens > capacity {
					tokens = capacity
				}
				used := capacity - tokens
				if used > 0 && duration > 0 {
					current = int(used / duration)
				}
			}
			personal.Total = metricFromCurrent(current, totalLimit, true)
		}
	}
	if degraded {
		personal.Status = "unavailable"
	}
	return personal, degraded, warning
}

func markPersonalUnavailable(personal *PersonalCapacity, totalLimit, successLimit int) {
	if totalLimit > 0 {
		personal.Total = unavailableMetric(totalLimit)
	}
	if successLimit > 0 {
		personal.Success = unavailableMetric(successLimit)
	}
}

func unavailableMetric(limit int) *CapacityMetric {
	return &CapacityMetric{Limit: limit, Available: false}
}

func metricFromCurrent(current, limit int, available bool) *CapacityMetric {
	metric := &CapacityMetric{Current: &current, Limit: limit, Available: available}
	metric.Unlimited = limit == 0
	if limit > 0 {
		ratio := float64(current) / float64(limit)
		metric.Utilization = &ratio
		metric.OverLimit = current > limit
	}
	return metric
}

var defaultRateLimitCapacityService = NewRateLimitCapacityService(nil)

func DefaultRateLimitCapacityService() *RateLimitCapacityService {
	return defaultRateLimitCapacityService
}

func GetRateLimitCapacity(ctx context.Context, request CapacityRequest) RateLimitCapacityResponse {
	return defaultRateLimitCapacityService.Get(ctx, request)
}
