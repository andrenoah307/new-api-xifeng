package controller

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/pkg/requestip"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"golang.org/x/sync/singleflight"
)

// summary 是三个 perf-metrics 查询里唯一挂在页面级加载上的：公开的模型广场
// 每次访问都触发一次全量聚合，而 /api/perf-metrics 是点开单个模型才发一次。
// 因此缓存只加在这里——按查询参数给 /api/perf-metrics 建 map 的话，
// 键空间是 模型 × 分组 × 窗口，那本身就是个内存漏洞。
const perfMetricsSummaryTTL = 30 * time.Second

// 与 perf_card 同型的单槽签名快照：签名不匹配即回源。
// 最坏情况（有人刻意轮换 hours 制造键抖动）退化成没有缓存时的行为，不会更差。
type perfMetricsSummarySnapshot struct {
	signature string
	result    perfmetrics.SummaryAllResult
	expireAt  time.Time
}

var (
	perfMetricsSummaryCache  atomic.Pointer[perfMetricsSummarySnapshot]
	perfMetricsSummaryFlight singleflight.Group
)

// cachedPerfMetricsSummary 的缓存键必须含可见分组集合。
// GetPerfMetricsSummaryBucketsAll 的 GROUP BY 里没有分组这一维，分组集合改变的是
// 聚合值本身而不是"丢哪些行"，所以 perf_card 那套"先查全集、事后按请求过滤"
// 在这里不成立。groups 由 getVisiblePerfMetricGroupNames 产出，已排序。
func cachedPerfMetricsSummary(hours int, groups []string) (perfmetrics.SummaryAllResult, error) {
	signature := strconv.Itoa(hours) + "\x1e" + strings.Join(groups, "\x1f")
	if snap := perfMetricsSummaryCache.Load(); snap != nil && snap.signature == signature && time.Now().Before(snap.expireAt) {
		return snap.result, nil
	}

	loaded, err, _ := perfMetricsSummaryFlight.Do(signature, func() (any, error) {
		result, err := perfmetrics.QuerySummaryAll(hours, groups)
		if err != nil {
			// 回源失败不覆盖快照：一次瞬时 DB 抖动不该把缓存击穿成逐请求查询。
			return nil, err
		}
		perfMetricsSummaryCache.Store(&perfMetricsSummarySnapshot{
			signature: signature,
			result:    result,
			expireAt:  time.Now().Add(perfMetricsSummaryTTL),
		})
		return result, nil
	})
	if err != nil {
		return perfmetrics.SummaryAllResult{}, err
	}
	return loaded.(perfmetrics.SummaryAllResult), nil
}

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	activeGroups := getVisiblePerfMetricGroupNames(c)
	result, err := cachedPerfMetricsSummary(hours, activeGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: c.Query("group"),
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result.Groups = filterVisibleGroups(c, result.Groups)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// getVisiblePerfMetricGroupNames returns the groups visible to the current
// account and region. Anonymous requests use the default user's permissions.
// The synthetic "auto" group is always included.
func getVisiblePerfMetricGroupNames(c *gin.Context) []string {
	group := "default"
	if userID, exists := c.Get("id"); exists {
		if userID, ok := userID.(int); ok {
			if user, err := model.GetUserCache(userID); err == nil && user.Group != "" {
				group = user.Group
			}
		}
	}

	usable := service.GetUserUsableGroups(group)
	regionSetting := operation_setting.GetRegionRestrictionSetting()
	countryCode := ""
	if regionSetting.Enabled && regionSetting.FilterConsole {
		countryCode = requestip.GetClientCountry(c)
	}

	visible := make([]string, 0, len(usable)+1)
	for groupName := range usable {
		if groupName == "auto" {
			continue
		}
		if countryCode != "" && operation_setting.IsGroupBlockedForCountry(countryCode, groupName) {
			continue
		}
		visible = append(visible, groupName)
	}
	sort.Strings(visible)
	return append(visible, "auto")
}

// filterVisibleGroups keeps only the groups returned by the shared visibility
// policy; "auto" is retained by that policy as well.
func filterVisibleGroups(c *gin.Context, groups []perfmetrics.GroupResult) []perfmetrics.GroupResult {
	visibleGroups := getVisiblePerfMetricGroupNames(c)
	visibleSet := make(map[string]struct{}, len(visibleGroups))
	for _, groupName := range visibleGroups {
		visibleSet[groupName] = struct{}{}
	}
	return lo.Filter(groups, func(g perfmetrics.GroupResult, _ int) bool {
		_, ok := visibleSet[g.Group]
		return ok
	})
}
