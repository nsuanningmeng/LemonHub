package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryRateLimiterLargeLimitDoesNotPreallocate(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)
	maxInt := int(^uint(0) >> 1)
	require.NotPanics(t, func() {
		assert.True(t, limiter.Request("large-limit", maxInt, 60))
	}, "a single accepted request must not allocate memory for the configured upper bound")
}

func TestInMemoryRateLimiterSlidingWindow(t *testing.T) {
	now := time.Unix(1_000, 0)
	limiter := InMemoryRateLimiter{now: func() time.Time { return now }}
	limiter.Init(0)
	for _, tt := range []struct {
		second  int64
		allowed bool
	}{
		{0, true}, {30, true}, {59, false},
		{60, true}, {60, false}, {90, true},
	} {
		now = time.Unix(1_000+tt.second, 0)
		assert.Equal(t, tt.allowed, limiter.Request("window", 2, 60), "second=%d", tt.second)
	}
	assert.True(t, limiter.Request("other-user", 2, 60))
}

func TestInMemoryRateLimiterReservationCompletionIsIdempotent(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)
	failed := limiter.Reserve("user", 1, 60)
	require.NotNil(t, failed)
	assert.Nil(t, limiter.Reserve("user", 1, 60), "an in-flight request reserves the slot")
	failed.Complete(false)
	failed.Complete(false)
	failed.Complete(true)

	successful := limiter.Reserve("user", 1, 60)
	require.NotNil(t, successful, "failure frees the slot exactly once")
	successful.Complete(true)
	successful.Complete(true)
	successful.Complete(false)
	assert.Nil(t, limiter.Reserve("user", 1, 60), "cleanup cannot undo an already recorded success")
}

func TestInMemoryRateLimiterReservationCountsAtCompletionAfterEviction(t *testing.T) {
	now := time.Unix(1_000, 0)
	limiter := InMemoryRateLimiter{now: func() time.Time { return now }}
	limiter.Init(0)
	// Drive cleanup explicitly instead of starting a background ticker.
	limiter.expirationDuration = 30 * time.Second
	reservation := limiter.Reserve("long-running", 1, 60)
	require.NotNil(t, reservation)

	now = now.Add(2 * time.Minute)
	limiter.deleteExpiredEntries(now)
	assert.Nil(t, limiter.Reserve("long-running", 1, 60), "idle-key eviction must preserve active reservations")
	reservation.Complete(true)
	assert.Nil(t, limiter.Reserve("long-running", 1, 60), "success is recorded at completion, not request start")

	now = now.Add(59 * time.Second)
	limiter.deleteExpiredEntries(now)
	assert.Nil(t, limiter.Reserve("long-running", 1, 60))
	now = now.Add(time.Second)
	replacement := limiter.Reserve("long-running", 1, 60)
	require.NotNil(t, replacement)
	replacement.Complete(false)
}

func TestInMemoryRateLimiterCleanupPreservesLongWindows(t *testing.T) {
	now := time.Unix(1_000, 0)
	limiter := InMemoryRateLimiter{now: func() time.Time { return now }}
	limiter.Init(0)
	limiter.expirationDuration = time.Minute
	require.True(t, limiter.Request("long-window", 1, 300))
	require.True(t, limiter.Request("short-window", 1, 60))
	reservation := limiter.Reserve("in-flight", 1, 60)
	require.NotNil(t, reservation)

	now = now.Add(61 * time.Second)
	limiter.deleteExpiredEntries(now)
	assert.False(t, limiter.Request("long-window", 1, 300), "cleanup cannot shorten a configured rate window")
	assert.True(t, limiter.Request("short-window", 1, 60), "expired quota is reusable")
	assert.Nil(t, limiter.Reserve("in-flight", 1, 60))
	reservation.Complete(false)
	replacement := limiter.Reserve("in-flight", 1, 60)
	require.NotNil(t, replacement, "failure also releases a reservation whose idle bucket was evicted")
	replacement.Complete(false)

	now = now.Add(239 * time.Second)
	limiter.deleteExpiredEntries(now)
	assert.True(t, limiter.Request("long-window", 1, 300))
}

func TestInMemoryRateLimiterConcurrentReservations(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)
	start := make(chan struct{})
	results := make(chan *RateLimitReservation, 2)
	for range 2 {
		go func() {
			<-start
			results <- limiter.Reserve("same-user", 1, 60)
		}()
	}
	close(start)
	first, second := <-results, <-results
	require.NotEqual(t, first == nil, second == nil, "exactly one simultaneous request must reserve the slot")
	first.Complete(false)
	second.Complete(false)
	replacement := limiter.Reserve("same-user", 1, 60)
	require.NotNil(t, replacement)
	replacement.Complete(true)
	assert.Nil(t, limiter.Reserve("same-user", 1, 60))
}
