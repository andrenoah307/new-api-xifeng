package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type logExportValidationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Id int `json:"id"`
	} `json:"data"`
}

func setupLogExportValidationDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMainDatabaseType := common.MainDatabaseType()
	oldLogDatabaseType := common.LogDatabaseType()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.LogExportTask{}))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainDatabaseType, oldLogDatabaseType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func performLogExportRequest(t *testing.T, handler gin.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	ctx.Set("id", 42)
	handler(ctx)
	return recorder
}

func decodeLogExportValidationResponse(t *testing.T, recorder *httptest.ResponseRecorder) logExportValidationResponse {
	t.Helper()

	var response logExportValidationResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestSynchronousLogExportTimeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupLogExportValidationDB(t)

	handlers := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "管理员导出", handler: ExportAllLogsCsv},
		{name: "用户导出", handler: ExportUserLogsCsv},
	}
	tests := []struct {
		name        string
		query       string
		wantStatus  int
		wantMessage string
	}{
		{name: "缺少 start", query: "end_timestamp=200", wantStatus: http.StatusBadRequest, wantMessage: "导出必须指定起止时间"},
		{name: "缺少 end", query: "start_timestamp=100", wantStatus: http.StatusBadRequest, wantMessage: "导出必须指定起止时间"},
		{name: "起止皆缺", query: "", wantStatus: http.StatusBadRequest, wantMessage: "导出必须指定起止时间"},
		{name: "start 为零", query: "start_timestamp=0&end_timestamp=200", wantStatus: http.StatusBadRequest, wantMessage: "导出必须指定起止时间"},
		{name: "end 为零", query: "start_timestamp=100&end_timestamp=0", wantStatus: http.StatusBadRequest, wantMessage: "导出必须指定起止时间"},
		{name: "start 为负数", query: "start_timestamp=-1&end_timestamp=200", wantStatus: http.StatusBadRequest, wantMessage: "导出必须指定起止时间"},
		{name: "end 为负数", query: "start_timestamp=100&end_timestamp=-1", wantStatus: http.StatusBadRequest, wantMessage: "导出必须指定起止时间"},
		{name: "起始晚于结束", query: "start_timestamp=201&end_timestamp=200", wantStatus: http.StatusBadRequest, wantMessage: "导出起始时间不能晚于结束时间"},
		{name: "起止相等", query: "start_timestamp=100&end_timestamp=100", wantStatus: http.StatusOK},
		{name: "合法区间", query: "start_timestamp=100&end_timestamp=200", wantStatus: http.StatusOK},
	}

	for _, handler := range handlers {
		t.Run(handler.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					recorder := performLogExportRequest(t, handler.handler, "/api/log/export?"+test.query)

					assert.Equal(t, test.wantStatus, recorder.Code)
					if test.wantMessage == "" {
						assert.Contains(t, recorder.Header().Get("Content-Type"), "text/csv")
						return
					}
					response := decodeLogExportValidationResponse(t, recorder)
					assert.False(t, response.Success)
					assert.Equal(t, test.wantMessage, response.Message)
				})
			}
		})
	}
}

func TestUserSynchronousLogExportRangeLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupLogExportValidationDB(t)

	const startTimestamp int64 = 1_700_000_000
	tests := []struct {
		name        string
		end         int64
		wantStatus  int
		wantMessage string
	}{
		{name: "31 天边界通过", end: startTimestamp + 31*86400, wantStatus: http.StatusOK},
		{name: "超过 31 天拒绝", end: startTimestamp + 31*86400 + 1, wantStatus: http.StatusBadRequest, wantMessage: "导出时间范围不能超过一个月"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := fmt.Sprintf("/api/log/self/export?start_timestamp=%d&end_timestamp=%d", startTimestamp, test.end)
			recorder := performLogExportRequest(t, ExportUserLogsCsv, target)

			assert.Equal(t, test.wantStatus, recorder.Code)
			if test.wantMessage != "" {
				response := decodeLogExportValidationResponse(t, recorder)
				assert.Equal(t, test.wantMessage, response.Message)
			}
		})
	}
}

func TestAdminSynchronousLogExportAllowsNinetyDayRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupLogExportValidationDB(t)

	const startTimestamp int64 = 1_700_000_000
	target := fmt.Sprintf("/api/log/export?start_timestamp=%d&end_timestamp=%d", startTimestamp, startTimestamp+90*86400)
	recorder := performLogExportRequest(t, ExportAllLogsCsv, target)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "text/csv")
}

func TestSynchronousLogExportFiltersByUpstreamRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupLogExportValidationDB(t)

	logs := []*model.Log{
		{UserId: 42, CreatedAt: 150, UpstreamRequestId: "upstream-match", Content: "included-row"},
		{UserId: 42, CreatedAt: 150, UpstreamRequestId: "upstream-other", Content: "excluded-row"},
	}
	require.NoError(t, db.Create(&logs).Error)

	handlers := []struct {
		name    string
		path    string
		handler gin.HandlerFunc
	}{
		{name: "管理员导出", path: "/api/log/export", handler: ExportAllLogsCsv},
		{name: "用户导出", path: "/api/log/self/export", handler: ExportUserLogsCsv},
	}
	for _, test := range handlers {
		t.Run(test.name, func(t *testing.T) {
			target := test.path + "?start_timestamp=100&end_timestamp=200&upstream_request_id=upstream-match"
			recorder := performLogExportRequest(t, test.handler, target)

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "included-row")
			assert.NotContains(t, recorder.Body.String(), "excluded-row")
			assert.NotContains(t, recorder.Body.String(), "上游请求ID")
		})
	}
}

func TestSubmitOfflineExportTimeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupLogExportValidationDB(t)

	oldOfflineEnabled := common.LogExportOfflineEnabled
	common.LogExportOfflineEnabled = true
	t.Cleanup(func() {
		common.LogExportOfflineEnabled = oldOfflineEnabled
	})

	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{name: "空 filters", body: `{"filters":{},"email":"test@example.com"}`, wantMessage: "导出必须指定起止时间"},
		{name: "只有 start", body: `{"filters":{"start_timestamp":100},"email":"test@example.com"}`, wantMessage: "导出必须指定起止时间"},
		{name: "只有 end", body: `{"filters":{"end_timestamp":200},"email":"test@example.com"}`, wantMessage: "导出必须指定起止时间"},
		{name: "反向区间", body: `{"filters":{"start_timestamp":201,"end_timestamp":200},"email":"test@example.com"}`, wantMessage: "导出起始时间不能晚于结束时间"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/log/self/export-offline", strings.NewReader(test.body))
			ctx.Request.Header.Set("Content-Type", gin.MIMEJSON)
			ctx.Set("id", 42)
			ctx.Set("username", "export-user")

			SubmitOfflineExport(ctx)

			assert.Equal(t, http.StatusOK, recorder.Code)
			response := decodeLogExportValidationResponse(t, recorder)
			assert.False(t, response.Success)
			assert.Equal(t, test.wantMessage, response.Message)
		})
	}

	t.Run("合法长区间通过且忽略 channel_id", func(t *testing.T) {
		body := `{"filters":{"start_timestamp":100,"end_timestamp":7776100,"channel_id":99},"email":"test@example.com"}`
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/log/self/export-offline", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", gin.MIMEJSON)
		ctx.Set("id", 42)
		ctx.Set("username", "export-user")

		SubmitOfflineExport(ctx)

		require.Equal(t, http.StatusOK, recorder.Code)
		response := decodeLogExportValidationResponse(t, recorder)
		require.True(t, response.Success, response.Message)
		require.Positive(t, response.Data.Id)
		var task model.LogExportTask
		require.NoError(t, db.First(&task, response.Data.Id).Error)
		assert.NotContains(t, task.Filters, "channel_id")
	})
}
