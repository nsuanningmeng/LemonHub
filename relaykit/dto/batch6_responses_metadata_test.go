package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch6ResponsesToolOutputTextAndMedia(t *testing.T) {
	for _, output := range []string{`"sensitive sentinel"`, `[{"type":"input_text","text":"sensitive sentinel"},{"type":"input_image","image_url":"data:image/png;base64,opaque-image-marker","detail":"low"}]`} {
		var r OpenAIResponsesRequest
		require.NoError(t, kitutil.Unmarshal([]byte(`{"input":[{"role":"user","content":"hello"},{"type":"function_call_output","call_id":"private-call-id","output":`+output+`},{"type":"reasoning","encrypted_content":"opaque-reasoning-marker"},{"type":"compaction","encrypted_content":"opaque-compaction-marker"}]}`), &r))
		meta := r.GetTokenCountMeta()
		assert.Contains(t, meta.CombineText, "hello")
		assert.Contains(t, meta.CombineText, "sensitive sentinel")
		assert.NotContains(t, meta.CombineText, "opaque-")
		assert.NotContains(t, meta.CombineText, "private-call-id")
		if output[0] == '[' {
			require.Len(t, meta.Files, 1)
			assert.Equal(t, "low", meta.Files[0].Detail)
		}
	}
}

func TestBatch6CompactStructuredMetadata(t *testing.T) {
	var r OpenAIResponsesCompactionRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"instructions":"exact instructions","tools":[{"type":"function","name":"lookup","description":"tool description"}],"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"message text"},{"type":"input_image","image_url":"https://example.test/image"}]},{"type":"function_call_output","output":[{"type":"input_text","text":"tool text"},{"type":"input_file","file_data":"data:application/pdf;base64,opaque-pdf-marker"}]},{"type":"compaction","encrypted_content":"opaque-blob-marker"}]}`), &r))
	meta := r.GetTokenCountMeta()
	assert.Contains(t, meta.CombineText, "exact instructions")
	assert.NotContains(t, meta.CombineText, `\"exact instructions\"`)
	assert.Contains(t, meta.CombineText, "tool description")
	assert.Contains(t, meta.CombineText, "message text")
	assert.Contains(t, meta.CombineText, "tool text")
	assert.NotContains(t, meta.CombineText, "opaque-")
	assert.NotContains(t, meta.CombineText, "https://example.test/image")
	require.Len(t, meta.Files, 2)
}

func TestBatch6ResponsesMetadataBoundaries(t *testing.T) {
	tests := []struct {
		name, input, text string
		files             int
	}{
		{"string input", `"plain input"`, "plain input", 0},
		{"literal tool string", `[{"type":"function_call_output","output":"{\"type\":\"input_image\",\"image_url\":\"literal text\"}"}]`, `{"type":"input_image","image_url":"literal text"}`, 0},
		{"text types and malformed siblings", `[{"type":"function_call_output","output":[{"type":"input_text","text":12},{"type":"output_text","text":"answer"},{"type":"text","text":"detail"},{"type":"unknown","text":"opaque-marker","content":"opaque-marker"}]}]`, "answer\ndetail", 0},
		{"multiple images", `[{"type":"message","content":[{"type":"input_image","image_url":"data:image/png;base64,opaque-marker"},{"type":"input_image","image_url":{"url":"https://example.test/image","detail":"low"}}]}]`, "", 2},
		{"file references are opaque", `[{"type":"message","content":[{"type":"input_file","file_id":"opaque-file-id"},{"type":"input_file","file_url":"https://example.test/document.pdf"}]}]`, "", 1},
		{"reasoning and compaction are opaque", `[{"type":"reasoning","content":"opaque-marker","summary":[{"type":"text","text":"opaque-marker"}],"encrypted_content":"opaque-marker"},{"type":"compaction","content":[{"type":"input_text","text":"opaque-marker"}],"encrypted_content":"opaque-marker"}]`, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r OpenAIResponsesRequest
			require.NoError(t, kitutil.Unmarshal([]byte(`{"input":`+tt.input+`}`), &r))
			meta := r.GetTokenCountMeta()
			assert.Equal(t, tt.text, meta.CombineText)
			assert.Len(t, meta.Files, tt.files)
			compact := OpenAIResponsesCompactionRequest{Input: r.Input}
			cm := compact.GetTokenCountMeta()
			assert.Equal(t, meta.CombineText, cm.CombineText)
			assert.Len(t, cm.Files, tt.files)
		})
	}
}

func TestBatch6ResponsesAndCompactInstructionsAreExactText(t *testing.T) {
	var r OpenAIResponsesRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"instructions":"line one\nline two","input":"question"}`), &r))
	assert.Equal(t, "question\nline one\nline two", r.GetTokenCountMeta().CombineText)
	compact := OpenAIResponsesCompactionRequest{Input: r.Input, Instructions: r.Instructions}
	assert.Equal(t, "question\nline one\nline two", compact.GetTokenCountMeta().CombineText)
}
