package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopiedUpstreamResponseCannotSetGatewayCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, cookieHeader, legacyCookieHeader string
	}{
		{"canonical headers", "Set-Cookie", "Set-Cookie2"},
		{"mixed case headers", "sEt-CoOkIe", "sEt-CoOkIe2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					tc.cookieHeader:       {"__Host-new_api_refresh=attacker-session; Path=/; Secure; HttpOnly"},
					tc.legacyCookieHeader: {"new_api_refresh=attacker-session; Path=/api/user/auth"},
					"Content-Type":        {"audio/mpeg"},
					"Content-Disposition": {`inline; filename="audio.mp3"`},
					"Content-Length":      {"999"},
					common.RequestIdKey:   {"upstream-id"},
				},
			}
			router := gin.New()
			router.GET("/audio", func(c *gin.Context) {
				c.Header(common.RequestIdKey, "local-id")
				IOCopyBytesGracefully(c, upstream, []byte("audio"))
				assert.Equal(t, "upstream-id", c.GetString(common.UpstreamRequestIdKey))
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/audio", nil))

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "audio", recorder.Body.String())
			assert.Empty(t, recorder.Header().Values("Set-Cookie"))
			assert.Empty(t, recorder.Header().Values("Set-Cookie2"))
			assert.Empty(t, recorder.Result().Cookies())
			assert.Equal(t, "audio/mpeg", recorder.Header().Get("Content-Type"))
			assert.Equal(t, `inline; filename="audio.mp3"`, recorder.Header().Get("Content-Disposition"))
			assert.Equal(t, "5", recorder.Header().Get("Content-Length"))
			assert.Equal(t, "local-id", recorder.Header().Get(common.RequestIdKey))
		})
	}
}
