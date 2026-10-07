package controller

import (
	"bytes"
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClaudeStopSequencesActualRelayWire(t *testing.T) {
	for _, stops := range []string{`["one"]`, `["one","two"]`, `[]`, `null`, `missing`} {
		t.Run(stops, func(t *testing.T) {
			_, user, token, _ := cancellationHostFixture(t, "openai")
			observed := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				observed <- body
				assert.Equal(t, "/v1/chat/completions", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"strict-stop","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
			}))
			defer server.Close()
			channel := model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-4o", Key: "local-stop-key", Group: "default", BaseURL: &server.URL}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			raw := `{"model":"gpt-4o","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
			if stops != "missing" {
				raw = raw[:len(raw)-1] + `,"stop_sequences":` + stops + `}`
			}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(raw))
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
			common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
			common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
			common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
			common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
			c.Set("token_quota", token.RemainQuota)
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-4o"))
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			Relay(c, types.RelayFormatClaude)
			require.Equal(t, 200, recorder.Code, recorder.Body.String())
			var wire map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(<-observed, &wire))
			if stops == `[]` || stops == `null` || stops == "missing" {
				assert.NotContains(t, wire, "stop")
			} else {
				assert.JSONEq(t, stops, string(wire["stop"]))
			}
			waitCancellationHostWork(t)
		})
	}
}
