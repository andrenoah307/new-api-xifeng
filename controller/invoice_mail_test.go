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

func TestInvoiceMailEndpointsEnforceStoredRoleBeforeReadingUpload(t *testing.T) {
	db := setupOptionControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	actor := model.User{Username: "invoice-operator", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&actor).Error)
	for _, handler := range []gin.HandlerFunc{UploadInvoiceMailBatch, ListInvoiceMailBatches, GetInvoiceMailBatch, SendInvoiceMailBatchItem, CancelInvoiceMailBatch, QueueInvoiceMailBatch, PauseInvoiceMailBatch} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Set("id", actor.Id)
		c.Set("role", common.RoleRootUser)
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("invalid upload"))
		handler(c)
		assert.Equal(t, http.StatusForbidden, response.Code)
	}
	for _, role := range []int{common.RoleFinanceUser, common.RoleRootUser} {
		require.NoError(t, db.Model(&actor).Update("role", role).Error)
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Set("id", actor.Id)
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("invalid upload"))
		UploadInvoiceMailBatch(c)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	}
}
