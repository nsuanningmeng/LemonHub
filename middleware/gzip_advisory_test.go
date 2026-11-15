package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecompressRequestRejectsZstdHuffmanCountOverflow(t *testing.T) {
	// Exact regression frames from klauspost/compress commit
	// 7f5d8528720c895a6106f201719f87eb030a565d, TestDecoderHuffWeightsTableLog
	// (GO-2026-6865). The second block overflowed an FSE count and reused the
	// first block's table, so a small window/output limit alone cannot stop it.
	oneFrame := slices.Concat(
		[]byte{0x28, 0xb5, 0x2f, 0xfd, 0x00, 0x38, 0x74, 0x06, 0x00, 0x42, 0xb8, 0x32, 0x0d, 0x1a, 0x00, 0x00, 0x00, 0x90, 0xaf, 0xfa, 0xff, 0x03, 0x00, 0x80, 0xff, 0x7f},
		bytes.Repeat([]byte{0xa5, 0x94, 0x52, 0x4a, 0x29}, 37),
		[]byte{0xa5, 0x94, 0x12, 0x00, 0xad, 0x56, 0x03, 0xee, 0xff, 0xdf, 0xb3, 0x1a, 0x1c, 0x1a, 0x00, 0xe0, 0xff, 0x1f, 0x0f, 0xb1, 0x9c, 0x5a, 0xcf, 0x4d, 0xe4, 0x1d, 0x7b, 0xab, 0x99, 0x5e, 0x01, 0x51, 0xa4, 0x73, 0xd8, 0xb1, 0xf3, 0x39, 0x4b, 0x67, 0xf6, 0xab, 0x1a, 0xab, 0x1a, 0xab, 0x1a},
		bytes.Repeat([]byte{0x8c, 0x31, 0xc6, 0x18, 0x63}, 1365),
		[]byte{0x8c, 0x31},
		bytes.Repeat([]byte{0x63, 0x8c, 0x31, 0xc6, 0x18}, 1365),
		[]byte{0x63, 0x2c},
		bytes.Repeat([]byte{0x31, 0xc6, 0x18, 0x63, 0x8c}, 1365),
		[]byte{0x31, 0x46},
		bytes.Repeat([]byte{0x31, 0xc6, 0x18, 0x63, 0x8c}, 1365),
		[]byte{0x31, 0x06, 0x00},
	)
	twoFrames := slices.Concat(oneFrame[:6], []byte{0x75, 0x06, 0x00}, oneFrame[9:215], oneFrame[:6], oneFrame[215:])
	previousLimit := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previousLimit })

	for name, compressed := range map[string][]byte{"one frame": oneFrame, "two frames": twoFrames} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(DecompressRequestMiddleware())
			router.POST("/", func(c *gin.Context) {
				_, err := io.ReadAll(c.Request.Body)
				require.Error(t, err, "malformed Huffman weights must be rejected without a panic")
				assert.False(t, common.IsRequestBodyTooLargeError(err), "the decoder must reject the table itself")
				c.AbortWithStatus(http.StatusBadRequest)
			})
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(compressed))
			request.Header.Set("Content-Encoding", "zstd")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.NoError(t, request.Body.Close())
			assert.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}

func TestDecompressRequestBrotliCompatibility(t *testing.T) {
	previousLimit := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previousLimit })
	for _, tc := range []struct {
		name       string
		payload    []byte
		malformed  bool
		wantStatus int
	}{
		{"valid body", []byte(`{"message":"hello"}`), false, http.StatusOK},
		{"exact decoded limit", bytes.Repeat([]byte{'a'}, 1<<20), false, http.StatusOK},
		{"decoded body over limit", bytes.Repeat([]byte{'a'}, (1<<20)+1), false, http.StatusRequestEntityTooLarge},
		{"malformed body", []byte{0xff, 0xff, 0xff}, true, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compressed := tc.payload
			if !tc.malformed {
				var encoded bytes.Buffer
				writer := brotli.NewWriterLevel(&encoded, 1)
				_, err := writer.Write(tc.payload)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				compressed = encoded.Bytes()
			}
			router := gin.New()
			router.Use(DecompressRequestMiddleware())
			router.POST("/", func(c *gin.Context) {
				data, err := io.ReadAll(c.Request.Body)
				if common.IsRequestBodyTooLargeError(err) {
					c.AbortWithStatus(http.StatusRequestEntityTooLarge)
					return
				}
				if err != nil {
					c.AbortWithStatus(http.StatusBadRequest)
					return
				}
				c.Data(http.StatusOK, "application/octet-stream", data)
			})
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(compressed))
			request.Header.Set("Content-Encoding", "br")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.NoError(t, request.Body.Close())
			require.Equal(t, tc.wantStatus, response.Code)
			if tc.wantStatus == http.StatusOK {
				assert.Equal(t, tc.payload, response.Body.Bytes())
			}
		})
	}
}
