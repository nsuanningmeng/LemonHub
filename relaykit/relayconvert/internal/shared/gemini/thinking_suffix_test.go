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

func TestGeminiArgonEffortSuffixUsesSupportedLevels(t *testing.T) {
	for _, tc := range []struct{ suffix, want string }{{"minimal", "low"}, {"low", "low"}, {"medium", "medium"}, {"high", "high"}, {"xhigh", "high"}, {"max", "high"}} {
		info := &convmeta.Values{UpstreamModelName: "gemini-4-argon-" + tc.suffix, ChannelMetaAttached: true, Options: &convmeta.Options{Gemini: convmeta.GeminiOptions{ThinkingAdapterEnabled: true}}}
		req := &dto.GeminiChatRequest{}
		ApplyThinkingConfig(req, info)
		require.NotNil(t, req.GenerationConfig.ThinkingConfig)
		assert.Equal(t, tc.want, req.GenerationConfig.ThinkingConfig.ThinkingLevel)
		assert.Nil(t, req.GenerationConfig.ThinkingConfig.ThinkingBudget)
	}
}
