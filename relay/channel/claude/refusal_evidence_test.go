package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const refusalStart = `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"usage":{"input_tokens":412,"output_tokens":0}}}`
const refusalDelta = `{"type":"message_delta","delta":{"stop_reason":"refusal"},"usage":{"output_tokens":0}}`
const refusalStop = `{"type":"message_stop"}`

func TestClaudeRefusalRequiresCompleteOriginalResponse(t *testing.T) {
	old := *model_setting.GetGlobalSettings()
	model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = true
	t.Cleanup(func() { *model_setting.GetGlobalSettings() = old })
	const valid = `{"type":"message","content":[],"stop_reason":"refusal","usage":{"input_tokens":412,"output_tokens":0}}`
	tests := []struct {
		name, body string
		exempt     bool
	}{
		{"explicit refusal", valid, true},
		{"explicit all-zero usage", strings.Replace(valid, "412", "0", 1), true},
		{"absent content", strings.Replace(valid, `"content":[],`, "", 1), false},
		{"null content", strings.Replace(valid, `"content":[]`, `"content":null`, 1), false},
		{"empty text block is output", strings.Replace(valid, `"content":[]`, `"content":[{"type":"text","text":""}]`, 1), false},
		{"duplicate content cannot hide output", strings.Replace(valid, `"content":[]`, `"content":[],"content":[{"type":"text","text":"output"}]`, 1), false},
		{"duplicate null content is ambiguous", strings.Replace(valid, `"content":[]`, `"content":[],"content":null`, 1), false},
		{"duplicate usage hides fallback work", strings.TrimSuffix(valid, "}") + `,"usage":{"input_tokens":412,"output_tokens":0,"iterations":[{"input_tokens":1}]}}`, false},
		{"duplicate server tools hides unmodeled billable work", strings.Replace(valid, `"input_tokens":412`, `"input_tokens":412,"server_tool_use":{},"server_tool_use":{"web_fetch_requests":1}`, 1), false},
		{"thinking block", strings.Replace(valid, `"content":[]`, `"content":[{"type":"thinking","thinking":"thought"}]`, 1), false},
		{"tool block", strings.Replace(valid, `"content":[]`, `"content":[{"type":"tool_use","id":"tool_1","name":"lookup","input":{}}]`, 1), false},
		{"legacy completion is output", strings.Replace(valid, `"content":[]`, `"content":[],"completion":"output"`, 1), false},
		{"positive output", strings.Replace(valid, `"output_tokens":0`, `"output_tokens":1`, 1), false},
		{"missing output count", strings.Replace(valid, `,"output_tokens":0`, "", 1), false},
		{"missing input count", strings.Replace(valid, `"input_tokens":412,`, "", 1), false},
		{"missing usage", `{"type":"message","content":[],"stop_reason":"refusal"}`, false},
		{"null usage", `{"type":"message","content":[],"stop_reason":"refusal","usage":null}`, false},
		{"negative token metadata", strings.Replace(valid, `"input_tokens":412`, `"input_tokens":412,"cache_read_input_tokens":-1`, 1), false},
		{"upstream tool work", strings.Replace(valid, `"input_tokens":412`, `"input_tokens":412,"server_tool_use":{"web_search_requests":1}`, 1), false},
		{"fallback iterations", strings.Replace(valid, `"input_tokens":412`, `"input_tokens":412,"iterations":[{"input_tokens":10}]`, 1), false},
		{"different terminal reason", strings.Replace(valid, `"refusal"`, `"end_turn"`, 1), false},
	}
	for _, tt := range tests {
		for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
			t.Run(tt.name+"/"+string(format), func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				info := mappedClaudeResponseInfo(format)
				parsed := &ClaudeResponseInfo{Usage: &dto.Usage{}}
				require.Nil(t, HandleClaudeResponseData(c, info, parsed, nil, []byte(tt.body)))
				assert.Equal(t, tt.exempt, service.TextBillingExemptionReason(parsed.Usage) != "")
				assert.Equal(t, tt.exempt, service.TextBillingExemptionReason(relayconvert.UsageFromClaudeUsage(parsed.Usage)) != "", "cross-protocol snapshot preserves trusted evidence")
				assert.NotContains(t, recorder.Body.String(), "ClaudePreOutputRefusal")
				if format == types.RelayFormatClaude {
					assert.JSONEq(t, tt.body, recorder.Body.String(), "native response/reference usage is untouched")
				}
				var decoded dto.BillingUsage
				require.NoError(t, common.UnmarshalJsonStr(`{"ClaudePreOutputRefusal":true,"claude_pre_output_refusal":true}`, &decoded))
				assert.False(t, decoded.ClaudePreOutputRefusal, "JSON cannot forge internal evidence")
			})
		}
	}
}

func TestClaudeRefusalStreamRequiresNoContentAndMessageStop(t *testing.T) {
	old := *model_setting.GetGlobalSettings()
	model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = true
	t.Cleanup(func() { *model_setting.GetGlobalSettings() = old })
	tests := []struct {
		name   string
		events []string
		exempt bool
	}{
		{"complete refusal", []string{refusalStart, refusalDelta, refusalStop}, true},
		{"keepalive is not output", []string{refusalStart, `{"type":"ping"}`, refusalDelta, refusalStop}, true},
		{"interrupted before message stop", []string{refusalStart, refusalDelta}, false},
		{"missing message start", []string{refusalDelta, refusalStop}, false},
		{"missing delta usage", []string{refusalStart, `{"type":"message_delta","delta":{"stop_reason":"refusal"}}`, refusalStop}, false},
		{"missing start usage", []string{`{"type":"message_start","message":{"content":[]}}`, refusalDelta, refusalStop}, false},
		{"missing start content", []string{strings.Replace(refusalStart, `"content":[],`, "", 1), refusalDelta, refusalStop}, false},
		{"duplicate start content is ambiguous", []string{strings.Replace(refusalStart, `"content":[]`, `"content":[],"content":null`, 1), refusalDelta, refusalStop}, false},
		{"empty text block", []string{refusalStart, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, refusalDelta, refusalStop}, false},
		{"thinking output", []string{refusalStart, `{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"thought"}}`, refusalDelta, refusalStop}, false},
		{"tool output", []string{refusalStart, `{"type":"content_block_start","content_block":{"type":"tool_use","name":"lookup","id":"t1","input":{}}}`, refusalDelta, refusalStop}, false},
		{"upstream reports output", []string{refusalStart, strings.Replace(refusalDelta, `"output_tokens":0`, `"output_tokens":2`, 1), refusalStop}, false},
		{"out of order terminal events", []string{refusalStart, refusalStop, refusalDelta}, false},
		{"output after terminal", []string{refusalStart, refusalDelta, refusalStop, `{"type":"content_block_start","content_block":{"type":"text","text":"late"}}`}, false},
	}
	for _, tt := range tests {
		for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
			t.Run(tt.name+"/"+string(format), func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				info := mappedClaudeResponseInfo(format)
				info.SetEstimatePromptTokens(412)
				parsed := &ClaudeResponseInfo{Usage: &dto.Usage{}}
				for _, event := range tt.events {
					require.Nil(t, HandleStreamResponseData(c, info, parsed, event))
				}
				HandleStreamFinalResponse(c, info, parsed)
				assert.Equal(t, tt.exempt, service.TextBillingExemptionReason(parsed.Usage) != "")
				assert.Equal(t, 412, parsed.Usage.PromptTokens)
			})
		}
	}
}

func TestClaudeRefusalEvidenceDoesNotLeakAcrossAttempts(t *testing.T) {
	old := *model_setting.GetGlobalSettings()
	model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = true
	t.Cleanup(func() { *model_setting.GetGlobalSettings() = old })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := mappedClaudeResponseInfo(types.RelayFormatClaude)
	for _, stopReason := range []string{"refusal", "end_turn"} {
		parsed := &ClaudeResponseInfo{Usage: &dto.Usage{}}
		body := `{"content":[],"stop_reason":"` + stopReason + `","usage":{"input_tokens":412,"output_tokens":0}}`
		require.Nil(t, HandleClaudeResponseData(c, info, parsed, nil, []byte(body)))
		assert.Equal(t, stopReason == "refusal", service.TextBillingExemptionReason(parsed.Usage) != "")
	}
}

func TestClaudeRefusalSSERequiresTerminalFrame(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	old := *model_setting.GetGlobalSettings()
	model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = true
	t.Cleanup(func() { *model_setting.GetGlobalSettings() = old })
	for _, complete := range []bool{false, true} {
		name := "truncated after refusal"
		if complete {
			name = "complete refusal"
		}
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			info := mappedClaudeResponseInfo(types.RelayFormatClaude)
			info.IsStream = true
			payload := "data: " + refusalStart + "\n\ndata: " + refusalDelta + "\n\n"
			if complete {
				payload += "data: " + refusalStop + "\n\n"
			}
			response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(payload))}
			usage, apiErr := ClaudeStreamHandler(c, response, info)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, complete, service.TextBillingExemptionReason(usage) != "")
		})
	}
}
