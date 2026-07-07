package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runLimiter drives one request through the given middleware and reports whether the
// handler ran (allowed) and the response status.
func runLimiter(t *testing.T, mw func(c *gin.Context)) (handlerRan bool, status int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/epay/notify", nil)
	mw(c)
	// gin defers WriteHeader until the first body write, so httptest recorder.Code can
	// lag; c.Writer.Status() reflects the status set via c.Status() immediately.
	return !c.IsAborted(), c.Writer.Status()
}

// When Redis is unreachable, the payment-webhook limiter must FAIL OPEN — a signed
// gateway callback must reach the handler, because a 500 would strand a paid order
// whose gateway only retries a few times. A normal (fail-closed) limiter must 500.
func TestPaymentWebhookRateLimitFailsOpenOnRedisError(t *testing.T) {
	origEnabled, origRDB := common.RedisEnabled, common.RDB
	t.Cleanup(func() { common.RedisEnabled, common.RDB = origEnabled, origRDB })

	common.RedisEnabled = true
	// A client pointing at a closed port makes every LLen return an error quickly.
	common.RDB = redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 200 * time.Millisecond,
		MaxRetries:  -1,
	})
	t.Cleanup(func() { _ = common.RDB.Close() })

	// Fail-open: payment webhook limiter lets the request through despite the Redis error.
	ran, status := runLimiter(t, PaymentWebhookRateLimit())
	assert.True(t, ran, "payment webhook limiter must fail OPEN and run the handler on Redis error")
	assert.NotEqual(t, http.StatusInternalServerError, status)

	// Fail-closed baseline: a standard Redis-backed limiter aborts 500 on the same error.
	ranClosed, statusClosed := runLimiter(t, rateLimitFactory(1800, 60, "TESTCLOSED"))
	assert.False(t, ranClosed, "fail-closed limiter must abort on Redis error")
	assert.Equal(t, http.StatusInternalServerError, statusClosed)
}

// With Redis DISABLED the payment-webhook limiter uses the in-memory limiter, which
// cannot error — a normal call is allowed through.
func TestPaymentWebhookRateLimitInMemoryAllows(t *testing.T) {
	origEnabled := common.RedisEnabled
	t.Cleanup(func() { common.RedisEnabled = origEnabled })
	common.RedisEnabled = false

	ran, status := runLimiter(t, PaymentWebhookRateLimit())
	require.True(t, ran, "in-memory limiter must allow a single request")
	assert.NotEqual(t, http.StatusTooManyRequests, status)
}
