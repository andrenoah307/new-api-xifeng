package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"golang.org/x/sync/singleflight"
)

const RateLimitCapacitySnapshotTTL = 5 * time.Second

// CapacityItem is one independent limiter bucket. A group item is a
// (model, group) pair; values for the same group across models are never
// combined because each pair has its own admission gate.
type CapacityItem struct {
	Model       string   `json:"model"`
	Group       string   `json:"group"`
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

type CapacityGroup struct {
	Group string         `json:"group"`
	Items []CapacityItem `json:"items"`
	Total int            `json:"total"`
}

type CapacityGroupSection struct {
	Groups []CapacityGroup `json:"groups"`
	Total  int             `json:"total"`
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
	UserLimits            []UserRPMCapacityLimit
	InstanceOnly          bool
}

type UserRPMCapacityLimit struct {
	Model string
	Limit int
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
	inspector     RPMCapacityInspector
	instanceOnly  func() bool
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
	ruleModels := make(map[string]string, len(modelNames))
	for _, modelName := range modelNames {
		ruleModels[modelName] = ratio_setting.FormatMatchingModelName(modelName)
	}
	keys := make([]string, 0)
	// The site Redis read contains only site model and model+group buckets.
	// Per-user limits are cached as metadata and read with the authenticated ID.
	for _, modelName := range modelNames {
		rule := rules.Models[modelName]
		ruleModel := ruleModels[modelName]
		keys = append(keys, model_name_limiter.ModelKey(ruleModel))
		if rule.UserRPM > 0 {
			snapshot.UserLimits = append(snapshot.UserLimits, UserRPMCapacityLimit{Model: ruleModel, Limit: rule.UserRPM})
		}
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
			keys = append(keys, model_name_limiter.GroupKey(ruleModels[modelName], groupName))
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
		snapshot.Global = append(snapshot.Global, makeCapacityItem(ruleModels[modelName], "", current, rule.GlobalRPM, available))
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
			snapshot.Groups = append(snapshot.Groups, makeCapacityItem(ruleModels[modelName], groupName, current, rule.GroupRPM[groupName], available))
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

// capacityItemLess applies the documented ranking keys. Both item and group
// sorting use this comparison so their ranking stays identical.
func capacityItemLess(a, b CapacityItem) bool {
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
}

// sortCapacityItems applies the documented stable ranking keys. It is
// package-local so table tests can exercise every tie-break without an HTTP
// fixture.
func sortCapacityItems(items []CapacityItem) {
	for i := range items {
		prepareCapacityItem(&items[i])
	}
	sort.SliceStable(items, func(i, j int) bool {
		return capacityItemLess(items[i], items[j])
	})
}

func sortCapacityGroups(groups []CapacityGroup) {
	for i := range groups {
		sortCapacityItems(groups[i].Items)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if len(a.Items) == 0 || len(b.Items) == 0 {
			if len(a.Items) != len(b.Items) {
				return len(a.Items) > 0
			}
			return a.Group < b.Group
		}
		firstA, firstB := a.Items[0], b.Items[0]
		// Model names are not visible at the group layer; clear both item
		// identity fields so the fallback uses the visible group names directly.
		firstA.Model = ""
		firstA.Group = ""
		firstB.Model = ""
		firstB.Group = ""
		if capacityItemLess(firstA, firstB) {
			return true
		}
		if capacityItemLess(firstB, firstA) {
			return false
		}
		return a.Group < b.Group
	})
}

func cloneSiteSnapshot(source SiteCapacitySnapshot) SiteCapacitySnapshot {
	clone := source
	clone.Global = cloneCapacityItems(source.Global)
	clone.Groups = cloneCapacityItems(source.Groups)
	clone.UserLimits = append([]UserRPMCapacityLimit(nil), source.UserLimits...)
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
	// Total is the number of rate-limit buckets, not the number of groups.
	Total int `json:"total"`
}

type SiteCapacityResponse struct {
	Global CapacitySection      `json:"global"`
	Groups CapacityGroupSection `json:"groups"`
}

type PersonalCapacity struct {
	Status        string         `json:"status"`
	WindowSeconds int            `json:"window_seconds"`
	ObservedAt    time.Time      `json:"observed_at"`
	InstanceOnly  bool           `json:"instance_only"`
	Total         int            `json:"total"`
	Items         []CapacityItem `json:"items"`
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
		common.SysError(fmt.Sprintf("rate_limit_capacity: A2 snapshot unavailable: %v", snapshotErr))
		response.Warning = joinWarnings(response.Warning, "A2 backend is unavailable")
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
	grouped := make(map[string][]CapacityItem)
	for _, item := range filteredGroups {
		grouped[item.Group] = append(grouped[item.Group], item)
	}
	groupSections := make([]CapacityGroup, 0, len(grouped))
	groupBucketTotal := 0
	for groupName, items := range grouped {
		groupSections = append(groupSections, CapacityGroup{
			Group: groupName,
			Items: items,
			Total: len(items),
		})
		groupBucketTotal += len(items)
	}
	sortCapacityGroups(groupSections)
	globalTotal, groupTotal := len(global), len(groupSections)
	if request.Scope == "top" {
		if len(global) > 3 {
			global = global[:3]
		}
		if len(groupSections) > 3 {
			groupSections = groupSections[:3]
		}
		for i := range groupSections {
			if len(groupSections[i].Items) > 3 {
				groupSections[i].Items = groupSections[i].Items[:3]
			}
		}
	}
	if globalTotal > 0 || groupTotal > 0 {
		if global == nil {
			global = []CapacityItem{}
		}
		if groupSections == nil {
			groupSections = []CapacityGroup{}
		}
		response.Site = &SiteCapacityResponse{
			Global: CapacitySection{Items: global, Total: globalTotal},
			Groups: CapacityGroupSection{Groups: groupSections, Total: groupTotal},
		}
	}
	response.Total = globalTotal + groupBucketTotal

	if len(snapshot.UserLimits) == 0 {
		return response
	}

	personal := &PersonalCapacity{
		Status:        "ok",
		WindowSeconds: model_name_limiter.WindowSeconds,
		ObservedAt:    s.now().UTC(),
		InstanceOnly:  snapshot.InstanceOnly,
		Total:         len(snapshot.UserLimits),
		Items:         make([]CapacityItem, 0, len(snapshot.UserLimits)),
	}
	var personalCounts []int
	var personalErr error
	if request.UserID <= 0 {
		personalErr = fmt.Errorf("invalid authenticated user id")
	} else if s.inspector == nil {
		personalErr = fmt.Errorf("capacity inspector is nil")
	} else {
		keys := make([]string, len(snapshot.UserLimits))
		for i, limit := range snapshot.UserLimits {
			keys[i] = model_name_limiter.UserKey(limit.Model, request.UserID)
		}
		personalCounts, personalErr = s.inspector.Inspect(ctx, keys)
		if personalErr == nil && len(personalCounts) != len(keys) {
			personalErr = fmt.Errorf("capacity inspector returned %d personal counts for %d keys", len(personalCounts), len(keys))
		}
	}
	if personalErr == nil {
		for _, count := range personalCounts {
			if count < 0 {
				personalErr = fmt.Errorf("capacity inspector returned a negative personal count")
				break
			}
		}
	}
	personalAvailable := personalErr == nil
	if personalErr != nil {
		personal.Status = "unavailable"
		response.Degraded = true
		common.SysError(fmt.Sprintf("rate_limit_capacity: personal snapshot unavailable: %v", personalErr))
		response.Warning = joinWarnings(response.Warning, "personal backend is unavailable")
	}
	for i, limit := range snapshot.UserLimits {
		var current *int
		if personalAvailable {
			value := personalCounts[i]
			current = &value
		}
		personal.Items = append(personal.Items, makeCapacityItem(limit.Model, "", current, limit.Limit, personalAvailable))
	}
	sortCapacityItems(personal.Items)
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
