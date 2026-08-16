package setting

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitSuccessCount = 1000
var ModelRequestRateLimitGroup = map[string][2]int{}
var ModelRequestRateLimitMutex sync.RWMutex
var modelRequestRateLimitConfigVersion atomic.Uint64

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	newGroup := make(map[string][2]int)
	if err := common.Unmarshal([]byte(jsonStr), &newGroup); err != nil {
		return err
	}

	ModelRequestRateLimitMutex.Lock()
	ModelRequestRateLimitGroup = newGroup
	ModelRequestRateLimitMutex.Unlock()
	modelRequestRateLimitConfigVersion.Add(1)
	return nil
}

// ListGroupRateLimits returns a detached copy of the configured A1 limits.
func ListGroupRateLimits() map[string][2]int {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()
	copyOfLimits := make(map[string][2]int, len(ModelRequestRateLimitGroup))
	for group, limits := range ModelRequestRateLimitGroup {
		copyOfLimits[group] = limits
	}
	return copyOfLimits
}

func ListGroupRateLimitsWithVersion() (map[string][2]int, uint64) {
	for {
		versionBefore := ModelRequestRateLimitConfigVersion()
		limits := ListGroupRateLimits()
		if versionBefore == ModelRequestRateLimitConfigVersion() {
			return limits, versionBefore
		}
	}
}

func ModelRequestRateLimitConfigVersion() uint64 {
	return modelRequestRateLimitConfigVersion.Load()
}

// IsRateLimitCapacityEnabled is a cheap public-card pre-gate. It cannot know
// a requesting user's visible groups; the capacity endpoint remains the
// authoritative per-user visibility check and may return total == 0.
func IsRateLimitCapacityEnabled() bool {
	if ModelRequestRateLimitEnabled {
		ModelRequestRateLimitMutex.RLock()
		hasCapacity := ModelRequestRateLimitCount > 0 || ModelRequestRateLimitSuccessCount > 0
		if !hasCapacity {
			for _, limits := range ModelRequestRateLimitGroup {
				if limits[0] > 0 || limits[1] > 0 {
					hasCapacity = true
					break
				}
			}
		}
		ModelRequestRateLimitMutex.RUnlock()
		if hasCapacity {
			return true
		}
	}
	rules := ListModelNameRPMRules()
	if !rules.Enabled {
		return false
	}
	for _, rule := range rules.Models {
		if rule.GlobalRPM > 0 {
			return true
		}
	}
	return false
}

func GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return 0, 0, false
	}

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, false
	}
	return limits[0], limits[1], true
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	checkModelRequestRateLimitGroup := make(map[string][2]int)
	err := common.Unmarshal([]byte(jsonStr), &checkModelRequestRateLimitGroup)
	if err != nil {
		return err
	}
	for group, limits := range checkModelRequestRateLimitGroup {
		if limits[0] < 0 || limits[1] < 1 {
			return fmt.Errorf("group %s has negative rate limit values: [%d, %d]", group, limits[0], limits[1])
		}
		if limits[0] > math.MaxInt32 || limits[1] > math.MaxInt32 {
			return fmt.Errorf("group %s [%d, %d] has max rate limits value 2147483647", group, limits[0], limits[1])
		}
	}

	return nil
}
