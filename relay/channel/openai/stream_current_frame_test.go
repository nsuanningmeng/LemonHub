package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type frameNotifyWriter struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
}

func (w *frameNotifyWriter) Flush() {
	w.ResponseRecorder.Flush()
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}
func streamFrameFixture(t *testing.T, writer http.ResponseWriter) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions, IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-test", SupportStreamOptions: true}, ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{}}
	info.SetEstimatePromptTokens(3)
	return c, info
}
func TestOpenAIStreamFirstFrameBeforeNextUpstreamFrame(t *testing.T) {
	w := &frameNotifyWriter{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{}, 1)}
	c, info := streamFrameFixture(t, w)
	reader, writer := io.Pipe()
	release := make(chan struct{})
	done := make(chan *types.NewAPIError, 1)
	go func() {
		_, err := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: reader})
		done <- err
	}()
	go func() {
		defer writer.Close()
		_, _ = io.WriteString(writer, "data: {\"id\":\"first\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		<-release
		_, _ = io.WriteString(writer, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}()
	observed := false
	select {
	case <-w.flushed:
		observed = true
	case <-time.After(2 * time.Second):
	}
	close(release)
	select {
	case err := <-done:
		require.Nil(t, err)
	case <-time.After(3 * time.Second):
		reader.Close()
		t.Fatal("handler did not finish")
	}
	assert.True(t, observed, "first role frame must arrive while next upstream frame remains gated")
	assert.Equal(t, 1, strings.Count(w.Body.String(), `"role":"assistant"`))
	assert.Equal(t, 1, strings.Count(w.Body.String(), `"content":"hello"`))
	assert.Equal(t, 1, strings.Count(w.Body.String(), "[DONE]"))
}
func TestOpenAIStreamCombinedUsageRetainsEverySemanticFrame(t *testing.T) {
	for _, delta := range []string{`{"role":"assistant"}`, `{"tool_calls":[{"index":0,"function":{"name":"tool_a","arguments":"{}"}}]}`, `{"function_call":{"name":"old","arguments":"{}"}}`, `{"refusal":"no"}`, `{"audio":{"data":"audio"}}`, `{"vendor_delta":{"value":"private"}}`} {
		for _, include := range []bool{false, true} {
			t.Run(delta+string(rune('0'+map[bool]int{false: 0, true: 1}[include])), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, info := streamFrameFixture(t, rec)
				info.ShouldIncludeUsage = include
				frame := `{"id":"x","model":"gpt-test","choices":[{"index":1,"delta":` + delta + `}],"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`
				usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + frame + "\n\ndata: [DONE]\n\n"))})
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, 16, usage.TotalTokens)
				var actual map[string]any
				first := strings.Split(rec.Body.String(), "\n")[0]
				require.True(t, strings.HasPrefix(first, "data: "))
				require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(first, "data: "), &actual))
				var expectedDelta any
				require.NoError(t, common.UnmarshalJsonStr(delta, &expectedDelta))
				actualChoices := actual["choices"].([]any)
				assert.Equal(t, expectedDelta, actualChoices[0].(map[string]any)["delta"])
				_, hasUsage := actual["usage"]
				assert.Equal(t, include, hasUsage)
				assert.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
			})
		}
	}
}
func TestOpenAIStreamAudioUsageObservedBeforeTail(t *testing.T) {
	rec := httptest.NewRecorder()
	c, info := streamFrameFixture(t, rec)
	info.UpstreamModelName = "gpt-audio"
	frames := []string{`{"choices":[{"delta":{"content":"hello"}}]}`, `{"choices":[],"usage":{"prompt_tokens":31,"completion_tokens":12,"total_tokens":43,"prompt_tokens_details":{"audio_tokens":20},"completion_tokens_details":{"audio_tokens":8}}}`, `{"choices":[{"delta":{},"finish_reason":"stop"}]}`}
	usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + strings.Join(frames, "\n\ndata: ") + "\n\ndata: [DONE]\n\n"))})
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 43, usage.TotalTokens)
	assert.Equal(t, 20, usage.PromptTokensDetails.AudioTokens)
	assert.Equal(t, 8, usage.CompletionTokenDetails.AudioTokens)
	assert.NotContains(t, rec.Body.String(), `"usage"`)
	assert.Equal(t, 1, strings.Count(rec.Body.String(), `"finish_reason":"stop"`))
}

func TestOpenAIStreamUsageSupplierCacheAndForcedPresentation(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw", true: "forced"}[force], func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, info := streamFrameFixture(t, rec)
			info.ChannelType = constant.ChannelTypeMoonshot
			info.ChannelSetting.ForceFormat = force
			frames := []string{`{"choices":[{"index":0,"delta":{"content":"hello"},"usage":{"cached_tokens":7}}],"usage":{"prompt_tokens":31,"completion_tokens":12,"total_tokens":43}}`, `{"choices":[{"delta":{},"finish_reason":"stop"}]}`}
			usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + strings.Join(frames, "\n\ndata: ") + "\n\ndata: [DONE]\n\n"))})
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 7, usage.PromptTokensDetails.CachedTokens)
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				if strings.HasPrefix(line, "data: {") {
					var frame map[string]any
					require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &frame))
					assert.NotContains(t, frame, "usage")
				}
			}
		})
	}
}
func TestOpenAIStreamFatalEmbeddedErrorNeverFinalizes(t *testing.T) {
	for _, prefix := range []string{"", "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"} {
		rec := httptest.NewRecorder()
		c, info := streamFrameFixture(t, rec)
		usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(prefix + "data: {\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"overload\"}}\n\ndata: [DONE]\n\n"))})
		require.NotNil(t, apiErr)
		assert.Equal(t, 503, apiErr.StatusCode)
		assert.Nil(t, usage)
		assert.NotContains(t, rec.Body.String(), "[DONE]")
		assert.NotContains(t, rec.Body.String(), "overload")
	}
}

func TestOpenAIStreamFinishAndPureUsagePolicy(t *testing.T) {
	for _, reason := range []string{"stop", "length", "content_filter", "tool_calls"} {
		t.Run(reason, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, info := streamFrameFixture(t, rec)
			frame := `{"choices":[{"index":0,"delta":{},"finish_reason":"` + reason + `"}],"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`
			tail := `{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`
			usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + frame + "\n\ndata: " + tail + "\n\ndata: [DONE]\n\n"))})
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 16, usage.TotalTokens)
			assert.Equal(t, 1, strings.Count(rec.Body.String(), `"finish_reason":"`+reason+`"`))
			assert.NotContains(t, rec.Body.String(), `"usage"`)
			assert.Equal(t, 2, strings.Count(rec.Body.String(), "data: "))
		})
	}
}
func TestOpenAIStreamClaudeTerminalOnlyAfterSuccessfulScanner(t *testing.T) {
	for _, withError := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "fatal"}[withError], func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, info := streamFrameFixture(t, rec)
			info.RelayFormat = types.RelayFormatClaude
			body := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":5,\"total_tokens\":16}}\n\n"
			if withError {
				body += "data: {\"error\":{\"code\":\"server_is_overloaded\"}}\n\n"
			}
			body += "data: [DONE]\n\n"
			usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))})
			if withError {
				require.NotNil(t, apiErr)
				assert.Nil(t, usage)
				assert.NotContains(t, rec.Body.String(), "message_stop")
				assert.NotContains(t, rec.Body.String(), "message_delta")
			} else {
				require.Nil(t, apiErr)
				assert.Equal(t, 1, strings.Count(rec.Body.String(), "event: message_stop\n"))
				assert.Contains(t, rec.Body.String(), `"output_tokens":5`)
			}
			assert.Contains(t, rec.Body.String(), `"text":"hello"`)
		})
	}
}

type rejectingStreamWriter struct {
	*httptest.ResponseRecorder
	reject       string
	flushFailure bool
}

func (w *rejectingStreamWriter) Write(data []byte) (int, error) { return w.WriteString(string(data)) }
func (w *rejectingStreamWriter) WriteString(data string) (int, error) {
	if w.reject == "all" || strings.Contains(data, w.reject) && w.reject != "" {
		return 0, io.ErrClosedPipe
	}
	return w.ResponseRecorder.WriteString(data)
}
func (w *rejectingStreamWriter) FlushError() error {
	if w.flushFailure {
		return io.ErrClosedPipe
	}
	w.ResponseRecorder.Flush()
	return nil
}

// Gin's legacy Flush method cannot expose an inner FlushError. This wrapper
// explicitly advertises the error-aware flush contract at the active boundary.
type errorAwareGinStreamWriter struct {
	gin.ResponseWriter
	underlying *rejectingStreamWriter
}

func (w *errorAwareGinStreamWriter) FlushError() error { return w.underlying.FlushError() }

func TestOpenAIStreamActualWriteAndFinalFlushFailures(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini} {
		for _, flush := range []bool{false, true} {
			t.Run(string(format)+map[bool]string{false: "write", true: "flush"}[flush], func(t *testing.T) {
				w := &rejectingStreamWriter{ResponseRecorder: httptest.NewRecorder(), flushFailure: flush}
				if !flush {
					w.reject = "all"
				}
				c, info := streamFrameFixture(t, w)
				if flush {
					c.Writer = &errorAwareGinStreamWriter{ResponseWriter: c.Writer, underlying: w}
				}
				info.RelayFormat = format
				usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))})
				require.NotNil(t, apiErr)
				assert.Nil(t, usage)
				assert.True(t, c.GetBool("relay_stream_write_failed"))
				assert.NotContains(t, apiErr.Error(), "closed pipe")
				assert.NotContains(t, w.Body.String(), "[DONE]")
				assert.NotContains(t, w.Body.String(), "message_stop")
			})
		}
	}
	t.Run("final-write", func(t *testing.T) {
		w := &rejectingStreamWriter{ResponseRecorder: httptest.NewRecorder(), reject: "[DONE]"}
		c, info := streamFrameFixture(t, w)
		usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))})
		require.NotNil(t, apiErr)
		assert.Nil(t, usage)
		assert.True(t, c.GetBool("relay_stream_write_failed"))
		assert.NotContains(t, w.Body.String(), "[DONE]")
	})
}
func TestOpenAIStreamUnknownTopLevelUsageFramePreserved(t *testing.T) {
	rec := httptest.NewRecorder()
	c, info := streamFrameFixture(t, rec)
	frame := `{"choices":[],"vendor_top":{"keep":true},"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`
	usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + frame + "\n\ndata: [DONE]\n\n"))})
	require.Nil(t, apiErr)
	assert.Equal(t, 16, usage.TotalTokens)
	assert.Contains(t, rec.Body.String(), `"vendor_top":{"keep":true}`)
	assert.NotContains(t, rec.Body.String(), `"usage"`)
}

func TestOpenAIStreamEmptyErrorEnvelopeIsNotSemanticUsageOutput(t *testing.T) {
	for _, empty := range []string{"null", "{}", "\"\""} {
		t.Run(empty, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, info := streamFrameFixture(t, rec)
			frame := `{"choices":[],"error":` + empty + `,"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`
			usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + frame + "\n\ndata: [DONE]\n\n"))})
			require.Nil(t, apiErr)
			assert.Equal(t, 16, usage.TotalTokens)
			assert.Equal(t, "data: [DONE]\n\n", rec.Body.String())
		})
	}
}
