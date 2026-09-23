package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiStreamHandlerCrossProtocolUsageOnlyTail(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	for _, format := range []types.RelayFormat{types.RelayFormatGemini, types.RelayFormatClaude} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				RelayFormat: format,
				RelayMode:   relayconstant.RelayModeChatCompletions,
				IsStream:    true,
				DisablePing: true,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-test", SupportStreamOptions: true,
				},
				ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{},
			}
			info.SetEstimatePromptTokens(3)
			body := strings.Join([]string{
				`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant","content":""}}],"usage":null}`,
				`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"}}],"usage":null}`,
				`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":null}`,
				`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`,
				`data: [DONE]`,
				"",
			}, "\n\n")
			response := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}

			usage, apiErr := OaiStreamHandler(c, info, response)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 11, usage.PromptTokens)
			assert.Equal(t, 5, usage.CompletionTokens)
			assert.Equal(t, 16, usage.TotalTokens)
			assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))

			var frames []string
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				if strings.HasPrefix(line, "data: ") {
					frames = append(frames, strings.TrimPrefix(line, "data: "))
				}
			}
			require.NotEmpty(t, frames)
			if format == types.RelayFormatGemini {
				var text strings.Builder
				var stopCount int
				var last dto.GeminiChatResponse
				for _, frame := range frames {
					require.NoError(t, common.UnmarshalJsonStr(frame, &last))
					for _, candidate := range last.Candidates {
						for _, part := range candidate.Content.Parts {
							text.WriteString(part.Text)
						}
						if candidate.FinishReason != nil {
							assert.Equal(t, "STOP", *candidate.FinishReason)
							stopCount++
						}
					}
				}
				assert.Equal(t, "hello", text.String())
				assert.Equal(t, 1, stopCount)
				assert.Len(t, frames, 3, "empty opening delta is suppressed; text, stop and usage are retained")
				assert.Empty(t, last.Candidates, "usage-only tail must not invent another finish event")
				assert.Equal(t, usage.PromptTokens, last.UsageMetadata.PromptTokenCount)
				assert.Equal(t, usage.CompletionTokens, last.UsageMetadata.CandidatesTokenCount)
				assert.Equal(t, usage.TotalTokens, last.UsageMetadata.TotalTokenCount)
				return
			}

			var stopCount, usageCount int
			for _, frame := range frames {
				var event dto.ClaudeResponse
				require.NoError(t, common.UnmarshalJsonStr(frame, &event))
				switch event.Type {
				case "message_delta":
					require.NotNil(t, event.Usage)
					assert.Equal(t, usage.PromptTokens, event.Usage.InputTokens)
					assert.Equal(t, usage.CompletionTokens, event.Usage.OutputTokens)
					usageCount++
				case "message_stop":
					stopCount++
				}
			}
			assert.Equal(t, 1, usageCount)
			assert.Equal(t, 1, stopCount)
			assert.Contains(t, recorder.Body.String(), `"text":"hello"`)
			var last dto.ClaudeResponse
			require.NoError(t, common.UnmarshalJsonStr(frames[len(frames)-1], &last))
			assert.Equal(t, "message_stop", last.Type)
		})
	}
}
