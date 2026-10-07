package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch8AdvancedResponsesClaudeSavedConfigAndPathScope(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/messages", "/v1/responses/compact", "/v1/alpha/search", "/v1/models", "/v1/dashboard/billing/credit_grants"} {
		var cfg AdvancedCustomConfig
		require.NoError(t, kitutil.Unmarshal([]byte(`{"advanced_routes":[{"incoming_path":"`+path+`","upstream_path":"/v1/messages","converter":"openai_responses_to_claude_messages","auth":{"type":"header","name":"x-api-key","value":"{api_key}"}}]}`), &cfg))
		err := cfg.Validate()
		if path != "/v1/responses" {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		wire, err := kitutil.Marshal(cfg)
		require.NoError(t, err)
		var restored AdvancedCustomConfig
		require.NoError(t, kitutil.Unmarshal(wire, &restored))
		require.NoError(t, restored.Validate())
		assert.Equal(t, cfg, restored)
	}
	for _, id := range []string{"claude_messages_to_gemini_generate_content", "gemini_generate_content_to_claude_messages", "private-unknown-ID"} {
		assert.False(t, IsAdvancedCustomConverterAllowed(id))
	}
}
