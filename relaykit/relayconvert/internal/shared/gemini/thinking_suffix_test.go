package gemini

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGemini25EffortSuffixUsesModelBudget(t *testing.T) {
	for _, tc := range []struct {
		model string
		want  int
	}{
		{"gemini-2.5-pro-xhigh", 32768}, {"gemini-2.5-pro-max", 32768}, {"gemini-2.5-pro-high", 24576},
		{"gemini-2.5-flash-max", 24576}, {"gemini-2.5-flash-lite-xhigh", 24576},
	} {
		t.Run(tc.model, func(t *testing.T) {
			info := &convmeta.Values{UpstreamModelName: tc.model, ChannelMetaAttached: true, Options: &convmeta.Options{Gemini: convmeta.GeminiOptions{ThinkingAdapterEnabled: true}}}
			req := &dto.GeminiChatRequest{}
			ApplyThinkingConfig(req, info)
			require.NotNil(t, req.GenerationConfig.ThinkingConfig)
			require.NotNil(t, req.GenerationConfig.ThinkingConfig.ThinkingBudget)
			assert.Equal(t, tc.want, *req.GenerationConfig.ThinkingConfig.ThinkingBudget)
			assert.Empty(t, req.GenerationConfig.ThinkingConfig.ThinkingLevel)
			info.Options.Gemini.ThinkingAdapterEnabled = false
			req = &dto.GeminiChatRequest{}
			ApplyThinkingConfig(req, info)
			assert.Nil(t, req.GenerationConfig.ThinkingConfig)
		})
	}
}
