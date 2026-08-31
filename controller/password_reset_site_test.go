package controller

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordResetVerificationKeyIsNormalizedAndSiteScoped(t *testing.T) {
	assert.Equal(t, passwordResetVerificationKey(" User@Example.COM ", 7), passwordResetVerificationKey("user@example.com", 7))
	assert.NotEqual(t, passwordResetVerificationKey("user@example.com", 7), passwordResetVerificationKey("user@example.com", 8))
}

func TestPasswordResetLinkReturnsToTrustedRequestSite(t *testing.T) {
	constant.SetTrustedRedirectDomains([]string{"tenant.example.com"})
	t.Cleanup(func() { constant.SetTrustedRedirectDomains(nil) })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "https://tenant.example.com/api/reset", nil)
	c.Request.Host = "tenant.example.com"
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	link := passwordResetLink(c, " User+tag@Example.COM ", "token/value")
	parsed, err := url.Parse(link)
	require.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "tenant.example.com", parsed.Host)
	assert.Equal(t, "/user/reset", parsed.Path)
	assert.Equal(t, "user+tag@example.com", parsed.Query().Get("email"))
	assert.Equal(t, "token/value", parsed.Query().Get("token"))
}
