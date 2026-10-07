package helper

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorStreamWriter struct {
	header   http.Header
	body     []byte
	err      error
	short    bool
	flushErr error
	flushes  int
}

func (w *errorStreamWriter) Header() http.Header { return w.header }
func (w *errorStreamWriter) WriteHeader(int)     {}
func (w *errorStreamWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.short {
		return len(p) - 1, nil
	}
	w.body = append(w.body, p...)
	return len(p), nil
}
func (w *errorStreamWriter) Flush()            { w.flushes++ }
func (w *errorStreamWriter) FlushError() error { w.flushes++; return w.flushErr }
func writerContext(w http.ResponseWriter) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	return c
}
func TestStreamWritersPropagateActualWriteAndFlushFailures(t *testing.T) {
	for _, emit := range []struct {
		name string
		fn   func(*gin.Context) error
	}{
		{"chat", func(c *gin.Context) error { return StringData(c, "hello") }},
		{"claude", func(c *gin.Context) error { return ClaudeData(c, dto.ClaudeResponse{Type: "message_start"}) }},
		{"responses", func(c *gin.Context) error {
			return ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.created"}, `{"type":"response.created"}`)
		}},
		{"done", Done},
		{"ping", PingData},
	} {
		t.Run(emit.name, func(t *testing.T) {
			sentinel := errors.New("actual writer failure")
			w := &errorStreamWriter{header: make(http.Header), err: sentinel}
			cWrite := writerContext(w)
			require.ErrorIs(t, emit.fn(cWrite), sentinel)
			assert.False(t, StreamDataWritten(cWrite))
			assert.True(t, cWrite.GetBool("relay_stream_write_failed"))
			assert.Zero(t, w.flushes)
			w = &errorStreamWriter{header: make(http.Header), short: true}
			require.ErrorIs(t, emit.fn(writerContext(w)), io.ErrShortWrite)
			w = &errorStreamWriter{header: make(http.Header), flushErr: sentinel}
			cFlush := writerContext(w)
			cFlush.Writer = &explicitFlushErrorWriter{ResponseWriter: cFlush.Writer, target: w}
			require.ErrorIs(t, emit.fn(cFlush), sentinel)
			assert.True(t, cFlush.GetBool("relay_stream_write_failed"))
			assert.Equal(t, 1, w.flushes)
			w = &errorStreamWriter{header: make(http.Header)}
			c := writerContext(w)
			ctx, cancel := context.WithCancel(c.Request.Context())
			cancel()
			c.Request = c.Request.WithContext(ctx)
			require.ErrorIs(t, emit.fn(c), context.Canceled)
			assert.False(t, c.GetBool("relay_stream_write_failed"))
			assert.Empty(t, w.body)
			assert.Zero(t, w.flushes)
		})
	}
}
func TestStreamWriterPreservesFrameBytesAndCommitsOnlyOnEmission(t *testing.T) {
	w := &errorStreamWriter{header: make(http.Header)}
	c := writerContext(w)
	SetEventStreamHeaders(c)
	assert.False(t, c.Writer.Written())
	require.NoError(t, StringData(c, "one\r\ntwo"))
	assert.Equal(t, "data: one\\r\ntwo\n\n", string(w.body))
	assert.True(t, c.Writer.Written())
	w = &errorStreamWriter{header: make(http.Header)}
	c = writerContext(w)
	require.NoError(t, ClaudeData(c, dto.ClaudeResponse{Type: "message_start"}))
	assert.NotContains(t, string(w.body), "\n\n\n")
	w = &errorStreamWriter{header: make(http.Header)}
	c = writerContext(w)
	require.NoError(t, ClaudeChunkData(c, dto.ClaudeResponse{Type: "content_block_delta"}, `{"delta":"x"}`))
	assert.Equal(t, "event: content_block_delta\ndata: {\"delta\":\"x\"}\n\n\n", string(w.body))
}

type bufferedFlushWriter struct {
	writer     *errorStreamWriter
	underlying http.ResponseWriter
	flushed    bool
}

func (w *bufferedFlushWriter) Header() http.Header         { return w.writer.Header() }
func (w *bufferedFlushWriter) WriteHeader(status int)      { w.writer.WriteHeader(status) }
func (w *bufferedFlushWriter) Write(p []byte) (int, error) { return w.writer.Write(p) }
func (w *bufferedFlushWriter) Unwrap() http.ResponseWriter { return w.underlying }
func (w *bufferedFlushWriter) Flush()                      { w.flushed = true; w.writer.Flush() }
func TestFlushWriterRespectsIntermediateBufferedFlusher(t *testing.T) {
	base := &errorStreamWriter{header: make(http.Header), flushErr: errors.New("must not bypass wrapper")}
	buffer := &bufferedFlushWriter{writer: &errorStreamWriter{header: make(http.Header)}, underlying: base}
	require.NoError(t, FlushWriter(writerContext(buffer)))
	assert.True(t, buffer.flushed)
	assert.Zero(t, base.flushes)
}

// The outer interface must explicitly expose FlushError; Gin's no-error Flush
// cannot reveal an inner transport error without bypassing wrapper semantics.
type explicitFlushErrorWriter struct {
	gin.ResponseWriter
	target *errorStreamWriter
}

func (w *explicitFlushErrorWriter) FlushError() error { return w.target.FlushError() }
func TestFlushWriterGinNoErrorInterfaceLimit(t *testing.T) {
	w := &errorStreamWriter{header: make(http.Header), flushErr: errors.New("inner hidden error")}
	c := writerContext(w)
	require.NoError(t, FlushWriter(c))
	assert.Equal(t, 1, w.flushes)
	assert.False(t, c.GetBool("relay_stream_write_failed"))
}

type outerBufferedGinWriter struct {
	gin.ResponseWriter
	flushed bool
}

func (w *outerBufferedGinWriter) Flush() { w.flushed = true; w.ResponseWriter.Flush() }
func TestFlushWriterRespectsOutermostGinWrapper(t *testing.T) {
	w := &errorStreamWriter{header: make(http.Header)}
	c := writerContext(w)
	outer := &outerBufferedGinWriter{ResponseWriter: c.Writer}
	c.Writer = outer
	require.NoError(t, FlushWriter(c))
	assert.True(t, outer.flushed)
}
