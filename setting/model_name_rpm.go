package setting

import (
	"fmt"
	"sort"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const (
	modelNameRPMMaxGlobal = 1_000_000
	modelNameRPMMaxModel  = 255
	modelNameRPMMaxGroup  = 64
)

// ModelNameRPMDecision is the result of looking up a model-name RPM rule.
type ModelNameRPMDecision struct {
	Matched   bool
	RuleModel string
	GlobalRPM int
	GroupRPM  int // 0 means that the group has no sub-limit.
}

type modelNameRPMRule struct {
	GlobalRPM int            `json:"global_rpm"`
	GroupRPM  map[string]int `json:"group_rpm,omitempty"`
}

type modelNameRPMConfig struct {
	Enabled bool                        `json:"enabled"`
	Models  map[string]modelNameRPMRule `json:"models"`
}

var modelNameRPMSnapshot atomic.Pointer[modelNameRPMConfig]

func init() {
	modelNameRPMSnapshot.Store(&modelNameRPMConfig{
		Models: make(map[string]modelNameRPMRule),
	})
}

// MatchModelNameRPM performs a lock-free lookup against the current immutable
// configuration snapshot. It deliberately does not consult the database or
// any model/ability catalog on a miss.
func MatchModelNameRPM(model, group string) ModelNameRPMDecision {
	snapshot := modelNameRPMSnapshot.Load()
	if snapshot == nil || !snapshot.Enabled {
		return ModelNameRPMDecision{}
	}

	rule, matched := snapshot.Models[model]
	ruleModel := model
	if !matched {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		rule, matched = snapshot.Models[normalizedModel]
		ruleModel = normalizedModel
	}
	if !matched {
		return ModelNameRPMDecision{}
	}

	groupRPM := 0
	if rule.GroupRPM != nil {
		groupRPM = rule.GroupRPM[group]
	}
	return ModelNameRPMDecision{
		Matched:   true,
		RuleModel: ruleModel,
		GlobalRPM: rule.GlobalRPM,
		GroupRPM:  groupRPM,
	}
}

// ModelNameRPMRateLimit2JSONString returns the current configuration as the
// JSON string stored in the option map.
func ModelNameRPMRateLimit2JSONString() string {
	snapshot := modelNameRPMSnapshot.Load()
	if snapshot == nil {
		snapshot = &modelNameRPMConfig{Models: make(map[string]modelNameRPMRule)}
	}

	jsonBytes, err := common.Marshal(snapshot)
	if err != nil {
		common.SysLog("error marshalling model name rpm rate limit: " + err.Error())
		return ""
	}
	return string(jsonBytes)
}

// UpdateModelNameRPMRateLimitByJSONString validates a complete temporary
// configuration before publishing it as the new immutable snapshot.
func UpdateModelNameRPMRateLimitByJSONString(jsonStr string) error {
	config, err := parseModelNameRPMRateLimit(jsonStr)
	if err != nil {
		return err
	}
	modelNameRPMSnapshot.Store(config)
	return nil
}

// CheckModelNameRPMRateLimit validates a configuration without changing the
// active snapshot.
func CheckModelNameRPMRateLimit(jsonStr string) error {
	_, err := parseModelNameRPMRateLimit(jsonStr)
	return err
}

func parseModelNameRPMRateLimit(jsonStr string) (*modelNameRPMConfig, error) {
	var parsed *modelNameRPMConfig
	if err := common.UnmarshalJsonStr(jsonStr, &parsed); err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, fmt.Errorf("model name rpm rate limit must be a JSON object")
	}
	if parsed.Models == nil {
		parsed.Models = make(map[string]modelNameRPMRule)
	}
	if err := validateModelNameRPMConfig(parsed); err != nil {
		return nil, err
	}
	return cloneModelNameRPMConfig(parsed), nil
}

func validateModelNameRPMConfig(config *modelNameRPMConfig) error {
	modelNames := make([]string, 0, len(config.Models))
	for modelName := range config.Models {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)

	normalizedNames := make(map[string]string, len(modelNames))
	for _, modelName := range modelNames {
		if err := validateModelNameRPMName("model", modelName, modelNameRPMMaxModel); err != nil {
			return err
		}

		rule := config.Models[modelName]
		if rule.GlobalRPM <= 0 {
			return fmt.Errorf("model %q global_rpm must be a positive integer", modelName)
		}
		if rule.GlobalRPM > modelNameRPMMaxGlobal {
			return fmt.Errorf("model %q global_rpm must not exceed %d", modelName, modelNameRPMMaxGlobal)
		}

		groupNames := make([]string, 0, len(rule.GroupRPM))
		for groupName := range rule.GroupRPM {
			groupNames = append(groupNames, groupName)
		}
		sort.Strings(groupNames)
		for _, groupName := range groupNames {
			if err := validateModelNameRPMName("group", groupName, modelNameRPMMaxGroup); err != nil {
				return err
			}
			groupRPM := rule.GroupRPM[groupName]
			if groupRPM < 1 {
				return fmt.Errorf("model %q group %q group_rpm must be at least 1", modelName, groupName)
			}
			if groupRPM > rule.GlobalRPM {
				return fmt.Errorf("model %q group %q group_rpm must not exceed global_rpm", modelName, groupName)
			}
		}

		normalizedName := ratio_setting.FormatMatchingModelName(modelName)
		if previousName, exists := normalizedNames[normalizedName]; exists && previousName != modelName {
			return fmt.Errorf("model names %q and %q normalize to the same model %q", previousName, modelName, normalizedName)
		}
		normalizedNames[normalizedName] = modelName
	}
	return nil
}

func validateModelNameRPMName(kind, name string, maxLength int) error {
	if name == "" {
		return fmt.Errorf("%s name must not be empty", kind)
	}
	if utf8.RuneCountInString(name) > maxLength {
		return fmt.Errorf("%s name must not exceed %d characters", kind, maxLength)
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("%s name %q must not contain whitespace or control characters", kind, name)
		}
	}
	return nil
}

func cloneModelNameRPMConfig(source *modelNameRPMConfig) *modelNameRPMConfig {
	clone := &modelNameRPMConfig{
		Enabled: source.Enabled,
		Models:  make(map[string]modelNameRPMRule, len(source.Models)),
	}
	for modelName, sourceRule := range source.Models {
		cloneRule := modelNameRPMRule{GlobalRPM: sourceRule.GlobalRPM}
		if sourceRule.GroupRPM != nil {
			cloneRule.GroupRPM = make(map[string]int, len(sourceRule.GroupRPM))
			for groupName, groupRPM := range sourceRule.GroupRPM {
				cloneRule.GroupRPM[groupName] = groupRPM
			}
		}
		clone.Models[modelName] = cloneRule
	}
	return clone
}
