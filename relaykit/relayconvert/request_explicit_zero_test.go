package relayconvert

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIChatToGeminiPreservesExplicitZeroGenerationOptions(t *testing.T) {
	zeroUint := uint(0)
	zeroFloat := float64(0)
	got, err := OpenAIChatRequestToGeminiGenerateContent(context.Background(), dto.GeneralOpenAIRequest{
		Model:               "gemini-test",
		MaxCompletionTokens: &zeroUint,
		MaxTokens:           uintPointer(128),
		TopP:                &zeroFloat,
		Seed:                &zeroFloat,
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	}, nil)
	require.NoError(t, err)

	require.NotNil(t, got.GenerationConfig.MaxOutputTokens)
	assert.Zero(t, *got.GenerationConfig.MaxOutputTokens)
	require.NotNil(t, got.GenerationConfig.TopP)
	assert.Zero(t, *got.GenerationConfig.TopP)
	require.NotNil(t, got.GenerationConfig.Seed)
	assert.Zero(t, *got.GenerationConfig.Seed)
}

func uintPointer(value uint) *uint {
	return &value
}
