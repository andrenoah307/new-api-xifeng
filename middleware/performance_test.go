package middleware

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckSystemPerformanceCountsOnlyTheTriggeredGate(t *testing.T) {
	previousConfig := common.GetPerformanceMonitorConfig()
	previousStatus := common.GetSystemStatus()
	previousStats := GetSystemGateStats()
	common.SetSystemStatus(common.SystemStatus{})
	systemGateRejectedCPUOverloaded.Store(0)
	systemGateRejectedMemoryOverloaded.Store(0)
	systemGateRejectedDiskOverloaded.Store(0)
	t.Cleanup(func() {
		common.SetPerformanceMonitorConfig(previousConfig)
		common.SetSystemStatus(previousStatus)
		systemGateRejectedCPUOverloaded.Store(previousStats.RejectedCPUOverloaded)
		systemGateRejectedMemoryOverloaded.Store(previousStats.RejectedMemoryOverloaded)
		systemGateRejectedDiskOverloaded.Store(previousStats.RejectedDiskOverloaded)
	})

	tests := []struct {
		name      string
		config    common.PerformanceMonitorConfig
		status    common.SystemStatus
		wantCode  string
		wantStats SystemGateStats
	}{
		{
			name:     "cpu overloaded",
			config:   common.PerformanceMonitorConfig{Enabled: true, CPUThreshold: 90},
			status:   common.SystemStatus{CPUUsage: 91},
			wantCode: "system_cpu_overloaded",
			wantStats: SystemGateStats{
				RejectedCPUOverloaded: 1,
			},
		},
		{
			name:     "memory overloaded",
			config:   common.PerformanceMonitorConfig{Enabled: true, MemoryThreshold: 90},
			status:   common.SystemStatus{MemoryUsage: 91},
			wantCode: "system_memory_overloaded",
			wantStats: SystemGateStats{
				RejectedMemoryOverloaded: 1,
			},
		},
		{
			name:     "disk overloaded",
			config:   common.PerformanceMonitorConfig{Enabled: true, DiskThreshold: 90},
			status:   common.SystemStatus{DiskUsage: 91},
			wantCode: "system_disk_overloaded",
			wantStats: SystemGateStats{
				RejectedDiskOverloaded: 1,
			},
		},
		{
			name:      "disabled monitor",
			config:    common.PerformanceMonitorConfig{Enabled: false, CPUThreshold: 1, MemoryThreshold: 1, DiskThreshold: 1},
			status:    common.SystemStatus{CPUUsage: 100, MemoryUsage: 100, DiskUsage: 100},
			wantStats: SystemGateStats{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			common.SetPerformanceMonitorConfig(test.config)
			common.SetSystemStatus(test.status)
			systemGateRejectedCPUOverloaded.Store(0)
			systemGateRejectedMemoryOverloaded.Store(0)
			systemGateRejectedDiskOverloaded.Store(0)

			err := checkSystemPerformance()
			if test.wantCode == "" {
				assert.Nil(t, err)
			} else {
				require.Error(t, err)
				assert.Equal(t, test.wantCode, string(err.GetErrorCode()))
				assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
			}

			stats := GetSystemGateStats()
			assert.Equal(t, test.wantStats.RejectedCPUOverloaded, stats.RejectedCPUOverloaded)
			assert.Equal(t, test.wantStats.RejectedMemoryOverloaded, stats.RejectedMemoryOverloaded)
			assert.Equal(t, test.wantStats.RejectedDiskOverloaded, stats.RejectedDiskOverloaded)
		})
	}
}
