package ollama

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaConvertersPreserveExplicitZeroMaxCompletionTokens(t *testing.T) {
	zero := uint(0)
	legacy := uint(128)
	request := &dto.GeneralOpenAIRequest{
		Model:               "llama-test",
		MaxCompletionTokens: &zero,
		MaxTokens:           &legacy,
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	}

	chatRequest, err := openAIChatToOllamaChat(nil, request)
	require.NoError(t, err)
	assert.Equal(t, 0, chatRequest.Options["num_predict"])

	generateRequest, err := openAIToGenerate(nil, request)
	require.NoError(t, err)
	assert.Equal(t, 0, generateRequest.Options["num_predict"])
}
