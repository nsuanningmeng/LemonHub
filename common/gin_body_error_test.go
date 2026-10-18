package common

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type interruptedRequestBody struct {
	closed bool
	reads  int
}

func (b *interruptedRequestBody) Read([]byte) (int, error) {
	b.reads++
	if b.closed {
		return 0, errors.New("http: invalid Read on closed Body")
	}
	return 0, io.ErrUnexpectedEOF
}

func (b *interruptedRequestBody) Close() error {
	b.closed = true
	return nil
}

func TestReusableRequestBodyPreservesFirstReadFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := &interruptedRequestBody{}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", body)
	_, err := GetBodyStorage(c)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.True(t, body.closed)
	firstReads := body.reads
	var request map[string]any
	require.ErrorIs(t, UnmarshalBodyReusable(c, &request), io.ErrUnexpectedEOF)
	_, repeated := GetRequestBody(c)
	require.ErrorIs(t, repeated, io.ErrUnexpectedEOF)
	assert.Equal(t, firstReads, body.reads, "later readers must not consume the closed source")
}

func TestReusableRequestBodyPreservesSizeLimitFailure(t *testing.T) {
	oldMax := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = oldMax })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	_, first := GetRequestBody(c)
	require.ErrorIs(t, first, ErrRequestBodyTooLarge)
	_, repeated := GetBodyStorage(c)
	require.ErrorIs(t, repeated, ErrRequestBodyTooLarge)
	assert.EqualError(t, repeated, first.Error())
	assert.Contains(t, repeated.Error(), "1 MB")
}
