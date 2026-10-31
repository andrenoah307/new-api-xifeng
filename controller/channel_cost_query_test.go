package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCostQueryAuthorizationAndCoverageContract(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.ChannelCostReportJob{}, &model.ChannelCostReportCheckpoint{}, &model.ChannelCostCheckpointBudget{}))
	root := model.User{Username: "live-query-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	identity, err := model.ChannelCostReportSourceIdentity(context.Background())
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ChannelCostReportJob{ID: "query-report", RequestKey: "query-report", SourceIdentity: identity, Cursor: 3700, Status: "completed", PolicyJSON: `{"start":100,"end":3700,"quota_per_unit":"500000","fallback_mode":"none"}`, RowsJSON: `[{"channel_id":1,"consume_count":1,"original_count":1,"consume_amount":"10","known_revenue":"10","original_cost":"12"}]`}).Error)
	call := func(role int, query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/channel-finance/query"+query, nil)
		c.Set("id", root.Id)
		c.Set("role", role)
		QueryChannelCostReports(c)
		return rec
	}
	response := call(common.RoleRootUser, "?start=100&end=3700")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"complete":true`)
	assert.Contains(t, response.Body.String(), `"known_profit":"-2"`)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	response = call(common.RoleRootUser, "?start=200&end=3700")
	assert.Contains(t, response.Body.String(), `"complete":false`)
	assert.Contains(t, response.Body.String(), `"total":null`)
	assert.Contains(t, response.Body.String(), `"missing":[{"start":200,"end":3700}]`)
	for _, role := range []int{common.RoleAdminUser, common.RoleCommonUser} {
		response = call(role, "?start=100&end=3700")
		assert.NotContains(t, response.Body.String(), `"known_profit"`)
	}
	require.NoError(t, db.Model(&root).Update("role", common.RoleAdminUser).Error)
	response = call(common.RoleRootUser, "?start=100&end=3700")
	assert.NotContains(t, response.Body.String(), `"known_profit"`, "stale root session must not read financial data")
}
