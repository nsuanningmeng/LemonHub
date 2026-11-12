package service

import (
	"crypto/tls"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// NewCredentialExchangeHTTPClient creates a short-lived client for OAuth token
// exchanges. It retains the configured proxy and transport policy, but never
// inherits the relay's insecure TLS setting or forwards credentials on redirects.
// The caller must close idle connections when the exchange finishes.
func NewCredentialExchangeHTTPClient(settings dto.ChannelSettings) (*http.Client, error) {
	transport := newRelayHTTPTransport()
	verifiedTLS := &tls.Config{}
	if original, ok := http.DefaultTransport.(*http.Transport); ok && original.TLSClientConfig != nil {
		verifiedTLS.RootCAs = original.TLSClientConfig.RootCAs
	}
	transport.TLSClientConfig = verifiedTLS
	// A custom TLS dialer could bypass the verified configuration entirely.
	transport.DialTLS = nil
	transport.DialTLSContext = nil
	if rawProxy := strings.TrimSpace(settings.Proxy); rawProxy != "" {
		proxyURL, _, err := common.ParseProxyURLRuntime(rawProxy)
		if err != nil {
			return nil, err
		}
		if err = configureProxyTransport(transport, proxyURL); err != nil {
			return nil, err
		}
	}
	client := newHTTPClientFromTransportFactory(NormalizeHTTPTransportPolicy(settings), func() *http.Transport { return transport.Clone() })
	client.Timeout = 30 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client, nil
}
