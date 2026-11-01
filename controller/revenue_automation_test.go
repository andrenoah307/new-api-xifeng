package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRevenueAutomationFreshRootAuthorization(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.RevenueAutomation{}, &model.RevenueAutomationRun{}))
	root := model.User{Username: "automation-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	call := func(role int) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/channel-finance/automation", nil)
		c.Set("id", root.Id)
		c.Set("role", role)
		RevenueAutomation(c)
		return r
	}
	response := call(common.RoleRootUser)
	assert.Contains(t, response.Body.String(), `"enabled":false`)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	for _, role := range []int{common.RoleAdminUser, common.RoleCommonUser} {
		assert.NotContains(t, call(role).Body.String(), `"recipients"`)
	}
	require.NoError(t, db.Model(&root).Update("role", common.RoleAdminUser).Error)
	assert.NotContains(t, call(common.RoleRootUser).Body.String(), `"recipients"`)
}
