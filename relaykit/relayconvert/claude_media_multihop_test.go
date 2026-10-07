package relayconvert

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeDocumentMultiHopWire(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-context")
	SetMediaResolver(MediaResolver{GetBase64Data: func(c context.Context, source types.FileSource, reason ...string) (string, string, error) {
		assert.Equal(t, "request-context", c.Value(contextKey{}))
		return "JVBERi0=", "application/pdf", nil
	}})
	t.Cleanup(func() {
		SetMediaResolver(MediaResolver{
			GetBase64Data: func(context.Context, types.FileSource, ...string) (string, string, error) {
				return "aGVsbG8=", "image/png", nil
			},
			DecodeBase64FileData: func(string) (string, string, error) { return "aGVsbG8=", "image/png", nil },
		})
	})
	for _, source := range []string{`{"type":"base64","media_type":"application/pdf","data":"JVBERi0="}`, `{"type":"url","url":"https://example.com/document.pdf"}`} {
		var req dto.ClaudeRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"user","content":[{"type":"document","source":`+source+`}]}]}`, &req))
		result, err := ConvertRequest(ctx, nil, types.RelayFormatOpenAIResponses, &req)
		require.NoError(t, err)
		responses := result.Value.(*dto.OpenAIResponsesRequest)
		var input []map[string]any
		require.NoError(t, kitutil.Unmarshal(responses.Input, &input))
		require.Len(t, input, 1)
		parts := input[0]["content"].([]any)
		require.Len(t, parts, 1)
		assert.Equal(t, map[string]any{"type": "input_file", "filename": "document.pdf", "file_data": "data:application/pdf;base64,JVBERi0="}, parts[0])
		result, err = ConvertRequest(ctx, nil, types.RelayFormatGemini, &req)
		require.NoError(t, err)
		wire, err := kitutil.Marshal(result.Value)
		require.NoError(t, err)
		assert.Contains(t, string(wire), `"mimeType":"application/pdf"`)
		assert.Contains(t, string(wire), `"data":"JVBERi0="`)
	}
}

func TestClaudeUnsupportedMediaErrorsAreTypedAndPrivate(t *testing.T) {
	for _, block := range []string{
		`{"type":"image"}`, `{"type":"document","source":{"type":"file","file_id":"private-id"}}`,
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":{"secret":"private-data"}}}`,
		`{"type":"image","source":{"type":"url","url":22}}`,
		`{"type":"document","source":{"type":"content","data":"private-data"}}`,
		`{"type":"audio","source":{"type":"base64","data":"private-data"}}`,
		`{"type":"document","source":{"type":"base64","media_type":"text/plain","data":"/w=="}}`,
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"JVBERi0="}}`,
	} {
		for _, nested := range []bool{false, true} {
			t.Run(block+map[bool]string{false: "ordinary", true: "nested"}[nested], func(t *testing.T) {
				content := block
				if nested {
					content = `{"type":"tool_result","tool_use_id":"a","content":[` + block + `]}`
				}
				var req dto.ClaudeRequest
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"user","content":[`+content+`]}]}`, &req))
				_, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, &req)
				require.Error(t, err)
				var apiErr *types.NewAPIError
				require.True(t, errors.As(err, &apiErr))
				assert.Equal(t, 400, apiErr.StatusCode)
				assert.True(t, types.IsSkipRetryError(apiErr))
				assert.NotContains(t, err.Error(), "private-")
				assert.NotContains(t, err.Error(), "JVBERi0=")
			})
		}
	}
}
