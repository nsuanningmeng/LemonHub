package ollama

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ollamaBlockedBody struct {
	first   *strings.Reader
	entered chan struct{}
	closed  chan struct{}
	once    sync.Once
	closes  atomic.Int32
}

func (b *ollamaBlockedBody) Read(p []byte) (int, error) {
	if b.first.Len() > 0 {
		return b.first.Read(p)
	}
	b.once.Do(func() { close(b.entered) })
	<-b.closed
	return 0, io.ErrClosedPipe
}
func (b *ollamaBlockedBody) Close() error {
	if b.closes.Add(1) == 1 {
		close(b.closed)
	}
	return nil
}
func TestOllamaBlockedReadCancellationClosesAndJoins(t *testing.T) {
	body := &ollamaBlockedBody{first: strings.NewReader("{\"message\":{\"content\":\"observed output\"},\"done\":false}\n"), entered: make(chan struct{}), closed: make(chan struct{})}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama"}}
	type result struct {
		usage *dto.Usage
		err   *types.NewAPIError
	}
	done := make(chan result, 1)
	go func() { u, e := ollamaStreamHandler(c, info, &http.Response{Body: body}); done <- result{u, e} }()
	select {
	case <-body.entered:
	case <-time.After(time.Second):
		t.Fatal("scanner never reached blocked read")
	}
	cancel()
	select {
	case out := <-done:
		require.Nil(t, out.err)
		require.NotNil(t, out.usage)
		assert.Positive(t, out.usage.CompletionTokens)
		assert.True(t, info.IsPureDownstreamCancellation())
	case <-time.After(time.Second):
		_ = body.Close()
		<-done
		t.Fatal("cancel did not release blocked read")
	}
	assert.Equal(t, int32(1), body.closes.Load())
}

type ollamaFaultWriter struct {
	gin.ResponseWriter
	mode   string
	calls  int
	cancel context.CancelFunc
}

func (w *ollamaFaultWriter) Write(p []byte) (int, error) { return w.WriteString(string(p)) }
func (w *ollamaFaultWriter) WriteString(s string) (int, error) {
	w.calls++
	if w.calls == 2 {
		switch w.mode {
		case "cancel":
			w.cancel()
			return 0, context.Canceled
		case "short":
			return len(s) - 1, nil
		case "write":
			return 0, errors.New("private-writer-sentinel")
		}
	}
	return w.ResponseWriter.WriteString(s)
}
func (w *ollamaFaultWriter) FlushError() error {
	if w.mode == "flush" && w.calls == 2 {
		return errors.New("private-flush-sentinel")
	}
	w.ResponseWriter.Flush()
	return nil
}
func TestOllamaCheckedWritesStopAndPreserveDoneUsage(t *testing.T) {
	for _, mode := range []string{"write", "short", "flush", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
			w := &ollamaFaultWriter{ResponseWriter: c.Writer, mode: mode, cancel: cancel}
			c.Writer = w
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama"}}
			u, e := ollamaStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader("{\"done\":true,\"message\":{\"content\":\"final\"},\"prompt_eval_count\":10,\"eval_count\":3}\n"))})
			if mode == "cancel" {
				require.Nil(t, e)
				require.NotNil(t, u)
				assert.Equal(t, 13, u.TotalTokens)
				assert.True(t, info.StreamOutcome().UpstreamCompleted)
			} else {
				require.Nil(t, e)
				assert.True(t, info.StreamOutcome().UpstreamFailed)
				assert.Equal(t, 13, u.TotalTokens)
			}
			assert.Equal(t, 2, w.calls)
		})
	}
}

type ollamaReadFailure struct{ cancel context.CancelFunc }

func (b *ollamaReadFailure) Read([]byte) (int, error) { b.cancel(); return 0, context.DeadlineExceeded }
func (b *ollamaReadFailure) Close() error             { return nil }
func TestOllamaSourceFailureWinsLateCancellation(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama"}}
	u, e := ollamaStreamHandler(c, info, &http.Response{Body: &ollamaReadFailure{cancel: cancel}})
	require.Nil(t, e)
	assert.Zero(t, u.TotalTokens)
	assert.True(t, info.StreamOutcome().UpstreamFailed)
	assert.False(t, info.IsPureDownstreamCancellation())
}
func TestOllamaDoneZeroPreservesAllPayload(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama"}}
	raw := `{"done":true,"message":{"content":"LAST","thinking":"THINK","tool_calls":[{"function":{"name":"tool","arguments":{"x":1}}}]},"prompt_eval_count":0,"eval_count":0}`
	u, e := ollamaStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(raw))})
	require.Nil(t, e)
	assert.Zero(t, u.TotalTokens)
	assert.True(t, info.StreamOutcome().UpstreamCompleted)
	assert.Contains(t, recorder.Body.String(), "LAST")
	assert.Contains(t, recorder.Body.String(), "THINK")
	assert.Contains(t, recorder.Body.String(), `\"x\":1`)
	assert.Equal(t, 1, strings.Count(recorder.Body.String(), "data: [DONE]"))
	assert.Contains(t, recorder.Body.String(), `"finish_reason":"tool_calls"`)
}

func TestOllamaWriteFaultStopsBeforeMoreNDJSON(t *testing.T) {
	for _, mode := range []string{"write", "short", "flush"} {
		t.Run(mode, func(t *testing.T) {
			body := &ollamaBlockedBody{first: strings.NewReader("{\"message\":{\"content\":\"observed output\"},\"done\":false}\n"), entered: make(chan struct{}), closed: make(chan struct{})}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			w := &ollamaFaultWriter{ResponseWriter: c.Writer, mode: mode}
			c.Writer = w
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama"}}
			u, e := ollamaStreamHandler(c, info, &http.Response{Body: body})
			require.Nil(t, e)
			require.NotNil(t, u)
			assert.Zero(t, u.TotalTokens)
			assert.True(t, info.StreamOutcome().UpstreamFailed)
			assert.Equal(t, int32(1), body.closes.Load())
			assert.Equal(t, 2, w.calls)
			select {
			case <-body.entered:
				t.Fatal("read continued after downstream failure")
			default:
			}
		})
	}
}
func TestOllamaCancellationBeforeGenerationHasNoBillableOutput(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
	body := &ollamaBlockedBody{first: strings.NewReader(""), entered: make(chan struct{}), closed: make(chan struct{})}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama"}}
	u, e := ollamaStreamHandler(c, info, &http.Response{Body: body})
	require.Nil(t, e)
	assert.Zero(t, u.TotalTokens)
	assert.True(t, info.StreamOutcome().CancelledWithoutBillableOutput)
	assert.Equal(t, int32(1), body.closes.Load())
}
func TestOllamaMalformedNDJSONIsStaticTypedFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama"}}
	_, e := ollamaStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader("{private-NDJSON-sentinel"))})
	require.NotNil(t, e)
	assert.Equal(t, 500, e.StatusCode)
	assert.NotContains(t, e.Error(), "sentinel")
	assert.True(t, info.StreamOutcome().UpstreamFailed)
}

type ollamaRoleCancelWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

func (w *ollamaRoleCancelWriter) FlushError() error { w.ResponseWriter.Flush(); w.cancel(); return nil }

type ollamaUnneededReadBody struct {
	reads  int
	closes int
}

func (b *ollamaUnneededReadBody) Read([]byte) (int, error) { b.reads++; return 0, io.ErrClosedPipe }
func (b *ollamaUnneededReadBody) Close() error             { b.closes++; return nil }
func TestOllamaRoleWriteCancellationDoesNotStartAnotherRead(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
	c.Writer = &ollamaRoleCancelWriter{ResponseWriter: c.Writer, cancel: cancel}
	body := &ollamaUnneededReadBody{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama"}}
	_, e := ollamaStreamHandler(c, info, &http.Response{Body: body})
	require.Nil(t, e)
	assert.Zero(t, body.reads)
	assert.Equal(t, 1, body.closes)
	assert.True(t, info.IsPureDownstreamCancellation())
	assert.True(t, info.StreamOutcome().CancelledWithoutBillableOutput)
}
