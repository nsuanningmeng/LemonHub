package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedRequestBodyLog inserts one record with a body of exactly sizeBytes and the
// given createdAt so eviction ordering (by id/created_at) is deterministic.
func seedRequestBodyLog(t *testing.T, requestId string, sizeBytes int, createdAt int64) {
	t.Helper()
	body := strings.Repeat("x", sizeBytes)
	rec := &RequestBodyLog{
		RequestId: requestId,
		UserId:    42,
		Body:      body,
		Size:      len(body),
		CreatedAt: createdAt,
	}
	require.NoError(t, persistRequestBodyLog(rec))
}

// TestRequestBodyRecordEviction verifies the space-cap invariant: once the total
// stored size exceeds the configured cap, persisting evicts the OLDEST records
// (lowest id first) until the total is back under the cap, and the newest records
// are retained. This is the guarantee the performance-page storage limit relies on.
func TestRequestBodyRecordEviction(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&RequestBodyLog{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&RequestBodyLog{}).Error)

	original := common.GetRequestBodyRecordConfig()
	common.SetRequestBodyRecordConfig(common.RequestBodyRecordConfig{MaxSizeMB: 1})
	t.Cleanup(func() {
		common.SetRequestBodyRecordConfig(original)
		DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&RequestBodyLog{})
	})

	const half = 512 * 1024 // 0.5 MB; cap is 1 MB
	// Insert three 0.5MB records (1.5MB total) with strictly increasing created_at.
	seedRequestBodyLog(t, "req-oldest", half, 1000)
	seedRequestBodyLog(t, "req-middle", half, 2000)
	seedRequestBodyLog(t, "req-newest", half, 3000)

	// Per-insert eviction is time-throttled + serialized (so bursts don't stampede the
	// primary DB), so force one synchronous pass to assert the eviction logic deterministically.
	enforceRequestBodyRecordLimit(true)

	total, count, err := GetRequestBodyRecordStats()
	require.NoError(t, err)
	assert.LessOrEqual(t, total, common.GetRequestBodyRecordMaxSizeBytes(),
		"total stored size must not exceed the configured cap after eviction")
	assert.Equal(t, int64(2), count, "eviction should leave exactly the two newest records")

	// Oldest evicted, two newest retained.
	_, err = GetRequestBodyLogByRequestId("req-oldest")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "oldest record must be evicted first")
	_, err = GetRequestBodyLogByRequestId("req-middle")
	assert.NoError(t, err)
	_, err = GetRequestBodyLogByRequestId("req-newest")
	assert.NoError(t, err)
}

// TestRequestBodyRecordIdempotentByRequestId verifies persisting the same
// request_id twice does not create a duplicate row (the consume-log and error-log
// paths may both attempt to record the same request).
func TestRequestBodyRecordIdempotentByRequestId(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&RequestBodyLog{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&RequestBodyLog{}).Error)
	t.Cleanup(func() {
		DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&RequestBodyLog{})
	})

	first := &RequestBodyLog{RequestId: "dup-req", UserId: 1, Body: "first", Size: len("first"), CreatedAt: 100}
	require.NoError(t, persistRequestBodyLog(first))
	second := &RequestBodyLog{RequestId: "dup-req", UserId: 1, Body: "second", Size: len("second"), CreatedAt: 200}
	require.NoError(t, persistRequestBodyLog(second))

	_, count, err := GetRequestBodyRecordStats()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "same request_id must not create a duplicate record")

	got, err := GetRequestBodyLogByRequestId("dup-req")
	require.NoError(t, err)
	assert.Equal(t, "first", got.Body, "first stored body is kept; duplicate is a no-op")
}

// TestGetRequestBodyLogByRequestIdNotFound verifies unknown/blank ids surface a
// not-found error so the admin view can render a graceful message.
func TestGetRequestBodyLogByRequestIdNotFound(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&RequestBodyLog{}))
	_, err := GetRequestBodyLogByRequestId("does-not-exist")
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
	_, err = GetRequestBodyLogByRequestId("   ")
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

// TestDeleteRequestBodyLogsByUserId verifies per-user purge (used when recording is
// disabled or the user is hard-deleted) removes ONLY that user's recorded bodies and
// leaves other users' records intact, so raw payloads don't linger indefinitely.
func TestDeleteRequestBodyLogsByUserId(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&RequestBodyLog{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&RequestBodyLog{}).Error)
	t.Cleanup(func() {
		DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&RequestBodyLog{})
	})

	require.NoError(t, persistRequestBodyLog(&RequestBodyLog{RequestId: "u7-a", UserId: 7, Body: "a", Size: 1, CreatedAt: 1}))
	require.NoError(t, persistRequestBodyLog(&RequestBodyLog{RequestId: "u7-b", UserId: 7, Body: "b", Size: 1, CreatedAt: 2}))
	require.NoError(t, persistRequestBodyLog(&RequestBodyLog{RequestId: "u9-a", UserId: 9, Body: "c", Size: 1, CreatedAt: 3}))

	deleted, err := DeleteRequestBodyLogsByUserId(7)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted, "both of user 7's records are purged")

	_, err = GetRequestBodyLogByRequestId("u7-a")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = GetRequestBodyLogByRequestId("u9-a")
	assert.NoError(t, err, "other users' records must be untouched")

	// userId 0 is a no-op guard (never mass-deletes).
	n, err := DeleteRequestBodyLogsByUserId(0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}
