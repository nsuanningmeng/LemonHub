package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Gemini allows up to 5 stop sequences, OpenAI up to 4. The conversion caps the
// slice at 4, but a fixed [:4] re-slice panics when the client sends 1-3
// sequences (len/cap < 4). This locks the min(len,4) cap so a valid short
// stop-sequence list converts instead of triggering a recovered panic / 500.
func TestGeminiToOpenAIRequestCapsStopSequences(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}

	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"none", nil, nil},
		{"one", []string{"a"}, []string{"a"}},
		{"three", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"four", []string{"a", "b", "c", "d"}, []string{"a", "b", "c", "d"}},
		{"five-capped", []string{"a", "b", "c", "d", "e"}, []string{"a", "b", "c", "d"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &dto.GeminiChatRequest{}
			req.GenerationConfig.StopSequences = tc.in

			got, err := GeminiToOpenAIRequest(req, info)
			require.NoError(t, err)

			if tc.want == nil {
				assert.Nil(t, got.Stop)
				return
			}
			stop, ok := got.Stop.([]string)
			require.True(t, ok, "Stop should be []string, got %T", got.Stop)
			assert.Equal(t, tc.want, stop)
		})
	}
}
