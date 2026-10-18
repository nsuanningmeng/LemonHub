package oairesponses

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResponsesGeminiToolMediaPreservesParallelResults(t *testing.T) {
	relaymedia.SetMediaResolver(relaymedia.MediaResolver{GetBase64Data: func(_ context.Context, s types.FileSource, _ ...string) (string, string, error) {
		return "cHJpdmF0ZS1tZWRpYQ==", "image/png", nil
	}})
	t.Cleanup(func() { relaymedia.SetMediaResolver(relaymedia.MediaResolver{}) })
	for _, model := range []string{"gemini-2.5-flash", "gemini-3-pro", "publishers/google/models/gemini-3-flash", "custom-target"} {
		t.Run(model, func(t *testing.T) {
			var req dto.OpenAIResponsesRequest
			require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"`+model+`","input":[{"type":"function_call","call_id":"a","name":"view_a","arguments":"{}"},{"type":"function_call","call_id":"b","name":"view_b","arguments":"{}"},{"type":"function_call_output","call_id":"a","output":[{"type":"input_text","text":"first"},{"type":"input_image","image_url":"https://private.invalid/a.png"}]},{"type":"function_call_output","call_id":"b","output":[{"type":"input_text","text":"second"},{"type":"input_image","image_url":"https://private.invalid/b.png"}]}]}`, &req))
			before, err := kitutil.Marshal(req)
			require.NoError(t, err)
			out, err := OpenAIResponsesRequestToGeminiChat(context.Background(), &req, nil)
			require.NoError(t, err)
			require.Len(t, out.Contents, 2)
			parts := out.Contents[1].Parts
			require.GreaterOrEqual(t, len(parts), 2)
			for i, want := range []string{"first", "second"} {
				require.NotNil(t, parts[i].FunctionResponse)
				assert.Equal(t, want, parts[i].FunctionResponse.Response["content"])
				response, err := kitutil.Marshal(parts[i].FunctionResponse.Response)
				require.NoError(t, err)
				assert.NotContains(t, string(response), "cHJpdmF0ZS1tZWRpYQ==")
			}
			if model == "gemini-2.5-flash" || model == "custom-target" {
				require.Len(t, parts, 4)
				assert.Empty(t, parts[0].FunctionResponse.Parts)
				require.NotNil(t, parts[2].InlineData)
				require.NotNil(t, parts[3].InlineData)
			} else {
				require.Len(t, parts, 2)
				for _, part := range parts {
					var media []dto.GeminiPart
					require.NoError(t, kitutil.Unmarshal(part.FunctionResponse.Parts, &media))
					require.Len(t, media, 1)
					assert.Equal(t, "image/png", media[0].InlineData.MimeType)
				}
			}
			after, err := kitutil.Marshal(req)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestResponsesGeminiToolMediaRejectsResolverFailuresPrivately(t *testing.T) {
	relaymedia.SetMediaResolver(relaymedia.MediaResolver{GetBase64Data: func(_ context.Context, s types.FileSource, _ ...string) (string, string, error) {
		return "", "", errors.New("secret URL credentials")
	}})
	t.Cleanup(func() { relaymedia.SetMediaResolver(relaymedia.MediaResolver{}) })
	_, _, err := responsesFunctionOutputItemToGeminiPart(context.Background(), map[string]any{"output": []any{map[string]any{"type": "input_image", "image_url": "https://private.invalid/a.png"}}}, nil, "gemini-3-pro")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "private")
}
