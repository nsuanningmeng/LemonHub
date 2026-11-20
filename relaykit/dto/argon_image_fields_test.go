package dto

import (
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestGeminiArgonFieldsPreserveNativeJSONAndAliases(t *testing.T) {
	for _, snake := range []bool{false, true} {
		token, media := "continuationToken", "mediaProcessing"
		if snake {
			token, media = "continuation_token", "media_processing"
		}
		raw := `{"` + token + `":{"opaque":9007199254740993},"contents":[{"parts":[{"` + media + `":{"quality":0}}]}],"generationConfig":{"` + media + `":{"enabled":false}}}`
		var req GeminiChatRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(raw, &req))
		wire, err := kitutil.Marshal(req)
		require.NoError(t, err)
		assert.Equal(t, `{"opaque":9007199254740993}`, gjson.GetBytes(wire, "continuationToken").Raw)
		assert.Equal(t, `{"quality":0}`, gjson.GetBytes(wire, "contents.0.parts.0.mediaProcessing").Raw)
		assert.Equal(t, `{"enabled":false}`, gjson.GetBytes(wire, "generationConfig.mediaProcessing").Raw)
	}
	var candidate GeminiChatCandidate
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"continuationToken":"opaque-token"}`, &candidate))
	wire, err := kitutil.Marshal(candidate)
	require.NoError(t, err)
	assert.Equal(t, "opaque-token", gjson.GetBytes(wire, "continuationToken").String())
}

func TestImage26FieldsPreserveExplicitZeroAndFalse(t *testing.T) {
	var req ImageRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gpt-image-2.6","prompt":"draw","complexity":"low","markup":{"image_url":"data:image/png;base64,eA=="},"svg":false,"frame_count":0,"frame_duration_ms":0}`, &req))
	wire, err := kitutil.Marshal(req)
	require.NoError(t, err)
	assert.Equal(t, "low", gjson.GetBytes(wire, "complexity").String())
	assert.Equal(t, `{"image_url":"data:image/png;base64,eA=="}`, gjson.GetBytes(wire, "markup").Raw)
	for _, key := range []string{"svg", "frame_count", "frame_duration_ms"} {
		assert.True(t, gjson.GetBytes(wire, key).Exists())
	}
	empty, err := kitutil.Marshal(ImageRequest{Model: "gpt-image-2.6", Prompt: "draw"})
	require.NoError(t, err)
	for _, key := range []string{"complexity", "markup", "svg", "frame_count", "frame_duration_ms"} {
		assert.False(t, gjson.GetBytes(empty, key).Exists())
	}
}
