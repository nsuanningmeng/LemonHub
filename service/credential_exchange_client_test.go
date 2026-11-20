package service

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialExchangeVerifiesTLSDespiteRelayInsecureSetting(t *testing.T) {
	oldFlag, oldConfig := common.TLSInsecureSkipVerify, common.InsecureTLSConfig
	common.TLSInsecureSkipVerify = true
	common.InsecureTLSConfig = &tls.Config{InsecureSkipVerify: true}
	t.Cleanup(func() { common.TLSInsecureSkipVerify, common.InsecureTLSConfig = oldFlag, oldConfig })
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "verified") }))
	t.Cleanup(server.Close)
	client, err := NewCredentialExchangeHTTPClient(dto.ChannelSettings{})
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	_, err = client.Get(server.URL)
	require.Error(t, err)
	var unknown x509.UnknownAuthorityError
	assert.ErrorAs(t, err, &unknown)
	// Explicit trusted roots still work; global relay TLS policy stays unchanged.
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original; transport.CloseIdleConnections() })
	trusted, err := NewCredentialExchangeHTTPClient(dto.ChannelSettings{HTTPProtocol: "http1"})
	require.NoError(t, err)
	t.Cleanup(trusted.CloseIdleConnections)
	response, err := trusted.Get(server.URL)
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, 200, response.StatusCode)
	assert.True(t, common.InsecureTLSConfig.InsecureSkipVerify)
}

func TestCredentialExchangeUsesProxyAndDoesNotReplayRedirects(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	t.Cleanup(target.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "credential-fixture.invalid", r.URL.Host)
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "signed-assertion", string(data))
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(proxy.Close)
	client, err := NewCredentialExchangeHTTPClient(dto.ChannelSettings{Proxy: proxy.URL})
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	response, err := client.Post("http://credential-fixture.invalid/token", "application/octet-stream", strings.NewReader("signed-assertion"))
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
	assert.Zero(t, redirected.Load())
}
