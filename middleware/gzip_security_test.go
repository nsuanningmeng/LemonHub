package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZstdRequestLimitsInternalWindowAndDecodedBody(t *testing.T) {
	previousLimit := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previousLimit })
	payload := []byte(`{"message":"hello"}`)
	// A valid non-single-segment zstd frame with a 2 MiB history window and
	// one tiny final raw block. Output-only limits cannot detect this window.
	largeWindow := []byte{0x28, 0xb5, 0x2f, 0xfd, 0x00, 0x58, byte(len(payload)<<3 | 1), 0, 0}
	largeWindow = append(largeWindow, payload...)
	probe, err := zstd.NewReader(nil)
	require.NoError(t, err)
	decoded, err := probe.DecodeAll(largeWindow, nil)
	probe.Close()
	require.NoError(t, err, "the fixture must be a valid frame accepted by an unconstrained decoder")
	require.Equal(t, payload, decoded)

	encoder, err := zstd.NewWriter(nil, zstd.WithWindowSize(1<<20), zstd.WithEncoderConcurrency(1))
	require.NoError(t, err)
	defer encoder.Close()
	for _, tc := range []struct {
		name       string
		compressed []byte
		wantStatus int
		wantBody   []byte
	}{
		{"small body", encoder.EncodeAll(payload, nil), http.StatusOK, payload},
		{"oversized window", largeWindow, http.StatusRequestEntityTooLarge, nil},
		{"exact body limit", encoder.EncodeAll(bytes.Repeat([]byte{'a'}, 1<<20), nil), http.StatusOK, bytes.Repeat([]byte{'a'}, 1<<20)},
		{"decoded body over limit", encoder.EncodeAll(bytes.Repeat([]byte{'a'}, (1<<20)+1), nil), http.StatusRequestEntityTooLarge, nil},
		{"invalid frame", []byte("not a zstd frame"), http.StatusBadRequest, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(DecompressRequestMiddleware())
			router.POST("/", func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				body, err := common.GetBodyStorage(c)
				if common.IsRequestBodyTooLargeError(err) {
					c.AbortWithStatus(http.StatusRequestEntityTooLarge)
					return
				}
				if err != nil {
					c.AbortWithStatus(http.StatusBadRequest)
					return
				}
				data, readErr := io.ReadAll(body)
				require.NoError(t, readErr)
				c.Data(http.StatusOK, "application/octet-stream", data)
			})
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(tc.compressed))
			request.Header.Set("Content-Encoding", "zstd")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.wantStatus, response.Code)
			if tc.wantStatus == http.StatusOK {
				assert.Equal(t, tc.wantBody, response.Body.Bytes())
			}
		})
	}
}

func TestZstdDecoderClosesWhenHandlerRejectsBeforeReading(t *testing.T) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	require.NoError(t, err)
	compressed := encoder.EncodeAll([]byte("private request body"), nil)
	require.NoError(t, encoder.Close())
	var body io.ReadCloser
	router := gin.New()
	router.Use(DecompressRequestMiddleware())
	router.POST("/", func(c *gin.Context) {
		body = c.Request.Body
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(compressed))
	request.Header.Set("Content-Encoding", "zstd")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.NotNil(t, body)
	_, err = body.Read(make([]byte, 1))
	assert.ErrorIs(t, err, zstd.ErrDecoderClosed, "an unread request must still release decoder resources")
}
