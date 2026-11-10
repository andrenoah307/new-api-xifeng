package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuickRepliesOnlyInAccessibleAdminTicketDetail(t *testing.T) {
	db := setupOptionControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.Ticket{}, &model.TicketMessage{}, &model.TicketAttachment{}))
	ticket := model.Ticket{UserId: 1, AssigneeId: 2, Type: model.TicketTypeGeneral, Subject: "Example", Status: model.TicketStatusOpen}
	require.NoError(t, db.Create(&ticket).Error)
	require.NoError(t, model.UpdateOption("TicketQuickReplies", "First reply\nSecond reply"))
	for _, tc := range []struct {
		name           string
		id, role       int
		admin, allowed bool
	}{
		{"owner", 1, common.RoleCommonUser, false, true},
		{"assigned staff", 2, common.RoleCustomerServiceUser, true, true},
		{"unassigned staff", 3, common.RoleCustomerServiceUser, true, false},
		{"administrator", 4, common.RoleAdminUser, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("id", tc.id)
			c.Set("role", tc.role)
			c.Params = gin.Params{{Key: "id", Value: "1"}}
			c.Request = httptest.NewRequest(http.MethodGet, "/ticket/1", nil)
			if tc.admin {
				GetTicket(c)
			} else {
				GetUserTicket(c)
			}
			body := decodeBody(t, w)
			require.Equal(t, tc.allowed, body["success"])
			if !tc.allowed {
				return
			}
			data, ok := body["data"].(map[string]any)
			require.True(t, ok)
			if tc.admin {
				assert.Equal(t, []any{"First reply", "Second reply"}, data["quick_replies"])
			} else {
				assert.NotContains(t, data, "quick_replies")
			}
		})
	}
}
