package channel_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/ali"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao"
	"github.com/QuantumNous/new-api/relay/channel/task/gemini"
	"github.com/QuantumNous/new-api/relay/channel/task/hailuo"
	"github.com/QuantumNous/new-api/relay/channel/task/jimeng"
	"github.com/QuantumNous/new-api/relay/channel/task/kling"
	"github.com/QuantumNous/new-api/relay/channel/task/sora"
	"github.com/QuantumNous/new-api/relay/channel/task/suno"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/relay/channel/task/vidu"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeTaskPollingHeaderOverrides(t *testing.T) {
	service.InitHttpClient()
	cases := []struct {
		name    string
		adaptor channel.TaskAdaptor
	}{
		{"ali", &ali.TaskAdaptor{}}, {"doubao", &doubao.TaskAdaptor{}},
		{"gemini", &gemini.TaskAdaptor{}}, {"hailuo", &hailuo.TaskAdaptor{}},
		{"jimeng", &jimeng.TaskAdaptor{}}, {"kling", &kling.TaskAdaptor{}},
		{"sora", &sora.TaskAdaptor{}}, {"suno batch", &suno.TaskAdaptor{}}, {"vidu", &vidu.TaskAdaptor{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan *http.Request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- r.Clone(r.Context())
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			overrides := map[string]any{"Authorization": "Custom {api_key}", "X-Task-Key": "{api_key}", "Host": "upstream.example", "*": true, "re:^X-Trace-": true, "X-Client": "{client_header:X-Trace-Id}"}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "channel-key", HeadersOverride: overrides}}
			tc.adaptor.Init(info)
			taskID := "task-1"
			if tc.name == "gemini" {
				taskID = taskcommon.EncodeLocalTaskID("models/veo/operations/task-1")
			}
			resp, err := tc.adaptor.FetchTask(server.URL, "sk-saved-task-key", map[string]any{"task_id": taskID, "action": "generate", "ids": []string{"task-1"}}, "")
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.NoError(t, resp.Body.Close())
			require.Len(t, received, 1)
			request := <-received
			assert.Equal(t, "Custom sk-saved-task-key", request.Header.Get("Authorization"))
			assert.Equal(t, "sk-saved-task-key", request.Header.Get("X-Task-Key"))
			assert.Equal(t, "upstream.example", request.Host)
			assert.Empty(t, request.Header.Get("X-Client"))
			assert.Empty(t, request.Header.Get("X-Trace-Id"))
			assert.Equal(t, "channel-key", info.ApiKey)
			assert.Equal(t, "{api_key}", overrides["X-Task-Key"])

			overrides["X-Task-Key"] = "invalid\r\nInjected: value"
			resp, err = tc.adaptor.FetchTask(server.URL, "sk-saved-task-key", map[string]any{"task_id": taskID, "action": "generate", "ids": []string{"task-1"}}, "")
			require.Error(t, err)
			assert.Nil(t, resp)
			assert.Empty(t, received, "invalid headers must fail before reaching the upstream")
		})
	}
}

func TestNativeTaskSubmissionHeaderOverrides(t *testing.T) {
	service.InitHttpClient()
	received := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Clone(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "submit-key", HeadersOverride: map[string]any{
		"Authorization": "Custom {api_key}", "X-Client": "{client_header:X-Trace-Id}", "*": true,
	}}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := &sora.TaskAdaptor{}
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	c.Request.Header.Set("X-Trace-Id", "client-trace")
	c.Request.Header.Set("Authorization", "client-secret")
	c.Request.Header.Set("Cookie", "client-cookie")
	resp, err := channel.DoTaskApiRequest(adaptor, c, info, strings.NewReader(`{}`))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Len(t, received, 1)
	request := <-received
	assert.Equal(t, "Custom submit-key", request.Header.Get("Authorization"))
	assert.Equal(t, "client-trace", request.Header.Get("X-Client"))
	assert.Equal(t, "client-trace", request.Header.Get("X-Trace-Id"))
	assert.Empty(t, request.Header.Get("Cookie"))
}

func TestTaskPollingRedirectSecurity(t *testing.T) {
	service.InitHttpClient()
	fetch := system_setting.GetFetchSetting()
	previous := *fetch
	t.Cleanup(func() { *fetch = previous })
	fetch.EnableSSRFProtection = false
	shared := service.GetHttpClient()
	originalPolicy := reflect.ValueOf(shared.CheckRedirect).Pointer()
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetHits.Add(1); w.WriteHeader(http.StatusOK) }))
	defer target.Close()
	var finalHeader atomic.Value
	finalHeader.Store("")
	var sameOriginHits atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "/done", http.StatusTemporaryRedirect)
		case "/cross":
			http.Redirect(w, r, target.URL+"?token=secret-destination", http.StatusTemporaryRedirect)
		case "/done":
			finalHeader.Store(r.Header.Get("X-Custom-Key"))
			sameOriginHits.Add(1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer source.Close()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: map[string]any{"X-Custom-Key": "{api_key}"}}}
	req, err := http.NewRequest(http.MethodGet, source.URL+"/same", nil)
	require.NoError(t, err)
	resp, err := channel.DoTaskPollingRequest(info, "secret", "", req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "secret", finalHeader.Load())
	req, err = http.NewRequest(http.MethodGet, source.URL+"/cross?api_key=secret-origin", nil)
	require.NoError(t, err)
	_, err = channel.DoTaskPollingRequest(info, "secret", "", req)
	require.ErrorContains(t, err, "cross-origin task redirect blocked")
	assert.NotContains(t, err.Error(), "secret-origin")
	assert.NotContains(t, err.Error(), "secret-destination")
	assert.Zero(t, targetHits.Load(), "custom credentials must not be sent to another origin")

	// Even a same-origin redirect must still pass the shared SSRF policy.
	fetch.EnableSSRFProtection = true
	fetch.AllowPrivateIp = false
	fetch.AllowedPorts = []string{"1-65535"}
	req, err = http.NewRequest(http.MethodGet, source.URL+"/same", nil)
	require.NoError(t, err)
	_, err = channel.DoTaskPollingRequest(info, "secret", "", req)
	require.ErrorContains(t, err, "blocked")
	assert.EqualValues(t, 1, sameOriginHits.Load(), "SSRF policy must block the second redirected request")
	assert.Equal(t, originalPolicy, reflect.ValueOf(shared.CheckRedirect).Pointer(), "the cached client must remain unchanged")
}

func TestTaskPollingErrorRedactsQueryAndPreservesCancellation(t *testing.T) {
	service.InitHttpClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://upstream.example/task?token=query-secret", nil)
	require.NoError(t, err)
	_, err = channel.DoTaskPollingRequest(nil, "task-key", "", req)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), "query-secret")
	assert.NotContains(t, err.Error(), "upstream.example")
}
