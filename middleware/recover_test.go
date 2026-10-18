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

func TestRelayPanicRecoverDoesNotExposePanicOrRequestSecrets(t *testing.T) {
	var logs bytes.Buffer
	oldWriter, oldErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	common.LogWriterMu.Lock()
	gin.DefaultWriter, gin.DefaultErrorWriter = &logs, &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldWriter, oldErrorWriter
		common.LogWriterMu.Unlock()
	})
	engine := gin.New()
	engine.Use(RelayPanicRecover())
	engine.GET("/panic", func(c *gin.Context) {
		c.Set(common.RequestIdKey, "panic-request-correlation")
		panic("panic-secret-sentinel")
	})
	request := httptest.NewRequest(http.MethodGet, "/panic?api_key=query-secret-sentinel", nil)
	request.Header.Set("Authorization", "Bearer authorization-secret-sentinel")
	request.Header.Set("Cookie", "session=cookie-secret-sentinel")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.JSONEq(t, `{"error":{"message":"An internal server error occurred.","type":"new_api_panic"}}`, response.Body.String())
	for _, secret := range []string{"panic-secret-sentinel", "query-secret-sentinel", "authorization-secret-sentinel", "cookie-secret-sentinel"} {
		assert.NotContains(t, response.Body.String(), secret)
		assert.NotContains(t, logs.String(), secret)
	}
	assert.Contains(t, logs.String(), "panic-request-correlation")
}
