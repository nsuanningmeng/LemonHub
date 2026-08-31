package oauth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type oauthRoundTripFunc func(*http.Request) (*http.Response, error)

func (f oauthRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBuiltInOAuthLogsDoNotExposeCredentialsOrPII(t *testing.T) {
	const (
		authorizationCode = "builtin-authorization-code-secret-marker"
		accessToken       = "builtin-access-token-secret-marker"
		refreshToken      = "builtin-refresh-token-secret-marker"
		idToken           = "builtin-id-token-secret-marker"
		tokenScope        = "builtin-scope-secret-marker"
		providerUserID    = "builtin-provider-user-secret-marker"
		username          = "builtin-username-pii-marker"
		displayName       = "builtin-display-name-pii-marker"
		email             = "builtin-email-pii-marker@example.test"
		githubErrorBody   = "github-userinfo-error-secret-marker"
		linuxTokenMessage = "linux-token-error-secret-marker"
		clientSecret      = "builtin-client-secret-marker"
		endpointUser      = "builtin-endpoint-user-secret-marker"
		endpointPassword  = "builtin-endpoint-password-secret-marker"
		endpointQuery     = "builtin-endpoint-query-secret-marker"
		transportError    = "builtin-transport-error-secret-marker"
	)

	var githubUserError, linuxTokenError, transportFailure bool
	previousTransport := http.DefaultTransport
	http.DefaultTransport = oauthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if transportFailure {
			return nil, errors.New(transportError)
		}
		status := http.StatusOK
		body := "{}"
		switch req.URL.Host + req.URL.Path {
		case "github.com/login/oauth/access_token":
			body = `{"access_token":"` + accessToken + `","scope":"` + tokenScope + `","token_type":"Bearer"}`
		case "api.github.com/user":
			if githubUserError {
				status = http.StatusUnauthorized
				body = `{"message":"` + githubErrorBody + `"}`
			} else {
				body = `{"id":101,"login":"` + username + `","name":"` + displayName + `","email":"` + email + `"}`
			}
		case "discord.com/api/v10/oauth2/token":
			body = `{"access_token":"` + accessToken + `","refresh_token":"` + refreshToken +
				`","id_token":"` + idToken + `","token_type":"Bearer","scope":"` + tokenScope + `"}`
		case "discord.com/api/v10/users/@me":
			body = `{"id":"` + providerUserID + `","username":"` + username + `","global_name":"` + displayName + `"}`
		case "oidc.example.test/token":
			body = `{"access_token":"` + accessToken + `","refresh_token":"` + refreshToken +
				`","id_token":"` + idToken + `","token_type":"Bearer","scope":"` + tokenScope + `"}`
		case "oidc.example.test/userinfo":
			body = `{"sub":"` + providerUserID + `","preferred_username":"` + username +
				`","name":"` + displayName + `","email":"` + email + `"}`
		case "linuxdo.example.test/token":
			if linuxTokenError {
				body = `{"message":"` + linuxTokenMessage + `"}`
			} else {
				body = `{"access_token":"` + accessToken + `"}`
			}
		case "linuxdo.example.test/user":
			body = `{"id":202,"username":"` + username + `","name":"` + displayName +
				`","active":true,"trust_level":3,"silenced":false}`
		default:
			status = http.StatusNotFound
		}
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	previousGitHubClientID := common.GitHubClientId
	previousGitHubClientSecret := common.GitHubClientSecret
	previousLinuxClientID := common.LinuxDOClientId
	previousLinuxClientSecret := common.LinuxDOClientSecret
	previousLinuxTrustLevel := common.LinuxDOMinimumTrustLevel
	common.GitHubClientId = "github-client-id-marker"
	common.GitHubClientSecret = clientSecret
	common.LinuxDOClientId = "linux-client-id-marker"
	common.LinuxDOClientSecret = clientSecret
	common.LinuxDOMinimumTrustLevel = 1
	t.Cleanup(func() {
		common.GitHubClientId = previousGitHubClientID
		common.GitHubClientSecret = previousGitHubClientSecret
		common.LinuxDOClientId = previousLinuxClientID
		common.LinuxDOClientSecret = previousLinuxClientSecret
		common.LinuxDOMinimumTrustLevel = previousLinuxTrustLevel
	})

	discordSettings := system_setting.GetDiscordSettings()
	previousDiscordSettings := *discordSettings
	*discordSettings = system_setting.DiscordSettings{Enabled: true, ClientId: "discord-client-id-marker", ClientSecret: clientSecret}
	oidcSettings := system_setting.GetOIDCSettings()
	previousOIDCSettings := *oidcSettings
	*oidcSettings = system_setting.OIDCSettings{
		Enabled:          true,
		ClientId:         "oidc-client-id-marker",
		ClientSecret:     clientSecret,
		TokenEndpoint:    "https://" + endpointUser + ":" + endpointPassword + "@oidc.example.test/token?api_key=" + endpointQuery,
		UserInfoEndpoint: "https://" + endpointUser + ":" + endpointPassword + "@oidc.example.test/userinfo?api_key=" + endpointQuery,
	}
	previousServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://app.example.test"
	t.Cleanup(func() {
		*discordSettings = previousDiscordSettings
		*oidcSettings = previousOIDCSettings
		system_setting.ServerAddress = previousServerAddress
	})
	t.Setenv("LINUX_DO_TOKEN_ENDPOINT", "https://"+endpointUser+":"+endpointPassword+"@linuxdo.example.test/token?api_key="+endpointQuery)
	t.Setenv("LINUX_DO_USER_ENDPOINT", "https://"+endpointUser+":"+endpointPassword+"@linuxdo.example.test/user?api_key="+endpointQuery)

	var logs bytes.Buffer
	previousDebug := common.DebugEnabled
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultWriter
	previousErrorWriter := gin.DefaultErrorWriter
	gin.DefaultWriter = &logs
	gin.DefaultErrorWriter = &logs
	common.DebugEnabled = true
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		common.DebugEnabled = previousDebug
		gin.DefaultWriter = previousWriter
		gin.DefaultErrorWriter = previousErrorWriter
		common.LogWriterMu.Unlock()
	})

	ctx := context.Background()
	providers := []Provider{
		&GitHubProvider{},
		&DiscordProvider{},
		&OIDCProvider{},
	}
	for _, provider := range providers {
		token, err := provider.ExchangeToken(ctx, authorizationCode, nil)
		require.NoError(t, err)
		require.Equal(t, accessToken, token.AccessToken)
		user, err := provider.GetUserInfo(ctx, token)
		require.NoError(t, err)
		require.NotEmpty(t, user.ProviderUserID)
	}

	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodGet, "https://app.example.test/api/oauth/linuxdo", nil)
	linuxProvider := &LinuxDOProvider{}
	linuxToken, err := linuxProvider.ExchangeToken(ctx, authorizationCode, ginContext)
	require.NoError(t, err)
	_, err = linuxProvider.GetUserInfo(ctx, linuxToken)
	require.NoError(t, err)

	githubUserError = true
	_, err = (&GitHubProvider{}).GetUserInfo(ctx, &OAuthToken{AccessToken: accessToken})
	require.Error(t, err)
	linuxTokenError = true
	_, err = linuxProvider.ExchangeToken(ctx, authorizationCode, ginContext)
	require.Error(t, err)
	var oauthErr *OAuthError
	require.ErrorAs(t, err, &oauthErr)
	assert.Empty(t, oauthErr.RawError)
	assert.NotContains(t, err.Error(), linuxTokenMessage)

	transportFailure = true
	for _, provider := range providers {
		_, err = provider.ExchangeToken(ctx, authorizationCode, nil)
		require.Error(t, err)
		_, err = provider.GetUserInfo(ctx, &OAuthToken{AccessToken: accessToken})
		require.Error(t, err)
	}
	_, err = linuxProvider.ExchangeToken(ctx, authorizationCode, ginContext)
	require.Error(t, err)
	_, err = linuxProvider.GetUserInfo(ctx, &OAuthToken{AccessToken: accessToken})
	require.Error(t, err)

	output := logs.String()
	for _, safeMarker := range []string{
		"[OAuth-GitHub] GetUserInfo success",
		"[OAuth-Discord] GetUserInfo success",
		"[OAuth-OIDC] GetUserInfo success",
		"[OAuth-LinuxDO] GetUserInfo success",
		"[OAuth-GitHub] GetUserInfo failed: status=401",
	} {
		assert.Contains(t, output, safeMarker)
	}
	for _, sensitive := range []string{
		authorizationCode,
		accessToken,
		refreshToken,
		idToken,
		tokenScope,
		providerUserID,
		username,
		displayName,
		email,
		githubErrorBody,
		linuxTokenMessage,
		clientSecret,
		endpointUser,
		endpointPassword,
		endpointQuery,
		transportError,
	} {
		assert.NotContainsf(t, output, sensitive, "OAuth log exposed sensitive marker %q", sensitive)
	}
}
