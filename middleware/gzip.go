package middleware

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

type readCloser struct {
	io.Reader
	closeFn func() error
}

func (rc *readCloser) Close() error {
	if rc.closeFn != nil {
		return rc.closeFn()
	}
	return nil
}

type boundedZstdReader struct {
	decoder  *zstd.Decoder
	maxBytes int64
}

func (r *boundedZstdReader) Read(p []byte) (int, error) {
	n, err := r.decoder.Read(p)
	if errors.Is(err, zstd.ErrDecoderSizeExceeded) || errors.Is(err, zstd.ErrWindowSizeExceeded) {
		// Preserve the same 413 classification used by the post-decompression
		// MaxBytesReader when the decoder rejects an oversized frame internally.
		return n, &http.MaxBytesError{Limit: r.maxBytes}
	}
	return n, err
}

func DecompressRequestMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		maxMB := constant.MaxRequestBodyMB
		if maxMB <= 0 {
			maxMB = 32
		}
		maxBytes := int64(maxMB) << 20

		origBody := c.Request.Body
		wrapMaxBytes := func(body io.ReadCloser) io.ReadCloser {
			return http.MaxBytesReader(c.Writer, body, maxBytes)
		}
		decompressed := false

		switch c.GetHeader("Content-Encoding") {
		case "gzip":
			gzipReader, err := gzip.NewReader(origBody)
			if err != nil {
				_ = origBody.Close()
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
			// Replace the request body with the decompressed data, and enforce a max size (post-decompression).
			c.Request.Body = wrapMaxBytes(&readCloser{
				Reader: gzipReader,
				closeFn: func() error {
					_ = gzipReader.Close()
					return origBody.Close()
				},
			})
			decompressed = true
		case "br":
			reader := brotli.NewReader(origBody)
			c.Request.Body = wrapMaxBytes(&readCloser{
				Reader: reader,
				closeFn: func() error {
					return origBody.Close()
				},
			})
			decompressed = true
		case "zstd":
			// Limit internal history/window allocation as well as decoded output.
			// A single synchronous decoder also avoids per-request background workers.
			reader, err := zstd.NewReader(origBody,
				zstd.WithDecoderMaxMemory(uint64(maxBytes)),
				zstd.WithDecoderConcurrency(1),
			)
			if err != nil {
				_ = origBody.Close()
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
			defer reader.Close()
			c.Request.Body = wrapMaxBytes(&readCloser{
				Reader: &boundedZstdReader{decoder: reader, maxBytes: maxBytes},
				closeFn: func() error {
					reader.Close()
					return origBody.Close()
				},
			})
			decompressed = true
		default:
			// Even for uncompressed bodies, enforce a max size to avoid huge request allocations.
			c.Request.Body = wrapMaxBytes(origBody)
		}

		if decompressed {
			c.Request.Header.Del("Content-Encoding")
		}

		// Continue processing the request
		c.Next()
	}
}
