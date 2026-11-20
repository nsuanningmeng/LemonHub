package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccessAndPrivateRecoveryLogsFollowRotation(t *testing.T) {
	before, err := os.Create(filepath.Join(t.TempDir(), "before.log"))
	require.NoError(t, err)
	after, err := os.Create(filepath.Join(t.TempDir(), "after.log"))
	require.NoError(t, err)
	common.LogWriterMu.Lock()
	oldWriter, oldErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = before, before
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldWriter, oldErrorWriter
		common.LogWriterMu.Unlock()
		_ = before.Close()
		_ = after.Close()
	})

	router := gin.New()
	router.Use(RelayPanicRecover())
	SetUpLogger(router)
	router.GET("/before", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/after/:secret", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/panic", func(c *gin.Context) { panic("private-panic-value") })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/before", nil))

	common.LogWriterMu.Lock()
	gin.DefaultWriter, gin.DefaultErrorWriter = after, after
	closeErr := before.Close()
	common.LogWriterMu.Unlock()
	require.NoError(t, closeErr)
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/after/private-path?key=private-query", nil))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "private-panic-value")

	oldContent, err := os.ReadFile(before.Name())
	require.NoError(t, err)
	newContent, err := os.ReadFile(after.Name())
	require.NoError(t, err)
	assert.Contains(t, string(oldContent), "GET /before")
	assert.NotContains(t, string(oldContent), "GET /after")
	assert.Contains(t, string(newContent), "GET /after/:secret")
	assert.Contains(t, string(newContent), "panic recovered (string); stacktrace:")
	assert.Contains(t, string(newContent), "middleware/logger_rotation_test.go")
	for _, secret := range []string{"private-path", "private-query", "private-panic-value"} {
		assert.NotContains(t, string(newContent), secret)
	}
}
