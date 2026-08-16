package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/QuantumNous/new-api/service/user_model_rpm"
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
// singleflight refresh. Personal observations are read outside this cache
// because they are keyed by the authenticated user and have their own window.
type RateLimitCapacityService struct {
	inspector            RPMCapacityInspector
	instanceOnly         func() bool
	personalRead         func(context.Context, int) ([]user_model_rpm.ModelRPM, string, error)
	personalInstanceOnly func() bool
	clockMu              sync.RWMutex
	clock                func() time.Time
	cacheMu              sync.Mutex
	cache                *cachedSiteCapacity
	refreshSingle        singleflight.Group
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
		inspector:            inspector,
		instanceOnly:         instanceOnlyDetector,
		personalRead:         user_model_rpm.Inspect,
		personalInstanceOnly: user_model_rpm.UsingMemoryBackend,
		clock:                time.Now,
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

func (s *RateLimitCapacityService) SetPersonalReader(reader func(context.Context, int) ([]user_model_rpm.ModelRPM, string, error)) {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	if reader == nil {
		s.personalRead = user_model_rpm.Inspect
	} else {
		s.personalRead = reader
	}
	s.cacheMu.Unlock()
}

// SetPersonalInstanceOnlyDetector is useful for deterministic endpoint tests.
func (s *RateLimitCapacityService) SetPersonalInstanceOnlyDetector(detector func() bool) {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	s.personalInstanceOnly = detector
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
	Status        string                    `json:"status"`
	WindowSeconds int                       `json:"window_seconds"`
	ObservedAt    time.Time                 `json:"observed_at"`
	InstanceOnly  bool                      `json:"instance_only"`
	Total         int                       `json:"total"`
	Items         []user_model_rpm.ModelRPM `json:"items"`
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

	s.cacheMu.Lock()
	personalReader := s.personalRead
	personalInstanceOnlyDetector := s.personalInstanceOnly
	s.cacheMu.Unlock()
	if personalReader == nil {
		personalReader = user_model_rpm.Inspect
	}
	items, personalStatus, personalErr := personalReader(ctx, request.UserID)
	personal := &PersonalCapacity{
		Status:        personalStatus,
		WindowSeconds: user_model_rpm.WindowSeconds,
		ObservedAt:    s.now().UTC(),
		Items:         []user_model_rpm.ModelRPM{},
	}
	if user_model_rpm.Enabled() && personalInstanceOnlyDetector != nil {
		personal.InstanceOnly = personalInstanceOnlyDetector()
	}
	if personalErr != nil {
		personal.Status = "unavailable"
		response.Degraded = true
		response.Warning = joinWarnings(response.Warning, "personal backend is unavailable: "+personalErr.Error())
	}
	if personal.Status != "available" && personal.Status != "empty" && personal.Status != "overflow" && personal.Status != "unavailable" {
		personal.Status = "unavailable"
		if personalErr == nil {
			response.Degraded = true
			response.Warning = joinWarnings(response.Warning, "personal backend returned an unknown status")
		}
	}
	if personal.Status == "available" && personalErr == nil {
		if items != nil {
			personal.Items = append(personal.Items, items...)
		}
		user_model_rpm.SortByRPM(personal.Items)
		personal.Total = len(personal.Items)
	}
	// Empty, unavailable, and overflow deliberately expose no synthetic model
	// rows. In particular, overflow is a state marker rather than a partial list.
	response.Personal = personal
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

var defaultRateLimitCapacityService = NewRateLimitCapacityService(nil)

func DefaultRateLimitCapacityService() *RateLimitCapacityService {
	return defaultRateLimitCapacityService
}

func GetRateLimitCapacity(ctx context.Context, request CapacityRequest) RateLimitCapacityResponse {
	return defaultRateLimitCapacityService.Get(ctx, request)
}
