package service

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPTransportsKeepInsecureTLSSettingsIsolated(t *testing.T) {
	for _, test := range []struct {
		name string
		make func() *http.Transport
	}{
		{name: "relay", make: newRelayHTTPTransport},
		{name: "protected fetch", make: func() *http.Transport {
			return (&ssrfProtectedRoundTripper{}).newTransport(nil)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			previousSkipVerify, previousConfig := common.TLSInsecureSkipVerify, common.InsecureTLSConfig
			common.TLSInsecureSkipVerify = true
			common.InsecureTLSConfig = &tls.Config{InsecureSkipVerify: true}
			t.Cleanup(func() {
				common.TLSInsecureSkipVerify, common.InsecureTLSConfig = previousSkipVerify, previousConfig
			})
			first, second := test.make(), test.make()
			t.Cleanup(first.CloseIdleConnections)
			t.Cleanup(second.CloseIdleConnections)
			require.NotNil(t, first.TLSClientConfig)
			require.NotNil(t, second.TLSClientConfig)
			// HTTP transports may configure ALPN on their TLS config. Such a
			// transport-local change must not affect another client or the template.
			first.TLSClientConfig.NextProtos = []string{"http/1.1"}
			first.TLSClientConfig.InsecureSkipVerify = false
			assert.Empty(t, second.TLSClientConfig.NextProtos)
			assert.True(t, second.TLSClientConfig.InsecureSkipVerify)
			assert.Empty(t, common.InsecureTLSConfig.NextProtos)
			assert.True(t, common.InsecureTLSConfig.InsecureSkipVerify)
		})
	}
}
