package operation_setting

import "strings"

var DemoSiteEnabled = false
var SelfUseModeEnabled = false

var AutomaticDisableKeywords = []string{
	"Your credit balance is too low",
	"This organization has been disabled.",
	"You exceeded your current quota",
	"Permission denied",
	"The security token included in the request is invalid",
	"Operation not allowed",
	"Your account is not authorized",
}

func AutomaticDisableKeywordsToString() string {
	return strings.Join(AutomaticDisableKeywords, "\n")
}

func AutomaticDisableKeywordsFromString(s string) {
	AutomaticDisableKeywords = []string{}
	ak := strings.Split(s, "\n")
	for _, k := range ak {
		k = strings.TrimSpace(k)
		k = strings.ToLower(k)
		if k != "" {
			AutomaticDisableKeywords = append(AutomaticDisableKeywords, k)
		}
	}
}

var AutomaticDisableWhitelist = []string{}

func AutomaticDisableWhitelistToString() string {
	return strings.Join(AutomaticDisableWhitelist, "\n")
}

func AutomaticDisableWhitelistFromString(s string) {
	AutomaticDisableWhitelist = []string{}
	parts := strings.Split(s, "\n")
	for _, k := range parts {
		k = strings.TrimSpace(k)
		k = strings.ToLower(k)
		if k != "" {
			AutomaticDisableWhitelist = append(AutomaticDisableWhitelist, k)
		}
	}
}

var HiddenModels = []string{}
var hiddenModelsSet = map[string]struct{}{}

func HiddenModelsToString() string {
	return strings.Join(HiddenModels, "\n")
}

func HiddenModelsFromString(s string) {
	list := []string{}
	set := map[string]struct{}{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			list = append(list, line)
			set[line] = struct{}{}
		}
	}
	HiddenModels = list
	hiddenModelsSet = set
}

func IsModelHidden(name string) bool {
	_, ok := hiddenModelsSet[name]
	return ok
}
