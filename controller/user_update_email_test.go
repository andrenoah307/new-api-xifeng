package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 管理员编辑用户时，请求体未携带 email 必须保持原邮箱；只有显式传值才修改或解绑。
func TestUpdateUserEmailAbsentKeepsBinding(t *testing.T) {
	tests := []struct {
		name      string
		body      map[string]any
		wantEmail string
	}{
		{
			name:      "absent keeps existing email",
			body:      map[string]any{"group": "vip"},
			wantEmail: "bound@example.com",
		},
		{
			name:      "explicit empty clears email",
			body:      map[string]any{"group": "vip", "email": ""},
			wantEmail: "",
		},
		{
			name:      "explicit value is normalized",
			body:      map[string]any{"group": "vip", "email": "  New@Example.COM "},
			wantEmail: "new@example.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupUserAccountAuditDB(t)
			admin := &model.User{
				Username: "email-edit-admin",
				Password: "password",
				AffCode:  "email-edit-admin-code",
				Role:     common.RoleAdminUser,
				Status:   common.UserStatusEnabled,
			}
			target := &model.User{
				Username: "email-edit-target",
				Password: "password",
				AffCode:  "email-edit-target-code",
				Role:     common.RoleCommonUser,
				Status:   common.UserStatusEnabled,
				Email:    "bound@example.com",
				Group:    "default",
			}
			require.NoError(t, db.Create(admin).Error)
			require.NoError(t, db.Create(target).Error)

			body := map[string]any{"id": target.Id, "username": target.Username, "display_name": target.Username}
			for k, v := range test.body {
				body[k] = v
			}
			raw, err := common.Marshal(body)
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader(raw))
			ctx.Request.Header.Set("Content-Type", gin.MIMEJSON)
			ctx.Set("id", admin.Id)
			ctx.Set("username", admin.Username)
			ctx.Set("role", admin.Role)
			UpdateUser(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			var resp struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
			require.True(t, resp.Success, resp.Message)

			var stored model.User
			require.NoError(t, db.First(&stored, target.Id).Error)
			assert.Equal(t, test.wantEmail, stored.Email)
			assert.Equal(t, "vip", stored.Group)
		})
	}
}
