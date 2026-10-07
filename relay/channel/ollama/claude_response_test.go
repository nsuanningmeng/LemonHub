package ollama

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaAdaptorReturnsClaudeMessage(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude, OriginModelName: "public-model",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "private-model", IsModelMapped: true},
	}
	raw := `{"model":"private-model","message":{"content":"answer","thinking":"reasoning","tool_calls":[{"id":"tool-one","function":{"name":"lookup","arguments":{"count":0}}}]},"done":true,"prompt_eval_count":10,"eval_count":2}`
	usage, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))}, info)
	require.Nil(t, apiErr)
	require.IsType(t, &dto.Usage{}, usage)
	assert.Equal(t, 12, usage.(*dto.Usage).TotalTokens)
	var out dto.ClaudeResponse
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "message", out.Type)
	assert.Equal(t, "public-model", out.Model)
	require.NotNil(t, out.Usage)
	assert.Equal(t, 10, out.Usage.InputTokens)
	assert.Equal(t, 2, out.Usage.OutputTokens)
	assert.Equal(t, "tool_use", out.StopReason)
	assert.Contains(t, w.Body.String(), `"type":"thinking"`)
	assert.Contains(t, w.Body.String(), `"thinking":"reasoning"`)
	assert.Contains(t, w.Body.String(), `"text":"answer"`)
	assert.Contains(t, w.Body.String(), `"id":"tool-one"`)
	assert.NotContains(t, w.Body.String(), "private-model")
	assert.EqualValues(t, types.RelayFormatClaude, info.RelayFormat)
}

func TestOllamaAdaptorStreamsClaudeEvents(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude, IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "ollama-model"},
	}
	raw := `{"model":"ollama-model","message":{"content":"answer","thinking":"reasoning","tool_calls":[{"function":{"name":"lookup","arguments":{"count":0}}}]},"done":true,"prompt_eval_count":10,"eval_count":2}`
	usage, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))}, info)
	require.Nil(t, apiErr)
	assert.Equal(t, 12, usage.(*dto.Usage).TotalTokens)
	assert.Equal(t, 1, strings.Count(w.Body.String(), "event: message_start\n"))
	assert.Equal(t, 1, strings.Count(w.Body.String(), "event: message_stop\n"))
	assert.Contains(t, w.Body.String(), `"thinking":"reasoning"`)
	assert.Contains(t, w.Body.String(), `"text":"answer"`)
	assert.Contains(t, w.Body.String(), `"name":"lookup"`)
	assert.Contains(t, w.Body.String(), `"input_tokens":10`)
	assert.Contains(t, w.Body.String(), `"output_tokens":2`)
	assert.NotContains(t, w.Body.String(), "[DONE]")
	assert.EqualValues(t, types.RelayFormatClaude, info.RelayFormat)
}

func TestOllamaClaudeNonStreamCompletionValidationKeepsNativeBehavior(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"single", `{"message":{"content":"answer"},"done":true,"prompt_eval_count":0,"eval_count":0}`, true},
		{"pretty", "{\n  \"message\": {\"content\": \"answer\"},\n  \"done\": true\n}", true},
		{"ndjson", "{\"message\":{\"content\":\"ans\"},\"done\":false}\n{\"message\":{\"content\":\"wer\"},\"done\":true}", true},
		{"no-done", `{"message":{"content":"answer"},"done":false}`, false},
		{"error-before-done", "{\"error\":\"private-upstream-sentinel\"}\n{\"done\":true}", false},
		{"malformed-before-done", "{private-upstream-sentinel}\n{\"done\":true}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				info := &relaycommon.RelayInfo{RelayFormat: format, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "model"}}
				u, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}, info)
				if format == types.RelayFormatClaude && !tc.valid {
					require.NotNil(t, apiErr)
					assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
					assert.NotContains(t, apiErr.Error(), "private-upstream-sentinel")
					assert.Empty(t, w.Body.String())
					continue
				}
				require.Nil(t, apiErr)
				assert.Zero(t, u.(*dto.Usage).TotalTokens)
				if format == types.RelayFormatClaude {
					var out dto.ClaudeResponse
					require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
					require.NotNil(t, out.Usage)
					assert.Zero(t, out.Usage.InputTokens)
					assert.Zero(t, out.Usage.OutputTokens)
					require.Len(t, out.Content, 1)
					assert.Equal(t, "answer", out.Content[0].GetText())
				} else {
					assert.Contains(t, w.Body.String(), `"object":"chat.completion"`)
				}
			}
		})
	}
}

func TestOllamaClaudeToolBlockLifecycleAcrossMixedRecords(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "model"}}
	raw := strings.Join([]string{
		`{"message":{"tool_calls":[{"function":{"name":"first","arguments":{"x":0}}}]}}`,
		`{"message":{"tool_calls":[{"function":{"name":"second","arguments":{"x":1}}}]}}`,
		`{"message":{"thinking":"think","content":"answer","tool_calls":[{"function":{"name":"third","arguments":{"x":2}}},{"id":"known-id","function":{"name":"fourth","arguments":{"x":3}}}]},"done":true,"prompt_eval_count":0,"eval_count":0}`,
	}, "\n")
	u, e := (&Adaptor{}).DoResponse(c, &http.Response{Body: io.NopCloser(strings.NewReader(raw))}, info)
	require.Nil(t, e)
	assert.Zero(t, u.(*dto.Usage).TotalTokens)
	open := map[int]string{}
	toolIDs := []string{}
	stopped, startCount, stopCount := false, 0, 0
	var usage *dto.ClaudeUsage
	for _, line := range strings.Split(w.Body.String(), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var event dto.ClaudeResponse
		require.NoError(t, common.Unmarshal([]byte(data), &event))
		require.False(t, stopped, "no event may follow message_stop")
		switch event.Type {
		case "message_start":
			startCount++
		case "content_block_start":
			require.NotNil(t, event.Index)
			_, exists := open[*event.Index]
			assert.False(t, exists, "content blocks cannot be opened twice")
			open[*event.Index] = event.ContentBlock.Type
			if event.ContentBlock.Type == "tool_use" {
				toolIDs = append(toolIDs, event.ContentBlock.Id)
			}
		case "content_block_delta":
			require.NotNil(t, event.Index)
			want := map[string]string{"thinking_delta": "thinking", "text_delta": "text", "input_json_delta": "tool_use"}[event.Delta.Type]
			assert.Equal(t, want, open[*event.Index])
		case "content_block_stop":
			require.NotNil(t, event.Index)
			_, exists := open[*event.Index]
			assert.True(t, exists, "stops cannot reference unopened tool-index gaps")
			delete(open, *event.Index)
		case "message_delta":
			assert.Empty(t, open)
			usage = event.Usage
			assert.Equal(t, "tool_use", *event.Delta.StopReason)
		case "message_stop":
			stopped = true
			stopCount++
		}
	}
	assert.Equal(t, 1, startCount)
	assert.Equal(t, 1, stopCount)
	assert.Equal(t, []string{"call_0", "call_1", "call_2", "known-id"}, toolIDs)
	require.NotNil(t, usage)
	assert.Zero(t, usage.InputTokens)
	assert.Zero(t, usage.OutputTokens)
	assert.True(t, info.StreamOutcome().UpstreamCompleted)
	assert.NotContains(t, w.Body.String(), "[DONE]")
}

func TestOllamaClaudeAbortsDoNotInventSuccessfulTerminal(t *testing.T) {
	for _, tc := range []struct {
		name, suffix string
		typed        bool
	}{
		{"EOF", "", false},
		{"upstream-error", `{"error":"private-upstream-sentinel"}`, false},
		{"malformed", "{private-upstream-sentinel", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "model"}}
			u, e := (&Adaptor{}).DoResponse(c, &http.Response{Body: io.NopCloser(strings.NewReader("{\"message\":{\"content\":\"partial\"},\"done\":false}\n" + tc.suffix))}, info)
			if tc.typed {
				require.NotNil(t, e)
				assert.Equal(t, 500, e.StatusCode)
				assert.NotContains(t, e.Error(), "sentinel")
			} else {
				require.Nil(t, e)
			}
			assert.Zero(t, u.(*dto.Usage).TotalTokens)
			assert.True(t, info.StreamOutcome().UpstreamFailed)
			assert.Contains(t, w.Body.String(), `"text":"partial"`)
			assert.NotContains(t, w.Body.String(), "message_stop")
			assert.NotContains(t, w.Body.String(), "message_delta")
			assert.NotContains(t, w.Body.String(), "[DONE]")
			assert.NotContains(t, w.Body.String(), "sentinel")
		})
	}
}

func TestOllamaClaudeDownstreamFaultRetainsAuthoritativeUsage(t *testing.T) {
	for _, mode := range []string{"write", "short", "flush", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
			writer := &ollamaFaultWriter{ResponseWriter: c.Writer, mode: mode, cancel: cancel}
			c.Writer = writer
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "model"}}
			u, e := (&Adaptor{}).DoResponse(c, &http.Response{Body: io.NopCloser(strings.NewReader(`{"message":{"content":"done payload"},"done":true,"prompt_eval_count":10,"eval_count":0}`))}, info)
			require.Nil(t, e)
			assert.Equal(t, 10, u.(*dto.Usage).TotalTokens)
			assert.True(t, info.StreamOutcome().UpstreamCompleted)
			assert.Equal(t, mode != "cancel", info.StreamOutcome().UpstreamFailed)
			assert.Equal(t, 2, writer.calls)
			assert.NotContains(t, w.Body.String(), "message_stop")
		})
	}
}

func TestOllamaAdaptorRejectsUnsupportedResponseFormat(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := &ollamaUnneededReadBody{}
	u, e := (&Adaptor{}).DoResponse(c, &http.Response{Body: body}, &relaycommon.RelayInfo{RelayFormat: types.RelayFormatGemini})
	require.NotNil(t, e)
	assert.Nil(t, u)
	assert.Equal(t, http.StatusBadRequest, e.StatusCode)
	assert.Zero(t, body.reads)
	assert.Equal(t, 1, body.closes)
	assert.Empty(t, w.Body.String())
}
