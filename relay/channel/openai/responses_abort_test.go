package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type responsesAbortBody struct {
	*strings.Reader
	end    error
	closed atomic.Bool
}

func (b *responsesAbortBody) Read(p []byte) (int, error) {
	n, e := b.Reader.Read(p)
	if e == io.EOF && b.end != nil {
		return n, b.end
	}
	return n, e
}
func (b *responsesAbortBody) Close() error { b.closed.Store(true); return nil }

func TestResponsesPartialAbortHasSingleFailedAndPreservesEndpointUsage(t *testing.T) {
	for _, converted := range []bool{false, true} {
		for _, reset := range []bool{false, true} {
			for _, zero := range []bool{false, true} {
				if !converted && zero {
					continue
				}
				name := "native"
				if converted {
					name = "chat"
				}
				if reset {
					name += "-reset"
				} else {
					name += "-eof"
				}
				if zero {
					name += "-zero"
				}
				t.Run(name, func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, info := streamFrameFixture(t, rec)
					info.RelayFormat = types.RelayFormatOpenAIResponses
					info.UpstreamModelName = "test-model"
					info.OriginModelName = "test-model"
					info.IsModelMapped = true
					frames := `data: {"type":"response.created","response":{"id":"resp_source","created_at":17,"model":"private-model","status":"in_progress"}}` + "\n\n" + `data: {"type":"response.output_text.delta","delta":"hello"}` + "\n\n"
					if converted {
						tokens := `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}`
						if zero {
							tokens = `{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`
						}
						frames = `data: {"id":"resp_source","created":17,"model":"private-model","choices":[{"index":0,"delta":{"content":"hello"}}]}` + "\n\n" + `data: {"choices":[],"usage":` + tokens + `}` + "\n\n"
					}
					body := &responsesAbortBody{Reader: strings.NewReader(frames)}
					if reset {
						body.end = errors.New("private-address reset detail")
					}
					var usageErr *types.NewAPIError
					if converted {
						usage, e := OaiChatToResponsesStreamHandler(c, info, &http.Response{StatusCode: 200, Body: body})
						usageErr = e
						require.NotNil(t, usage)
						if zero {
							assert.Zero(t, usage.TotalTokens)
						} else {
							assert.Equal(t, 12, usage.TotalTokens)
						}
					} else {
						usage, e := OaiResponsesStreamHandler(c, info, &http.Response{StatusCode: 200, Body: body})
						usageErr = e
						require.NotNil(t, usage)
					}
					require.Nil(t, usageErr)
					assert.True(t, body.closed.Load())
					assert.True(t, info.StreamOutcome().UpstreamFailed)
					assert.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed\n"))
					assert.NotContains(t, rec.Body.String(), "event: response.completed\n")
					assert.NotContains(t, rec.Body.String(), "private-address")
					assert.NotContains(t, rec.Body.String(), "private-model")
					assert.True(t, c.GetBool("relay_stream_error_emitted"))
					existingID := ""
					var existingCreated any
					for _, line := range strings.Split(rec.Body.String(), "\n") {
						if strings.HasPrefix(line, "data: ") {
							var event map[string]any
							require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
							if event["type"] == "response.created" {
								existingID = event["response"].(map[string]any)["id"].(string)
								existingCreated = event["response"].(map[string]any)["created_at"]
							}
							if event["type"] == "response.failed" {
								response := event["response"].(map[string]any)
								assert.Equal(t, existingID, response["id"])
								assert.Equal(t, "failed", response["status"])
								assert.Equal(t, existingCreated, response["created_at"])
								failure := response["error"].(map[string]any)
								assert.Equal(t, "server_error", failure["code"])
								assert.Equal(t, "upstream_stream_interrupted", failure["message"])
								assert.Equal(t, "test-model", response["model"])
							}
						}
					}
				})
			}
		}
	}
}

func TestChatResponsesAbortTerminalAndQueuedFatalPriority(t *testing.T) {
	partial := `data: {"choices":[{"index":0,"delta":{"content":"hello"}}]}` + "\n\n"
	for _, tc := range []struct {
		name, tail string
		failed     bool
		status     int
	}{
		{"done", "data: [DONE]\n\n", false, 0},
		{"finished_choice", `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n", false, 0},
		{"unfinished_choice", "", true, 0},
		{"second_choice_rejected", `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"},{"index":1,"delta":{"content":"another alternative"}}]}` + "\n\ndata: [DONE]\n\n", true, 500},
		{"fatal_before_queued_done", `data: {"error":{"code":"server_is_overloaded","message":"private-provider"}}` + "\n\ndata: [DONE]\n\n", true, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, info := streamFrameFixture(t, rec)
			info.RelayFormat = types.RelayFormatOpenAIResponses
			info.UpstreamModelName = "test-model"
			body := &responsesAbortBody{Reader: strings.NewReader(partial + tc.tail)}
			usage, e := OaiChatToResponsesStreamHandler(c, info, &http.Response{StatusCode: 200, Body: body})
			if tc.status != 0 {
				require.NotNil(t, e)
				assert.Equal(t, tc.status, e.StatusCode)
				assert.Nil(t, usage)
			} else {
				require.Nil(t, e)
				require.NotNil(t, usage)
			}
			assert.Equal(t, tc.failed, info.StreamOutcome().UpstreamFailed)
			if tc.failed {
				assert.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed\n"))
				assert.NotContains(t, rec.Body.String(), "event: response.completed\n")
			} else {
				assert.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.completed\n"))
				assert.NotContains(t, rec.Body.String(), "event: response.failed\n")
			}
			assert.NotContains(t, rec.Body.String(), "private-provider")
			assert.True(t, body.closed.Load())
		})
	}
}

func TestResponsesAbortClientCancellationAndBrokenWriterCloseOnly(t *testing.T) {
	for _, converted := range []bool{false, true} {
		for _, broken := range []bool{false, true} {
			name := "native"
			if converted {
				name = "chat"
			}
			if broken {
				name += "-broken"
			} else {
				name += "-cancel"
			}
			t.Run(name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var writer http.ResponseWriter = &responsesCancelOnFlushWriter{ResponseRecorder: rec, cancel: cancel, match: "hello"}
				if broken {
					writer = &rejectingStreamWriter{ResponseRecorder: rec, reject: "hello"}
				}
				c, info := streamFrameFixture(t, writer)
				c.Request = c.Request.WithContext(ctx)
				info.RelayFormat = types.RelayFormatOpenAIResponses
				info.UpstreamModelName = "test-model"
				frame := `{"type":"response.output_text.delta","delta":"hello"}`
				if converted {
					frame = `{"choices":[{"index":0,"delta":{"content":"hello"}}]}`
				}
				body := &responsesAbortBody{Reader: strings.NewReader("data: " + frame + "\n\n")}
				if converted {
					_, e := OaiChatToResponsesStreamHandler(c, info, &http.Response{StatusCode: 200, Body: body})
					require.Nil(t, e)
				} else {
					_, e := OaiResponsesStreamHandler(c, info, &http.Response{StatusCode: 200, Body: body})
					require.Nil(t, e)
				}
				assert.NotContains(t, rec.Body.String(), "event: response.failed\n")
				assert.NotContains(t, rec.Body.String(), "event: response.completed\n")
				assert.True(t, body.closed.Load())
				if broken {
					assert.True(t, c.GetBool("relay_stream_write_failed"))
				} else {
					assert.True(t, info.IsPureDownstreamCancellation())
				}
			})
		}
	}
}

func TestNativeResponsesEmptyEOFDoesNotExpandAbortPolicy(t *testing.T) {
	rec := httptest.NewRecorder()
	c, info := streamFrameFixture(t, rec)
	info.RelayFormat = types.RelayFormatOpenAIResponses
	info.UpstreamModelName = "test-model"
	usage, e := OaiResponsesStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))})
	require.Nil(t, e)
	require.NotNil(t, usage)
	assert.False(t, info.StreamOutcome().UpstreamFailed)
	assert.Empty(t, rec.Body.String())
	assert.Zero(t, usage.PromptTokens)
}
