package operation_setting

import (
	"sync"

	"github.com/QuantumNous/new-api/setting/config"
)

type GroupModelBlacklistSetting struct {
	Enabled       bool                `json:"enabled"`
	FilterConsole bool                `json:"filter_console"`
	BlockRelay    bool                `json:"block_relay"`
	BlockedModels map[string][]string `json:"blocked_models"`
	BlockMessage  string              `json:"block_message"`
}

var groupModelBlacklistSetting = GroupModelBlacklistSetting{
	Enabled:       false,
	FilterConsole: true,
	BlockRelay:    true,
	BlockedModels: map[string][]string{},
	BlockMessage:  "",
}

func init() {
	config.GlobalConfig.Register("group_model_blacklist", &groupModelBlacklistSetting)
}

func GetGroupModelBlacklistSetting() GroupModelBlacklistSetting {
	return groupModelBlacklistSetting
}

var (
	groupBlacklistMatcherIndex     map[string][]modelMatcher
	groupBlacklistMatcherIndexLock sync.RWMutex
)

func RebuildGroupModelBlacklistIndex() {
	newIndex := make(map[string][]modelMatcher, len(groupModelBlacklistSetting.BlockedModels))

	for group, patterns := range groupModelBlacklistSetting.BlockedModels {
		if group == "" {
			continue
		}
		matchers := compileModelPatterns(patterns)
		if len(matchers) > 0 {
			newIndex[group] = matchers
		}
	}

	groupBlacklistMatcherIndexLock.Lock()
	groupBlacklistMatcherIndex = newIndex
	groupBlacklistMatcherIndexLock.Unlock()
}

func IsModelBlockedForGroup(group, modelName string) bool {
	if group == "" || group == "auto" {
		return false
	}

	groupBlacklistMatcherIndexLock.RLock()
	matchers, ok := groupBlacklistMatcherIndex[group]
	groupBlacklistMatcherIndexLock.RUnlock()

	if !ok {
		return false
	}

	return matchModelAgainstMatchers(matchers, modelName)
}
