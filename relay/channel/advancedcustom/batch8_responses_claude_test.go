package advancedcustom

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const batch8ResponsesClaudeID = "openai_responses_to_claude_messages"

func TestBatch8ResponsesClaudeRouteWire(t *testing.T) {
	info := advancedCustomRelayInfo(&dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/messages", Converter: batch8ResponsesClaudeID, Auth: &dto.AdvancedCustomRouteAuth{Type: "header", Name: "x-api-key", Value: "{api_key}"}}}})
	info.RelayFormat = types.RelayFormatOpenAIResponses
	info.RequestURLPath = "/v1/responses"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	adaptor := &Adaptor{}
	adaptor.Init(info)
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"claude","max_output_tokens":64,"input":"hello"}`), &request))
	converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, request)
	require.NoError(t, err)
	wire, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.Contains(t, string(wire), `"messages"`)
	assert.Contains(t, string(wire), `"max_tokens":64`)
	assert.NotContains(t, string(wire), `"input"`)
	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &header, info))
	assert.Equal(t, "2023-06-01", header.Get("anthropic-version"))
	assert.Equal(t, "sk-test", header.Get("x-api-key"))
	usage, apiErr := adaptor.DoResponse(c, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"msg1","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":3}}`))}, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	var response map[string]any
	require.NoError(t, common.Unmarshal([]byte(recorder.Body.String()), &response))
	assert.Equal(t, "response", response["object"])
}

func TestBatch8ResponsesClaudeStreamLifecycleAndUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	info := advancedCustomRelayInfo(&dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/messages", Converter: batch8ResponsesClaudeID}}})
	info.RequestURLPath = "/v1/responses"
	info.RelayFormat = types.RelayFormatOpenAIResponses
	info.IsStream = true
	info.DisablePing = true
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	adaptor := &Adaptor{}
	adaptor.Init(info)
	events := []string{
		`{"type":"message_start","message":{"id":"msg1","type":"message","role":"assistant","model":"claude","content":[],"usage":{"input_tokens":10,"cache_read_input_tokens":3,"cache_creation_input_tokens":2,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"answer"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"x\"}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
		`{"type":"message_stop"}`,
	}
	var wire strings.Builder
	for _, event := range events {
		wire.WriteString("data: " + event + "\n\n")
	}
	usage, apiErr := adaptor.DoResponse(c, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire.String()))}, info)
	require.Nil(t, apiErr)
	native := usage.(*dto.Usage)
	assert.Equal(t, 10, native.PromptTokens)
	assert.Equal(t, 4, native.CompletionTokens)
	assert.Equal(t, 3, native.PromptTokensDetails.CachedTokens)
	require.NotNil(t, native.BillingUsage)
	body := recorder.Body.String()
	assert.Contains(t, body, "event: response.created")
	assert.Contains(t, body, "event: response.function_call_arguments.delta")
	assert.NotContains(t, body, "event: message_start")
	assert.Equal(t, 1, strings.Count(body, "event: response.completed"))
	assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
	var completed dto.ResponsesStreamResponse
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			var event dto.ResponsesStreamResponse
			require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
			if event.Type == "response.completed" {
				completed = event
			}
		}
	}
	require.NotNil(t, completed.Response)
	require.NotNil(t, completed.Response.Usage)
	assert.Equal(t, 15, completed.Response.Usage.InputTokens)
	assert.Equal(t, 4, completed.Response.Usage.OutputTokens)
}

func TestBatch8ResponsesClaudeIncompleteAndMalformedStreams(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, events := range []string{
		`data: {"type":"message_start"}` + "\n\n" + `data: {"type":"message_stop"}` + "\n\n",
		`data: {"type":"message_start","message":{"model":"claude","usage":{"input_tokens":1}}}` + "\n\n",
		`data: {"type":"message_start","message":{"model":"claude","usage":{"input_tokens":1}}}` + "\n\n" + `data: {"type":"content_block_delta","delta":{"type":"input_json_delta"}}` + "\n\n",
	} {
		info := advancedCustomRelayInfo(&dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/messages", Converter: batch8ResponsesClaudeID}}})
		info.RequestURLPath = "/v1/responses"
		info.RelayFormat = types.RelayFormatOpenAIResponses
		info.IsStream = true
		info.DisablePing = true
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		adaptor := &Adaptor{}
		adaptor.Init(info)
		_, err := adaptor.DoResponse(c, &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(events))}, info)
		require.NotNil(t, err)
		assert.Equal(t, 502, err.StatusCode)
		assert.NotContains(t, recorder.Body.String(), "response.completed")
	}
}

type batch8FailingWriter struct {
	*httptest.ResponseRecorder
	short bool
}

func (w batch8FailingWriter) Write(_ []byte) (int, error) {
	if w.short {
		return 0, nil
	}
	return 0, errors.New("private write failure")
}

func (w batch8FailingWriter) WriteString(_ string) (int, error) {
	if w.short {
		return 0, nil
	}
	return 0, errors.New("private write failure")
}

func TestBatch8ResponsesClaudeWriteFailure(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint(short), func(t *testing.T) {
			oldTimeout := constant.StreamingTimeout
			constant.StreamingTimeout = 30
			t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
			info := advancedCustomRelayInfo(&dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/messages", Converter: batch8ResponsesClaudeID}}})
			info.RequestURLPath, info.RelayFormat, info.IsStream, info.DisablePing = "/v1/responses", types.RelayFormatOpenAIResponses, true, true
			writer := batch8FailingWriter{ResponseRecorder: httptest.NewRecorder(), short: short}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			adaptor := &Adaptor{}
			adaptor.Init(info)
			wire := "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
			usage, apiErr := adaptor.DoResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(wire))}, info)
			require.NotNil(t, apiErr)
			assert.Nil(t, usage)
			assert.NotContains(t, apiErr.Error(), "private write failure")
			assert.True(t, c.Writer.Written())
			assert.True(t, c.GetBool("relay_stream_write_failed"))
		})
	}
}
