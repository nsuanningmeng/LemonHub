package controller

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPerformanceCacheRefreshesSuccessAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		ttl  time.Duration
	}{
		{name: "success", ttl: 30 * time.Second},
		{name: "database failure", err: errors.New("database unavailable"), ttl: 2 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := newPerformanceQueryCache()
			now := time.Unix(1700000000, 0)
			cache.now = func() time.Time { return now }
			calls := 0
			load := func() ([]byte, error) {
				calls++
				return []byte("snapshot"), tc.err
			}
			key := sha256.Sum256([]byte("models"))
			body, err := cache.get(context.Background(), key, load)
			assert.Equal(t, tc.err, err)
			assert.Equal(t, "snapshot", string(body))
			now = now.Add(tc.ttl - time.Nanosecond)
			_, err = cache.get(context.Background(), key, load)
			assert.Equal(t, tc.err, err)
			assert.Equal(t, 1, calls, "polling within the TTL must not re-query the database")
			now = now.Add(time.Nanosecond)
			_, err = cache.get(context.Background(), key, load)
			assert.Equal(t, tc.err, err)
			assert.Equal(t, 2, calls, "both failures and successes must expire")
		})
	}
}

// Signals when a request starts waiting for the shared result, allowing the
// test to control cancellation and completion without scheduling sleeps.
type waitingPerformanceContext struct {
	context.Context
	waiting chan struct{}
}

func (ctx waitingPerformanceContext) Done() <-chan struct{} {
	close(ctx.waiting)
	return ctx.Context.Done()
}

func TestPerformanceCacheCoalescesAndBoundsConcurrentQueries(t *testing.T) {
	cache := newPerformanceQueryCache()
	cache.maxConcurrent = 1
	cache.maxWaiters = 1
	key := sha256.Sum256([]byte("same query"))
	started, release := make(chan struct{}), make(chan struct{})
	type result struct {
		body []byte
		err  error
	}
	first := make(chan result, 1)
	go func() {
		body, err := cache.get(context.Background(), key, func() ([]byte, error) {
			close(started)
			<-release
			return []byte("shared result"), nil
		})
		first <- result{body, err}
	}()
	<-started

	waiting := make(chan struct{})
	second := make(chan result, 1)
	go func() {
		body, err := cache.get(waitingPerformanceContext{context.Background(), waiting}, key, func() ([]byte, error) {
			return nil, errors.New("duplicate query must not execute")
		})
		second <- result{body, err}
	}()
	<-waiting
	_, err := cache.get(context.Background(), key, nil)
	assert.ErrorIs(t, err, errPerformanceQueriesBusy, "shared queries must also bound waiting HTTP requests")
	_, err = cache.get(context.Background(), sha256.Sum256([]byte("different query")), func() ([]byte, error) {
		return nil, errors.New("excess concurrent query must not execute")
	})
	assert.ErrorIs(t, err, errPerformanceQueriesBusy)
	close(release)
	for _, response := range []result{<-first, <-second} {
		require.NoError(t, response.err)
		assert.Equal(t, "shared result", string(response.body))
	}
	_, err = cache.get(context.Background(), sha256.Sum256([]byte("after completion")), func() ([]byte, error) {
		return []byte("new result"), nil
	})
	require.NoError(t, err, "completion must release the concurrency slot")
}

func TestPerformanceCacheCancelledWaiterDoesNotCancelSharedQuery(t *testing.T) {
	cache := newPerformanceQueryCache()
	key := sha256.Sum256([]byte("models"))
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := cache.get(context.Background(), key, func() ([]byte, error) {
			close(started)
			<-release
			return []byte("result"), nil
		})
		finished <- err
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := make(chan struct{})
	cancelled := make(chan error, 1)
	go func() {
		_, err := cache.get(waitingPerformanceContext{ctx, waiting}, key, nil)
		cancelled <- err
	}()
	<-waiting
	cancel()
	assert.ErrorIs(t, <-cancelled, context.Canceled)
	close(release)
	require.NoError(t, <-finished)
	body, err := cache.get(context.Background(), key, nil)
	require.NoError(t, err)
	assert.Equal(t, "result", string(body))
}

func TestPerformanceCacheStopsWaitingForSlowQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newPerformanceQueryCache()
		key := sha256.Sum256([]byte("slow query"))
		started, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			_, err := cache.get(context.Background(), key, func() ([]byte, error) {
				close(started)
				<-release
				return []byte("result"), nil
			})
			finished <- err
		}()
		<-started
		// Virtual time advances while both goroutines are blocked. A slow
		// database must not keep all coalesced HTTP requests waiting forever.
		_, err := cache.get(context.Background(), key, nil)
		assert.ErrorIs(t, err, errPerformanceQueriesBusy)
		close(release)
		require.NoError(t, <-finished)
	})
}

func TestPerformanceCacheEvictsWithinEntryAndByteLimits(t *testing.T) {
	for _, tc := range []struct {
		name       string
		maxEntries int
		maxBytes   int
	}{
		{name: "entry limit", maxEntries: 2, maxBytes: 100},
		{name: "byte limit", maxEntries: 100, maxBytes: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := newPerformanceQueryCache()
			cache.maxEntries, cache.maxBytes = tc.maxEntries, tc.maxBytes
			calls := map[string]int{}
			for _, value := range []string{"aa", "bb", "aa", "cc", "aa", "bb"} {
				body, err := cache.get(context.Background(), sha256.Sum256([]byte(value)), func() ([]byte, error) {
					calls[value]++
					return []byte(value), nil
				})
				require.NoError(t, err)
				assert.Equal(t, value, string(body))
			}
			assert.Equal(t, map[string]int{"aa": 1, "bb": 2, "cc": 1}, calls)
		})
	}
}

func TestPerformanceEndpointsReuseQueries(t *testing.T) {
	db := setupPerfMetricsControllerTest(t)
	require.NoError(t, db.Create(&model.Ability{Model: "cached-model", Group: "default", ChannelId: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.PerfMetric{ChannelID: 1, ModelName: "cached-model", Group: "default", BucketTs: time.Now().Add(-time.Hour).Unix(), RequestCount: 1, SuccessCount: 1}).Error)
	queries := 0
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("count_performance_queries", func(tx *gorm.DB) {
		if tx.Statement.Table == "channel_perf_metrics" {
			queries++
		}
	}))
	for _, tc := range []struct {
		path    string
		handler gin.HandlerFunc
	}{
		{path: "/api/perf-metrics/models", handler: GetModelPerformance},
		{path: "/api/perf-metrics/summary", handler: GetPerfMetricsSummary},
		{path: "/api/perf-metrics?model=cached-model", handler: GetPerfMetrics},
	} {
		t.Run(tc.path, func(t *testing.T) {
			before := queries
			var response map[string]any
			requestPerfMetrics(t, tc.path, 0, tc.handler, &response)
			requestPerfMetrics(t, tc.path, 0, tc.handler, &response)
			assert.Equal(t, before+1, queries, "all performance endpoints should share the query protection")
		})
	}
}

func TestModelPerformanceCacheTracksVisiblePairs(t *testing.T) {
	db := setupPerfMetricsControllerTest(t)
	require.NoError(t, db.Create(&[]model.Ability{
		{Model: "cached-model", Group: "default", ChannelId: 1, Enabled: true},
		{Model: "cached-model", Group: "Gemini", ChannelId: 1, Enabled: true},
	}).Error)
	var response struct {
		Data perfmetrics.ModelsResult `json:"data"`
	}
	requestPerfMetrics(t, "/api/perf-metrics/models", 0, GetModelPerformance, &response)
	require.Len(t, response.Data.Models, 1)
	require.Len(t, response.Data.Models[0].Groups, 2)
	require.NoError(t, db.Where(&model.Ability{Model: "cached-model", Group: "Gemini"}).Delete(&model.Ability{}).Error)
	model.InvalidatePricingCache()
	requestPerfMetrics(t, "/api/perf-metrics/models", 0, GetModelPerformance, &response)
	require.Len(t, response.Data.Models, 1)
	require.Len(t, response.Data.Models[0].Groups, 1)
	assert.Equal(t, "default", response.Data.Models[0].Groups[0].Group, "permission changes must bypass cached broader results")
}

func TestPerformanceQueryOverloadReturnsRetryAfter(t *testing.T) {
	original := cachedPerformanceQueries
	cachedPerformanceQueries = newPerformanceQueryCache()
	cachedPerformanceQueries.maxConcurrent = 0
	t.Cleanup(func() { cachedPerformanceQueries = original })
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics/models", nil)
	writePerformanceQuery(ctx, performanceQueryScope{Endpoint: "models", Hours: 24}, func() (any, error) {
		return nil, errors.New("overloaded query must not execute")
	})
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "1", recorder.Header().Get("Retry-After"))
	var body map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "PERF_METRICS_BUSY", body["code"])
}
