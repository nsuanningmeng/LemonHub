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
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenericOAuthDebugLogsDoNotExposeCredentialsOrPII(t *testing.T) {
	const (
		authorizationCode = "authorization-code-secret-marker"
		accessToken       = "access-token-secret-marker"
		refreshToken      = "refresh-token-secret-marker"
		idToken           = "id-token-secret-marker"
		tokenScope        = "scope-secret-marker"
		oauthErrorCode    = "oauth-error-secret-marker"
		oauthErrorDesc    = "oauth-error-description-secret-marker"
		providerUserID    = "provider-user-secret-marker"
		username          = "username-pii-marker"
		displayName       = "display-name-pii-marker"
		email             = "email-pii-marker@example.test"
		policyExpected    = "allowed-email-policy-marker@example.test"
		endpointUser      = "endpoint-user-secret-marker"
		endpointPassword  = "endpoint-password-secret-marker"
		endpointQuery     = "endpoint-query-secret-marker"
		transportError    = "transport-error-secret-marker"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = io.WriteString(w, `{"access_token":"`+accessToken+`","refresh_token":"`+
				refreshToken+`","id_token":"`+idToken+`","token_type":"Bearer","scope":"`+tokenScope+`"}`)
		case "/token-error":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"`+oauthErrorCode+`","error_description":"`+oauthErrorDesc+`"}`)
		case "/userinfo":
			assert.Equal(t, "Bearer "+accessToken, r.Header.Get("Authorization"))
			_, _ = io.WriteString(w, `{"sub":"`+providerUserID+`","preferred_username":"`+
				username+`","name":"`+displayName+`","email":"`+email+`"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	previousTransport := http.DefaultTransport
	http.DefaultTransport = oauthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "transport.example.test" {
			return nil, errors.New(transportError)
		}
		return previousTransport.RoundTrip(req)
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	provider := NewGenericOAuthProvider(&model.CustomOAuthProvider{
		Name:                "Log Redaction Provider",
		Slug:                "log-redaction-provider",
		ClientId:            "client-id-marker",
		ClientSecret:        "client-secret-marker",
		TokenEndpoint:       server.URL + "/token",
		UserInfoEndpoint:    server.URL + "/userinfo",
		AuthStyle:           AuthStyleInParams,
		UserIdField:         "sub",
		UsernameField:       "preferred_username",
		DisplayNameField:    "name",
		EmailField:          "email",
		AccessPolicy:        `{"logic":"and","conditions":[{"field":"email","op":"eq","value":"` + policyExpected + `"}]}`,
		AccessDeniedMessage: "Access denied",
	})

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

	token, err := provider.ExchangeToken(context.Background(), authorizationCode, nil)
	require.NoError(t, err)
	require.Equal(t, accessToken, token.AccessToken)
	require.Equal(t, tokenScope, token.Scope)
	_, err = provider.GetUserInfo(context.Background(), token)
	var denied *AccessDeniedError
	require.ErrorAs(t, err, &denied)

	errorProvider := NewGenericOAuthProvider(&model.CustomOAuthProvider{
		Name:          "Log Redaction Error Provider",
		Slug:          "log-redaction-error-provider",
		ClientId:      "client-id-marker",
		ClientSecret:  "client-secret-marker",
		TokenEndpoint: server.URL + "/token-error",
		AuthStyle:     AuthStyleInParams,
	})
	_, err = errorProvider.ExchangeToken(context.Background(), authorizationCode, nil)
	require.Error(t, err)
	var oauthErr *OAuthError
	require.ErrorAs(t, err, &oauthErr)
	assert.Empty(t, oauthErr.RawError)
	assert.NotContains(t, err.Error(), oauthErrorCode)
	assert.NotContains(t, err.Error(), oauthErrorDesc)

	credentialedEndpoint := "https://" + endpointUser + ":" + endpointPassword +
		"@transport.example.test/oauth?api_key=" + endpointQuery
	failingProvider := NewGenericOAuthProvider(&model.CustomOAuthProvider{
		Name:             "Transport Failure Provider",
		Slug:             "transport-failure-provider",
		TokenEndpoint:    credentialedEndpoint,
		UserInfoEndpoint: credentialedEndpoint,
	})
	_, err = failingProvider.ExchangeToken(context.Background(), authorizationCode, nil)
	require.Error(t, err)
	_, err = failingProvider.GetUserInfo(context.Background(), &OAuthToken{AccessToken: accessToken})
	require.Error(t, err)

	output := logs.String()
	assert.Contains(t, output, "ExchangeToken response status: 200")
	assert.Contains(t, output, "GetUserInfo success")
	for _, sensitive := range []string{
		authorizationCode,
		accessToken,
		refreshToken,
		idToken,
		tokenScope,
		oauthErrorCode,
		oauthErrorDesc,
		providerUserID,
		username,
		displayName,
		email,
		policyExpected,
		"client-secret-marker",
		endpointUser,
		endpointPassword,
		endpointQuery,
		transportError,
	} {
		assert.Falsef(t, strings.Contains(output, sensitive), "debug log exposed sensitive marker %q", sensitive)
	}
}
