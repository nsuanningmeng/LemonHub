package controller

import (
	"container/list"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/gin-gonic/gin"
)

var errPerformanceQueriesBusy = errors.New("performance queries are busy")

type performanceQueryScope struct {
	Endpoint      string
	Hours         int
	Model         string
	Group         string
	Models        map[string][]string
	BucketSeconds int64
}

type performanceCacheEntry struct {
	key       [32]byte
	body      []byte
	err       error
	expiresAt time.Time
}

type performanceQueryFlight struct {
	done chan struct{}
	body []byte
	err  error
}

// Cache only serialized responses: callers cannot mutate shared results, and
// both entry count and retained response bytes have a hard bound. This is local
// to each process because unflushed performance samples are also local.
type performanceQueryCache struct {
	mu            sync.Mutex
	entries       map[[32]byte]*list.Element
	lru           *list.List
	flights       map[[32]byte]*performanceQueryFlight
	bytes         int
	maxEntries    int
	maxBytes      int
	maxConcurrent int
	maxWaiters    int
	waiters       int
	now           func() time.Time
}

func newPerformanceQueryCache() *performanceQueryCache {
	return &performanceQueryCache{
		entries: make(map[[32]byte]*list.Element), lru: list.New(),
		flights:    make(map[[32]byte]*performanceQueryFlight),
		maxEntries: 128, maxBytes: 16 << 20, maxConcurrent: 4, maxWaiters: 64, now: time.Now,
	}
}

var cachedPerformanceQueries = newPerformanceQueryCache()

func (cache *performanceQueryCache) evict(element *list.Element) {
	entry := element.Value.(performanceCacheEntry)
	delete(cache.entries, entry.key)
	cache.bytes -= len(entry.body)
	cache.lru.Remove(element)
}

func (cache *performanceQueryCache) get(ctx context.Context, key [32]byte, load func() ([]byte, error)) (body []byte, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cache.mu.Lock()
	if element, exists := cache.entries[key]; exists {
		entry := element.Value.(performanceCacheEntry)
		if cache.now().Before(entry.expiresAt) {
			cache.lru.MoveToFront(element)
			cache.mu.Unlock()
			return entry.body, entry.err
		}
		cache.evict(element)
	}
	if flight, exists := cache.flights[key]; exists {
		if cache.waiters >= cache.maxWaiters {
			cache.mu.Unlock()
			return nil, errPerformanceQueriesBusy
		}
		cache.waiters++
		cache.mu.Unlock()
		defer func() {
			cache.mu.Lock()
			cache.waiters--
			cache.mu.Unlock()
		}()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, errPerformanceQueriesBusy
		case <-flight.done:
			return flight.body, flight.err
		}
	}
	if len(cache.flights) >= cache.maxConcurrent {
		cache.mu.Unlock()
		return nil, errPerformanceQueriesBusy
	}
	flight := &performanceQueryFlight{done: make(chan struct{})}
	cache.flights[key] = flight
	cache.mu.Unlock()

	completed := false
	defer func() {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		// Release waiters even if a loader panics and Gin recovers the request.
		if !completed {
			flight.err = errors.New("performance query did not complete")
		} else {
			flight.body, flight.err = body, err
			if len(body) <= cache.maxBytes && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				ttl := 30 * time.Second
				if err != nil {
					// A failed database must not be retried on every incoming request.
					ttl = 2 * time.Second
				}
				for cache.lru.Len() > 0 && (cache.lru.Len() >= cache.maxEntries || cache.bytes+len(body) > cache.maxBytes) {
					cache.evict(cache.lru.Back())
				}
				entry := performanceCacheEntry{key: key, body: body, err: err, expiresAt: cache.now().Add(ttl)}
				cache.entries[key] = cache.lru.PushFront(entry)
				cache.bytes += len(body)
			}
		}
		delete(cache.flights, key)
		close(flight.done)
	}()
	body, err = load()
	completed = true
	return body, err
}

// Recompute visibility before every call. The key contains the exact visible
// model/group pairs, never just a user-supplied filter or a cached user group.
func writePerformanceQuery(c *gin.Context, scope performanceQueryScope, query func() (any, error)) {
	if scope.Hours <= 0 {
		scope.Hours = 24
	}
	if scope.Hours > 720 {
		scope.Hours = 720
	}
	scope.BucketSeconds = perf_metrics_setting.GetBucketSeconds()
	canonical := make(map[string][]string, len(scope.Models))
	for name, groups := range scope.Models {
		groups = append([]string(nil), groups...)
		sort.Strings(groups)
		canonical[name] = slices.Compact(groups)
	}
	scope.Models = canonical
	keyBytes, err := common.Marshal(scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "PERF_METRICS_QUERY_FAILED", "message": "failed to query model performance"})
		return
	}
	body, err := cachedPerformanceQueries.get(c.Request.Context(), sha256.Sum256(keyBytes), func() ([]byte, error) {
		result, queryErr := query()
		if queryErr != nil {
			logger.LogError(c, "failed to query model performance: "+queryErr.Error())
			return nil, queryErr
		}
		return common.Marshal(gin.H{"success": true, "message": "", "data": result})
	})
	if errors.Is(err, errPerformanceQueriesBusy) {
		c.Header("Retry-After", "1")
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "code": "PERF_METRICS_BUSY", "message": "performance queries are busy; retry later"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "PERF_METRICS_QUERY_FAILED", "message": "failed to query model performance"})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}
