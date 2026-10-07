package controller

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch9CommittedResponseNeverRetries(t *testing.T) {
	for _, err := range []*types.NewAPIError{
		types.NewErrorWithStatusCode(errors.New("overload"), types.ErrorCodeBadResponse, 503),
		types.NewErrorWithStatusCode(errors.New("channel"), types.ErrorCodeChannelNoAvailableKey, 503),
		types.NewErrorWithStatusCode(errors.New("invalid status"), types.ErrorCodeBadResponse, 999),
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Writer.Flush()
		assert.False(t, shouldRetry(c, err, 3))
	}
}

func TestBatch9CommittedErrorUsesClientProtocolOnce(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini, types.RelayFormatOpenAIResponses} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			c.Header("Content-Type", "text/event-stream")
			c.Writer.WriteString(": ping\n\n")
			err := types.WithOpenAIError(types.OpenAIError{Code: "server_is_overloaded", Message: "overloaded"}, 503)
			emitCommittedRelayError(c, format, err)
			emitCommittedRelayError(c, format, err)
			body := recorder.Body.String()
			assert.Equal(t, 1, strings.Count(body, "data: "))
			assert.NotContains(t, body, "[DONE]")
			assert.NotContains(t, body, "response.completed")
			assert.Equal(t, 200, recorder.Code)
			lines := strings.Split(body, "\n")
			var data string
			for _, line := range lines {
				if strings.HasPrefix(line, "data: ") {
					data = strings.TrimPrefix(line, "data: ")
				}
			}
			var payload map[string]any
			require.NoError(t, common.Unmarshal([]byte(data), &payload))
			switch format {
			case types.RelayFormatOpenAIResponses:
				assert.Equal(t, "error", payload["type"])
				assert.Equal(t, "server_is_overloaded", payload["code"])
			case types.RelayFormatClaude:
				assert.Equal(t, "error", payload["type"])
				assert.Contains(t, body, "event: error")
			case types.RelayFormatGemini:
				assert.Equal(t, float64(503), payload["error"].(map[string]any)["code"])
			default:
				assert.Equal(t, "server_is_overloaded", payload["error"].(map[string]any)["code"])
			}
		})
	}
}

func TestBatch9CommittedBrokenOrCancelledConnectionCloses(t *testing.T) {
	for _, mode := range []string{"broken", "cancelled", "nonstream", "emitted"} {
		t.Run(mode, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			c.Header("Content-Type", "text/event-stream")
			c.Writer.WriteString(": ping\n\n")
			switch mode {
			case "broken":
				c.Set("relay_stream_write_failed", true)
			case "emitted":
				c.Set("relay_stream_error_emitted", true)
			case "cancelled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "nonstream":
				c.Header("Content-Type", "application/json")
			}
			before := recorder.Body.String()
			emitCommittedRelayError(c, types.RelayFormatOpenAI, types.WithOpenAIError(types.OpenAIError{Message: "private-sentinel"}, 502))
			assert.Equal(t, before, recorder.Body.String())
		})
	}
}

func TestBatch9ResponsesErrorCodeWireType(t *testing.T) {
	for _, code := range []any{float64(429), "provider_code", nil, map[string]any{"secret": "private-sentinel"}} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		c.Header("Content-Type", "text/event-stream")
		c.Writer.Flush()
		emitCommittedRelayError(c, types.RelayFormatOpenAIResponses, types.WithOpenAIError(types.OpenAIError{Code: code, Message: "failure"}, 502))
		line := strings.Split(recorder.Body.String(), "\n")[1]
		var payload map[string]any
		require.NoError(t, common.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload))
		switch value := code.(type) {
		case float64:
			assert.Equal(t, "429", payload["code"])
		case string:
			assert.Equal(t, value, payload["code"])
		default:
			assert.Nil(t, payload["code"])
		}
		assert.NotContains(t, recorder.Body.String(), "private-sentinel")
	}
}
