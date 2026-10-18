package geminichat

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGeminiStreamCumulativeModalitiesAreNotAddedAcrossFrames(t *testing.T) {
	state := NewGeminiToChatStreamState("id", 1)
	for _, candidateTokens := range []int{1, 2, 3} {
		metadata := dto.GeminiUsageMetadata{PromptTokenCount: 1100, CandidatesTokenCount: candidateTokens,
			PromptTokensDetails:        []dto.GeminiPromptTokensDetails{{Modality: "IMAGE", TokenCount: 1089}, {Modality: "TEXT", TokenCount: 11}},
			ToolUsePromptTokensDetails: []dto.GeminiPromptTokensDetails{{Modality: "IMAGE", TokenCount: 5}},
			CacheTokensDetails:         []dto.GeminiPromptTokensDetails{{Modality: "IMAGE", TokenCount: 10}},
			CandidatesTokensDetails:    []dto.GeminiPromptTokensDetails{{Modality: "TEXT", TokenCount: candidateTokens}},
		}
		usage := UsageFromGeminiMetadata(&metadata, 0)
		frame := &dto.GeminiChatResponse{UsageMetadata: metadata, HasUsageMetadata: true}
		state.ConvertChunk(frame, "public-model", usage)
		// Re-observing the same authoritative snapshot at another conversion step
		// must not accumulate prompt images or cache modalities again.
		state.ConvertChunk(frame, "public-model", UsageFromGeminiMetadata(&metadata, 0))
		got := state.Usage()
		require.NotNil(t, got)
		assert.Equal(t, 1094, got.PromptTokensDetails.ImageTokens)
		assert.Equal(t, candidateTokens, got.CompletionTokenDetails.TextTokens)
		require.NotNil(t, got.BillingUsage)
		require.NotNil(t, got.BillingUsage.GeminiUsageMetadata)
		assert.Equal(t, 10, got.BillingUsage.GeminiUsageMetadata.CacheTokensDetails[0].TokenCount)
		metadata.PromptTokensDetails[0].TokenCount = 999
		assert.Equal(t, 1089, got.BillingUsage.GeminiUsageMetadata.PromptTokensDetails[0].TokenCount)
	}
}

func TestGeminiTypedToolSchemaConvertsWithoutMutatingFullJSONSchema(t *testing.T) {
	for _, field := range []string{"parameters", "parametersJsonSchema"} {
		var request dto.GeminiChatRequest
		schema := `{"type":"OBJECT","properties":{"count":{"type":"INTEGER","nullable":true,"minimum":0},"values":{"type":"ARRAY","minItems":"2","items":{"type":"STRING"}}},"propertyOrdering":["count","values"]}`
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"tools":[{"functionDeclarations":[{"name":"lookup","`+field+`":`+schema+`}]}]}`, &request))
		before, err := kitutil.Marshal(request)
		require.NoError(t, err)
		out, err := GeminiGenerateContentRequestToOpenAIChat(&request, nil)
		require.NoError(t, err)
		require.Len(t, out.Tools, 1)
		actual, err := kitutil.Marshal(out.Tools[0].Function.Parameters)
		require.NoError(t, err)
		if field == "parameters" {
			assert.JSONEq(t, `{"type":"object","properties":{"count":{"type":["integer","null"],"minimum":0},"values":{"type":"array","minItems":2,"items":{"type":"string"}}}}`, string(actual))
		} else {
			assert.JSONEq(t, schema, string(actual))
		}
		after, err := kitutil.Marshal(request)
		require.NoError(t, err)
		assert.Equal(t, before, after)
	}
}

func TestGeminiMediaUsesChatModalityAndRejectsUnsupportedPrivately(t *testing.T) {
	for _, tc := range []struct{ mime, kind string }{
		{"image/png", dto.ContentTypeImageURL}, {"application/pdf", dto.ContentTypeFile}, {"text/plain", dto.ContentTypeText}, {"audio/wav", dto.ContentTypeInputAudio}, {"audio/mpeg", dto.ContentTypeInputAudio},
	} {
		t.Run(tc.mime, func(t *testing.T) {
			req := &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{{Role: "user", Parts: []dto.GeminiPart{{InlineData: &dto.GeminiInlineData{MimeType: tc.mime, Data: "aGVsbG8="}}}}}}
			out, err := GeminiGenerateContentRequestToOpenAIChat(req, nil)
			require.NoError(t, err)
			require.Len(t, out.Messages, 1)
			parts := out.Messages[0].ParseContent()
			require.Len(t, parts, 1)
			assert.Equal(t, tc.kind, parts[0].Type)
			wire, err := kitutil.Marshal(out)
			require.NoError(t, err)
			assert.NotContains(t, string(wire), "MimeType")
		})
	}
	for _, part := range []dto.GeminiPart{
		{InlineData: &dto.GeminiInlineData{MimeType: "video/mp4", Data: "aGVsbG8="}},
		{InlineData: &dto.GeminiInlineData{MimeType: "application/private-type", Data: "private"}},
		{FileData: &dto.GeminiFileData{MimeType: "application/pdf", FileUri: "https://private.invalid/doc.pdf"}},
		{FileData: &dto.GeminiFileData{MimeType: "image/png", FileUri: "gs://private-bucket/file"}},
	} {
		_, err := GeminiGenerateContentRequestToOpenAIChat(&dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{{Role: "user", Parts: []dto.GeminiPart{part}}}}, nil)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "private")
	}
}
