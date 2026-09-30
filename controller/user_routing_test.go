package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRoutingAdminPermissionsAndSave(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.UserRoutingPolicy{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 77, Username: "routing-user", Role: common.RoleCommonUser}).Error)
	t.Cleanup(func() { require.NoError(t, model.SaveUserRoutingRules(77, nil)) })
	for _, tc := range []struct {
		name         string
		role         int
		method, body string
		want         bool
	}{
		{"ordinary read", common.RoleCommonUser, http.MethodGet, "", false},
		{"ordinary write", common.RoleCommonUser, http.MethodPut, `{"rules":[]}`, false},
		{"missing rules", common.RoleAdminUser, http.MethodPut, `{}`, false},
		{"null rules", common.RoleAdminUser, http.MethodPut, `{"rules":null}`, false},
		{"invalid group", common.RoleAdminUser, http.MethodPut, `{"rules":[{"target_group":"nonexistent","enabled":true}]}`, false},
		{"auto target", common.RoleAdminUser, http.MethodPut, `{"rules":[{"target_group":"auto","enabled":true}]}`, false},
		{"save", common.RoleAdminUser, http.MethodPut, `{"rules":[{"model":"model-a","target_group":"default","enabled":true}]}`, true},
		{"read", common.RoleAdminUser, http.MethodGet, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(tc.method, "/api/user/77/routing", strings.NewReader(tc.body))
			c.Params = gin.Params{{Key: "id", Value: "77"}}
			c.Set("role", tc.role)
			c.Set("id", 78)
			UserRouting(c)
			var result struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
			assert.Equal(t, tc.want, result.Success)
		})
	}
	rules, err := model.GetUserRoutingRules(77)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "model-a", rules[0].Model)
}
