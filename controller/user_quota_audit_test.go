package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserAccountAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMainType := common.MainDatabaseType()
	oldLogType := common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	require.NoError(t, i18n.Init())

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func callManageUserQuotaAudit(t *testing.T, req ManageRequest, admin *model.User) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(req)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", gin.MIMEJSON)
	ctx.Request.RemoteAddr = "198.51.100.77:4321"
	ctx.Set("id", admin.Id)
	ctx.Set("username", admin.Username)
	ctx.Set("role", admin.Role)
	ManageUser(ctx)
	return recorder
}

func TestManageUserQuotaAuditUsesTargetSubject(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		action     string
		value      int
		paramNames []string
		paramValue map[string]int
	}{
		{
			name:       "add",
			mode:       "add",
			action:     "user.quota_add",
			value:      123456,
			paramNames: []string{"quota"},
			paramValue: map[string]int{"quota": 123456},
		},
		{
			name:       "subtract",
			mode:       "subtract",
			action:     "user.quota_subtract",
			value:      234567,
			paramNames: []string{"quota"},
			paramValue: map[string]int{"quota": 234567},
		},
		{
			name:       "override",
			mode:       "override",
			action:     "user.quota_override",
			value:      345678,
			paramNames: []string{"from", "to"},
			paramValue: map[string]int{"from": 1000000, "to": 345678},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupUserAccountAuditDB(t)
			admin := &model.User{
				Username: "quota-audit-admin",
				Password: "password",
				AffCode:  "quota-audit-admin-code",
				Role:     common.RoleRootUser,
				Status:   common.UserStatusEnabled,
				Quota:    1000000,
			}
			target := &model.User{
				Username: "quota-audit-target",
				Password: "password",
				AffCode:  "quota-audit-target-code",
				Role:     common.RoleCommonUser,
				Status:   common.UserStatusEnabled,
				Quota:    1000000,
			}
			require.NoError(t, db.Create(admin).Error)
			require.NoError(t, db.Create(target).Error)

			recorder := callManageUserQuotaAudit(t, ManageRequest{
				Id:     target.Id,
				Action: "add_quota",
				Mode:   test.mode,
				Value:  test.value,
			}, admin)
			require.Equal(t, http.StatusOK, recorder.Code)

			var logs []*model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeManage).Find(&logs).Error)
			require.Len(t, logs, 1)
			log := logs[0]
			assert.Equal(t, target.Id, log.UserId)
			assert.Equal(t, target.Username, log.Username)

			other, err := common.StrToMap(log.Other)
			require.NoError(t, err)
			op, ok := other["op"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, test.action, op["action"])
			params, ok := op["params"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, float64(target.Id), params["target_user_id"])
			for _, name := range test.paramNames {
				assert.Equal(t, float64(test.paramValue[name]), params[name])
			}
			adminInfo, ok := other["admin_info"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, float64(admin.Id), adminInfo["admin_id"])
		})
	}
}
