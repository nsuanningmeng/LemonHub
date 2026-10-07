package oaichat

import (
	"errors"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChatResponsesFileWire(t *testing.T) {
	for _, file := range []string{`{"filename":"a.pdf","file_data":"data:application/pdf;base64,JVBERi0="}`, `{"file_id":"file-native"}`} {
		var req dto.GeneralOpenAIRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"user","content":[{"type":"file","file":`+file+`}]}]}`, &req))
		out, err := ChatCompletionsRequestToResponsesRequest(&req)
		require.NoError(t, err)
		var input []map[string]any
		require.NoError(t, kitutil.Unmarshal(out.Input, &input))
		part := input[0]["content"].([]any)[0].(map[string]any)
		var expected map[string]any
		require.NoError(t, kitutil.UnmarshalJsonStr(file, &expected))
		expected["type"] = "input_file"
		assert.Equal(t, expected, part)
	}
	for _, file := range []string{`null`, `{}`, `{"filename":"empty.pdf"}`, `{"file_id":"private-id","file_data":"private-data"}`} {
		var req dto.GeneralOpenAIRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"user","content":[{"type":"file","file":`+file+`}]}]}`, &req))
		_, err := ChatCompletionsRequestToResponsesRequest(&req)
		require.Error(t, err)
		var api *types.NewAPIError
		require.True(t, errors.As(err, &api))
		assert.Equal(t, 400, api.StatusCode)
		assert.True(t, types.IsSkipRetryError(api))
		assert.NotContains(t, err.Error(), "private-")
	}
}
