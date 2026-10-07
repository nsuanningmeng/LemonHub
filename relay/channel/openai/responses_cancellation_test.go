package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type responsesCloseNotifyingBody struct {
	io.ReadCloser
	closed chan struct{}
	once   sync.Once
}

func (b *responsesCloseNotifyingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return b.ReadCloser.Close()
}

type responsesCancelOnFlushWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	match  string
	once   sync.Once
}

func (w *responsesCancelOnFlushWriter) Flush() {
	w.ResponseRecorder.Flush()
	if strings.Contains(w.Body.String(), w.match) {
		w.once.Do(w.cancel)
	}
}

type responsesHandlerResult struct {
	usage *dto.Usage
	err   *types.NewAPIError
}

func TestResponsesCompletedStopsBeforeUpstreamEOF(t *testing.T) {
	for _, converted := range []bool{false, true} {
		for _, terminal := range []string{"response.completed", "response.done"} {
			t.Run(map[bool]string{false: "native", true: "converted"}[converted]+terminal, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, info := streamFrameFixture(t, rec)
				if !converted {
					info.RelayFormat = types.RelayFormatOpenAIResponses
				}
				ctx, cancel := context.WithCancel(c.Request.Context())
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				reader, writer := io.Pipe()
				body := &responsesCloseNotifyingBody{ReadCloser: reader, closed: make(chan struct{})}
				result := make(chan responsesHandlerResult, 1)
				go func() {
					var u *dto.Usage
					var e *types.NewAPIError
					resp := &http.Response{StatusCode: 200, Body: body}
					if converted {
						u, e = OaiResponsesToChatStreamHandler(c, info, resp)
					} else {
						u, e = OaiResponsesStreamHandler(c, info, resp)
					}
					result <- responsesHandlerResult{u, e}
				}()
				go func() {
					defer writer.Close()
					_, _ = io.WriteString(writer, `data: {"type":"`+terminal+`","response":{"id":"resp-finished","status":"completed","usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13}}}`+"\n\n")
					<-body.closed
				}()
				timely := true
				var got responsesHandlerResult
				select {
				case got = <-result:
				case <-time.After(2 * time.Second):
					timely = false
					cancel()
					select {
					case got = <-result:
					case <-time.After(3 * time.Second):
						reader.Close()
						t.Fatal("handler failed to stop")
					}
				}
				assert.True(t, timely, "protocol completion must close upstream without waiting for EOF")
				require.Nil(t, got.err)
				require.NotNil(t, got.usage)
				assert.Equal(t, 13, got.usage.TotalTokens)
				require.NotNil(t, info.StreamStatus)
				assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
			})
		}
	}
}
func TestResponsesCompletedUsageSurvivesTerminalClientCancel(t *testing.T) {
	for _, converted := range []bool{false, true} {
		for _, zero := range []bool{false, true} {
			t.Run(map[bool]string{false: "native", true: "converted"}[converted]+map[bool]string{false: "usage", true: "zero"}[zero], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				match := "response.completed"
				if converted {
					match = "finish_reason"
				}
				rec := &responsesCancelOnFlushWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, match: match}
				c, info := streamFrameFixture(t, rec)
				c.Request = c.Request.WithContext(ctx)
				info.ShouldIncludeUsage = true
				if !converted {
					info.RelayFormat = types.RelayFormatOpenAIResponses
				}
				usageJSON := `{"input_tokens":9,"output_tokens":4,"total_tokens":13}`
				if zero {
					usageJSON = `{"input_tokens":0,"output_tokens":0,"total_tokens":0}`
				}
				body := `data: {"type":"response.output_text.delta","output_index":0,"delta":"already generated text"}` + "\n\n" + `data: {"type":"response.completed","response":{"status":"completed","usage":` + usageJSON + `}}` + "\n\n"
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
				if converted {
					usage, apiErr = OaiResponsesToChatStreamHandler(c, info, resp)
				} else {
					usage, apiErr = OaiResponsesStreamHandler(c, info, resp)
				}
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				expected := 13
				if zero {
					expected = 0
				}
				assert.Equal(t, expected, usage.TotalTokens)
				require.NotNil(t, info.StreamStatus)
				assert.True(t, info.StreamStatus.IsNormalEnd())
				assert.True(t, info.StreamOutcome().UpstreamCompleted)
				assert.False(t, info.IsPureDownstreamCancellation())
			})
		}
	}
}

func TestResponsesPartialCancellationRetainsOnlyObservedUsage(t *testing.T) {
	for _, converted := range []bool{false, true} {
		for _, kind := range []string{"text", "tool", "reasoning", "created"} {
			t.Run(map[bool]string{false: "native", true: "converted"}[converted]+kind, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				match := "generated output"
				var events []string
				expectedText := "generated output"
				switch kind {
				case "text":
					events = []string{`{"type":"response.output_text.delta","output_index":0,"item_id":"text","delta":"generated output"}`}
				case "tool":
					events = []string{`{"type":"response.output_item.added","output_index":0,"item":{"id":"tool","type":"function_call","call_id":"call","name":"tool_a","arguments":""}}`, `{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"tool","delta":"generated output"}`}
				case "reasoning":
					events = []string{`{"type":"response.reasoning_text.delta","output_index":1,"item_id":"reason","delta":"generated output"}`, `{"type":"response.output_text.delta","output_index":0,"item_id":"text","delta":"cancel trigger"}`}
					match = "cancel trigger"
					expectedText = "generated outputcancel trigger"
				case "created":
					events = []string{`{"type":"response.created","response":{"id":"r","status":"in_progress","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`}
					match = "response.created"
					if converted {
						match = `"role":"assistant"`
					}
				}
				rec := &responsesCancelOnFlushWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, match: match}
				c, info := streamFrameFixture(t, rec)
				c.Request = c.Request.WithContext(ctx)
				info.UpstreamModelName = "test-model"
				if !converted {
					info.RelayFormat = types.RelayFormatOpenAIResponses
				}
				var body strings.Builder
				for _, event := range events {
					body.WriteString("data: " + event + "\n\n")
				}
				reader, writer := io.Pipe()
				closeBody := &responsesCloseNotifyingBody{ReadCloser: reader, closed: make(chan struct{})}
				go func() { defer writer.Close(); _, _ = io.WriteString(writer, body.String()); <-closeBody.closed }()
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				resp := &http.Response{StatusCode: 200, Body: closeBody}
				if converted {
					usage, apiErr = OaiResponsesToChatStreamHandler(c, info, resp)
				} else {
					usage, apiErr = OaiResponsesStreamHandler(c, info, resp)
				}
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.True(t, info.IsPureDownstreamCancellation())
				assert.False(t, info.StreamOutcome().UpstreamCompleted)
				assert.False(t, info.StreamOutcome().UpstreamFailed)
				if kind == "created" {
					assert.Zero(t, usage.TotalTokens)
					assert.True(t, info.StreamOutcome().CancelledWithoutBillableOutput)
				} else {
					assert.Equal(t, 3, usage.PromptTokens)
					assert.Equal(t, service.CountTextToken(expectedText, "test-model"), usage.CompletionTokens)
					assert.False(t, info.StreamOutcome().CancelledWithoutBillableOutput)
				}
				assert.NotContains(t, rec.Body.String(), "[DONE]")
				assert.NotContains(t, rec.Body.String(), "message_stop")
			})
		}
	}
}

func TestResponsesConvertedNonCancellationWriteErrorsKeepTypedFailure(t *testing.T) {
	for _, reject := range []string{"finish_reason", `"usage":{"prompt_tokens"`, "[DONE]"} {
		t.Run(reject, func(t *testing.T) {
			rec := &rejectingStreamWriter{ResponseRecorder: httptest.NewRecorder(), reject: reject}
			c, info := streamFrameFixture(t, rec)
			info.ShouldIncludeUsage = true
			body := `data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13}}}` + "\n\n"
			usage, apiErr := OaiResponsesToChatStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))})
			require.NotNil(t, apiErr)
			assert.Equal(t, 500, apiErr.StatusCode)
			assert.Nil(t, usage)
			assert.NoError(t, c.Request.Context().Err())
			assert.True(t, c.GetBool("relay_stream_write_failed"))
			assert.True(t, info.StreamOutcome().UpstreamCompleted)
			assert.False(t, info.IsPureDownstreamCancellation())
			assert.NotContains(t, apiErr.Error(), "closed pipe")
		})
	}
}
func TestResponsesNativeNonCancellationWriteFailurePreservesEndpointBilling(t *testing.T) {
	rec := &rejectingStreamWriter{ResponseRecorder: httptest.NewRecorder(), reject: "response.completed"}
	c, info := streamFrameFixture(t, rec)
	info.RelayFormat = types.RelayFormatOpenAIResponses
	body := `data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13}}}` + "\n\n"
	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, apiErr)
	assert.Equal(t, 13, usage.TotalTokens)
	assert.True(t, c.GetBool("relay_stream_write_failed"))
	assert.NoError(t, c.Request.Context().Err())
	assert.True(t, info.StreamOutcome().UpstreamCompleted)
}
func TestResponsesNativeCompletedImageCancellationAndIncompleteControl(t *testing.T) {
	for _, kind := range []string{"completed-duplicate", "partial", "empty", "source-incomplete"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			match := "extension.cancel"
			if kind == "source-incomplete" {
				match = "response.incomplete"
			}
			rec := &responsesCancelOnFlushWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, match: match}
			c, info := streamFrameFixture(t, rec)
			info.RelayFormat = types.RelayFormatOpenAIResponses
			c.Request = c.Request.WithContext(ctx)
			item := `{"type":"response.output_item.done","output_index":0,"item":{"id":"image","type":"image_generation_call","status":"completed","result":"opaque-image-result"}}`
			if kind == "partial" {
				item = strings.Replace(item, `"status":"completed"`, `"status":"partial"`, 1)
			} else if kind == "empty" {
				item = strings.Replace(item, "opaque-image-result", "", 1)
			}
			tail := `{"type":"extension.cancel"}`
			if kind == "source-incomplete" {
				tail = `{"type":"response.incomplete","response":{"status":"incomplete","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`
			}
			reader, writer := io.Pipe()
			body := &responsesCloseNotifyingBody{ReadCloser: reader, closed: make(chan struct{})}
			go func() {
				defer writer.Close()
				_, _ = io.WriteString(writer, "data: "+item+"\n\ndata: "+item+"\n\ndata: "+tail+"\n\n")
				<-body.closed
			}()
			usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{StatusCode: 200, Body: body})
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			count := 0
			if info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration] != nil {
				count = info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount
			}
			if kind == "completed-duplicate" {
				assert.Equal(t, 1, count)
				assert.True(t, info.IsPureDownstreamCancellation())
				assert.False(t, info.StreamOutcome().CancelledWithoutBillableOutput)
				assert.Equal(t, 3, usage.PromptTokens)
				assert.Zero(t, usage.CompletionTokens)
			} else {
				assert.Zero(t, count)
				assert.Zero(t, usage.TotalTokens)
				if kind == "source-incomplete" {
					assert.True(t, info.StreamOutcome().OtherUpstreamTerminal)
					assert.False(t, info.IsPureDownstreamCancellation())
				} else {
					assert.True(t, info.StreamOutcome().CancelledWithoutBillableOutput)
				}
			}
		})
	}
}

func TestResponsesTerminalConflictErrorAndOtherStatus(t *testing.T) {
	for _, converted := range []bool{false, true} {
		for _, kind := range []string{"incomplete", "cancelled", "canceled", "completed-status-incomplete", "completed-error", "completed-empty-error"} {
			t.Run(map[bool]string{false: "native", true: "converted"}[converted]+kind, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, info := streamFrameFixture(t, rec)
				if !converted {
					info.RelayFormat = types.RelayFormatOpenAIResponses
				}
				eventType := "response." + kind
				status := kind
				extra := ""
				if strings.HasPrefix(kind, "completed-") {
					eventType = "response.completed"
					status = "completed"
				}
				if kind == "completed-status-incomplete" {
					status = "incomplete"
				}
				if kind == "completed-error" {
					extra = `,"error":{"code":"server_is_overloaded","message":"overloaded"}`
				}
				if kind == "completed-empty-error" {
					extra = `,"error":{}`
				}
				frame := `{"type":"` + eventType + `","response":{"status":"` + status + `"` + extra + `,"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13}}}`
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + frame + "\n\n"))}
				if converted {
					usage, apiErr = OaiResponsesToChatStreamHandler(c, info, resp)
				} else {
					usage, apiErr = OaiResponsesStreamHandler(c, info, resp)
				}
				if converted && kind == "completed-error" {
					require.NotNil(t, apiErr)
					assert.Equal(t, 503, apiErr.StatusCode)
					assert.Nil(t, usage)
				} else {
					require.Nil(t, apiErr)
					assert.Equal(t, 13, usage.TotalTokens)
				}
				outcome := info.StreamOutcome()
				assert.False(t, info.IsPureDownstreamCancellation())
				assert.Equal(t, kind == "completed-empty-error", outcome.UpstreamCompleted)
				assert.Equal(t, kind == "completed-error", outcome.UpstreamFailed)
				assert.Equal(t, kind != "completed-error" && kind != "completed-empty-error", outcome.OtherUpstreamTerminal)
			})
		}
	}
}

func TestResponsesStreamEndPreservesActualCauseAndProviderStatus(t *testing.T) {
	for _, reason := range []relaycommon.StreamEndReason{relaycommon.StreamEndReasonScannerErr, relaycommon.StreamEndReasonPingFail} {
		for _, sourceErr := range []error{context.Canceled, context.DeadlineExceeded} {
			t.Run(string(reason)+sourceErr.Error(), func(t *testing.T) {
				c, info := streamFrameFixture(t, httptest.NewRecorder())
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
				info.StreamStatus = &relaycommon.StreamStatus{EndReason: reason, EndError: sourceErr}
				observeResponsesStreamEnd(c, info)
				assert.Equal(t, sourceErr == context.DeadlineExceeded, info.StreamOutcome().UpstreamFailed)
			})
		}
	}
	for _, reason := range []relaycommon.StreamEndReason{relaycommon.StreamEndReasonScannerErr, relaycommon.StreamEndReasonPingFail, relaycommon.StreamEndReasonTimeout, relaycommon.StreamEndReasonPanic} {
		t.Run("provider-status-"+string(reason), func(t *testing.T) {
			c, info := streamFrameFixture(t, httptest.NewRecorder())
			info.MarkUpstreamFailureStatus(http.StatusServiceUnavailable)
			info.StreamStatus = &relaycommon.StreamStatus{EndReason: reason, EndError: context.DeadlineExceeded}
			observeResponsesStreamEnd(c, info)
			assert.True(t, info.StreamOutcome().UpstreamFailed)
			assert.Equal(t, http.StatusServiceUnavailable, info.StreamOutcome().UpstreamFailureStatus)
		})
	}
}
