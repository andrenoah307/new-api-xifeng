package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMonitoringRouterTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMainType := common.MainDatabaseType()
	oldLogType := common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}))

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled = oldRedisEnabled
	})

	return db
}

func saveMonitoringRouterConfig(t *testing.T) {
	t.Helper()
	saved := make(map[string]string)
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "group_monitoring_setting.") {
			saved[key] = value
		}
		return nil
	}))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_monitoring_setting.enabled":           "true",
		"group_monitoring_setting.monitoring_groups": "[]",
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
}

func setMonitoringHeaderNavModules(t *testing.T, raw string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous, hadPrevious := common.OptionMap["HeaderNavModules"]
	common.OptionMap["HeaderNavModules"] = raw
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if hadPrevious {
			common.OptionMap["HeaderNavModules"] = previous
			return
		}
		delete(common.OptionMap, "HeaderNavModules")
	})
}

func monitoringRouterRequest(t *testing.T, engine *gin.Engine, accessToken string, userID int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/monitoring/public/groups", nil)
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("New-Api-User", strconv.Itoa(userID))
	}
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestMonitoringPublicRouteUsesHeaderNavModuleAuth(t *testing.T) {
	db := setupMonitoringRouterTestDB(t)
	saveMonitoringRouterConfig(t)

	accessToken := "monitoring-system-access-token"
	user := &model.User{
		Username:    "monitoring-route-user",
		Password:    "password",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AccessToken: &accessToken,
	}
	require.NoError(t, db.Create(user).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("monitoring-route-test"))))
	SetApiRouter(engine)

	setMonitoringHeaderNavModules(t, `{"monitoring":{"enabled":true,"requireAuth":true}}`)
	unauthenticated := monitoringRouterRequest(t, engine, "", 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	authenticated := monitoringRouterRequest(t, engine, accessToken, user.Id)
	require.Equal(t, http.StatusOK, authenticated.Code)

	setMonitoringHeaderNavModules(t, `{"monitoring":true}`)
	anonymousLegacy := monitoringRouterRequest(t, engine, "", 0)
	require.Equal(t, http.StatusOK, anonymousLegacy.Code)
}
