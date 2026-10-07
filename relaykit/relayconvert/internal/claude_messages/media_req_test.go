package claudemessages

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClaudeMediaWireAndToolBatch(t *testing.T) {
	var req dto.ClaudeRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"assistant","content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"a","name":"A","input":{}},{"type":"tool_use","id":"b","name":"B","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":[{"type":"text","text":"first"},{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}}]}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="}}]}]},{"role":"assistant","content":"done"}]}`, &req))
	out, err := ClaudeMessagesRequestToOpenAIChat(req, nil)
	require.NoError(t, err)
	require.Len(t, out.Messages, 5)
	assert.Equal(t, "checking", out.Messages[0].ParseContent()[0].Text)
	assert.Equal(t, []string{"assistant", "tool", "tool", "user", "assistant"}, []string{out.Messages[0].Role, out.Messages[1].Role, out.Messages[2].Role, out.Messages[3].Role, out.Messages[4].Role})
	assert.Equal(t, "first", out.Messages[1].StringContent())
	assert.Equal(t, "", out.Messages[2].StringContent())
	assert.Equal(t, "A", *out.Messages[1].Name)
	assert.Equal(t, "B", *out.Messages[2].Name)
	parts := out.Messages[3].ParseContent()
	require.Len(t, parts, 2)
	assert.Equal(t, "https://example.com/a.png", parts[0].GetImageMedia().Url)
	assert.Equal(t, "data:application/pdf;base64,JVBERi0=", parts[1].GetFile().FileData)
}

func TestClaudeToolResultTextContractsAndMediaEOF(t *testing.T) {
	for _, tc := range []struct {
		content any
		want    string
	}{
		{nil, "null"}, {map[string]any{"answer": 42}, `{"answer":42}`}, {[]any{1, 2}, `[1,2]`}, {`[{"type":"image"}]`, `[{"type":"image"}]`},
		{[]dto.ClaudeMediaMessage{{Type: "text", Text: pointerText("one")}, {Type: "text", Text: pointerText("two")}}, "one\ntwo"},
	} {
		req := dto.ClaudeRequest{Model: "m", Messages: []dto.ClaudeMessage{{Role: "user", Content: []dto.ClaudeMediaMessage{{Type: "tool_result", ToolUseId: "a", Content: tc.content}}}}}
		out, err := ClaudeMessagesRequestToOpenAIChat(req, nil)
		require.NoError(t, err)
		require.Len(t, out.Messages, 1)
		assert.Equal(t, tc.want, out.Messages[0].StringContent())
	}
	req := dto.ClaudeRequest{Model: "m", Messages: []dto.ClaudeMessage{{Role: "user", Content: []dto.ClaudeMediaMessage{
		{Type: "text", Text: pointerText("before")},
		{Type: "tool_result", ToolUseId: "a", Content: []dto.ClaudeMediaMessage{{Type: "image", Source: &dto.ClaudeMessageSource{Type: "url", Url: "https://example.com/image.png"}}}},
		{Type: "text", Text: pointerText("after")},
	}}}}
	out, err := ClaudeMessagesRequestToOpenAIChat(req, nil)
	require.NoError(t, err)
	require.Len(t, out.Messages, 2)
	assert.Equal(t, "tool", out.Messages[0].Role)
	assert.Equal(t, "", out.Messages[0].StringContent())
	parts := out.Messages[1].ParseContent()
	require.Len(t, parts, 3)
	assert.Equal(t, "before", parts[0].Text)
	assert.Equal(t, "image_url", parts[1].Type)
	assert.Equal(t, "after", parts[2].Text)
}

func pointerText(s string) *string { return &s }

func TestClaudeOrdinaryMediaWire(t *testing.T) {
	for _, tc := range []struct{ block, want string }{
		{`{"type":"image","source":{"type":"url","url":"https://example.com/image.png"}}`, `{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}`},
		{`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}`, `{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}`},
		{`{"type":"document","source":{"type":"text","data":"hello world"}}`, `{"type":"text","text":"hello world"}`},
		{`{"type":"document","source":{"type":"base64","media_type":"text/plain","data":"aGVsbG8="}}`, `{"type":"text","text":"hello"}`},
	} {
		t.Run(tc.block, func(t *testing.T) {
			var req dto.ClaudeRequest
			require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"user","content":[`+tc.block+`]}]}`, &req))
			out, err := ClaudeMessagesRequestToOpenAIChat(req, nil)
			require.NoError(t, err)
			require.Len(t, out.Messages, 1)
			wire, err := kitutil.Marshal(out.Messages[0].Content)
			require.NoError(t, err)
			assert.JSONEq(t, "["+tc.want+"]", string(wire))
		})
	}
}
