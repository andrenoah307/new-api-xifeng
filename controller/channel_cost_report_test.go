package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCostReportProfitCoverage(t *testing.T) {
	row := model.ChannelCostReportRow{ChannelID: 7, ConsumeCount: 10, OriginalCount: 6, FallbackCount: 2, MissingCount: 2, RefundCount: 1,
		ConsumeAmount: "100", RefundAmount: "5", OriginalCost: "20", FallbackCost: "10", KnownRevenue: "80", UnknownRevenue: "20"}
	result, err := projectChannelCostReportRow(row)
	require.NoError(t, err)
	assert.Equal(t, "30", result.Cost)
	assert.Equal(t, "95", result.NetAmount)
	assert.Equal(t, "50", result.KnownProfit)
	assert.Equal(t, "0.8", result.Coverage)
	require.NotNil(t, result.CostRatio)
	assert.Equal(t, "0.375", *result.CostRatio)
	assert.Nil(t, result.Profit)
	assert.Nil(t, result.ProfitMargin)

	row.OriginalCount, row.MissingCount, row.RefundCount = 8, 0, 0
	row.RefundAmount, row.UnknownRevenue, row.KnownRevenue = "0", "0", "100"
	result, err = projectChannelCostReportRow(row)
	require.NoError(t, err)
	require.NotNil(t, result.Profit)
	require.NotNil(t, result.ProfitMargin)
	assert.Equal(t, "70", *result.Profit)
	assert.Equal(t, "0.7", *result.ProfitMargin)
	assert.Equal(t, "1", result.Coverage)

	row.ConsumeAmount, row.KnownRevenue = "0", "0"
	result, err = projectChannelCostReportRow(row)
	require.NoError(t, err)
	assert.Equal(t, "-30", *result.Profit)
	assert.Nil(t, result.ProfitMargin)
	assert.Nil(t, result.CostRatio)
}

func TestChannelCostReportCreatesVersionAndReadsOnlyPublishedAggregates(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.ChannelCostReportJob{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	root := model.User{Username: "report-owner", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	channel := model.Channel{Name: "report-channel", Key: "must-not-appear"}
	require.NoError(t, db.Create(&channel).Error)
	call := func(handler gin.HandlerFunc, id, action, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel-finance/jobs", strings.NewReader(body))
		c.Params = gin.Params{{Key: "id", Value: id}, {Key: "action", Value: action}}
		c.Set("id", root.Id)
		c.Set("role", root.Role)
		handler(c)
		return rec
	}
	body := `{"request_key":"` + uuid.NewString() + `","start":100,"end":3700,"channel_id":0,"fallback_mode":"none","rates":[],"quota_per_unit":"1"}`
	created := call(CreateChannelCostReportJob, "", "", body)
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	var createdResponse struct {
		Data channelCostReportJobDTO
	}
	require.NoError(t, common.Unmarshal(created.Body.Bytes(), &createdResponse))
	job := createdResponse.Data.ChannelCostReportJob
	require.NotNil(t, job)
	assert.NotEqual(t, "1", createdResponse.Data.Policy.QuotaPerUnit, "conversion unit is frozen by server, never supplied by client")
	duplicate := call(CreateChannelCostReportJob, "", "", body)
	require.Equal(t, http.StatusOK, duplicate.Code, duplicate.Body.String())
	assert.Contains(t, duplicate.Body.String(), job.ID)
	assert.NotContains(t, duplicate.Body.String(), "request_key")
	assert.NotContains(t, duplicate.Body.String(), "rows_json")
	rows := []model.ChannelCostReportRow{{ChannelID: channel.Id, ConsumeCount: 2, OriginalCount: 1, FallbackCount: 1, ConsumeAmount: "10", RefundAmount: "0", OriginalCost: "2", FallbackCost: "1", KnownRevenue: "10", UnknownRevenue: "0"}}
	encoded, err := common.Marshal(rows)
	require.NoError(t, err)
	require.NoError(t, db.Model(job).Updates(map[string]any{"rows_json": string(encoded), "cursor_timestamp": 3700, "processed_windows": 1, "status": "completed"}).Error)
	previousLogDB := model.LOG_DB
	model.LOG_DB = nil
	t.Cleanup(func() { model.LOG_DB = previousLogDB })
	response := call(GetChannelCostReport, job.ID, "", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Contains(t, response.Body.String(), `"channel_name":"report-channel"`)
	assert.NotContains(t, response.Body.String(), "must-not-appear")
	var report struct {
		Data struct {
			Total    channelCostReportRowDTO
			Channels []channelCostReportRowDTO
		}
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &report))
	require.Len(t, report.Data.Channels, 1)
	require.NotNil(t, report.Data.Total.Profit)
	assert.Equal(t, "7", *report.Data.Total.Profit)
	assert.Equal(t, "3", report.Data.Total.Cost)
	require.NoError(t, db.Model(job).Update("status", "paused").Error)
	partial := call(GetChannelCostReport, job.ID, "", "")
	require.Equal(t, http.StatusOK, partial.Code)
	require.NoError(t, common.Unmarshal(partial.Body.Bytes(), &report))
	assert.Nil(t, report.Data.Total.Profit, "partial generation must not imply full-range profit")
	assert.Equal(t, "7", report.Data.Total.KnownProfit)
	assert.Nil(t, report.Data.Channels[0].Profit)
}

func TestChannelCostReportEndpointsRequireFreshRoot(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Username: "report-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	handlers := []gin.HandlerFunc{ListChannelCostReportJobs, CreateChannelCostReportJob, GetChannelCostReportJob, ControlChannelCostReportJob, GetChannelCostReport}
	call := func(role int, handler gin.HandlerFunc) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/channel-finance/jobs/test", nil)
		c.Set("id", user.Id)
		c.Set("role", role)
		handler(c)
		return rec
	}
	for _, role := range []int{1, 3, 5, 10, 99, 101} {
		for _, handler := range handlers {
			response := call(role, handler)
			assert.Equal(t, http.StatusServiceUnavailable, response.Code)
			assert.NotContains(t, response.Body.String(), "original_cost")
			assert.NotContains(t, response.Body.String(), "policy")
		}
	}
	require.NoError(t, db.Model(&user).Update("role", common.RoleAdminUser).Error)
	for _, handler := range handlers {
		assert.Equal(t, http.StatusServiceUnavailable, call(common.RoleRootUser, handler).Code)
	}
	require.NoError(t, db.Model(&user).Updates(map[string]any{"role": common.RoleRootUser, "status": common.UserStatusDisabled}).Error)
	for _, handler := range handlers {
		assert.Equal(t, http.StatusServiceUnavailable, call(common.RoleRootUser, handler).Code)
	}
}
