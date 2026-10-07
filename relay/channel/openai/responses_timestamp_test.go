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
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesFloatTimestampStreamPreservesSnapshotsAndUsage(t *testing.T) {
	events := []string{
		`{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","model":"private-model","created_at":1786588600.0,"status":"in_progress"}}`,
		`{"type":"response.in_progress","sequence_number":1,"response":{"id":"resp_1","model":"private-model","created_at":1786588600.0,"status":"in_progress"}}`,
		`{"type":"response.output_text.delta","sequence_number":2,"delta":"generated output"}`,
		`{"type":"response.completed","sequence_number":3,"response":{"id":"resp_1","model":"private-model","created_at":1786588600.0,"status":"completed","usage":{"input_tokens":12,"output_tokens":48,"total_tokens":60}}}`,
	}
	usage, _, _, body := runResponsesUsageStream(t, events...)
	assert.Equal(t, 12, usage.PromptTokens)
	assert.Equal(t, 48, usage.CompletionTokens)
	assert.Equal(t, 60, usage.TotalTokens)
	require.NotNil(t, usage.BillingUsage)
	assert.False(t, usage.BillingUsage.Estimated, "terminal upstream usage must replace local estimates")
	assert.Equal(t, dto.BillingUsageSourceOAIResponses, usage.BillingUsage.Source)
	previous := -1
	for _, event := range events {
		// The required model rewrite must not normalize the timestamp or drop
		// unknown fields such as sequence_number from the upstream wire payload.
		expected := "data: " + strings.ReplaceAll(event, "private-model", "public-model")
		index := strings.Index(body, expected)
		require.NotEqual(t, -1, index, "missing original event: %s", expected)
		assert.Greater(t, index, previous)
		previous = index
	}
	assert.Equal(t, 3, strings.Count(body, `"created_at":1786588600.0`))
}

func TestResponsesFloatTimestampNonStreamPreservesBytesAndUsage(t *testing.T) {
	const upstream = `{"id":"resp_1","model":"private-model","created_at":1.7865886009e9,"status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":48,"total_tokens":60}}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "private-model", IsModelMapped: true}}
	info.SetEstimatePromptTokens(37)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(upstream))}
	usage, apiErr := OaiResponsesHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 12, usage.PromptTokens)
	assert.Equal(t, 48, usage.CompletionTokens)
	assert.Equal(t, 60, usage.TotalTokens)
	assert.Equal(t, strings.ReplaceAll(upstream, "private-model", "public-model"), recorder.Body.String())
}

func TestResponsesFloatTimestampConvertsToChat(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream", true: "stream"}[stream], func(t *testing.T) {
			body := `{"id":"resp_1","created_at":1786588600.9,"model":"gpt-test","status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":48,"total_tokens":60}}`
			if stream {
				body = "data: " + `{"type":"response.created","response":{"id":"resp_1","created_at":1786588600.9,"model":"gpt-test","status":"in_progress"}}` + "\n\n" +
					"data: " + `{"type":"response.completed","response":` + body + "}\n\n"
			}
			c, recorder, resp, info := newResponsesChatTestContext(t, body, stream)
			handler := OaiResponsesToChatHandler
			if stream {
				handler = OaiResponsesToChatStreamHandler
			}
			usage, apiErr := handler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 12, usage.PromptTokens)
			assert.Equal(t, 48, usage.CompletionTokens)
			assert.Equal(t, 60, usage.TotalTokens)
			var payloads []string
			if stream {
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if strings.HasPrefix(line, "data: {") {
						payloads = append(payloads, strings.TrimPrefix(line, "data: "))
					}
				}
			} else {
				payloads = []string{recorder.Body.String()}
			}
			require.NotEmpty(t, payloads)
			convertedChunks := 0
			for _, payload := range payloads {
				var chat struct {
					Created int64      `json:"created"`
					Choices []any      `json:"choices"`
					Usage   *dto.Usage `json:"usage"`
				}
				require.NoError(t, common.Unmarshal([]byte(payload), &chat))
				if stream && len(chat.Choices) == 0 {
					// The host's additional usage-only frame retains its existing
					// local timestamp; it is not an upstream response conversion.
					require.NotNil(t, chat.Usage)
					assert.Equal(t, 60, chat.Usage.TotalTokens)
					continue
				}
				assert.Equal(t, int64(1786588600), chat.Created)
				convertedChunks++
			}
			assert.Positive(t, convertedChunks)
		})
	}
}
