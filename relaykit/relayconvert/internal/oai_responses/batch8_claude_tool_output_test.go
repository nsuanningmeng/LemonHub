package oairesponses

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

func TestBatch8ClaudeToolOutputMediaWire(t *testing.T) {
	relaymedia.SetMediaResolver(relaymedia.MediaResolver{GetBase64Data: func(_ context.Context, s types.FileSource, _ ...string) (string, string, error) {
		if s.GetRawData() == "data:application/pdf;base64,eA==" {
			return "eA==", "application/pdf", nil
		}
		return "eA==", "image/png", nil
	}})
	defer relaymedia.SetMediaResolver(relaymedia.MediaResolver{})
	var req dto.OpenAIResponsesRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"claude","max_output_tokens":64,"input":[{"type":"function_call","call_id":"call1","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call1","output":[{"type":"input_text","text":"result"},{"type":"input_image","image_url":"data:image/png;base64,eA=="},{"type":"input_file","file_data":"data:application/pdf;base64,eA=="}]}]}`), &req))
	out, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, &req)
	require.NoError(t, err)
	wire, err := kitutil.Marshal(out)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), `"input_image"`)
	assert.NotContains(t, string(wire), `"input_file"`)
	assert.Contains(t, string(wire), `"type":"image"`)
	assert.Contains(t, string(wire), `"type":"document"`)
	assert.Contains(t, string(wire), `"text":"result"`)
}

func TestBatch8ClaudeToolOutputOpaqueAndUnsupported(t *testing.T) {
	for _, output := range []string{`"{\"type\":\"input_image\",\"image_url\":\"literal\"}"`, `{"unknown":"literal"}`, `[1,2]`, `[{"type":"custom_result","value":1}]`} {
		var req dto.OpenAIResponsesRequest
		require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"claude","max_output_tokens":64,"input":[{"type":"function_call_output","call_id":"call1","output":`+output+`}]}`), &req))
		out, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, &req)
		require.NoError(t, err)
		parts, err := out.Messages[len(out.Messages)-1].ParseContent()
		require.NoError(t, err)
		require.Len(t, parts, 1)
		text, ok := parts[0].Content.(string)
		require.True(t, ok)
		if output[0] == '"' {
			var original string
			require.NoError(t, kitutil.Unmarshal([]byte(output), &original))
			assert.Equal(t, original, text)
		} else {
			assert.JSONEq(t, output, text)
		}
	}
	for _, part := range []string{`{"type":"input_file","file_id":"private-id"}`, `{"type":"input_file","file_data":{"private":"data"}}`, `{"type":"input_image"}`, `{"type":"input_audio","input_audio":{"data":"private-data","format":"wav"}}`} {
		var req dto.OpenAIResponsesRequest
		require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"claude","max_output_tokens":64,"input":[{"type":"function_call_output","call_id":"call1","output":[`+part+`]}]}`), &req))
		_, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, &req)
		var api *types.NewAPIError
		require.ErrorAs(t, err, &api)
		assert.Equal(t, 400, api.StatusCode)
		assert.True(t, types.IsSkipRetryError(api))
		assert.NotContains(t, err.Error(), "private-")
	}
}
