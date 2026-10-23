package kling

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKlingSingleKeySubmissionAndPolling(t *testing.T) {
	service.InitHttpClient()
	for _, tc := range []struct{ name, key, want, pathPrefix string }{
		{"single API key", "upstream-token", "upstream-token", "/v1/videos/"},
		{"trimmed single key", "  upstream-token\n", "upstream-token", "/v1/videos/"},
		{"gateway key", "sk-gateway-key", "sk-gateway-key", "/kling/v1/videos/"},
		{"trimmed gateway key", " sk-gateway-key ", "sk-gateway-key", "/kling/v1/videos/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan *http.Request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- r.Clone(r.Context())
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: tc.key, ChannelBaseUrl: server.URL}, TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionGenerate}}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			target, err := adaptor.BuildRequestURL(info)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(target, server.URL+tc.pathPrefix))
			req, err := http.NewRequest(http.MethodPost, target, nil)
			require.NoError(t, err)
			require.NoError(t, adaptor.BuildRequestHeader(nil, req, info))
			assert.Equal(t, "Bearer "+tc.want, req.Header.Get("Authorization"))
			resp, err := adaptor.FetchTask(server.URL, tc.key, map[string]any{"task_id": "task-1", "action": constant.TaskActionGenerate}, "")
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Len(t, received, 1)
			request := <-received
			assert.Equal(t, "Bearer "+tc.want, request.Header.Get("Authorization"))
			assert.True(t, strings.HasPrefix(request.URL.Path, tc.pathPrefix))
		})
	}
}

func TestKlingAccessSecretSigningAndInvalidKeys(t *testing.T) {
	adaptor := &TaskAdaptor{}
	encoded, err := adaptor.createJWTTokenWithKey(" access-key | signing-secret ")
	require.NoError(t, err)
	token, err := jwt.Parse(encoded, func(token *jwt.Token) (any, error) {
		assert.Equal(t, jwt.SigningMethodHS256, token.Method)
		return []byte("signing-secret"), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	require.NoError(t, err)
	require.True(t, token.Valid)
	issuer, err := token.Claims.GetIssuer()
	require.NoError(t, err)
	assert.Equal(t, "access-key", issuer)
	for _, key := range []string{"", " \n ", "access|secret|extra", "access|", "|secret"} {
		t.Run(key, func(t *testing.T) {
			_, err := adaptor.createJWTTokenWithKey(key)
			require.Error(t, err)
			response, err := adaptor.FetchTask("http://unused.invalid", key, map[string]any{"task_id": "task-1", "action": constant.TaskActionGenerate}, "")
			require.Error(t, err)
			assert.Nil(t, response)
		})
	}
}
