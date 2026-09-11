package service

import (
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayResponseHeaderTimeoutConfiguration(t *testing.T) {
	previousSeconds := common.RelayResponseHeaderTimeout
	previousTransport := http.DefaultTransport
	t.Cleanup(func() {
		common.RelayResponseHeaderTimeout = previousSeconds
		http.DefaultTransport = previousTransport
	})

	// An explicit disabled setting must also clear an inherited timeout.
	http.DefaultTransport = &http.Transport{ResponseHeaderTimeout: time.Minute}
	for _, tc := range []struct {
		name    string
		seconds int
		want    time.Duration
	}{
		{name: "configured", seconds: 1800, want: 30 * time.Minute},
		{name: "disabled", seconds: 0},
		{name: "negative disables", seconds: -1},
		{name: "overflow saturates", seconds: math.MaxInt, want: time.Duration(math.MaxInt64/int64(time.Second)) * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.RelayResponseHeaderTimeout = tc.seconds
			transport := newRelayHTTPTransport()
			t.Cleanup(transport.CloseIdleConnections)
			assert.Equal(t, tc.want, transport.ResponseHeaderTimeout)
		})
	}
}

func TestRelayResponseHeaderTimeoutCancelsStalledRequest(t *testing.T) {
	previous := common.RelayResponseHeaderTimeout
	common.RelayResponseHeaderTimeout = 1
	t.Cleanup(func() { common.RelayResponseHeaderTimeout = previous })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep the connection open without sending headers, like a stalled upstream.
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	transport := newRelayHTTPTransport()
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	resp, err := client.Get(server.URL)
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err)
	var timeoutError net.Error
	require.ErrorAs(t, err, &timeoutError)
	assert.True(t, timeoutError.Timeout())
	assert.ErrorContains(t, err, "timeout awaiting response headers")
}

func TestRelayResponseHeaderTimeoutDoesNotLimitResponseBody(t *testing.T) {
	previous := common.RelayResponseHeaderTimeout
	common.RelayResponseHeaderTimeout = 1
	t.Cleanup(func() { common.RelayResponseHeaderTimeout = previous })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		// Exercise the actual timeout boundary: the response body is delayed
		// beyond the configured header deadline after headers have been flushed.
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			_, _ = io.WriteString(w, "data: complete\n\n")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	transport := newRelayHTTPTransport()
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "data: complete\n\n", string(body))
}
