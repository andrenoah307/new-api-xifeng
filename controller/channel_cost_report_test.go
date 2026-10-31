package controller

import (
	"encoding/csv"
	"fmt"
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

func TestChannelCostReportTaskPaginationAndFilters(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ChannelCostReportJob{}, &model.SystemTask{}))
	root := model.User{Username: "report-list-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	for i := 0; i < 63; i++ {
		status := "completed"
		if i%2 == 0 {
			status = "paused"
		}
		job := model.ChannelCostReportJob{ID: fmt.Sprintf("report-%03d", i), RequestKey: uuid.NewString(), OwnerID: root.Id, CreatedAt: 100,
			Status: status, PolicyJSON: `{"start":1,"end":3601,"quota_per_unit":"500000"}`, RowsJSON: "not-loaded"}
		require.NoError(t, db.Create(&job).Error)
	}
	// Listings must work with no source log database available.
	oldLogDB := model.LOG_DB
	model.LOG_DB = nil
	t.Cleanup(func() { model.LOG_DB = oldLogDB })
	call := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/channel-finance/jobs"+query, nil)
		c.Set("id", root.Id)
		c.Set("role", common.RoleRootUser)
		ListChannelCostReportJobs(c)
		return rec
	}
	var legacy struct {
		Data []channelCostReportJobDTO
	}
	response := call("")
	require.Equal(t, http.StatusOK, response.Code)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &legacy))
	require.Len(t, legacy.Data, 50)
	assert.Equal(t, "report-062", legacy.Data[0].ID)
	for _, test := range []struct {
		query string
		total int64
		count int
		first string
	}{
		{"?page=6", 63, 10, "report-012"},
		{"?page=7", 63, 3, "report-002"},
		{"?page=8", 63, 0, ""},
		{"?page=1&page_size=100&status=paused", 32, 32, "report-062"},
		{"?page=1&keyword=report-06&status=completed", 1, 1, "report-061"},
		{"?page=1&keyword=%25", 0, 0, ""},
		{"?page=1&keyword=_", 0, 0, ""},
	} {
		t.Run(test.query, func(t *testing.T) {
			response := call(test.query)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var result struct {
				Data struct {
					Items    []channelCostReportJobDTO
					Total    int64
					Page     int
					PageSize int `json:"page_size"`
				}
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.Equal(t, test.total, result.Data.Total)
			require.Len(t, result.Data.Items, test.count)
			assert.Positive(t, result.Data.Page)
			assert.Positive(t, result.Data.PageSize)
			if test.first != "" {
				assert.Equal(t, test.first, result.Data.Items[0].ID)
			}
			assert.NotContains(t, response.Body.String(), "not-loaded")
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		})
	}
	for _, query := range []string{"?page=", "?page=0", "?page=-1", "?page=1000001", "?page=1&page_size=0", "?page=1&page_size=101", "?page=1&page_size=x", "?page=1&status=unknown", "?page=1&keyword=" + strings.Repeat("x", 129)} {
		assert.Equal(t, http.StatusServiceUnavailable, call(query).Code, query)
	}
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser} {
		require.NoError(t, db.Model(&root).Update("role", role).Error)
		response := call("?page=1")
		assert.Equal(t, http.StatusServiceUnavailable, response.Code)
		assert.NotContains(t, response.Body.String(), "report-062")
	}
}

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
	// Download only stored results, with the same conservative partial semantics.
	require.NoError(t, db.Model(&channel).Update("name", " \t=SUM(1,2)\"渠道").Error)
	exported := call(ExportChannelCostReport, job.ID, "", "")
	require.Equal(t, http.StatusOK, exported.Code, exported.Body.String())
	assert.Equal(t, "text/csv; charset=utf-8", exported.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", exported.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", exported.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, exported.Header().Get("Content-Disposition"), job.ID+".csv")
	assert.NotContains(t, exported.Body.String(), "must-not-appear")
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(exported.Body.String(), "\ufeff"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "paused", records[1][1])
	assert.Equal(t, "' \t=SUM(1,2)\"渠道", records[1][6])
	assert.Equal(t, "3", records[1][12])
	assert.Empty(t, records[1][14], "partial profit must be blank")
	assert.Empty(t, records[1][15])
	require.NoError(t, db.Model(job).Update("rows_json", "invalid").Error)
	invalid := call(ExportChannelCostReport, job.ID, "", "")
	assert.NotEqual(t, http.StatusOK, invalid.Code)
	assert.NotContains(t, invalid.Header().Get("Content-Type"), "text/csv")
}

func TestChannelCostReportEndpointsRequireFreshRoot(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Username: "report-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	handlers := []gin.HandlerFunc{ListChannelCostReportJobs, CreateChannelCostReportJob, GetChannelCostReportJob, ControlChannelCostReportJob, GetChannelCostReport, PreviewChannelCostReportConfig, ExportChannelCostReport}
	call := func(role int, handler gin.HandlerFunc) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/channel-finance/jobs/test", nil)
		c.Set("id", user.Id)
		c.Set("role", role)
		c.Params = gin.Params{{Key: "id", Value: uuid.NewString()}}
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
