package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failedDistributorBody struct{ err error }

func (b failedDistributorBody) Read([]byte) (int, error) { return 0, b.err }
func (b failedDistributorBody) Close() error             { return nil }

func TestDistributorPreservesRequestBodyFailureAndStatus(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, tc := range []struct {
		name   string
		body   io.ReadCloser
		status int
		cause  string
	}{
		{"too large", failedDistributorBody{common.ErrRequestBodyTooLarge}, http.StatusRequestEntityTooLarge, common.ErrRequestBodyTooLarge.Error()},
		{"interrupted upload", failedDistributorBody{errors.New("upload interrupted")}, http.StatusBadRequest, "upload interrupted"},
		{"invalid JSON", io.NopCloser(strings.NewReader("{")), http.StatusBadRequest, "invalid JSON request body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/v1/chat/completions", Distribute(), func(c *gin.Context) { t.Error("invalid body reached relay") })
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			request.Body = tc.body
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code)
			assert.Contains(t, response.Body.String(), tc.cause)
			assert.Equal(t, 1, strings.Count(response.Body.String(), tc.cause))
		})
	}
}
