package controller

import (
	"math"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
