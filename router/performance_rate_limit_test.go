package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPerformanceRateLimitRouter(t *testing.T, useRedis bool) (*gin.Engine, *model.User) {
	t.Helper()
	engine, user := setupModelPerformanceRouter(t)
	if useRedis {
		server := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
		require.NoError(t, client.Ping(context.Background()).Err())
		previousClient := common.RDB
		common.RedisEnabled, common.RDB = true, client
		t.Cleanup(func() {
			assert.NoError(t, client.Close())
			common.RDB = previousClient
		})
		engine = gin.New()
		SetApiRouter(engine)
	}
	t.Setenv("TRUSTED_PROXIES", "")
	require.NoError(t, middleware.ConfigureTrustedProxies(engine))
	return engine, user
}

func TestPerformanceMetricsShareIPLimitBeforeAuthentication(t *testing.T) {
	for _, backend := range []struct {
		name     string
		useRedis bool
	}{
		{name: "memory"},
		{name: "redis", useRedis: true},
	} {
		t.Run(backend.name, func(t *testing.T) {
			t.Setenv("PERF_METRICS_IP_RATE_LIMIT", "3")
			t.Setenv("PERF_METRICS_USER_RATE_LIMIT", "2")
			engine, user := setupPerformanceRateLimitRouter(t, backend.useRedis)
			require.False(t, common.GlobalApiRateLimitEnable)

			for index, tc := range []struct {
				path string
				code int
			}{
				{path: "/api/perf-metrics/summary", code: http.StatusOK},
				{path: "/api/perf-metrics?model=router-performance-model", code: http.StatusOK},
				{path: "/api/perf-metrics/models", code: http.StatusUnauthorized},
				{path: "/api/perf-metrics/models?hours=1", code: http.StatusTooManyRequests},
				{path: "/api/perf-metrics/summary?hours=12", code: http.StatusTooManyRequests},
				{path: "/api/perf-metrics?model=another-model&group=public", code: http.StatusTooManyRequests},
			} {
				request := httptest.NewRequest(http.MethodGet, tc.path, nil)
				request.RemoteAddr = fmt.Sprintf("[2001:db8::%x]:12345", user.Id)
				// Public peers cannot reset their budget with forged proxy headers.
				request.Header.Set("X-Forwarded-For", "192.0.2."+strconv.Itoa(index+1))
				request.Header.Set("X-Real-IP", "192.0.2."+strconv.Itoa(index+1))
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				require.Equal(t, tc.code, response.Code, "%s: %s", tc.path, response.Body.String())
				assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
				assert.Contains(t, response.Header().Get("Cache-Control"), "private")
				if tc.code == http.StatusTooManyRequests {
					retryAfter, err := strconv.Atoi(response.Header().Get("Retry-After"))
					require.NoError(t, err)
					assert.Positive(t, retryAfter)
					assert.LessOrEqual(t, retryAfter, 60)
				}
			}
		})
	}
}

func TestPerformanceMetricsUserLimitSurvivesIPAndTokenChanges(t *testing.T) {
	for _, backend := range []struct {
		name     string
		useRedis bool
	}{
		{name: "memory"},
		{name: "redis", useRedis: true},
	} {
		t.Run(backend.name, func(t *testing.T) {
			t.Setenv("PERF_METRICS_IP_RATE_LIMIT", "3")
			t.Setenv("PERF_METRICS_USER_RATE_LIMIT", "2")
			engine, user := setupPerformanceRateLimitRouter(t, backend.useRedis)
			otherUser := &model.User{
				Id: int(performanceRouterUserID.Add(1)), Username: "performance-other-user", Role: common.RoleCommonUser,
				Status: common.UserStatusEnabled, Group: "member", AuthVersion: 1,
				AffCode: "performance-other-user-aff",
			}
			otherUser.SetAccessToken("other-performance-pat")
			require.NoError(t, model.DB.Create(otherUser).Error)
			session := &model.UserSession{
				SID: "performance-rate-limit-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
				Status: model.UserSessionStatusActive, RefreshHash: "performance-rate-limit-refresh",
				LoginMethod: "password", LastActiveAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
			}
			require.NoError(t, model.CreateUserSession(session))
			accessToken, _, err := service.IssueAccessToken(service.AuthIdentity{
				UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1,
			})
			require.NoError(t, err)

			for index, tc := range []struct {
				authorization string
				code          int
			}{
				{code: http.StatusUnauthorized},
				{authorization: "Bearer " + user.GetAccessToken(), code: http.StatusOK},
				{authorization: user.GetAccessToken(), code: http.StatusOK},
				{authorization: "Bearer " + accessToken, code: http.StatusTooManyRequests},
				{authorization: "Bearer " + otherUser.GetAccessToken(), code: http.StatusOK},
			} {
				request := httptest.NewRequest(http.MethodGet, "/api/perf-metrics/models", nil)
				request.RemoteAddr = fmt.Sprintf("[2001:db8:%x::%x]:12345", user.Id, index+1)
				request.Header.Set("Authorization", tc.authorization)
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				require.Equal(t, tc.code, response.Code, response.Body.String())
				assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
				assert.Contains(t, response.Header().Get("Cache-Control"), "private")
				if tc.code == http.StatusTooManyRequests {
					retryAfter, err := strconv.Atoi(response.Header().Get("Retry-After"))
					require.NoError(t, err)
					assert.Positive(t, retryAfter)
					assert.LessOrEqual(t, retryAfter, 60)
				}
			}
		})
	}
}
