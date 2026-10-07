package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextHTTPAdaptor struct {
	Adaptor
	target string
}

func (a contextHTTPAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.target, nil
}
func (a contextHTTPAdaptor) SetupRequestHeader(_ *gin.Context, h *http.Header, _ *relaycommon.RelayInfo) error {
	h.Set("Authorization", "Bearer fixture")
	return nil
}

type contextHTTPClosedBody struct {
	io.Reader
	closed atomic.Bool
}

func (body *contextHTTPClosedBody) Close() error { body.closed.Store(true); return nil }

func TestOutgoingHTTPPreCancelledNeverDispatches(t *testing.T) {
	service.InitHttpClient()
	for _, path := range []string{"api", "form", "direct"} {
		t.Run(path, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
			defer server.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/incoming", strings.NewReader("incoming")).WithContext(ctx)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			requestBody := &contextHTTPClosedBody{Reader: strings.NewReader("body")}
			var resp *http.Response
			var err error
			switch path {
			case "api":
				resp, err = DoApiRequest(contextHTTPAdaptor{target: server.URL}, c, info, requestBody)
			case "form":
				resp, err = DoFormRequest(contextHTTPAdaptor{target: server.URL}, c, info, requestBody)
			default:
				req, e := http.NewRequest(http.MethodPost, server.URL, requestBody)
				require.NoError(t, e)
				resp, err = DoRequest(c, req, info)
			}
			if resp != nil {
				resp.Body.Close()
			}
			require.ErrorIs(t, err, context.Canceled)
			assert.Zero(t, calls.Load())
			assert.True(t, requestBody.closed.Load(), "undispatched outbound body must be released")
		})
	}
}

func TestOutgoingHTTPHeaderWaitAndMidBodyCancellation(t *testing.T) {
	service.InitHttpClient()
	for _, bodyStarted := range []bool{false, true} {
		t.Run(map[bool]string{false: "headers", true: "body"}[bodyStarted], func(t *testing.T) {
			entered := make(chan struct{})
			reading := make(chan struct{})
			ended := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				if bodyStarted {
					w.WriteHeader(200)
					io.WriteString(w, "first")
					w.(http.Flusher).Flush()
				}
				close(entered)
				select {
				case <-r.Context().Done():
					close(ended)
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/incoming", strings.NewReader("incoming")).WithContext(ctx)
			result := make(chan error, 1)
			go func() {
				resp, err := DoApiRequest(contextHTTPAdaptor{target: server.URL}, c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, strings.NewReader("body"))
				if err == nil {
					defer resp.Body.Close()
					if bodyStarted {
						first := make([]byte, 5)
						_, err = io.ReadFull(resp.Body, first)
						close(reading)
					}
					if err == nil {
						_, err = io.ReadAll(resp.Body)
					}
				}
				result <- err
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("upstream request missing")
			}
			if bodyStarted {
				select {
				case <-reading:
				case <-time.After(2 * time.Second):
					t.Fatal("body first bytes missing")
				}
			}
			cancel()
			select {
			case err := <-result:
				assert.ErrorIs(t, err, context.Canceled)
			case <-time.After(2 * time.Second):
				t.Fatal("outgoing request ignored cancellation")
			}
			select {
			case <-ended:
			case <-time.After(2 * time.Second):
				t.Fatal("upstream connection not released")
			}
		})
	}
}

type contextHTTPRoundTripper func(*http.Request) (*http.Response, error)

func (fn contextHTTPRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestOutgoingHTTPPreservesContextMetadataAndCallerLifetime(t *testing.T) {
	service.InitHttpClient()
	type key string
	incomingDeadline := time.Now().Add(time.Hour)
	incoming, cancelIncoming := context.WithDeadline(context.WithValue(context.Background(), key("incoming"), "incoming-value"), incomingDeadline)
	defer cancelIncoming()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/incoming", strings.NewReader("incoming")).WithContext(incoming)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=fixture")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: map[string]any{"Host": "override.example", "Authorization": "Bearer override"}}}
	client, err := service.GetHttpClientWithProxySettings("", info.ChannelSetting)
	require.NoError(t, err)
	original := client.Transport
	defer func() { client.Transport = original }()
	var captured *http.Request
	client.Transport = contextHTTPRoundTripper(func(req *http.Request) (*http.Response, error) {
		captured = req
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("response")), Request: req}, nil
	})
	for _, path := range []string{"api", "form", "direct"} {
		t.Run(path, func(t *testing.T) {
			body, closer, err := relaycommon.NewOutboundJSONBody([]byte("payload"))
			require.NoError(t, err)
			defer closer.Close()
			var resp *http.Response
			caller, cancelCaller := context.WithDeadline(context.WithValue(context.Background(), key("caller"), "caller-value"), time.Now().Add(30*time.Minute))
			defer cancelCaller()
			switch path {
			case "api":
				resp, err = DoApiRequest(contextHTTPAdaptor{target: "http://upstream.example"}, c, info, body)
			case "form":
				resp, err = DoFormRequest(contextHTTPAdaptor{target: "http://upstream.example"}, c, info, body)
			default:
				req, e := http.NewRequestWithContext(caller, http.MethodPost, "http://upstream.example", body)
				require.NoError(t, e)
				ApplyUpstreamBodyMetadata(req, body)
				resp, err = DoRequest(c, req, info)
			}
			require.NoError(t, err)
			require.NotNil(t, captured)
			assert.Equal(t, "incoming-value", captured.Context().Value(key("incoming")))
			assert.NoError(t, captured.Context().Err(), "returning headers must leave the body context active")
			require.NotNil(t, captured.GetBody)
			replay, e := captured.GetBody()
			require.NoError(t, e)
			data, e := io.ReadAll(replay)
			require.NoError(t, e)
			replay.Close()
			assert.Equal(t, "payload", string(data))
			assert.EqualValues(t, 7, captured.ContentLength)
			if path == "direct" {
				assert.Equal(t, "caller-value", captured.Context().Value(key("caller")))
				deadline, _ := captured.Context().Deadline()
				want, _ := caller.Deadline()
				assert.Equal(t, want, deadline)
				cancelCaller()
				select {
				case <-captured.Context().Done():
				case <-time.After(2 * time.Second):
					t.Fatal("caller cancellation lost")
				}
			} else {
				assert.Equal(t, "override.example", captured.Host)
				assert.Equal(t, "Bearer override", captured.Header.Get("Authorization"))
				deadline, _ := captured.Context().Deadline()
				assert.Equal(t, incomingDeadline, deadline)
			}
			require.NoError(t, resp.Body.Close())
			if path == "direct" {
				assert.ErrorIs(t, captured.Context().Err(), context.Canceled)
			}
		})
	}
	req, err := http.NewRequest(http.MethodGet, "http://internal.example", nil)
	require.NoError(t, err)
	resp, err := DoRequest(nil, req, info)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}
