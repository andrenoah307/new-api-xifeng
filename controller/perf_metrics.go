package controller

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/pkg/requestip"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	activeGroups := getVisiblePerfMetricGroupNames(c)
	result, err := perfmetrics.QuerySummaryAll(hours, activeGroups)
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
