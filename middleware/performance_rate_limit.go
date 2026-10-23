package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// PerformanceMetricsIPRateLimit shares one per-IP budget across all performance
// endpoints and runs before authentication. It stays enabled independently of
// the global API limiter. Configuration is read when routes are registered.
func PerformanceMetricsIPRateLimit() gin.HandlerFunc {
	limit := common.GetEnvOrDefault("PERF_METRICS_IP_RATE_LIMIT", 120)
	if limit <= 0 {
		limit = 120
	}
	return rateLimitFactory(limit, 60, "PERF")
}

// PerformanceMetricsUserRateLimit limits authenticated access by user ID, so
// switching tokens or client IPs does not reset the budget. Use after UserAuth.
func PerformanceMetricsUserRateLimit() gin.HandlerFunc {
	limit := common.GetEnvOrDefault("PERF_METRICS_USER_RATE_LIMIT", 60)
	if limit <= 0 {
		limit = 60
	}
	return userRateLimitFactory(limit, 60, "PERF")
}
