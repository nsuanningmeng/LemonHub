package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/ali"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the same mapping and adaptor boundaries as the channel probe and relay.
func convertChatCapabilityRequest(t *testing.T, request *dto.GeneralOpenAIRequest, channelType int, mappedModel string) []byte {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if mappedModel != "" {
		mapping, err := common.Marshal(map[string]string{request.Model: mappedModel})
		require.NoError(t, err)
		c.Set("model_mapping", string(mapping))
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: request.Model,
		Request:         request,
		RelayFormat:     types.RelayFormatOpenAI,
		IsStream:        request.IsStream(nil),
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          channelType,
			UpstreamModelName:    request.Model,
			SupportStreamOptions: true,
		},
	}
	require.NoError(t, helper.ModelMappedHelper(c, info, request))
	var converted any
	var err error
	if channelType == constant.ChannelTypeAli {
		converted, err = (&ali.Adaptor{}).ConvertOpenAIRequest(c, info, request)
	} else {
		converted, err = (&openai.Adaptor{}).ConvertOpenAIRequest(c, info, request)
	}
	require.NoError(t, err)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	return encoded
}

func TestChannelProbeOpenAIChatCapabilities(t *testing.T) {
	for _, tt := range []struct {
		name, model, upstream, endpoint, limit string
		channelType                            int
		stream                                 bool
	}{
		{name: "astra automatic", model: "gpt-6-astra", upstream: "gpt-6-astra", channelType: constant.ChannelTypeOpenAI, limit: "max_completion_tokens"},
		{name: "sol azure streaming", model: "gpt-6-sol", upstream: "gpt-6-sol", endpoint: string(constant.EndpointTypeOpenAI), channelType: constant.ChannelTypeAzure, stream: true, limit: "max_completion_tokens"},
		{name: "luna", model: "gpt-6-luna", upstream: "gpt-6-luna", channelType: constant.ChannelTypeOpenAI, limit: "max_completion_tokens"},
		{name: "public alias maps to sol", model: "customer-model", upstream: "gpt-6-sol", channelType: constant.ChannelTypeOpenAI, limit: "max_completion_tokens"},
		{name: "gpt alias maps to qwen", model: "gpt-6-sol", upstream: "qwen-turbo", channelType: constant.ChannelTypeAli, limit: "max_tokens"},
		{name: "gpt5 alias maps to qwen", model: "gpt-5.4", upstream: "qwen-turbo", channelType: constant.ChannelTypeAli, limit: "max_tokens"},
		{name: "gpt4 unchanged", model: "gpt-4.1", upstream: "gpt-4.1", endpoint: string(constant.EndpointTypeOpenAI), channelType: constant.ChannelTypeOpenAI, limit: "max_tokens"},
		{name: "o model", model: "o3-mini", upstream: "o3-mini", channelType: constant.ChannelTypeAzure, limit: "max_completion_tokens"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request, ok := buildTestRequest(tt.model, tt.endpoint, &model.Channel{}, tt.stream).(*dto.GeneralOpenAIRequest)
			require.True(t, ok)
			encoded := convertChatCapabilityRequest(t, request, tt.channelType, tt.upstream)
			want := map[string]any{"model": tt.upstream, "messages": []dto.Message{{Role: "user", Content: "hi"}}, "stream": tt.stream, tt.limit: 16}
			if tt.stream {
				want["stream_options"] = map[string]any{"include_usage": true}
			}
			wantJSON, err := common.Marshal(want)
			require.NoError(t, err)
			assert.JSONEq(t, string(wantJSON), string(encoded))
		})
	}
}

func TestOpenAIChatModelSamplingCapabilities(t *testing.T) {
	const sampling = `{"temperature":0.2,"top_p":0.8,"logprobs":true,"top_logprobs":5}`
	for _, tt := range []struct{ name, model, mapping, effort, wantModel, wantEffort, wantRole, params string }{
		{name: "gpt5.1 none", model: "gpt-5.1", effort: "none", wantEffort: "none", wantRole: "developer", params: sampling},
		{name: "gpt5.2 default", model: "gpt-5.2", wantRole: "developer", params: sampling},
		{name: "gpt5.4 snapshot", model: "gpt-5.4-2026-03-05", wantRole: "developer", params: sampling},
		{name: "gpt5.4 reasoning", model: "gpt-5.4", effort: "high", wantEffort: "high", wantRole: "developer", params: `{}`},
		{name: "gpt5 original", model: "gpt-5", wantRole: "developer", params: `{}`},
		{name: "pro variant", model: "gpt-5.2-pro-2025-12-11", wantRole: "developer", params: `{}`},
		{name: "astra", model: "gpt-6-astra", wantRole: "developer", params: `{}`},
		{name: "astra snapshot", model: "gpt-6-astra-2026-09-03", wantRole: "developer", params: `{}`},
		{name: "sol default", model: "gpt-6-sol", wantRole: "developer", params: sampling},
		{name: "luna explicit none", model: "gpt-6-luna", effort: "none", wantEffort: "none", wantRole: "developer", params: sampling},
		{name: "luna reasoning suffix", model: "gpt-6-luna-high", wantModel: "gpt-6-luna", wantEffort: "high", wantRole: "developer", params: `{}`},
		{name: "none suffix overrides effort", model: "gpt-5.2-none", effort: "high", wantModel: "gpt-5.2", wantEffort: "none", wantRole: "developer", params: sampling},
		{name: "mapped upstream controls restrictions", model: "gpt-6-astra", mapping: "gpt-4.1", wantModel: "gpt-4.1", wantRole: "system", params: sampling},
		{name: "mapped reasoning suffix", model: "customer-model", mapping: "gpt-6-sol-high", wantModel: "gpt-6-sol", wantEffort: "high", wantRole: "developer", params: `{}`},
		{name: "o1 mini role exception", model: "o1-mini", wantRole: "system", params: `{"top_p":0.8,"logprobs":true,"top_logprobs":5}`},
		{name: "future family unchanged", model: "gpt-7", wantRole: "system", params: sampling},
		{name: "unrecognized gpt50 unchanged", model: "gpt-50", wantRole: "system", params: sampling},
		{name: "unrecognized astra variant", model: "gpt-6-astra-custom", wantRole: "system", params: sampling},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := &dto.GeneralOpenAIRequest{Model: tt.model, ReasoningEffort: tt.effort, Messages: []dto.Message{{Role: "system", Content: "first"}, {Role: "system", Content: "second"}, {Role: "user", Content: "hi"}}}
			require.NoError(t, common.UnmarshalJsonStr(sampling, request))
			encoded := convertChatCapabilityRequest(t, request, constant.ChannelTypeOpenAI, tt.mapping)
			var want map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tt.params, &want))
			want["model"] = tt.model
			if tt.wantModel != "" {
				want["model"] = tt.wantModel
			}
			want["messages"] = []dto.Message{{Role: tt.wantRole, Content: "first"}, {Role: "system", Content: "second"}, {Role: "user", Content: "hi"}}
			if tt.wantEffort != "" {
				want["reasoning_effort"] = tt.wantEffort
			}
			wantJSON, err := common.Marshal(want)
			require.NoError(t, err)
			assert.JSONEq(t, string(wantJSON), string(encoded))
		})
	}
}

func TestOpenAIChatCapabilityPreservesExplicitZero(t *testing.T) {
	for _, modelName := range []string{"gpt-5.2", "gpt-6-sol", "gpt-6-luna"} {
		for _, tt := range []struct{ name, input, want string }{
			{name: "absent", input: `{}`, want: `{}`},
			{name: "legacy zero", input: `{"max_tokens":0}`, want: `{"max_completion_tokens":0}`},
			{name: "completion zero wins", input: `{"max_tokens":64,"max_completion_tokens":0}`, want: `{"max_tokens":64,"max_completion_tokens":0}`},
			{name: "sampling zero", input: `{"temperature":0,"top_p":0,"logprobs":false,"top_logprobs":0}`, want: `{"temperature":0,"top_p":0,"logprobs":false,"top_logprobs":0}`},
		} {
			t.Run(modelName+"/"+tt.name, func(t *testing.T) {
				request := &dto.GeneralOpenAIRequest{Model: modelName}
				require.NoError(t, common.UnmarshalJsonStr(tt.input, request))
				encoded := convertChatCapabilityRequest(t, request, constant.ChannelTypeOpenAI, "")
				want := map[string]any{}
				require.NoError(t, common.UnmarshalJsonStr(tt.want, &want))
				want["model"] = modelName
				wantJSON, err := common.Marshal(want)
				require.NoError(t, err)
				assert.JSONEq(t, string(wantJSON), string(encoded))
			})
		}
	}
}

func TestDirectResponsesUnaffectedByChatCapabilities(t *testing.T) {
	const body = `{"model":"gpt-6-astra","input":"hi","max_output_tokens":100,"temperature":0.2,"top_p":0.8,"top_logprobs":5,"reasoning":{"effort":"high"}}`
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.UnmarshalJsonStr(body, &request))
	info := &relaycommon.RelayInfo{OriginModelName: request.Model, RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: request.Model}}
	converted, err := (&openai.Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
	require.NoError(t, err)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, body, string(encoded))
}

func TestOpenRouterChatCapabilitiesUseResolvedEffort(t *testing.T) {
	for _, tt := range []struct {
		name          string
		model         string
		controls      string
		wantEffort    string
		wantTopEffort string
		wantSampling  bool
		wantReasoning string
	}{
		{name: "top-level high becomes nested", controls: `"reasoning_effort":"high"`, wantEffort: "high", wantReasoning: `{"enabled":true,"effort":"high"}`},
		{name: "nested high", controls: `"reasoning":{"effort":"high"}`, wantEffort: "high", wantReasoning: `{"effort":"high"}`},
		{name: "nested high overrides ignored top-level none", controls: `"reasoning_effort":"none","reasoning":{"effort":"high"}`, wantEffort: "high", wantReasoning: `{"effort":"high"}`},
		{name: "nested none overrides ignored top-level high", controls: `"reasoning_effort":"high","reasoning":{"effort":"none"}`, wantEffort: "none", wantSampling: true, wantReasoning: `{"effort":"none"}`},
		{name: "enabled reasoning ignores dropped none", controls: `"reasoning_effort":"none","reasoning":{"enabled":true}`, wantReasoning: `{"enabled":true}`},
		{name: "enabled reasoning does not invent a logged effort", controls: `"reasoning_effort":"high","reasoning":{"enabled":true}`, wantReasoning: `{"enabled":true}`},
		{name: "token budget ignores dropped none", controls: `"reasoning_effort":"none","reasoning":{"max_tokens":2048}`, wantReasoning: `{"max_tokens":2048}`},
		{name: "empty object is unknown", controls: `"reasoning_effort":"none","reasoning":{}`, wantReasoning: `{}`},
		{name: "empty effort is unknown", controls: `"reasoning_effort":"none","reasoning":{"effort":""}`, wantReasoning: `{"effort":""}`},
		{name: "non-string effort is unknown", controls: `"reasoning_effort":"none","reasoning":{"effort":false}`, wantReasoning: `{"effort":false}`},
		{name: "explicit nested effort precedes disabled flag", controls: `"reasoning":{"effort":"high","enabled":false}`, wantEffort: "high", wantReasoning: `{"effort":"high","enabled":false}`},
		{name: "disabled reasoning permits sampling", controls: `"reasoning":{"enabled":false}`, wantEffort: "none", wantSampling: true, wantReasoning: `{"enabled":false}`},
		{name: "disabled reasoning overrides dropped high", controls: `"reasoning_effort":"high","reasoning":{"enabled":false}`, wantEffort: "none", wantSampling: true, wantReasoning: `{"enabled":false}`},
		{name: "no nested reasoning retains none rule", controls: `"reasoning_effort":"none"`, wantEffort: "none", wantSampling: true},
		{name: "nested high overrides none suffix", model: "gpt-5.2-none", controls: `"reasoning":{"effort":"high"}`, wantTopEffort: "none", wantEffort: "high", wantReasoning: `{"effort":"high"}`},
		{name: "nested none overrides high suffix", model: "gpt-5.2-high", controls: `"reasoning":{"effort":"none"}`, wantTopEffort: "high", wantEffort: "none", wantSampling: true, wantReasoning: `{"effort":"none"}`},
		{name: "enabled reasoning overrides none suffix", model: "gpt-6-sol-none", controls: `"reasoning":{"enabled":true}`, wantTopEffort: "none", wantReasoning: `{"enabled":true}`},
		{name: "no nested reasoning retains high suffix", model: "gpt-6-luna-high", controls: `"messages":[]`, wantTopEffort: "high", wantEffort: "high"},
		{name: "luna unknown reasoning is conservative", model: "gpt-6-luna", controls: `"reasoning":{"max_tokens":2048}`, wantReasoning: `{"max_tokens":2048}`},
		{name: "older model retains sampling", model: "gpt-4.1", controls: `"reasoning":{"enabled":true}`, wantSampling: true, wantReasoning: `{"enabled":true}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			modelName := tt.model
			if modelName == "" {
				modelName = "gpt-5.2"
			}
			request := dto.GeneralOpenAIRequest{Model: modelName}
			require.NoError(t, common.UnmarshalJsonStr(`{"temperature":0.2,"top_p":0.8,"logprobs":true,"top_logprobs":5,`+tt.controls+`}`, &request))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info, err := relaycommon.GenRelayInfo(ctx, types.RelayFormatOpenAI, &request, nil)
			require.NoError(t, err)
			info.ChannelMeta = &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: request.Model}
			_, err = (&openai.Adaptor{}).ConvertOpenAIRequest(ctx, info, &request)
			require.NoError(t, err)
			if tt.wantSampling {
				require.NotNil(t, request.Temperature)
				require.NotNil(t, request.TopP)
				require.NotNil(t, request.LogProbs)
				require.NotNil(t, request.TopLogProbs)
				assert.Equal(t, 0.2, *request.Temperature)
				assert.Equal(t, 0.8, *request.TopP)
				assert.True(t, *request.LogProbs)
				assert.EqualValues(t, 5, *request.TopLogProbs)
			} else {
				assert.Nil(t, request.Temperature)
				assert.Nil(t, request.TopP)
				assert.Nil(t, request.LogProbs)
				assert.Nil(t, request.TopLogProbs)
			}
			assert.Equal(t, tt.wantEffort, info.ReasoningEffort)
			assert.Equal(t, tt.wantTopEffort, request.ReasoningEffort, "retain the existing outbound top-level field behavior")
			if tt.wantReasoning == "" {
				assert.Empty(t, request.Reasoning)
			} else {
				assert.JSONEq(t, tt.wantReasoning, string(request.Reasoning), "retain the existing outbound nested reasoning payload")
			}
		})
	}
}
