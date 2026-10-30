package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCostRootOnlyAndVersionConflict(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}))
	user := model.User{Username: "cost-owner", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	channel := model.Channel{Name: "cost-channel", Key: "test-key"}
	require.NoError(t, db.Create(&channel).Error)
	call := func(role int, handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/channel-cost/1", strings.NewReader(body))
		c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
		c.Set("id", user.Id)
		c.Set("role", role)
		handler(c)
		return rec
	}
	for _, role := range []int{1, 3, 5, 10} {
		assert.Equal(t, http.StatusServiceUnavailable, call(role, UpdateChannelCost, `{"mode":"manual","ratio":0.25}`).Code)
		assert.Equal(t, http.StatusServiceUnavailable, call(role, GetChannelCost, "").Code)
	}
	response := call(common.RoleRootUser, UpdateChannelCost, `{"mode":"manual","ratio":0.25}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"ratio":0.25`)
	assert.NotContains(t, response.Body.String(), "test-key")
	stale, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", "replacement-key").Error)
	assert.Error(t, service.SaveChannelCost(httptest.NewRequest(http.MethodGet, "/", nil).Context(), stale, stale.GetCostSettings()), "a delayed probe must not overwrite a different credential")
	assert.Equal(t, http.StatusServiceUnavailable, call(common.RoleRootUser, UpdateChannelCost, `{"mode":"manual","ratio":0.8}`).Code, "stale editor must not overwrite a newer setting")
	require.NoError(t, db.Model(&user).Update("role", common.RoleAdminUser).Error)
	assert.Equal(t, http.StatusServiceUnavailable, call(common.RoleRootUser, GetChannelCost, "").Code, "stale root session must not read cost")
}

func TestLogCostAdminAndRootProjection(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Username: "log-cost", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.Log{UserId: user.Id, CreatedAt: 100, Type: 2, Other: `{"root_cost":{"amount_usd":"0.123"},"admin_info":{"use_channel":[1]}}`}).Error)
	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/log/?start_timestamp=1&end_timestamp=200", nil)
		c.Set("id", user.Id)
		c.Set("role", role)
		GetAllLogs(c)
		require.Equal(t, http.StatusOK, rec.Code)
		if role == common.RoleRootUser {
			assert.Contains(t, rec.Body.String(), "root_cost")
		} else {
			assert.NotContains(t, rec.Body.String(), "root_cost")
		}
		assert.Contains(t, rec.Body.String(), "admin_info")
	}
}
