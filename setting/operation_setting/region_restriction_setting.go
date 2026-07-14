package operation_setting

import (
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type RegionRestrictionSetting struct {
	Enabled       bool                `json:"enabled"`
	FilterConsole bool                `json:"filter_console"`
	BlockRelay    bool                `json:"block_relay"`
	BlockedModels map[string][]string `json:"blocked_models"`
	BlockedGroups map[string][]string `json:"blocked_groups"`
	BlockMessage  string              `json:"block_message"`
	// 控制台被屏蔽分组的展示文案（令牌管理/监控面板），与 BlockMessage（relay 403 错误体）语义不同
	ConsoleMessage string `json:"console_message"`
	XdbPath        string `json:"xdb_path"`
}

var regionRestrictionSetting = RegionRestrictionSetting{
	Enabled:        false,
	FilterConsole:  true,
	BlockRelay:     true,
	BlockedModels:  map[string][]string{},
	BlockedGroups:  map[string][]string{},
	BlockMessage:   "",
	ConsoleMessage: "",
	XdbPath:        "data/ip2region.xdb",
}

func init() {
	config.GlobalConfig.Register("region_restriction", &regionRestrictionSetting)
}

var getAllGroupNamesFunc func() []string

func SetGetAllGroupNamesFunc(f func() []string) {
	getAllGroupNamesFunc = f
}

func GetAllGroupNames() []string {
	if getAllGroupNamesFunc != nil {
		return getAllGroupNamesFunc()
	}
	return []string{}
}

func GetRegionRestrictionSetting() RegionRestrictionSetting {
	return regionRestrictionSetting
}

func IsRegionRestrictionEnabled() bool {
	return regionRestrictionSetting.Enabled
}

func IsRegionRestrictionConsoleEnabled() bool {
	return regionRestrictionSetting.Enabled && regionRestrictionSetting.FilterConsole
}

func IsRegionRestrictionRelayEnabled() bool {
	return regionRestrictionSetting.Enabled && regionRestrictionSetting.BlockRelay
}

// modelMatcher holds a pre-compiled match rule for a single model pattern.
type modelMatcher struct {
	matchAll bool
	prefix   string
	exact    string
}

func compileModelPatterns(patterns []string) []modelMatcher {
	matchers := make([]modelMatcher, 0, len(patterns))
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if p == "*" {
			matchers = append(matchers, modelMatcher{matchAll: true})
		} else if strings.HasSuffix(p, "*") {
			matchers = append(matchers, modelMatcher{prefix: strings.TrimSuffix(p, "*")})
		} else {
			matchers = append(matchers, modelMatcher{exact: p})
		}
	}
	return matchers
}

func matchModelAgainstMatchers(matchers []modelMatcher, modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range matchers {
		if m.matchAll {
			return true
		}
		if m.prefix != "" && strings.HasPrefix(modelName, m.prefix) {
			return true
		}
		if m.exact != "" && modelName == m.exact {
			return true
		}
	}
	return false
}

var (
	regionMatcherIndex     map[string][]modelMatcher
	regionMatcherIndexLock sync.RWMutex

	regionGroupMatcherIndex     map[string][]modelMatcher
	regionGroupMatcherIndexLock sync.RWMutex
)

// RebuildRegionRestrictionIndex rebuilds the pre-compiled matcher index from
// the current BlockedModels and BlockedGroups maps. Must be called after config changes.
func RebuildRegionRestrictionIndex() {
	// Rebuild model matcher index
	newModelIndex := make(map[string][]modelMatcher, len(regionRestrictionSetting.BlockedModels))
	for country, patterns := range regionRestrictionSetting.BlockedModels {
		upperCountry := strings.ToUpper(strings.TrimSpace(country))
		if upperCountry == "" {
			continue
		}
		matchers := compileModelPatterns(patterns)
		if len(matchers) > 0 {
			newModelIndex[upperCountry] = matchers
		}
	}
	regionMatcherIndexLock.Lock()
	regionMatcherIndex = newModelIndex
	regionMatcherIndexLock.Unlock()

	// Rebuild group matcher index
	newGroupIndex := make(map[string][]modelMatcher, len(regionRestrictionSetting.BlockedGroups))
	for country, patterns := range regionRestrictionSetting.BlockedGroups {
		upperCountry := strings.ToUpper(strings.TrimSpace(country))
		if upperCountry == "" {
			continue
		}
		matchers := compileModelPatterns(patterns)
		if len(matchers) > 0 {
			newGroupIndex[upperCountry] = matchers
		}
	}
	regionGroupMatcherIndexLock.Lock()
	regionGroupMatcherIndex = newGroupIndex
	regionGroupMatcherIndexLock.Unlock()

	common.SysLog(fmt.Sprintf(
		"region restriction index rebuilt: %d model countries, %d group countries",
		len(newModelIndex), len(newGroupIndex),
	))
}

// IsModelBlockedForCountry returns true if the given model is blocked for the
// specified country code according to the pre-compiled matcher index.
func IsModelBlockedForCountry(countryCode, modelName string) bool {
	if countryCode == "" || !regionRestrictionSetting.Enabled {
		return false
	}

	regionMatcherIndexLock.RLock()
	matchers, ok := regionMatcherIndex[strings.ToUpper(countryCode)]
	regionMatcherIndexLock.RUnlock()

	if !ok {
		return false
	}

	return matchModelAgainstMatchers(matchers, modelName)
}

// IsGroupBlockedForCountry returns true if the given group is blocked for the
// specified country code.
func IsGroupBlockedForCountry(countryCode, group string) bool {
	if countryCode == "" || group == "" || group == "auto" || !regionRestrictionSetting.Enabled {
		return false
	}

	regionGroupMatcherIndexLock.RLock()
	matchers, ok := regionGroupMatcherIndex[strings.ToUpper(countryCode)]
	regionGroupMatcherIndexLock.RUnlock()

	if !ok {
		return false
	}

	return matchModelAgainstMatchers(matchers, group)
}

// GetBlockedGroupsForCountry returns the list of group names that are blocked
// for the given country code. Used by /api/status to inform the frontend.
func GetBlockedGroupsForCountry(countryCode string) []string {
	if countryCode == "" || !regionRestrictionSetting.Enabled {
		return []string{}
	}

	regionGroupMatcherIndexLock.RLock()
	matchers, ok := regionGroupMatcherIndex[strings.ToUpper(countryCode)]
	regionGroupMatcherIndexLock.RUnlock()

	if !ok || len(matchers) == 0 {
		return []string{}
	}

	allGroups := GetAllGroupNames()
	blocked := make([]string, 0)
	for _, g := range allGroups {
		if matchModelAgainstMatchers(matchers, g) {
			blocked = append(blocked, g)
		}
	}
	return blocked
}
