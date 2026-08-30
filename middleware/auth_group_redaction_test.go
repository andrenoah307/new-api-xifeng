package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenAuthRedactsDeprecatedTokenGroup(t *testing.T) {
	require.NoError(t, i18n.Init())
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalGroups := setting.UserUsableGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		common.RedisEnabled = originalRedisEnabled
		_ = setting.UpdateUserUsableGroupsByJSONString(originalGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalRatios)
	})

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"default":"Default",
		"retired-group-secret":"Retired"
	}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth-group-redaction.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	model.DB = db
	require.NoError(t, model.InitLogDB())
	require.NoError(t, db.Create(&model.User{Id: 901, Username: "redaction-user", Group: "default", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:             902,
		UserId:         901,
		Key:            "retiredtoken",
		Status:         common.TokenStatusEnabled,
		Group:          "retired-group-secret",
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Request.Header.Set("Authorization", "Bearer retiredtoken")
	TokenAuth()(c)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "retired-group-secret")
	assert.NotContains(t, recorder.Body.String(), "分组 retired-group-secret")
}

func TestTokenAuthRedactsUnlistedTokenGroup(t *testing.T) {
	require.NoError(t, i18n.Init())
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalGroups := setting.UserUsableGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		common.RedisEnabled = originalRedisEnabled
		_ = setting.UpdateUserUsableGroupsByJSONString(originalGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalRatios)
	})

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth-group-redaction-unlisted.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	model.DB = db
	require.NoError(t, model.InitLogDB())
	require.NoError(t, db.Create(&model.User{Id: 903, Username: "redaction-user", Group: "default", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:             904,
		UserId:         903,
		Key:            "unlistedtoken",
		Status:         common.TokenStatusEnabled,
		Group:          "unlisted-group-sentinel",
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Request.Header.Set("Authorization", "Bearer unlistedtoken")
	TokenAuth()(c)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "unlisted-group-sentinel")
	assert.NotContains(t, recorder.Body.String(), "{{")
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	errorBody, ok := response["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, string(types.ErrorCodeAccessDenied), errorBody["code"])
	assert.Equal(t, common.MessageWithRequestId(common.TranslateMessage(c, i18n.MsgDistributorGroupAccessDenied), ""), errorBody["message"])
}
