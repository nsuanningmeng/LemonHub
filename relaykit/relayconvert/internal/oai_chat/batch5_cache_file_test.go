package oaichat

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch5RawCache(t *testing.T) {
	var req dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"max_tokens":100,"messages":[{"role":"system","content":[{"type":"text","text":"rules","cache_control":{"type":"ephemeral","ttl":"1h"}}]},{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}},{"type":"text","text":"plain"}]}]}`), &req))
	out, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, req)
	require.NoError(t, err)
	wire, err := kitutil.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(wire), `"ttl":"1h"`)
	b := out.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.NotEmpty(t, b[0].CacheControl)
	assert.Empty(t, b[1].CacheControl)
	b[0].CacheControl[0] = 'x'
	assert.Equal(t, byte('{'), req.Messages[1].ParseContent()[0].CacheControl[0])
}
func TestBatch5FileID(t *testing.T) {
	var req dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"max_tokens":100,"messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"secret-ID"}}]}]}`), &req))
	_, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, req)
	var e *types.NewAPIError
	require.ErrorAs(t, err, &e)
	assert.Equal(t, 400, e.StatusCode)
	assert.True(t, types.IsSkipRetryError(e))
	assert.NotContains(t, err.Error(), "secret-ID")
	_, err = OpenAIChatRequestToGeminiGenerateContent(context.Background(), req, nil)
	require.ErrorAs(t, err, &e)
	assert.Equal(t, 400, e.StatusCode)
}

func TestBatch5TypedContentCache(t *testing.T) {
	req := dto.GeneralOpenAIRequest{MaxTokens: kitutil.GetPointer(uint(100))}
	var m dto.Message
	m.Role = "user"
	m.SetMediaContent([]dto.MediaContent{{Type: "text", Text: "typed", CacheControl: []byte(`{"type":"ephemeral","ttl":"1h"}`)}})
	req.Messages = []dto.Message{m}
	out, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, req)
	require.NoError(t, err)
	b := out.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.Len(t, b, 1)
	assert.Equal(t, "typed", *b[0].Text)
	require.NotEmpty(t, b[0].CacheControl)
	b[0].CacheControl[0] = 'x'
	assert.Equal(t, byte('{'), m.ParseContent()[0].CacheControl[0])
	out, err = OpenAIChatRequestToClaudeMessages(context.Background(), nil, req)
	require.NoError(t, err)
	assert.Equal(t, byte('{'), out.Messages[0].Content.([]dto.ClaudeMediaMessage)[0].CacheControl[0])
}

func TestBatch5InvalidFileSources(t *testing.T) {
	for _, part := range []string{`{"type":"file","file":{}}`, `{"type":"file","file":{"file_data":{"secret":"payload"}}}`, `{"type":"file","file":{"file_id":"secret","file_data":"data:application/pdf;base64,eA=="}}`, `{"type":"file"}`} {
		t.Run(part, func(t *testing.T) {
			var req dto.GeneralOpenAIRequest
			require.NoError(t, kitutil.Unmarshal([]byte(`{"max_tokens":100,"messages":[{"role":"user","content":[`+part+`]}]}`), &req))
			_, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, req)
			var e *types.NewAPIError
			require.ErrorAs(t, err, &e)
			assert.Equal(t, 400, e.StatusCode)
			assert.NotContains(t, err.Error(), "secret")
			_, err = OpenAIChatRequestToGeminiGenerateContent(context.Background(), req, nil)
			require.ErrorAs(t, err, &e)
			assert.Equal(t, 400, e.StatusCode)
		})
	}
}

func TestBatch5InlineMediaCacheFinalWire(t *testing.T) {
	relaymedia.SetMediaResolver(relaymedia.MediaResolver{GetBase64Data: func(_ context.Context, source types.FileSource, _ ...string) (string, string, error) {
		if source.GetRawData() == "data:application/pdf;base64,eA==" {
			return "eA==", "application/pdf", nil
		}
		return "eA==", "image/png", nil
	}})
	defer relaymedia.SetMediaResolver(relaymedia.MediaResolver{})
	var req dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"max_tokens":100,"messages":[{"role":"user","content":[{"type":"file","file":{"file_id":null,"filename":"a.pdf","file_data":"data:application/pdf;base64,eA=="},"cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"image_url","image_url":{"url":"data:image/png;base64,eA=="},"cache_control":{"type":"ephemeral"}}]}]}`), &req))
	out, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, req)
	require.NoError(t, err)
	b := out.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.Len(t, b, 2)
	assert.Equal(t, "document", b[0].Type)
	assert.Equal(t, "image", b[1].Type)
	wire, err := kitutil.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(wire), `"media_type":"application/pdf","data":"eA=="`)
	assert.Contains(t, string(wire), `"ttl":"1h"`)
	b[1].CacheControl[0] = 'x'
	assert.Equal(t, byte('{'), req.Messages[0].ParseContent()[1].CacheControl[0])
	gemini, err := OpenAIChatRequestToGeminiGenerateContent(context.Background(), req, nil)
	require.NoError(t, err)
	assert.Equal(t, "application/pdf", gemini.Contents[0].Parts[0].InlineData.MimeType)
}

func TestBatch5NullableUnselectedFileDataResponses(t *testing.T) {
	var req dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"m","messages":[{"role":"user","content":[{"type":"file","file":{"file_data":null,"file_id":"file-native"}}]}]}`), &req))
	out, err := ChatCompletionsRequestToResponsesRequest(&req)
	require.NoError(t, err)
	assert.Contains(t, string(out.Input), `"file_id":"file-native"`)
	assert.NotContains(t, string(out.Input), `"file_data"`)
}
