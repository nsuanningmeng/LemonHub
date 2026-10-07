package claude

import (
	"errors"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClaudeResponsesPartialEOFProducesSingleFailedTerminal(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "public-model"}, StartTime: time.Now()}
	payload := `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"public-model","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}` + "\n" + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`
	var wire strings.Builder
	for _, data := range strings.Split(payload, "\n") {
		wire.WriteString("data: " + data + "\n\n")
	}
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(wire.String()))}
	usage, err := ClaudeResponsesStreamHandler(c, resp, info)
	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Contains(t, w.Body.String(), "partial")
	require.Equal(t, 1, strings.Count(w.Body.String(), "event: response.failed\n"))
	require.NotContains(t, w.Body.String(), "event: response.completed\n")
	require.True(t, c.GetBool("relay_stream_error_emitted"))
}

type claudeQueuedResetBody struct {
	data  *strings.Reader
	reset chan struct{}
}

func (b *claudeQueuedResetBody) Read(p []byte) (int, error) {
	if b.data.Len() > 0 {
		return b.data.Read(p)
	}
	return 0, errors.New("private_source_reset")
}
func (b *claudeQueuedResetBody) Close() error {
	select {
	case <-b.reset:
	default:
		close(b.reset)
	}
	return nil
}

type claudeQueuedResetWriter struct {
	*httptest.ResponseRecorder
	reset chan struct{}
	once  sync.Once
}

func (w *claudeQueuedResetWriter) Flush() {
	w.once.Do(func() { <-w.reset })
	w.ResponseRecorder.Flush()
}

func TestClaudeResponsesQueuedMessageStopSurvivesTrailingReset(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	reset := make(chan struct{})
	w := &claudeQueuedResetWriter{ResponseRecorder: httptest.NewRecorder(), reset: reset}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "public-model"}, StartTime: time.Now()}
	var wire strings.Builder
	for _, data := range []string{
		`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"public-model","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"complete"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	} {
		wire.WriteString("data: " + data + "\n\n")
	}
	body := &claudeQueuedResetBody{data: strings.NewReader(wire.String()), reset: reset}
	usage, err := ClaudeResponsesStreamHandler(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, info)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 1, strings.Count(w.Body.String(), "event: response.completed\n"))
	require.NotContains(t, w.Body.String(), "event: response.failed\n")
	require.NotContains(t, w.Body.String(), "private_source_reset")
}
