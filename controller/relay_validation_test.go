package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayInvalidRequestReturnsBadRequest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format types.RelayFormat
		path   string
		body   string
	}{
		{name: "chat invalid token count", format: types.RelayFormatOpenAI, path: "/v1/chat/completions", body: `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"max_tokens":-1}`},
		{name: "responses missing input", format: types.RelayFormatOpenAIResponses, path: "/v1/responses", body: `{"model":"gpt-4o"}`},
		{name: "claude malformed JSON", format: types.RelayFormatClaude, path: "/v1/messages", body: `{`},
		{name: "gemini malformed JSON", format: types.RelayFormatGemini, path: "/v1beta/models/gemini-2.5-flash:generateContent", body: `{`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(c) })

			// No user, channel, or billing fixture is needed: invalid input must
			// return before any routing or quota access.
			Relay(c, tc.format)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			var response struct {
				Type  string `json:"type"`
				Error struct {
					Message string `json:"message"`
					Code    string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.NotEmpty(t, response.Error.Message)
			if tc.format == types.RelayFormatClaude {
				assert.Equal(t, "error", response.Type)
			} else {
				assert.Equal(t, string(types.ErrorCodeInvalidRequest), response.Error.Code)
			}
		})
	}
}

func TestRelayOversizedRequestRemainsEntityTooLarge(t *testing.T) {
	previous := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previous })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat(" ", (1<<20)+1)))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	Relay(c, types.RelayFormatOpenAI)
	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	var response struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, string(types.ErrorCodeReadRequestBodyFailed), response.Error.Code)
}
