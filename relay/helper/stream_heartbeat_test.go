package helper

import (
	"errors"
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

func TestStreamCommentsFollowValidatedWrittenData(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	raw := ": PRIVATE_BEFORE\n\ndata: {\"value\":\"first\"}\n\n: PRIVATE_AFTER\n\ndata: {\"value\":\"second\"}\n\ndata: [DONE]\n\n"
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	var received []string
	StreamScannerHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(raw))}, info, func(data string, sr *StreamResult) {
		received = append(received, data)
		if err := StringData(c, data); err != nil {
			sr.Stop(err)
		}
	})
	require.Len(t, received, 2)
	assert.Equal(t, 2, info.ReceivedResponseCount)
	assert.Equal(t, "data: {\"value\":\"first\"}\n\n: keep-alive\n\ndata: {\"value\":\"second\"}\n\n", recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "PRIVATE")
}
func TestPreDataCommentCannotCommitRejectedFirstFrame(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	StreamScannerHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(": PRIVATE\n\ndata: {\"error\":{\"code\":503}}\n\n: AFTER_REJECTED\n\n"))}, info, func(_ string, sr *StreamResult) { sr.Stop(errors.New("validated upstream error")) })
	assert.False(t, c.Writer.Written())
	assert.Empty(t, recorder.Body.String())
}

func TestStreamReadinessRequiresSuccessfulBusinessWrite(t *testing.T) {
	for _, kind := range []string{"headers", "ping", "done", "empty", "failed", "terminal", "business"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			switch kind {
			case "headers":
				SetEventStreamHeaders(c)
				require.NoError(t, FlushWriter(c))
			case "ping":
				require.NoError(t, PingData(c))
			case "done":
				require.NoError(t, Done(c))
			case "empty":
				require.NoError(t, StringData(c, ""))
			case "failed":
				common.SetContextKey(c, constant.ContextKeyResponseFailed, true)
				require.NoError(t, ResponseChunkData(c, dto.ResponsesStreamResponse{}, `{"error":{"code":503}}`))
			case "terminal":
				require.NoError(t, ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.done"}, `{"type":"response.done"}`))
			case "business":
				require.NoError(t, StringData(c, `{"value":"valid"}`))
			}
			assert.Equal(t, kind == "business", StreamDataWritten(c))
		})
	}
}
