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
var rateLimitCapacityCardEnabled atomic.Bool

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

// IsRateLimitCapacityEnabled is the O(1) public-card pre-gate. The status
// endpoint is high traffic, so this reads the immutable snapshot directly
// instead of cloning all model and group rules.
func IsRateLimitCapacityEnabled() bool {
	snapshot := modelNameRPMSnapshot.Load()
	return rateLimitCapacityCardEnabled.Load() && snapshot != nil && snapshot.Enabled &&
		(len(snapshot.Models) > 0 || len(snapshot.Groups) > 0)
}

func IsRateLimitCapacityCardEnabled() bool {
	return rateLimitCapacityCardEnabled.Load()
}

func SetRateLimitCapacityCardEnabled(enabled bool) {
	rateLimitCapacityCardEnabled.Store(enabled)
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
