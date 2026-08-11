package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type perfMetricsDetailPayload struct {
	Success bool                    `json:"success"`
	Data    perfmetrics.QueryResult `json:"data"`
}

type perfMetricsSummaryPayload struct {
	Success bool                         `json:"success"`
	Data    perfmetrics.SummaryAllResult `json:"data"`
}

func setupPerfMetricsControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalIsMasterNode := common.IsMasterNode
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	originalRedisEnabled := common.RedisEnabled
	originalDB := model.DB
	originalLogDB := model.LOG_DB

	common.IsMasterNode = false
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	db := model.DB
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.IsMasterNode = originalIsMasterNode
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})

	return db
}

func withPerfMetricsUserGroups(t *testing.T, groups map[string]string) {
	t.Helper()

	original := setting.UserUsableGroups2JSONString()
	groupBytes, err := common.Marshal(groups)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(groupBytes)))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(original))
	})
}

func withPerfMetricsRegionRestriction(t *testing.T, enabled, filterConsole bool, blockedGroups map[string][]string) {
	t.Helper()

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "region_restriction.") {
			saved[key] = value
		}
		return nil
	}))
	blockedBytes, err := common.Marshal(blockedGroups)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"region_restriction.enabled":        strconv.FormatBool(enabled),
		"region_restriction.filter_console": strconv.FormatBool(filterConsole),
		"region_restriction.blocked_groups": string(blockedBytes),
	}))
	operation_setting.RebuildRegionRestrictionIndex()

	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		operation_setting.RebuildRegionRestrictionIndex()
	})
}

func newPerfMetricsContext(query, country string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodGet, "/api/perf-metrics"+query, nil)
	if country != "" {
		request.Header.Set("Cf-Ipcountry", country)
	}
	context.Request = request
	return context, recorder
}

func seedPerfMetric(t *testing.T, db *gorm.DB, modelName, group string) {
	t.Helper()
	seedPerfMetricAt(t, db, modelName, group, time.Now().Unix())
}

func seedPerfMetricAt(t *testing.T, db *gorm.DB, modelName, group string, bucketTs int64) {
	t.Helper()
	require.NoError(t, db.Create(&model.PerfMetric{
		ModelName:      modelName,
		Group:          group,
		BucketTs:       bucketTs,
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 100,
	}).Error)
}

func decodePerfMetricsDetail(t *testing.T, recorder *httptest.ResponseRecorder) perfMetricsDetailPayload {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload perfMetricsDetailPayload
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	return payload
}

func decodePerfMetricsSummary(t *testing.T, recorder *httptest.ResponseRecorder) perfMetricsSummaryPayload {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload perfMetricsSummaryPayload
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	return payload
}

func TestGetPerfMetricsFiltersRegionBlockedGroups(t *testing.T) {
	db := setupPerfMetricsControllerTestDB(t)
	withPerfMetricsUserGroups(t, map[string]string{
		"custom-visible": "Custom visible",
		"default":        "Default",
		"vip":            "VIP",
	})
	withPerfMetricsRegionRestriction(t, true, true, map[string][]string{
		"CN": {"vip"},
	})

	const modelName = "perf-metrics-region-filter"
	seedPerfMetric(t, db, modelName, "default")
	seedPerfMetric(t, db, modelName, "vip")
	seedPerfMetric(t, db, modelName, "auto")

	context, recorder := newPerfMetricsContext("?model="+modelName, "CN")
	GetPerfMetrics(context)
	payload := decodePerfMetricsDetail(t, recorder)

	groups := make([]string, 0, len(payload.Data.Groups))
	for _, result := range payload.Data.Groups {
		groups = append(groups, result.Group)
	}
	assert.ElementsMatch(t, []string{"default", "auto"}, groups)
}

func TestGetPerfMetricsSummaryUsesVisibleGroups(t *testing.T) {
	db := setupPerfMetricsControllerTestDB(t)
	withPerfMetricsUserGroups(t, map[string]string{
		"custom-visible": "Custom visible",
		"default":        "Default",
		"vip":            "VIP",
	})
	withPerfMetricsRegionRestriction(t, true, true, map[string][]string{
		"CN": {"vip"},
	})

	seedPerfMetric(t, db, "perf-summary-visible", "default")
	seedPerfMetric(t, db, "perf-summary-custom-visible", "custom-visible")
	seedPerfMetric(t, db, "perf-summary-blocked", "vip")
	seedPerfMetric(t, db, "perf-summary-hidden", "svip")
	seedPerfMetric(t, db, "perf-summary-auto", "auto")

	context, recorder := newPerfMetricsContext("", "CN")
	GetPerfMetricsSummary(context)
	payload := decodePerfMetricsSummary(t, recorder)

	modelNames := make([]string, 0, len(payload.Data.Models))
	for _, summary := range payload.Data.Models {
		modelNames = append(modelNames, summary.ModelName)
	}
	assert.ElementsMatch(t, []string{
		"perf-summary-visible",
		"perf-summary-custom-visible",
		"perf-summary-auto",
	}, modelNames)
}

func TestGetPerfMetricsSummaryHonorsHoursQueryParam(t *testing.T) {
	db := setupPerfMetricsControllerTestDB(t)
	withPerfMetricsUserGroups(t, map[string]string{"default": "Default"})
	withPerfMetricsRegionRestriction(t, false, false, nil)

	const (
		recentModel = "perf-summary-hours-recent"
		oldModel    = "perf-summary-hours-old"
	)
	now := time.Now().Unix()
	seedPerfMetricAt(t, db, recentModel, "default", now-30*60)
	seedPerfMetricAt(t, db, oldModel, "default", now-40*60*60)

	defaultContext, defaultRecorder := newPerfMetricsContext("", "")
	GetPerfMetricsSummary(defaultContext)
	defaultPayload := decodePerfMetricsSummary(t, defaultRecorder)
	defaultModels := make([]string, 0, len(defaultPayload.Data.Models))
	for _, summary := range defaultPayload.Data.Models {
		defaultModels = append(defaultModels, summary.ModelName)
	}
	assert.ElementsMatch(t, []string{recentModel}, defaultModels)

	extendedContext, extendedRecorder := newPerfMetricsContext("?hours=48", "")
	GetPerfMetricsSummary(extendedContext)
	extendedPayload := decodePerfMetricsSummary(t, extendedRecorder)
	extendedModels := make([]string, 0, len(extendedPayload.Data.Models))
	for _, summary := range extendedPayload.Data.Models {
		extendedModels = append(extendedModels, summary.ModelName)
	}
	assert.ElementsMatch(t, []string{recentModel, oldModel}, extendedModels)
}

func TestGetPerfMetricsSummaryReturnsNoModelsWhenAllUsableGroupsAreRegionBlocked(t *testing.T) {
	db := setupPerfMetricsControllerTestDB(t)
	withPerfMetricsUserGroups(t, map[string]string{
		"default": "Default",
		"vip":     "VIP",
	})
	withPerfMetricsRegionRestriction(t, true, true, map[string][]string{
		"CN": {"*"},
	})

	seedPerfMetric(t, db, "perf-summary-hidden-only", "svip")

	context, recorder := newPerfMetricsContext("", "CN")
	GetPerfMetricsSummary(context)
	payload := decodePerfMetricsSummary(t, recorder)

	assert.Empty(t, payload.Data.Models)
}

func TestGetPerfMetricsRegionRestrictionDisabledKeepsGroupsVisible(t *testing.T) {
	tests := []struct {
		name          string
		enabled       bool
		filterConsole bool
	}{
		{name: "disabled", enabled: false, filterConsole: true},
		{name: "console filter disabled", enabled: true, filterConsole: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupPerfMetricsControllerTestDB(t)
			withPerfMetricsUserGroups(t, map[string]string{
				"default": "Default",
				"vip":     "VIP",
			})
			withPerfMetricsRegionRestriction(t, tt.enabled, tt.filterConsole, map[string][]string{
				"CN": {"vip"},
			})

			const modelName = "perf-metrics-region-disabled"
			seedPerfMetric(t, db, modelName, "default")
			seedPerfMetric(t, db, modelName, "vip")
			seedPerfMetric(t, db, modelName, "auto")

			context, recorder := newPerfMetricsContext("?model="+modelName, "CN")
			GetPerfMetrics(context)
			payload := decodePerfMetricsDetail(t, recorder)

			groups := make([]string, 0, len(payload.Data.Groups))
			for _, result := range payload.Data.Groups {
				groups = append(groups, result.Group)
			}
			assert.ElementsMatch(t, []string{"default", "vip", "auto"}, groups)
		})
	}
}

func TestGetPerfMetricsRegionRestrictionWithEmptyCountryKeepsGroupsVisible(t *testing.T) {
	db := setupPerfMetricsControllerTestDB(t)
	withPerfMetricsUserGroups(t, map[string]string{
		"default": "Default",
		"vip":     "VIP",
	})
	withPerfMetricsRegionRestriction(t, true, true, map[string][]string{
		"CN": {"vip"},
	})

	const modelName = "perf-metrics-region-empty-country"
	seedPerfMetric(t, db, modelName, "default")
	seedPerfMetric(t, db, modelName, "vip")
	seedPerfMetric(t, db, modelName, "auto")

	context, recorder := newPerfMetricsContext("?model="+modelName, "")
	GetPerfMetrics(context)
	payload := decodePerfMetricsDetail(t, recorder)

	groups := make([]string, 0, len(payload.Data.Groups))
	for _, result := range payload.Data.Groups {
		groups = append(groups, result.Group)
	}
	assert.ElementsMatch(t, []string{"default", "vip", "auto"}, groups)
}

func TestGetPerfMetricsRequiresModelParam(t *testing.T) {
	context, recorder := newPerfMetricsContext("", "")
	GetPerfMetrics(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "model is required")
}

func TestPerfMetricsVisibilityFollowsAuthenticatedUserGroup(t *testing.T) {
	db := setupPerfMetricsControllerTestDB(t)
	withPerfMetricsUserGroups(t, map[string]string{"default": "Default"})
	withPerfMetricsRegionRestriction(t, false, false, nil)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))

	user := &model.User{
		Username: "perf-visibility-enterprise-user",
		Group:    "enterprise",
	}
	require.NoError(t, db.Create(user).Error)
	seedPerfMetric(t, db, "perf-visibility-default", "default")
	seedPerfMetric(t, db, "perf-visibility-enterprise", "enterprise")

	tests := []struct {
		name           string
		setID          func(*gin.Context)
		expectedModels []string
		mustNotPanic   bool
	}{
		{
			name:           "anonymous",
			expectedModels: []string{"perf-visibility-default"},
		},
		{
			name: "authenticated enterprise user",
			setID: func(c *gin.Context) {
				c.Set("id", user.Id)
			},
			expectedModels: []string{
				"perf-visibility-default",
				"perf-visibility-enterprise",
			},
		},
		{
			name: "invalid id type is anonymous",
			setID: func(c *gin.Context) {
				c.Set("id", "not-an-int")
			},
			expectedModels: []string{"perf-visibility-default"},
			mustNotPanic:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context, recorder := newPerfMetricsContext("", "")
			if tt.setID != nil {
				tt.setID(context)
			}
			if tt.mustNotPanic {
				require.NotPanics(t, func() { GetPerfMetricsSummary(context) })
			} else {
				GetPerfMetricsSummary(context)
			}
			payload := decodePerfMetricsSummary(t, recorder)
			modelNames := make([]string, 0, len(payload.Data.Models))
			for _, summary := range payload.Data.Models {
				modelNames = append(modelNames, summary.ModelName)
			}
			assert.ElementsMatch(t, tt.expectedModels, modelNames)
		})
	}
}

func TestGetPerfMetricsReturnsInternalServerErrorWhenQueryFails(t *testing.T) {
	db := setupPerfMetricsControllerTestDB(t)
	withPerfMetricsUserGroups(t, map[string]string{"default": "Default"})
	withPerfMetricsRegionRestriction(t, false, false, nil)
	require.NoError(t, db.Migrator().DropTable(&model.PerfMetric{}))

	context, recorder := newPerfMetricsContext("?model=perf-query-error&hours=1", "")
	GetPerfMetrics(context)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, false, payload["success"])
	assert.NotEmpty(t, payload["message"])
}

func TestGetPerfMetricsSummaryReturnsInternalServerErrorWhenQueryFails(t *testing.T) {
	db := setupPerfMetricsControllerTestDB(t)
	withPerfMetricsUserGroups(t, map[string]string{"default": "Default"})
	withPerfMetricsRegionRestriction(t, false, false, nil)
	require.NoError(t, db.Migrator().DropTable(&model.PerfMetric{}))

	context, recorder := newPerfMetricsContext("", "")
	GetPerfMetricsSummary(context)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, false, payload["success"])
	assert.NotEmpty(t, payload["message"])
}

func TestPerfMetricsVisibleGroupsDeduplicatesAutoGroup(t *testing.T) {
	db := setupPerfMetricsControllerTestDB(t)
	withPerfMetricsUserGroups(t, map[string]string{
		"default": "Default",
		"auto":    "Auto",
	})
	withPerfMetricsRegionRestriction(t, false, false, nil)

	const modelName = "perf-metrics-auto-dedup"
	seedPerfMetric(t, db, modelName, "auto")

	context, recorder := newPerfMetricsContext("?model="+modelName, "")
	GetPerfMetrics(context)
	payload := decodePerfMetricsDetail(t, recorder)

	autoCount := 0
	for _, group := range payload.Data.Groups {
		if group.Group == "auto" {
			autoCount++
		}
	}
	assert.Equal(t, 1, autoCount)
}
