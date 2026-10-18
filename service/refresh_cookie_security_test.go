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

func TestRefreshCookieScopeAndDeletionMatchSecurityMode(t *testing.T) {
	previousSecure := common.SessionCookieSecure
	t.Cleanup(func() { common.SessionCookieSecure = previousSecure })
	for _, secure := range []bool{false, true} {
		name, path := "new_api_refresh", "/api/user/auth"
		if secure {
			name, path = "__Host-new_api_refresh", "/"
		}
		t.Run(name, func(t *testing.T) {
			common.SessionCookieSecure = secure
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			WriteRefreshCookie(c, "opaque-refresh-secret")
			cookies := recorder.Result().Cookies()
			require.NotEmpty(t, cookies)
			assert.Equal(t, name, cookies[0].Name)
			assert.Equal(t, path, cookies[0].Path)
			assert.Empty(t, cookies[0].Domain)
			assert.Equal(t, secure, cookies[0].Secure)
			assert.True(t, cookies[0].HttpOnly)
			assert.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
			assert.Positive(t, cookies[0].MaxAge)
			assert.Equal(t, "opaque-refresh-secret", cookies[0].Value)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/user/auth/refresh", nil)
			c.Request.AddCookie(cookies[0])
			value, err := ReadRefreshCookie(c)
			require.NoError(t, err)
			assert.Equal(t, "opaque-refresh-secret", value)
			c.Request.AddCookie(cookies[0])
			_, err = ReadRefreshCookie(c)
			assert.ErrorIs(t, err, http.ErrNoCookie, "ambiguous same-name cookies must be rejected")
			if secure {
				require.Len(t, cookies, 2)
				assert.Equal(t, "new_api_refresh", cookies[1].Name)
				assert.Equal(t, "/api/user/auth", cookies[1].Path)
				assert.Equal(t, -1, cookies[1].MaxAge, "secure login expires the old host-only cookie")
			} else {
				assert.Len(t, cookies, 1)
			}

			recorder = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(recorder)
			ClearRefreshCookie(c)
			cleared := recorder.Result().Cookies()
			require.Len(t, cleared, len(cookies))
			for index := range cleared {
				assert.Equal(t, cookies[index].Name, cleared[index].Name)
				assert.Equal(t, cookies[index].Path, cleared[index].Path)
				assert.Empty(t, cleared[index].Domain)
				assert.Equal(t, -1, cleared[index].MaxAge)
			}
		})
	}
}
