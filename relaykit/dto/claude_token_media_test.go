package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeToolResultTokenMediaRawJSON(t *testing.T) {
	var req ClaudeRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"claude-test","max_tokens":128,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-id","content":[{"type":"text","text":"visible tool text"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}},{"type":"image","source":{"type":"url","url":"https://example.com/tool.png"}}]}]}]}`, &req))
	meta := req.GetTokenCountMeta()
	assert.Equal(t, "user\nvisible tool text", meta.CombineText)
	assert.Equal(t, 128, meta.MaxTokens)
	assert.Equal(t, 1, meta.MessagesCount)
	require.Len(t, meta.Files, 2)
	assert.Equal(t, types.FileTypeImage, meta.Files[0].FileType)
	assert.Equal(t, "iVBORw0KGgo=", meta.Files[0].Source.GetRawData())
	assert.Equal(t, "https://example.com/tool.png", meta.Files[1].Source.GetRawData())
	assert.True(t, meta.Files[1].Source.IsURL())
}

func TestClaudeStringToolResultTokenTextExact(t *testing.T) {
	var req ClaudeRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"literal \"quotes\"\nand [JSON-looking] text"}]}]}`, &req))
	meta := req.GetTokenCountMeta()
	assert.Equal(t, "user\nliteral \"quotes\"\nand [JSON-looking] text", meta.CombineText)
	assert.Empty(t, meta.Files)
}

func TestClaudeTokenMediaRawAndTypedDocuments(t *testing.T) {
	for _, tc := range []struct {
		name, block, text string
		fileType          types.FileType
		data              string
	}{
		{name: "text document", block: `{"type":"document","source":{"type":"text","data":"document text"}}`, text: "document text"},
		{name: "base64 text", block: `{"type":"document","source":{"type":"base64","media_type":"text/plain","data":"ZG9jdW1lbnQgdGV4dA=="}}`, text: "document text"},
		{name: "PDF", block: `{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="}}`, fileType: types.FileTypeFile, data: "JVBERi0="},
		{name: "URL document", block: `{"type":"document","source":{"type":"url","url":"https://example.com/document.pdf"}}`, fileType: types.FileTypeFile, data: "https://example.com/document.pdf"},
		{name: "image", block: `{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}`, fileType: types.FileTypeImage, data: "iVBORw0KGgo="},
	} {
		for _, typed := range []bool{false, true} {
			for _, nested := range []bool{false, true} {
				t.Run(tc.name+map[bool]string{false: " raw", true: " typed"}[typed]+map[bool]string{false: " ordinary", true: " tool"}[nested], func(t *testing.T) {
					content := tc.block
					if nested {
						content = `{"type":"tool_result","tool_use_id":"a","content":[` + content + `]}`
					}
					var req ClaudeRequest
					require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"user","content":[`+content+`]}]}`, &req))
					if typed {
						var block ClaudeMediaMessage
						require.NoError(t, kitutil.UnmarshalJsonStr(tc.block, &block))
						if nested {
							req.Messages[0].Content = []ClaudeMediaMessage{{Type: "tool_result", Content: []ClaudeMediaMessage{block}}}
						} else {
							req.Messages[0].Content = []ClaudeMediaMessage{block}
						}
					}
					meta := req.GetTokenCountMeta()
					want := "user"
					if tc.text != "" {
						want += "\n" + tc.text
					}
					assert.Equal(t, want, meta.CombineText)
					if tc.fileType != "" {
						require.Len(t, meta.Files, 1)
						assert.Equal(t, tc.fileType, meta.Files[0].FileType)
						assert.Equal(t, tc.data, meta.Files[0].Source.GetRawData())
					} else {
						assert.Empty(t, meta.Files)
					}
				})
			}
		}
	}
}

func TestClaudeToolTokenUnknownObjectsRemainOpaque(t *testing.T) {
	for _, content := range []string{
		`{"type":"unknown","text":"opaque-text","data":"opaque-payload"}`,
		`[{"type":"unknown","text":"opaque-text","source":{"data":"opaque-payload"}},{"answer":"opaque-text"}]`,
		`[{"type":"image","source":{"type":"file","file_id":"opaque-file-id"}},{"type":"document","source":{"type":"content","data":"opaque-payload"}}]`,
		`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":{"value":"opaque-payload"}}},{"type":"image"}]`,
		`null`,
	} {
		var req ClaudeRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"user","content":[{"type":"tool_result","content":`+content+`}]}]}`, &req))
		meta := req.GetTokenCountMeta()
		assert.Equal(t, "user", meta.CombineText)
		assert.Empty(t, meta.Files)
	}
}

func TestClaudeTokenMetaPreservesExistingToolAndSystemText(t *testing.T) {
	var req ClaudeRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","system":"system text","messages":[{"role":"assistant","content":[{"type":"text","text":"assistant text"},{"type":"tool_use","name":"run","input":{"query":"text"}}]},{"role":"user","content":"user text"}],"tools":[{"name":"search","description":"description","input_schema":{"type":"object"}}]}`, &req))
	req.Tools = []any{Tool{Name: "search", Description: "description", InputSchema: map[string]any{"type": "object"}}}
	meta := req.GetTokenCountMeta()
	assert.Equal(t, "system text\nassistant\nassistant text\nrun\n{\"query\":\"text\"}\nuser\nuser text\nsearch\ndescription\n{\"type\":\"object\"}", meta.CombineText)
	assert.Equal(t, 2, meta.MessagesCount)
	assert.Equal(t, 1, meta.ToolsCount)
	assert.Empty(t, meta.Files)
}
