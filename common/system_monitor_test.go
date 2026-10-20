package common

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSystemOverloadRequiresConsecutiveSamples(t *testing.T) {
	config := PerformanceMonitorConfig{Enabled: true, CPUThreshold: 90, MemoryThreshold: 90, DiskThreshold: 95}
	now := time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC)
	cpuSamples := []struct {
		usage        float64
		wantOverload bool
	}{
		{usage: 95, wantOverload: false},
		{usage: 95, wantOverload: false},
		{usage: 95, wantOverload: true},
		{usage: 95, wantOverload: true},
		{usage: 50, wantOverload: false}, // 回落即恢复
		{usage: 95, wantOverload: false}, // 重新累计
		{usage: 95, wantOverload: false},
		{usage: 95, wantOverload: true},
	}

	var detector overloadDetector
	for i, sample := range cpuSamples {
		status := SystemStatus{CPUUsage: sample.usage, SampledAt: now}
		detector.mark(&status, config)
		overload := CheckSystemOverload(status, config, now)
		if !sample.wantOverload {
			assert.Nil(t, overload, "sample %d", i+1)
			continue
		}
		require.NotNil(t, overload, "sample %d", i+1)
		assert.Equal(t, "system_cpu_overloaded", overload.Code)
	}
}

func TestCheckSystemOverload(t *testing.T) {
	now := time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC)
	enabled := PerformanceMonitorConfig{Enabled: true, CPUThreshold: 90, MemoryThreshold: 90, DiskThreshold: 95}
	cpuOverloaded := SystemStatus{CPUUsage: 99.5, CPUOverloaded: true, SampledAt: now}
	tests := []struct {
		name        string
		status      SystemStatus
		config      PerformanceMonitorConfig
		wantCode    string
		wantMessage string
	}{
		{
			name:        "fresh cpu overload",
			status:      cpuOverloaded,
			config:      enabled,
			wantCode:    "system_cpu_overloaded",
			wantMessage: "system cpu overloaded (current: 99.5%, threshold: 90%)",
		},
		{
			name:        "fresh memory overload",
			status:      SystemStatus{MemoryUsage: 96.2, MemoryOverloaded: true, SampledAt: now},
			config:      enabled,
			wantCode:    "system_memory_overloaded",
			wantMessage: "system memory overloaded (current: 96.2%, threshold: 90%)",
		},
		{
			name:        "fresh disk overload",
			status:      SystemStatus{DiskUsage: 97, DiskOverloaded: true, SampledAt: now},
			config:      enabled,
			wantCode:    "system_disk_overloaded",
			wantMessage: "system disk overloaded (current: 97.0%, threshold: 95%)",
		},
		{
			name:        "lowered threshold reports the active threshold",
			status:      cpuOverloaded,
			config:      PerformanceMonitorConfig{Enabled: true, CPUThreshold: 80},
			wantCode:    "system_cpu_overloaded",
			wantMessage: "system cpu overloaded (current: 99.5%, threshold: 80%)",
		},
		{name: "monitor disabled", status: cpuOverloaded, config: PerformanceMonitorConfig{Enabled: false, CPUThreshold: 90}},
		{name: "cpu threshold disabled at runtime", status: cpuOverloaded, config: PerformanceMonitorConfig{Enabled: true, CPUThreshold: 0}},
		{name: "cpu threshold raised above usage", status: cpuOverloaded, config: PerformanceMonitorConfig{Enabled: true, CPUThreshold: 100}},
		{name: "stale sample", status: SystemStatus{CPUUsage: 99.5, CPUOverloaded: true, SampledAt: now.Add(-31 * time.Second)}, config: enabled},
		{name: "missing sample", status: SystemStatus{CPUUsage: 99.5, CPUOverloaded: true}, config: enabled},
		{name: "usage above threshold without consecutive samples", status: SystemStatus{CPUUsage: 99.5, SampledAt: now}, config: enabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overload := CheckSystemOverload(tt.status, tt.config, now)
			if tt.wantCode == "" {
				assert.Nil(t, overload)
				return
			}
			require.NotNil(t, overload)
			assert.Equal(t, tt.wantCode, overload.Code)
			assert.Equal(t, tt.wantMessage, overload.Error())
		})
	}
}
