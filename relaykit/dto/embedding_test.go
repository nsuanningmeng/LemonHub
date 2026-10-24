package dto

import (
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

// Embedding input can mix bare strings with chat-style parts; text parts count
// as text and image and video parts as files, like chat message content.
func TestEmbeddingTokenCountMetaCountsChatStyleParts(t *testing.T) {
	const imageData = "data:image/png;base64,iVBORw0KGgo="
	var request EmbeddingRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","input":["a cat",{"type":"text","text":"on a mat"},`+
		`{"type":"image_url","image_url":{"url":"`+imageData+`","detail":"low"}},`+
		`{"type":"video_url","video_url":{"url":"https://example.com/cat.mp4"}}]}`, &request))

	meta := request.GetTokenCountMeta()
	assert.Equal(t, "a cat\non a mat", meta.CombineText)
	assert.Equal(t, []*types.FileMeta{
		{FileType: types.FileTypeImage, Source: types.NewBase64FileSource(imageData, ""), Detail: "low"},
		{FileType: types.FileTypeVideo, Source: types.NewURLFileSource("https://example.com/cat.mp4")},
	}, meta.Files)
}
