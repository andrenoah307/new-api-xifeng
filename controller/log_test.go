package controller

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCachedLogTotal(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int64
	}{
		{name: "negative", raw: "-1", want: 0},
		{name: "zero", raw: "0", want: 0},
		{name: "above legacy limit", raw: "1000001", want: 1000001},
		{name: "above int32", raw: "2147483648", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseCachedLogTotal(tt.raw))
		})
	}

	assert.Equal(t, int64(math.MaxInt32), parseCachedLogTotal("2147483647"))
}

func TestLogListHandlersReuseLargeCachedTotal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		handler gin.HandlerFunc
		path    string
	}{
		{name: "admin", handler: GetAllLogs, path: "/api/log/?total_count=1000001"},
		{name: "user", handler: GetUserLogs, path: "/api/log/self?total_count=1000001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupLogExportValidationDB(t)
			recorder := performLogExportRequest(t, tt.handler, tt.path)
			require.Equal(t, http.StatusOK, recorder.Code)

			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Total int `json:"total"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.True(t, response.Success)
			assert.Equal(t, 1000001, response.Data.Total)
		})
	}
}

func TestLogStatHandlersForwardRequestIdentifiers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		handler gin.HandlerFunc
		path    string
	}{
		{name: "admin request id uses default time range", handler: GetLogsStat, path: "/api/log/stat?request_id=req-match"},
		{name: "user request id uses default time range", handler: GetLogsSelfStat, path: "/api/log/self/stat?request_id=req-match"},
		{name: "admin request id", handler: GetLogsStat, path: "/api/log/stat?start_timestamp=%d&end_timestamp=%d&request_id=req-match"},
		{name: "admin upstream request id", handler: GetLogsStat, path: "/api/log/stat?start_timestamp=%d&end_timestamp=%d&upstream_request_id=up-match"},
		{name: "user request id", handler: GetLogsSelfStat, path: "/api/log/self/stat?start_timestamp=%d&end_timestamp=%d&request_id=req-match"},
		{name: "user upstream request id", handler: GetLogsSelfStat, path: "/api/log/self/stat?start_timestamp=%d&end_timestamp=%d&upstream_request_id=up-match"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupLogExportValidationDB(t)
			now := time.Now().Unix()
			rows := []*model.Log{
				{UserId: 42, CreatedAt: now, Type: model.LogTypeConsume, Quota: 7, RequestId: "req-match", UpstreamRequestId: "up-match"},
				{UserId: 42, CreatedAt: now, Type: model.LogTypeConsume, Quota: 11, RequestId: "req-other", UpstreamRequestId: "up-other"},
			}
			require.NoError(t, db.Create(&rows).Error)

			target := tt.path
			if strings.Contains(target, "%d") {
				target = fmt.Sprintf(target, now-10, now+10)
			}
			recorder := performLogExportRequest(
				t,
				tt.handler,
				target,
			)
			require.Equal(t, http.StatusOK, recorder.Code)

			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Quota int `json:"quota"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.True(t, response.Success)
			assert.Equal(t, 7, response.Data.Quota)
		})
	}
}

func TestLogStatHandlersReportDatabaseErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name    string
		handler gin.HandlerFunc
		path    string
	}{
		{name: "admin", handler: GetLogsStat, path: "/api/log/stat"},
		{name: "user", handler: GetLogsSelfStat, path: "/api/log/self/stat"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db := setupLogExportValidationDB(t)
			require.NoError(t, db.Migrator().DropTable(&model.Log{}))

			recorder := performLogExportRequest(t, testCase.handler, testCase.path)
			require.Equal(t, http.StatusOK, recorder.Code)

			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
		})
	}
}
