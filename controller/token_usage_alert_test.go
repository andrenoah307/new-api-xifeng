package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenUsageAlertPreviewAuthorizationAndNoWrites(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.TokenUsageAlertDelivery{}))
	user := model.User{Username: "preview-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	call := func(role int) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/option/token_usage_alerts/preview", strings.NewReader(`{"config":{"enabled":false,"rules":[],"subject":"{{username}}","body":"{{scope}} {{usage_percent}} {{reset_at}}"},"scope":"period"}`))
		c.Set("id", user.Id)
		c.Set("role", role)
		PreviewTokenUsageAlertEmail(c)
		return recorder
	}
	result := call(common.RoleRootUser)
	assert.Contains(t, result.Body.String(), `"success":true`)
	assert.Contains(t, result.Body.String(), "当前周期额度")
	assert.Equal(t, "no-store", result.Header().Get("Cache-Control"))
	var count int64
	require.NoError(t, db.Model(&model.TokenUsageAlertDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.Model(&model.Option{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.NotContains(t, call(common.RoleAdminUser).Body.String(), "当前周期额度")
	require.NoError(t, db.Model(&user).Update("role", common.RoleAdminUser).Error)
	assert.NotContains(t, call(common.RoleRootUser).Body.String(), "当前周期额度")
}
