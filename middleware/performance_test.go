package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemPerformanceCheckMemoryPressure(t *testing.T) {
	previous := common.GetPerformanceMonitorConfig()
	t.Cleanup(func() { common.SetPerformanceMonitorConfig(previous) })
	for _, tt := range []struct {
		name       string
		path       string
		enabled    bool
		threshold  int
		usage      float64
		wantStatus int
	}{
		{name: "container pressure blocks OpenAI relay", path: "/v1/chat/completions", enabled: true, threshold: 90, usage: 95, wantStatus: 503},
		{name: "parent shared pressure blocks Claude relay", path: "/v1/messages", enabled: true, threshold: 90, usage: 95, wantStatus: 503},
		{name: "disabled monitor allows high pressure", path: "/v1/chat/completions", enabled: false, threshold: 90, usage: 95, wantStatus: 204},
		{name: "disabled memory threshold allows high pressure", path: "/v1/chat/completions", enabled: true, threshold: 0, usage: 95, wantStatus: 204},
		{name: "below threshold allows relay", path: "/v1/chat/completions", enabled: true, threshold: 90, usage: 85, wantStatus: 204},
		{name: "existing integer threshold semantics", path: "/v1/chat/completions", enabled: true, threshold: 90, usage: 90.9, wantStatus: 204},
	} {
		t.Run(tt.name, func(t *testing.T) {
			common.SetPerformanceMonitorConfig(common.PerformanceMonitorConfig{Enabled: tt.enabled, MemoryThreshold: tt.threshold})
			router := gin.New()
			router.Use(systemPerformanceCheck(func() common.SystemStatus { return common.SystemStatus{MemoryUsage: tt.usage} }))
			called := false
			router.POST(tt.path, func(c *gin.Context) {
				called = true
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, tt.path, nil))
			assert.Equal(t, tt.wantStatus, response.Code)
			assert.Equal(t, tt.wantStatus == http.StatusNoContent, called)
			if tt.wantStatus == http.StatusServiceUnavailable {
				var body struct {
					Error struct {
						Message string `json:"message"`
						Code    string `json:"code"`
						Type    string `json:"type"`
					} `json:"error"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
				assert.Contains(t, body.Error.Message, "system memory overloaded")
				assert.Contains(t, body.Error.Message, "95.0%")
				if tt.path == "/v1/messages" {
					assert.Equal(t, "new_api_error", body.Error.Type)
				} else {
					assert.Equal(t, "system_memory_overloaded", body.Error.Code)
				}
			}
		})
	}
}

func TestSystemPerformanceCheckRetainsCPUAndDiskThresholds(t *testing.T) {
	previous := common.GetPerformanceMonitorConfig()
	t.Cleanup(func() { common.SetPerformanceMonitorConfig(previous) })
	common.SetPerformanceMonitorConfig(common.PerformanceMonitorConfig{Enabled: true, CPUThreshold: 90, MemoryThreshold: 90, DiskThreshold: 90})
	for _, tt := range []struct {
		name   string
		status common.SystemStatus
		code   string
	}{
		{name: "CPU retains priority", status: common.SystemStatus{CPUUsage: 95, MemoryUsage: 95, DiskUsage: 95}, code: "system_cpu_overloaded"},
		{name: "disk still protects cache space", status: common.SystemStatus{CPUUsage: 20, MemoryUsage: 20, DiskUsage: 95}, code: "system_disk_overloaded"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(systemPerformanceCheck(func() common.SystemStatus { return tt.status }))
			router.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			assert.Equal(t, http.StatusServiceUnavailable, response.Code)
			assert.Contains(t, response.Body.String(), tt.code)
		})
	}
}
