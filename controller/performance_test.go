package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPerformanceStatsExposesAdmissionGateAndCgroupSnapshots(t *testing.T) {
	previousHigh := common.RelayMemoryBreakerHighPercent
	previousLow := common.RelayMemoryBreakerLowPercent
	previousMaxRequests := common.RelayMaxConcurrentRequests
	previousMaxBodyBytes := common.RelayMaxActiveBodyBytes
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	common.RelayMemoryBreakerHighPercent = 90
	common.RelayMemoryBreakerLowPercent = 75
	common.RelayMaxConcurrentRequests = 5000
	common.RelayMaxActiveBodyBytes = 10 << 30
	model.DB = nil
	model.LOG_DB = nil
	t.Cleanup(func() {
		common.RelayMemoryBreakerHighPercent = previousHigh
		common.RelayMemoryBreakerLowPercent = previousLow
		common.RelayMaxConcurrentRequests = previousMaxRequests
		common.RelayMaxActiveBodyBytes = previousMaxBodyBytes
		model.DB = previousDB
		model.LOG_DB = previousLogDB
	})
	require.Nil(t, model.DB)
	require.Nil(t, model.LOG_DB)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/performance/stats", nil)
	GetPerformanceStats(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			RelayAdmission middleware.RelayAdmissionStats `json:"relay_admission"`
			SystemGate     middleware.SystemGateStats     `json:"system_gate"`
			CgroupMemory   common.CgroupMemoryStatus      `json:"cgroup_memory"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	var rawResponse struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &rawResponse))
	for _, field := range []string{"relay_admission", "system_gate", "cgroup_memory"} {
		value, exists := rawResponse.Data[field]
		require.True(t, exists, "%s must be present", field)
		assert.IsType(t, map[string]any{}, value)
	}

	assert.Equal(t, middleware.GetRelayAdmissionStats(), response.Data.RelayAdmission)
	assert.Equal(t, middleware.GetSystemGateStats(), response.Data.SystemGate)
	assert.Equal(t, common.GetCgroupMemoryStatus(), response.Data.CgroupMemory)
	assert.Equal(t, 90, response.Data.CgroupMemory.HighPercent)
	assert.Equal(t, 75, response.Data.CgroupMemory.LowPercent)
	assert.Equal(t, 5000, response.Data.RelayAdmission.MaxConcurrentRequests)
	assert.EqualValues(t, 10<<30, response.Data.RelayAdmission.MaxActiveBodyBytes)
	assert.Nil(t, model.DB)
	assert.Nil(t, model.LOG_DB)
}
