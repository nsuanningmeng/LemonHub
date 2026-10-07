package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch5StandardFileAndWireMetadata(t *testing.T) {
	var m Message
	require.NoError(t, kitutil.Unmarshal([]byte(`{"role":"user","content":[{"type":"file","file":{"filename":"a.pdf","file_id":"","file_data":"data:application/pdf;base64,eA=="},"cache_control":{"type":"ephemeral"}}]}`), &m))
	p := m.ParseContent()
	require.Len(t, p, 1)
	f := p[0].GetFile()
	require.NotNil(t, f)
	assert.Equal(t, "a.pdf", f.FileName)
	assert.Equal(t, "data:application/pdf;base64,eA==", f.FileData)
	assert.NotEmpty(t, p[0].CacheControl)
	wire, err := kitutil.Marshal(MessageImageUrl{Url: "https://example.test/image", MimeType: "image/png"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"url":"https://example.test/image"}`, string(wire))
	var typed = Message{Content: []MediaContent{{Type: "text", Text: "direct"}}}
	require.Len(t, typed.ParseContent(), 1)
	assert.Equal(t, "direct", typed.ParseContent()[0].Text)
}

func TestBatch5NullableUnselectedFileSources(t *testing.T) {
	for _, file := range []string{`{"filename":"a.pdf","file_id":null,"file_data":"data:application/pdf;base64,eA=="}`, `{"file_data":null,"file_id":"file-native"}`} {
		var m Message
		require.NoError(t, kitutil.Unmarshal([]byte(`{"content":[{"type":"file","file":`+file+`}]}`), &m))
		parts := m.ParseContent()
		require.Len(t, parts, 1)
		f := parts[0].GetFile()
		require.NotNil(t, f)
		assert.NotEmpty(t, f.FileData+f.FileId)
	}
}
