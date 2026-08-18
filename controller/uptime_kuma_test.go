package controller

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUptimeKumaStatusHonorsEnabledSettingBeforeFetchingGroups(t *testing.T) {
	var upstreamRequests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests.Add(1)
		var payload any
		switch r.URL.Path {
		case "/api/status-page/public":
			payload = map[string]any{
				"publicGroupList": []any{map[string]any{
					"id": 1, "name": "Core", "monitorList": []any{map[string]any{"id": 7, "name": "API"}},
				}},
			}
		case "/api/status-page/heartbeat/public":
			payload = map[string]any{
				"heartbeatList": map[string]any{"7": []any{map[string]any{"status": 1}}},
				"uptimeList":    map[string]any{"7_24": 0.999},
			}
		default:
			http.NotFound(w, r)
			return
		}
		encoded, err := common.Marshal(payload)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(encoded)
		require.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)

	console := console_setting.GetConsoleSetting()
	previousEnabled := console.UptimeKumaEnabled
	previousGroups := console.UptimeKumaGroups
	t.Cleanup(func() {
		console.UptimeKumaEnabled = previousEnabled
		console.UptimeKumaGroups = previousGroups
	})
	groups, err := common.Marshal([]map[string]any{{
		"categoryName": "Primary",
		"url":          upstream.URL,
		"slug":         "public",
	}})
	require.NoError(t, err)
	console.UptimeKumaGroups = string(groups)

	console.UptimeKumaEnabled = false
	disabled := callUptimeKumaHandler(t)
	assert.Equal(t, int64(0), upstreamRequests.Load())
	assert.Equal(t, map[string]any{
		"success": true,
		"message": "",
		"data":    []any{},
	}, disabled)

	console.UptimeKumaEnabled = true
	enabled := callUptimeKumaHandler(t)
	assert.Equal(t, int64(2), upstreamRequests.Load())
	data, ok := enabled["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	group, ok := data[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Primary", group["categoryName"])
	monitors, ok := group["monitors"].([]any)
	require.True(t, ok)
	require.Len(t, monitors, 1)
	monitor, ok := monitors[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "API", monitor["name"])
	assert.Equal(t, float64(1), monitor["status"])
}

func callUptimeKumaHandler(t *testing.T) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/uptime/status", nil)
	GetUptimeKumaStatus(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}
