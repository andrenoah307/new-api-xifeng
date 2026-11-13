package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundAdjustmentReceiptUsesFreshFinanceAuthorization(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.Ticket{}, &model.TicketRefund{}, &model.RefundAdjustment{}, &model.Option{}))
	owner := model.User{Username: "adjustment-owner", AffCode: "adjustment-owner", Status: common.UserStatusEnabled}
	actor := model.User{Username: "adjustment-actor", AffCode: "adjustment-actor", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&actor).Error)
	require.NoError(t, db.Create(&model.Ticket{Id: 909, UserId: owner.Id, Type: model.TicketTypeRefund, Status: model.TicketStatusProcessing}).Error)
	require.NoError(t, db.Create(&model.TicketRefund{TicketId: 909, UserId: owner.Id, RefundStatus: model.RefundStatusPending}).Error)
	revision := strings.Repeat("a", 64)
	raw, err := common.Marshal(model.RefundAdjustmentPlan{TicketID: 909, Revision: revision, Applied: true, Reason: "verified evidence", BalanceAfter: "-19.848446", Calculation: &model.RefundExportData{CashCents: 5000}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.RefundAdjustment{TicketID: 909, UserID: owner.Id, SourceRevision: revision, ResultJSON: string(raw)}).Error)
	for _, role := range []int{common.RoleCommonUser, common.RoleCustomerServiceUser, common.RoleFinanceUser, common.RoleAdminUser, common.RoleRootUser} {
		t.Run(strconv.Itoa(role), func(t *testing.T) {
			require.NoError(t, db.Model(&actor).Update("role", role).Error)
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				response := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(response)
				c.Set("id", actor.Id)
				c.Set("role", common.RoleRootUser)
				c.Params = gin.Params{{Key: "id", Value: "909"}}
				c.Request = httptest.NewRequest(method, "/api/ticket/refund/909/adjustment", strings.NewReader(`{"revision":"`+revision+`","reason":"verified evidence"}`))
				RefundAdjustment(c)
				if role == common.RoleCommonUser || role == common.RoleCustomerServiceUser {
					assert.Equal(t, http.StatusForbidden, response.Code)
					assert.NotContains(t, response.Body.String(), "-19.848446")
					continue
				}
				assert.Equal(t, http.StatusOK, response.Code)
				assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
				assert.Contains(t, response.Body.String(), `"applied":true`)
				assert.Contains(t, response.Body.String(), `"cash_cents":5000`)
			}
		})
	}
	require.NoError(t, db.Model(&actor).Update("status", common.UserStatusDisabled).Error)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Set("id", actor.Id)
	c.Params = gin.Params{{Key: "id", Value: "909"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ticket/refund/909/adjustment", nil)
	RefundAdjustment(c)
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.NotContains(t, response.Body.String(), "verified evidence")
}
