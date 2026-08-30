package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDistributorRedactsAutoGroupSelectionFailure(t *testing.T) {
	require.NoError(t, i18n.Init())
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMemoryCache := common.MemoryCacheEnabled
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalRedisEnabled := common.RedisEnabled
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.MemoryCacheEnabled = originalMemoryCache
		common.SetDatabaseTypes(originalMainType, originalLogType)
		_ = setting.UpdateAutoGroupsByJsonString(originalAutoGroups)
		_ = setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups)
		common.RedisEnabled = originalRedisEnabled
	})

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[
		"internal-group-secret"
	]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"default":"Default",
		"internal-group-secret":"Internal"
	}`))
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "distributor-redaction.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	model.DB = db
	require.NoError(t, model.InitLogDB())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"internal-model-secret"}`))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyModelNameRPMChecked, true)

	Distribute()(c)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), string(types.ErrorCodeModelNotFound))
	assert.NotContains(t, recorder.Body.String(), "internal-group-secret")
	assert.NotContains(t, recorder.Body.String(), "auto(")
	assert.NotContains(t, recorder.Body.String(), "internal-model-secret")
	assert.NotContains(t, recorder.Body.String(), "no such table")
}

func TestDistributorRedactsDefaultRegionBlockMessage(t *testing.T) {
	require.NoError(t, i18n.Init())
	originalRegion := operation_setting.GetRegionRestrictionSetting()
	t.Cleanup(func() {
		blockedModels, _ := common.Marshal(originalRegion.BlockedModels)
		blockedGroups, _ := common.Marshal(originalRegion.BlockedGroups)
		require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("region_restriction"), map[string]string{
			"enabled":        strconv.FormatBool(originalRegion.Enabled),
			"filter_console": strconv.FormatBool(originalRegion.FilterConsole),
			"block_relay":    strconv.FormatBool(originalRegion.BlockRelay),
			"blocked_models": string(blockedModels),
			"blocked_groups": string(blockedGroups),
			"block_message":  originalRegion.BlockMessage,
		}))
		operation_setting.RebuildRegionRestrictionIndex()
	})

	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("region_restriction"), map[string]string{
		"enabled":        "true",
		"block_relay":    "true",
		"blocked_models": "{}",
		"blocked_groups": `{"US":["blocked-group-sentinel"]}`,
		"block_message":  "",
	}))
	operation_setting.RebuildRegionRestrictionIndex()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"safe-model"}`))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)
	c.Request.Header.Set("Cf-Ipcountry", "US")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "blocked-group-sentinel")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "blocked-group-sentinel")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "blocked-group-sentinel")

	Distribute()(c)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "blocked-group-sentinel")
	assert.NotContains(t, recorder.Body.String(), "{{")
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	errorBody, ok := response["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, common.MessageWithRequestId(common.TranslateMessage(c, i18n.MsgDistributorGroupRegionBlocked), ""), errorBody["message"])
}
