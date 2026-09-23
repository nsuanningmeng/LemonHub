package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runResponsesUsageStream(t *testing.T, events ...string) (*dto.Usage, *relaycommon.RelayInfo, *gin.Context, string) {
	t.Helper()
	oldMode, oldTimeout := gin.Mode(), constant.StreamingTimeout
	gin.SetMode(gin.TestMode)
	constant.StreamingTimeout = 30
	t.Cleanup(func() { gin.SetMode(oldMode); constant.StreamingTimeout = oldTimeout })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "public-model", DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-model", IsModelMapped: true},
	}
	info.SetEstimatePromptTokens(37)
	var body strings.Builder
	for _, event := range events {
		body.WriteString("data: " + event + "\n\n")
	}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body.String()))}
	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	return usage, info, c, w.Body.String()
}

func TestResponsesMissingUsageGeneratedDeltas(t *testing.T) {
	for _, eventType := range []string{"response.output_text.delta", "response.function_call_arguments.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.refusal.delta"} {
		t.Run(eventType, func(t *testing.T) {
			usage, _, _, _ := runResponsesUsageStream(t, `{"type":"`+eventType+`","output_index":0,"delta":"generated output"}`)
			assert.Equal(t, 37, usage.PromptTokens)
			assert.Equal(t, service.CountTextToken("generated output", "test-model"), usage.CompletionTokens)
			assert.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
		})
	}
}

func TestResponsesOpeningUsagePlaceholderDoesNotSuppressMissingTerminalEstimate(t *testing.T) {
	usage, _, _, _ := runResponsesUsageStream(t,
		`{"type":"response.created","response":{"status":"in_progress","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		`{"type":"response.function_call_arguments.delta","delta":"generated output"}`,
	)
	assert.Equal(t, 37, usage.PromptTokens)
	assert.Equal(t, service.CountTextToken("generated output", "test-model"), usage.CompletionTokens)
}

func TestResponsesMissingUsageRequiresGenerationEvidence(t *testing.T) {
	for name, events := range map[string][]string{
		"empty":            nil,
		"created":          {`{"type":"response.created","response":{"status":"in_progress"}}`},
		"empty completed":  {`{"type":"response.completed","response":{"status":"completed","output":[]}}`},
		"explicit failure": {`{"type":"response.failed","response":{"status":"failed","error":{"type":"server_error","message":"secret"}}}`},
		"unknown event":    {`{"type":"extension.ping"}`},
	} {
		t.Run(name, func(t *testing.T) {
			usage, _, _, _ := runResponsesUsageStream(t, events...)
			assert.Zero(t, usage.TotalTokens)
		})
	}
}

func TestResponsesRealUsageOverridesEstimatesIncludingZero(t *testing.T) {
	for _, terminal := range []string{"response.completed", "response.incomplete", "response.failed"} {
		t.Run(terminal, func(t *testing.T) {
			usage, _, _, _ := runResponsesUsageStream(t,
				`{"type":"response.output_text.delta","delta":"generated output"}`,
				`{"type":"`+terminal+`","response":{"usage":{"input_tokens":11,"output_tokens":0,"total_tokens":11,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":2},"output_tokens_details":{"reasoning_tokens":0}}}}`,
			)
			assert.Equal(t, 11, usage.PromptTokens)
			assert.Zero(t, usage.CompletionTokens)
			assert.Equal(t, 11, usage.TotalTokens)
			assert.Equal(t, 3, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, 2, usage.PromptTokensDetails.CacheWriteTokens)
		})
	}
	usage, _, _, _ := runResponsesUsageStream(t,
		`{"type":"response.output_text.delta","delta":"generated output"}`,
		`{"type":"response.completed","response":{"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
	)
	assert.Zero(t, usage.TotalTokens)
}

func TestResponsesTerminalOutputEstimatedWithoutDoubleCounting(t *testing.T) {
	item := `{"type":"function_call","id":"call-1","arguments":"generated output"}`
	for name, events := range map[string][]string{
		"terminal only":  {`{"type":"response.completed","response":{"output":[` + item + `]}}`},
		"item done only": {`{"type":"response.output_item.done","output_index":0,"item":` + item + `}`},
		"delta item terminal": {
			`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"call-1","delta":"generated output"}`,
			`{"type":"response.output_item.done","output_index":0,"item":` + item + `}`,
			`{"type":"response.completed","response":{"output":[` + item + `]}}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			usage, _, _, _ := runResponsesUsageStream(t, events...)
			assert.Equal(t, 37, usage.PromptTokens)
			assert.Equal(t, service.CountTextToken("generated output", "test-model"), usage.CompletionTokens)
		})
	}
}

func TestResponsesMissingUsageFailureKeepsObservedGeneration(t *testing.T) {
	for _, terminal := range []string{
		`{"type":"response.failed","response":{"status":"failed"}}`,
		`{"type":"response.error","message":"upstream failure"}`,
		`{"type":"error","message":"upstream failure"}`,
		`{"type":"response.done","response":{"status":"failed"}}`,
	} {
		t.Run(terminal, func(t *testing.T) {
			usage, _, c, _ := runResponsesUsageStream(t,
				`{"type":"response.function_call_arguments.delta","delta":"generated output"}`, terminal,
			)
			assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyResponseFailed))
			assert.Equal(t, 37, usage.PromptTokens)
			assert.Equal(t, service.CountTextToken("generated output", "test-model"), usage.CompletionTokens)
			require.NotNil(t, usage.BillingUsage)
			assert.True(t, usage.BillingUsage.Estimated)
		})
	}
}

func TestResponsesIncompleteAndEOFDoNotInventProtocolFailure(t *testing.T) {
	for _, terminal := range []string{"", `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`} {
		usage, _, c, _ := runResponsesUsageStream(t,
			`{"type":"response.output_text.delta","delta":"generated output"}`, terminal,
		)
		assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyResponseFailed))
		assert.Positive(t, usage.TotalTokens)
	}
}

func TestResponsesUsageSnapshotPreservesDetailsAndPublicModel(t *testing.T) {
	usage, _, _, body := runResponsesUsageStream(t,
		`{"type":"response.completed","response":{"model":"private-model","usage":{"input_tokens":41,"output_tokens":7,"total_tokens":48,"input_tokens_details":{"cached_tokens":13,"cache_write_tokens":5,"audio_tokens":2,"image_tokens":3},"output_tokens_details":{"reasoning_tokens":4,"audio_tokens":2}}}}`,
	)
	assert.Equal(t, 41, usage.PromptTokens)
	assert.Equal(t, 7, usage.CompletionTokens)
	assert.Equal(t, 5, usage.PromptTokensDetails.CacheWriteTokens)
	assert.Equal(t, 2, usage.PromptTokensDetails.AudioTokens)
	assert.Equal(t, 3, usage.PromptTokensDetails.ImageTokens)
	assert.Equal(t, 4, usage.CompletionTokenDetails.ReasoningTokens)
	assert.Equal(t, 2, usage.CompletionTokenDetails.AudioTokens)
	require.NotNil(t, usage.BillingUsage)
	assert.False(t, usage.BillingUsage.Estimated)
	assert.Equal(t, dto.BillingUsageSourceOAIResponses, usage.BillingUsage.Source)
	require.NotNil(t, usage.BillingUsage.OpenAIUsage)
	assert.Equal(t, usage.PromptTokensDetails, usage.BillingUsage.OpenAIUsage.PromptTokensDetails)
	assert.NotContains(t, body, "private-model")
	assert.NotContains(t, body, "billing_usage")
}

func TestResponsesTerminalReasoningRefusalAndDistinctOutputs(t *testing.T) {
	usage, _, _, _ := runResponsesUsageStream(t,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"item_id":"r","delta":"reasoning "}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"r","type":"reasoning","summary":[{"type":"summary_text","text":"reasoning "}]}}`,
		`{"type":"response.refusal.delta","output_index":1,"item_id":"m","delta":"refusal "}`,
		`{"type":"response.completed","response":{"output":[{"id":"r","type":"reasoning","summary":[{"type":"summary_text","text":"reasoning "}]},{"id":"m","type":"message","content":[{"type":"refusal","refusal":"refusal "}]},{"id":"f","type":"function_call","arguments":"arguments"}]}}`,
	)
	assert.Equal(t, 37, usage.PromptTokens)
	assert.Equal(t, service.CountTextToken("reasoning refusal arguments", "test-model"), usage.CompletionTokens)
}

func TestResponsesGeneratedToolWithoutTextEstimatesPromptAndCountsOnce(t *testing.T) {
	operation_setting.SetToolPriceForTest("priced_fn", 5)
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest("priced_fn") })
	item := `{"type":"function_call","id":"f","name":"priced_fn","status":"completed"}`
	usage, info, _, _ := runResponsesUsageStream(t,
		`{"type":"response.output_item.done","output_index":0,"item":`+item+`}`,
		`{"type":"response.completed","response":{"output":[`+item+`]}}`,
	)
	assert.Equal(t, 37, usage.PromptTokens)
	assert.Zero(t, usage.CompletionTokens)
	require.NotNil(t, info.ResponsesUsageInfo)
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, "priced_fn")
	assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools["priced_fn"].CallCount)
}

func TestResponsesHandlerMissingUsageAndRealZero(t *testing.T) {
	for _, realZero := range []bool{false, true} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		info := &relaycommon.RelayInfo{OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-model"}}
		info.SetEstimatePromptTokens(37)
		response := dto.OpenAIResponsesResponse{Status: []byte(`"completed"`), Output: []dto.ResponsesOutput{{Type: "message", Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: "generated output"}}}}}
		if realZero {
			response.Usage = &dto.Usage{}
		}
		body, err := common.Marshal(response)
		require.NoError(t, err)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}
		usage, apiErr := OaiResponsesHandler(c, info, resp)
		require.Nil(t, apiErr)
		if realZero {
			assert.Zero(t, usage.TotalTokens)
		} else {
			assert.Equal(t, 37, usage.PromptTokens)
			assert.Equal(t, service.CountTextToken("generated output", "test-model"), usage.CompletionTokens)
		}
	}
}
