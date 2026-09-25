package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONLoginOriginGuard(t *testing.T) {
	previousSecure, previousTrustedURLs := common.SessionCookieSecure, common.SessionCookieTrustedURLs
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		common.SessionCookieSecure, common.SessionCookieTrustedURLs = previousSecure, previousTrustedURLs
		gin.SetMode(previousGinMode)
	})

	tests := []struct {
		name           string
		requestURL     string
		contentTypes   []string
		origins        []string
		referers       []string
		secure         bool
		trustedOrigins []string
		forwarded      bool
		wantStatus     int
		wantCode       string
	}{
		{
			name:         "same origin JSON with charset",
			contentTypes: []string{"application/json; charset=utf-8"}, origins: []string{"https://panel.example.com"},
			secure: true, wantStatus: http.StatusNoContent,
		},
		{
			name:         "originless JSON command line client",
			contentTypes: []string{"application/json"}, secure: true, wantStatus: http.StatusNoContent,
		},
		{
			name:   "missing content type",
			secure: true, wantStatus: http.StatusUnsupportedMediaType, wantCode: "AUTH_JSON_REQUIRED",
		},
		{
			name:         "empty content type",
			contentTypes: []string{""},
			secure:       true, wantStatus: http.StatusUnsupportedMediaType, wantCode: "AUTH_JSON_REQUIRED",
		},
		{
			name:         "duplicate content type",
			contentTypes: []string{"application/json", "application/json"},
			secure:       true, wantStatus: http.StatusUnsupportedMediaType, wantCode: "AUTH_JSON_REQUIRED",
		},
		{
			name:         "malformed media type parameter",
			contentTypes: []string{"application/json; charset"},
			secure:       true, wantStatus: http.StatusUnsupportedMediaType, wantCode: "AUTH_JSON_REQUIRED",
		},
		{
			name:         "cross site text plain form",
			contentTypes: []string{"text/plain"}, origins: []string{"https://attacker.example"},
			secure: true, wantStatus: http.StatusUnsupportedMediaType, wantCode: "AUTH_JSON_REQUIRED",
		},
		{
			name:         "urlencoded form in local development",
			requestURL:   "http://localhost:3000/api/user/login",
			contentTypes: []string{"application/x-www-form-urlencoded"}, origins: []string{"http://localhost:5173"},
			wantStatus: http.StatusUnsupportedMediaType, wantCode: "AUTH_JSON_REQUIRED",
		},
		{
			name:         "multipart form",
			contentTypes: []string{"multipart/form-data; boundary=login"},
			secure:       true, wantStatus: http.StatusUnsupportedMediaType, wantCode: "AUTH_JSON_REQUIRED",
		},
		{
			name:         "media type prefix is insufficient",
			contentTypes: []string{"application/jsonp"},
			secure:       true, wantStatus: http.StatusUnsupportedMediaType, wantCode: "AUTH_JSON_REQUIRED",
		},
		{
			name:         "foreign JSON origin",
			contentTypes: []string{"application/json"}, origins: []string{"https://attacker.example"},
			secure: true, wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "null origin cannot fall back to trusted referer",
			contentTypes: []string{"application/json"}, origins: []string{"null"}, referers: []string{"https://panel.example.com/sign-in"},
			secure: true, wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "empty explicit origin is not a command line request",
			contentTypes: []string{"application/json"}, origins: []string{""},
			secure: true, wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "malformed origin in local mode",
			requestURL:   "http://localhost:3000/api/user/login",
			contentTypes: []string{"application/json"}, origins: []string{"http://localhost:5173/sign-in"},
			wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "duplicate same origin headers",
			contentTypes: []string{"application/json"}, origins: []string{"https://panel.example.com", "https://panel.example.com"},
			secure: true, wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "comma combined origins",
			contentTypes: []string{"application/json"}, origins: []string{"https://panel.example.com, https://attacker.example"},
			secure: true, wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "same origin referer fallback",
			contentTypes: []string{"application/json"}, referers: []string{"https://panel.example.com/sign-in?redirect=dashboard"},
			secure: true, wantStatus: http.StatusNoContent,
		},
		{
			name:         "foreign referer is not a command line request",
			contentTypes: []string{"application/json"}, referers: []string{"https://attacker.example/form"},
			secure: true, wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "duplicate referer headers",
			contentTypes: []string{"application/json"}, referers: []string{"https://panel.example.com/a", "https://panel.example.com/b"},
			secure: true, wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "HTTP loopback development proxy across host and port",
			requestURL:   "http://127.0.0.1:3000/api/user/login",
			contentTypes: []string{"application/json"}, origins: []string{"http://localhost:5173"},
			wantStatus: http.StatusNoContent,
		},
		{
			name:         "IPv6 HTTP loopback development proxy",
			requestURL:   "http://[::1]:3000/api/user/login",
			contentTypes: []string{"application/json"}, origins: []string{"http://localhost:5173"},
			wantStatus: http.StatusNoContent,
		},
		{
			name:         "secure mode has no loopback cross port exception",
			requestURL:   "http://localhost:3000/api/user/login",
			contentTypes: []string{"application/json"}, origins: []string{"http://localhost:5173"},
			secure: true, wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "loopback exception requires HTTP at both ends",
			requestURL:   "http://localhost:3000/api/user/login",
			contentTypes: []string{"application/json"}, origins: []string{"https://localhost:5173"},
			wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "local mode does not trust remote development origin",
			requestURL:   "http://localhost:3000/api/user/login",
			contentTypes: []string{"application/json"}, origins: []string{"http://localhost.attacker.example:5173"},
			wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "HTTPS reverse proxy uses configured exact public origin",
			requestURL:   "http://internal:3000/api/user/login",
			contentTypes: []string{"application/json"}, origins: []string{"https://panel.example.com"},
			trustedOrigins: []string{"https://panel.example.com"}, secure: true, wantStatus: http.StatusNoContent,
		},
		{
			name:         "forwarded headers cannot extend origin trust",
			requestURL:   "http://internal:3000/api/user/login",
			contentTypes: []string{"application/json"}, origins: []string{"https://panel.example.com"},
			forwarded: true, secure: true, wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
		{
			name:         "trusted origin suffix is not trusted",
			contentTypes: []string{"application/json"}, origins: []string{"https://trusted.example.com.attacker.example"},
			trustedOrigins: []string{"https://trusted.example.com"}, secure: true,
			wantStatus: http.StatusForbidden, wantCode: "AUTH_ORIGIN_FORBIDDEN",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			common.SessionCookieSecure, common.SessionCookieTrustedURLs = test.secure, test.trustedOrigins
			router := gin.New()
			router.POST("/api/user/login", JSONLoginOriginGuard(), func(c *gin.Context) {
				http.SetCookie(c.Writer, &http.Cookie{Name: "login-reached", Value: "yes"})
				c.Status(http.StatusNoContent)
			})
			requestURL := test.requestURL
			if requestURL == "" {
				requestURL = "https://panel.example.com/api/user/login"
			}
			request := httptest.NewRequest(http.MethodPost, requestURL, strings.NewReader(`{"username":"owner","password":"test"}`))
			for _, contentType := range test.contentTypes {
				request.Header.Add("Content-Type", contentType)
			}
			for _, origin := range test.origins {
				request.Header.Add("Origin", origin)
			}
			for _, referer := range test.referers {
				request.Header.Add("Referer", referer)
			}
			if test.forwarded {
				request.Header.Set("X-Forwarded-Proto", "https")
				request.Header.Set("X-Forwarded-Host", "panel.example.com")
				request.Header.Set("Forwarded", "proto=https;host=panel.example.com")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			assert.Equal(t, test.wantStatus, response.Code, response.Body.String())
			assert.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
			if test.wantCode == "" {
				require.Len(t, response.Result().Cookies(), 1)
				assert.Equal(t, "login-reached", response.Result().Cookies()[0].Name)
				return
			}
			assert.Empty(t, response.Result().Cookies(), "rejected requests must not reach the login handler")
			var result struct {
				Code string `json:"code"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.Equal(t, test.wantCode, result.Code)
		})
	}
}
