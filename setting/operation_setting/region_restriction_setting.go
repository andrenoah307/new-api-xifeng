package operation_setting

import (
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/setting/config"
)

type RegionRestrictionSetting struct {
	Enabled       bool                `json:"enabled"`
	FilterConsole bool                `json:"filter_console"`
	BlockRelay    bool                `json:"block_relay"`
	BlockedModels map[string][]string `json:"blocked_models"`
	BlockMessage  string              `json:"block_message"`
	XdbPath       string              `json:"xdb_path"`
}

var regionRestrictionSetting = RegionRestrictionSetting{
	Enabled:       false,
	FilterConsole: true,
	BlockRelay:    true,
	BlockedModels: map[string][]string{},
	BlockMessage:  "",
	XdbPath:       "data/ip2region.xdb",
}

func init() {
	config.GlobalConfig.Register("region_restriction", &regionRestrictionSetting)
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

var (
	regionMatcherIndex     map[string][]modelMatcher
	regionMatcherIndexLock sync.RWMutex
)

// RebuildRegionRestrictionIndex rebuilds the pre-compiled matcher index from
// the current BlockedModels map. Must be called after config changes.
func RebuildRegionRestrictionIndex() {
	newIndex := make(map[string][]modelMatcher, len(regionRestrictionSetting.BlockedModels))

	for country, patterns := range regionRestrictionSetting.BlockedModels {
		upperCountry := strings.ToUpper(strings.TrimSpace(country))
		if upperCountry == "" {
			continue
		}
		matchers := make([]modelMatcher, 0, len(patterns))
		for _, pattern := range patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			lowerPattern := strings.ToLower(pattern)
			if lowerPattern == "*" {
				matchers = append(matchers, modelMatcher{matchAll: true})
			} else if strings.HasSuffix(lowerPattern, "*") {
				matchers = append(matchers, modelMatcher{prefix: strings.TrimSuffix(lowerPattern, "*")})
			} else {
				matchers = append(matchers, modelMatcher{exact: lowerPattern})
			}
		}
		if len(matchers) > 0 {
			newIndex[upperCountry] = matchers
		}
	}

	regionMatcherIndexLock.Lock()
	regionMatcherIndex = newIndex
	regionMatcherIndexLock.Unlock()
}

// IsModelBlockedForCountry returns true if the given model is blocked for the
// specified country code according to the pre-compiled matcher index.
func IsModelBlockedForCountry(countryCode, modelName string) bool {
	if countryCode == "" {
		return false
	}

	regionMatcherIndexLock.RLock()
	matchers, ok := regionMatcherIndex[strings.ToUpper(countryCode)]
	regionMatcherIndexLock.RUnlock()

	if !ok {
		return false
	}

	lowerModel := strings.ToLower(modelName)
	for _, m := range matchers {
		if m.matchAll {
			return true
		}
		if m.prefix != "" && strings.HasPrefix(lowerModel, m.prefix) {
			return true
		}
		if m.exact != "" && lowerModel == m.exact {
			return true
		}
	}
	return false
}
