package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
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
