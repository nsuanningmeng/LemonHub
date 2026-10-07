package oairesponses

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch5DirectResponsesRejectFileReferences(t *testing.T) {
	for _, part := range []map[string]any{{"type": "input_file", "file_id": "secret"}, {"type": "input_file", "file": map[string]any{"file_id": "secret"}}, {"type": "input_file", "file": map[string]any{"file_data": map[string]any{"secret": 1}}}, {"type": "input_file", "file_id": 12, "file_data": "data:application/pdf;base64,eA=="}, {"type": "input_file"}, {"type": "input_image"}} {
		_, err := responsesInputContentToClaudeMediaMessages(context.Background(), []any{part})
		var e *types.NewAPIError
		require.ErrorAs(t, err, &e)
		assert.Equal(t, 400, e.StatusCode)
		assert.True(t, types.IsSkipRetryError(e))
		assert.NotContains(t, err.Error(), "secret")
		_, err = responsesContentPartToGeminiParts(context.Background(), part)
		require.ErrorAs(t, err, &e)
		assert.Equal(t, 400, e.StatusCode)
	}
}
