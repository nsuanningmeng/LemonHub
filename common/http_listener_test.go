package common

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPListenAddressConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name string
		host string
		want string
	}{
		{name: "gateway default remains public", want: ":3000"},
		{name: "desktop loopback", host: "127.0.0.1", want: "127.0.0.1:3000"},
		{name: "IPv6 loopback", host: "::1", want: "[::1]:3000"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BIND_ADDRESS", tt.host)
			assert.Equal(t, tt.want, HTTPListenAddress("3000"))
		})
	}
}

func TestPprofListenAddressDefaultsToLoopback(t *testing.T) {
	t.Setenv("PPROF_LISTEN_ADDRESS", "")
	host, port, err := net.SplitHostPort(PprofListenAddress())
	require.NoError(t, err)
	assert.True(t, net.ParseIP(host).IsLoopback())
	assert.Equal(t, "8005", port)

	t.Setenv("PPROF_LISTEN_ADDRESS", "192.0.2.10:9005")
	assert.Equal(t, "192.0.2.10:9005", PprofListenAddress())
}

func TestConfiguredLocalListenersServeOnlyOnLoopback(t *testing.T) {
	for _, tt := range []struct {
		name string
		addr func() string
	}{
		{name: "desktop HTTP", addr: func() string { return HTTPListenAddress("0") }},
		{name: "diagnostics", addr: PprofListenAddress},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BIND_ADDRESS", "127.0.0.1")
			t.Setenv("PPROF_LISTEN_ADDRESS", "127.0.0.1:0")
			listener, err := net.Listen("tcp", tt.addr())
			require.NoError(t, err)
			t.Cleanup(func() { _ = listener.Close() })
			require.True(t, listener.Addr().(*net.TCPAddr).IP.IsLoopback())

			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})}
			t.Cleanup(func() { _ = server.Close() })
			go func() { _ = server.Serve(listener) }()
			transport := &http.Transport{Proxy: nil}
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			response, err := client.Get("http://" + listener.Addr().String())
			require.NoError(t, err)
			t.Cleanup(func() { _ = response.Body.Close() })
			assert.Equal(t, http.StatusNoContent, response.StatusCode)
		})
	}
}
