package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAccessLoggerRedactsRequestSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultWriter
	gin.DefaultWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = previousWriter
		common.LogWriterMu.Unlock()
	})

	server := gin.New()
	SetUpLogger(server)
	server.GET("/privacy-check", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	server.GET("/oauth/telegram/bind/:flow_token", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/privacy-check?private_key=private-query-secret&code=oauth-code&sign=webhook-signature",
		nil,
	)
	server.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Contains(t, logs.String(), "GET /privacy-check")
	assert.NotContains(t, logs.String(), "private-query-secret")
	assert.NotContains(t, logs.String(), "oauth-code")
	assert.NotContains(t, logs.String(), "webhook-signature")
	assert.NotContains(t, logs.String(), "private_key=")

	telegramRecorder := httptest.NewRecorder()
	telegramRequest := httptest.NewRequest(
		http.MethodGet,
		"/oauth/telegram/bind/private-flow-token?hash=telegram-callback-hash",
		nil,
	)
	server.ServeHTTP(telegramRecorder, telegramRequest)

	assert.Equal(t, http.StatusNoContent, telegramRecorder.Code)
	assert.Contains(t, logs.String(), "GET /oauth/telegram/bind/:flow_token")
	assert.NotContains(t, logs.String(), "private-flow-token")
	assert.NotContains(t, logs.String(), "telegram-callback-hash")
}
