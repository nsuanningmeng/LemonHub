package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptorCrossProtocolStreamUsage(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatGemini, types.RelayFormatClaude} {
		for _, tc := range []struct {
			name        string
			channelType int
			stream      bool
			supported   bool
		}{
			{"openai stream", constant.ChannelTypeOpenAI, true, true},
			{"azure stream", constant.ChannelTypeAzure, true, true},
			{"non-stream", constant.ChannelTypeOpenAI, false, true},
			{"unsupported stream", constant.ChannelTypeOpenAI, true, false},
		} {
			t.Run(string(format)+"/"+tc.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				info := &relaycommon.RelayInfo{
					RelayFormat:     format,
					OriginModelName: "gpt-test",
					IsStream:        tc.stream,
					ChannelMeta: &relaycommon.ChannelMeta{
						ChannelType:          tc.channelType,
						UpstreamModelName:    "gpt-test",
						SupportStreamOptions: tc.supported,
					},
				}
				adaptor := &Adaptor{}
				adaptor.Init(info)
				var converted any
				var err error
				if format == types.RelayFormatGemini {
					converted, err = adaptor.ConvertGeminiRequest(c, info, &dto.GeminiChatRequest{
						Contents: []dto.GeminiChatContent{{Role: "user", Parts: []dto.GeminiPart{{Text: "hello"}}}},
					})
				} else {
					converted, err = adaptor.ConvertClaudeRequest(c, info, &dto.ClaudeRequest{
						Model: "gpt-test", Stream: &tc.stream,
						Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
					})
				}
				require.NoError(t, err)
				payload, err := common.Marshal(converted)
				require.NoError(t, err)
				var body map[string]any
				require.NoError(t, common.Unmarshal(payload, &body))
				assert.Equal(t, tc.stream, body["stream"])
				assert.Equal(t, "gpt-test", body["model"])
				if tc.stream && tc.supported {
					assert.Equal(t, map[string]any{"include_usage": true}, body["stream_options"])
				} else {
					assert.NotContains(t, body, "stream_options")
				}
			})
		}
	}
}
