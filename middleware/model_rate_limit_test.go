package middleware

import (
	"context"
	"github.com/QuantumNous/new-api/setting"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var modelRateLimitTestUserID atomic.Int64

func newModelRateLimitTestRouter(limiter gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	userID := 817100 + int(modelRateLimitTestUserID.Add(1))
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", userID) })
	router.Use(limiter)
	return router
}

func TestMemoryModelRateLimitFailedRequestReleasesSuccessQuota(t *testing.T) {
	router := newModelRateLimitTestRouter(memoryRateLimitHandler(strconv.FormatInt(modelRateLimitTestUserID.Add(1), 10), 60, 0, 1))
	router.GET("/fail", func(c *gin.Context) { c.Status(http.StatusBadGateway) })
	router.GET("/success", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	failed := httptest.NewRecorder()
	router.ServeHTTP(failed, httptest.NewRequest(http.MethodGet, "/fail", nil))
	require.Equal(t, http.StatusBadGateway, failed.Code)

	successful := httptest.NewRecorder()
	router.ServeHTTP(successful, httptest.NewRequest(http.MethodGet, "/success", nil))
	require.Equal(t, http.StatusNoContent, successful.Code, "failed requests must not consume success quota")

	limited := httptest.NewRecorder()
	router.ServeHTTP(limited, httptest.NewRequest(http.MethodGet, "/success", nil))
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
}

func TestMemoryModelRateLimitZeroSuccessLimitIsDisabled(t *testing.T) {
	router := newModelRateLimitTestRouter(memoryRateLimitHandler(strconv.FormatInt(modelRateLimitTestUserID.Add(1), 10), 60, 2, 0))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for _, expected := range []int{http.StatusNoContent, http.StatusNoContent, http.StatusTooManyRequests} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, expected, response.Code)
	}
}

func TestModelRateLimitHTTP200ProtocolFailureReleasesSuccessQuota(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			limiter := memoryRateLimitHandler(strconv.FormatInt(modelRateLimitTestUserID.Add(1), 10), 60, 0, 1)
			if backend == "redis" {
				useRateLimitMiniRedis(t)
				limiter = redisRateLimitHandler(strconv.FormatInt(modelRateLimitTestUserID.Add(1), 10), 60, 0, 1)
			}
			router := newModelRateLimitTestRouter(limiter)
			router.GET("/failed-stream", func(c *gin.Context) {
				c.String(http.StatusOK, "data: failure\n\n")
				common.SetContextKey(c, constant.ContextKeyResponseFailed, true)
			})
			router.GET("/success", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			for _, tt := range []struct {
				path   string
				status int
			}{
				{"/failed-stream", http.StatusOK},
				{"/success", http.StatusNoContent},
				{"/success", http.StatusTooManyRequests},
			} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
				assert.Equal(t, tt.status, response.Code, tt.path)
			}
		})
	}
}

func TestMemoryModelRateLimitFailureStillConsumesTotalQuota(t *testing.T) {
	router := newModelRateLimitTestRouter(memoryRateLimitHandler(strconv.FormatInt(modelRateLimitTestUserID.Add(1), 10), 60, 1, 1))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusBadGateway) })
	for _, expected := range []int{http.StatusBadGateway, http.StatusTooManyRequests} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, expected, response.Code)
	}
}

func TestMemoryModelRateLimitReservesInFlightAndReleasesPanic(t *testing.T) {
	router := newModelRateLimitTestRouter(memoryRateLimitHandler(strconv.FormatInt(modelRateLimitTestUserID.Add(1), 10), 60, 0, 1))
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	router.GET("/pending", func(c *gin.Context) {
		close(started)
		<-release
		c.Status(http.StatusBadGateway)
	})
	router.GET("/panic", func(c *gin.Context) { panic("test handler failure") })
	router.GET("/success", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	pending := httptest.NewRecorder()
	go func() {
		defer close(finished)
		router.ServeHTTP(pending, httptest.NewRequest(http.MethodGet, "/pending", nil))
	}()
	<-started
	limited := httptest.NewRecorder()
	router.ServeHTTP(limited, httptest.NewRequest(http.MethodGet, "/success", nil))
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	close(release)
	<-finished
	assert.Equal(t, http.StatusBadGateway, pending.Code)

	require.Panics(t, func() {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/panic", nil))
	})
	successful := httptest.NewRecorder()
	router.ServeHTTP(successful, httptest.NewRequest(http.MethodGet, "/success", nil))
	assert.Equal(t, http.StatusNoContent, successful.Code, "panic must release its reservation")
}

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}

func TestModelRateLimitConfiguredGroupsHaveIndependentBudgets(t *testing.T) {
	previousEnabled, previousMinutes := setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes
	previousTotal, previousSuccess := setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount
	previousGroups, previousRedis := setting.ModelRequestRateLimitGroup, common.RedisEnabled
	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes = previousEnabled, previousMinutes
		setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount = previousTotal, previousSuccess
		setting.ModelRequestRateLimitGroup, common.RedisEnabled = previousGroups, previousRedis
	})
	setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes = true, 1
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			common.RedisEnabled = false
			if backend == "redis" {
				useRateLimitMiniRedis(t)
			}
			for _, limit := range []string{"total", "success"} {
				t.Run(limit, func(t *testing.T) {
					if limit == "total" {
						setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount = 1, 0
						setting.ModelRequestRateLimitGroup = map[string][2]int{"a": {3, 0}, "b": {1, 0}}
					} else {
						setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount = 0, 1
						setting.ModelRequestRateLimitGroup = map[string][2]int{"a": {0, 3}, "b": {0, 1}}
					}
					userID := int(modelRateLimitTestUserID.Add(1)) + 950000
					router := gin.New()
					router.GET("/:group", func(c *gin.Context) {
						c.Set("id", userID)
						common.SetContextKey(c, constant.ContextKeyTokenGroup, c.Param("group")+",fallback")
						common.SetContextKey(c, constant.ContextKeyUsingGroup, c.Param("group"))
					}, ModelRequestRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
					for _, step := range []struct {
						group  string
						status int
					}{
						{"a", 204}, {"a", 204}, {"b", 204}, {"b", 429}, {"a", 204}, {"a", 429},
						{"c", 204}, {"d", 429},
					} {
						assert.Equal(t, step.status, performRateLimitRequest(router, "/"+step.group, "127.0.0.1:1234").Code, step.group)
					}
				})
			}
		})
	}
}

func TestMemoryModelRateLimitReturnsExplainedOpenAIError(t *testing.T) {
	for _, limits := range [][2]int{{1, 0}, {0, 1}} {
		router := newModelRateLimitTestRouter(memoryRateLimitHandler(strconv.FormatInt(modelRateLimitTestUserID.Add(1), 10), 60, limits[0], limits[1]))
		router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		require.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/", "127.0.0.1:1234").Code)
		response := performRateLimitRequest(router, "/", "127.0.0.1:1234")
		require.Equal(t, http.StatusTooManyRequests, response.Code)
		var body struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
		assert.Contains(t, body.Error.Message, "请求数限制")
		assert.Equal(t, "new_api_error", body.Error.Type)
	}
}
