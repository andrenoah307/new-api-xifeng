package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTicketUnreadLifecycle(t *testing.T) {
	db := setupOptionControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.Ticket{}, &model.TicketMessage{}, &model.TicketAttachment{}, &model.TicketInvoice{}, &model.TicketRefund{}))
	ticket := model.Ticket{UserId: 1, AssigneeId: 2, Subject: "Support", Type: model.TicketTypeGeneral}
	require.NoError(t, db.Create(&ticket).Error)
	message := model.TicketMessage{TicketId: ticket.Id, UserId: 1, Role: common.RoleCommonUser, Content: "Question", CreatedTime: 100}
	require.NoError(t, db.Create(&message).Error)
	request := func(path string, role, id int, detail bool) map[string]any {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", role)
		c.Set("id", id)
		c.Request = httptest.NewRequest("GET", path, nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		if detail {
			GetTicket(c)
		} else {
			GetAllTickets(c)
		}
		return decodeBody(t, w)
	}
	list := request("/?unread=true", common.RoleAdminUser, 3, false)
	data := list["data"].(map[string]any)
	items := data["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, true, items[0].(map[string]any)["has_unread_messages"])
	assert.Equal(t, false, request("/1", common.RoleCustomerServiceUser, 4, true)["success"])
	assert.Equal(t, true, request("/1", common.RoleCustomerServiceUser, 2, true)["success"])
	list = request("/?unread=true", common.RoleAdminUser, 3, false)
	assert.Empty(t, list["data"].(map[string]any)["items"])
	message.Id = 0
	message.Content = "Another question in the same second"
	require.NoError(t, db.Create(&message).Error)
	list = request("/?unread=true", common.RoleAdminUser, 3, false)
	assert.Len(t, list["data"].(map[string]any)["items"], 1)
}

func TestInvoiceExportIncludesRemark(t *testing.T) {
	db := setupOptionControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.Ticket{}, &model.TicketInvoice{}))
	ticket := model.Ticket{UserId: 1, Subject: "Invoice", Type: model.TicketTypeInvoice}
	require.NoError(t, db.Create(&ticket).Error)
	remark := "Line one, \"quoted\"\nLine two"
	require.NoError(t, db.Create(&model.TicketInvoice{TicketId: ticket.Id, UserId: 1, Remark: remark}).Error)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleAdminUser)
	c.Request = httptest.NewRequest("GET", "/", nil)
	GetInvoiceExportList(c)
	data := decodeBody(t, w)["data"].(map[string]any)
	items := data["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, remark, items[0].(map[string]any)["remark"])
}

func TestTicketStaffScopeAndBatchValidation(t *testing.T) {
	db := setupOptionControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.Ticket{}, &model.TicketMessage{}, &model.TicketInvoice{}, &model.TicketRefund{}))
	oldConfig := setting.TicketAssignConfig2JSONString()
	t.Cleanup(func() { require.NoError(t, setting.UpdateTicketAssignConfigFromJSON(oldConfig)) })
	require.NoError(t, setting.UpdateTicketAssignConfigFromJSON(`{"enabled":true,"rules":{"general":{"users":[2]}}}`))
	general := model.Ticket{UserId: 1, Subject: "General", Type: model.TicketTypeGeneral}
	invoice := model.Ticket{UserId: 1, Subject: "Invoice", Type: model.TicketTypeInvoice}
	require.NoError(t, db.Create(&general).Error)
	require.NoError(t, db.Create(&invoice).Error)
	require.NoError(t, db.Create(&model.TicketInvoice{TicketId: invoice.Id, UserId: 1, Remark: "Private"}).Error)
	for _, tc := range []struct {
		path, body             string
		batch, export, success bool
	}{
		{path: "/?scope=unassigned&type=invoice", success: true},
		{path: "/?unread=invalid", success: false},
		{path: "/invoice/export-list", export: true, success: true},
		{path: "/batch/status", body: `{"ids":[],"status":2}`, batch: true},
		{path: "/batch/status", body: `{"ids":[1],"status":9}`, batch: true},
		{path: "/batch/status", body: `{"ids":[1,2],"status":2}`, batch: true},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", common.RoleCustomerServiceUser)
		c.Set("id", 2)
		c.Request = httptest.NewRequest("PUT", tc.path, strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		switch {
		case tc.batch:
			BatchUpdateTicketStatus(c)
		case tc.export:
			GetInvoiceExportList(c)
		default:
			GetAllTickets(c)
		}
		body := decodeBody(t, w)
		assert.Equal(t, tc.success, body["success"], tc.path)
		if tc.success {
			assert.Empty(t, body["data"].(map[string]any)["items"], tc.path)
		}
	}
	require.NoError(t, db.First(&general, general.Id).Error)
	assert.Equal(t, model.TicketStatusOpen, general.Status)
}
