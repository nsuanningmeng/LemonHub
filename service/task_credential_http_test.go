package service_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/ali"
	"github.com/QuantumNous/new-api/relay/channel/task/hailuo"
	"github.com/QuantumNous/new-api/relay/channel/task/suno"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPollingHTTPUsesSubmissionCredentialAcrossRotation(t *testing.T) {
	service.InitHttpClient()
	oldFactory := service.GetTaskAdaptorFunc
	oldCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { service.GetTaskAdaptorFunc = oldFactory; common.MemoryCacheEnabled = oldCache })
	for _, provider := range []string{"ali", "hailuo"} {
		t.Run(provider, func(t *testing.T) {
			var mu sync.Mutex
			headers := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				headers = append(headers, r.Header.Get("Authorization"))
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if provider == "ali" {
					fmt.Fprint(w, `{"output":{"task_status":"RUNNING"}}`)
					return
				}
				if strings.Contains(r.URL.Path, "retrieve") {
					fmt.Fprint(w, `{"base_resp":{"status_code":0},"file":{"download_url":"https://example.test/video"}}`)
					return
				}
				fmt.Fprint(w, `{"task_id":"upstream","status":"Success","file_id":"file","base_resp":{"status_code":0}}`)
			}))
			defer server.Close()
			channelType := constant.ChannelTypeAli
			platform := constant.TaskPlatform("ali")
			if provider == "hailuo" {
				channelType = constant.ChannelTypeMiniMax
				platform = constant.TaskPlatform("hailuo")
			}
			ch := &model.Channel{Type: channelType, Key: "rotated-b\nrotated-a", BaseURL: &server.URL, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
			ch.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
			require.NoError(t, model.DB.Create(ch).Error)
			t.Cleanup(func() { model.DB.Delete(ch) })
			service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor {
				if provider == "ali" {
					return &ali.TaskAdaptor{}
				}
				return &hailuo.TaskAdaptor{}
			}
			tasks := map[string]*model.Task{}
			ids := []string{}
			for i, key := range []string{"submitted-a", "submitted-b"} {
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: ch.Id, ChannelType: channelType, ChannelIsMultiKey: true, ChannelMultiKeyIndex: i, ApiKey: key}}
				task := model.InitTask(platform, info)
				task.PrivateData.UpstreamTaskID = fmt.Sprintf("upstream-%d", i)
				require.NoError(t, model.DB.Create(task).Error)
				t.Cleanup(func() { model.DB.Delete(task) })
				var loaded model.Task
				require.NoError(t, model.DB.First(&loaded, task.ID).Error)
				ids = append(ids, task.TaskID)
				tasks[task.TaskID] = &loaded
			}
			require.NoError(t, service.UpdateVideoTasks(context.Background(), platform, map[int][]string{ch.Id: ids}, tasks))
			expected := []string{"Bearer submitted-a", "Bearer submitted-b"}
			if provider == "hailuo" {
				expected = []string{"Bearer submitted-a", "Bearer submitted-a", "Bearer submitted-b", "Bearer submitted-b"}
			}
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, expected, headers, "both poll and retrieve must use the submitting credential")
		})
	}
}

func TestSunoPollingHTTPIsolatesCredentialGroups(t *testing.T) {
	service.InitHttpClient()
	oldFactory, oldCache := service.GetTaskAdaptorFunc, common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { service.GetTaskAdaptorFunc = oldFactory; common.MemoryCacheEnabled = oldCache })
	var mu sync.Mutex
	got := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []string `json:"ids"`
		}
		data, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			w.WriteHeader(400)
			return
		}
		if !assert.NoError(t, common.Unmarshal(data, &body)) {
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		got = append(got, r.Header.Get("Authorization")+":"+fmt.Sprint(body.IDs))
		mu.Unlock()
		fmt.Fprint(w, `{"code":"success","data":[]}`)
	}))
	defer server.Close()
	ch := &model.Channel{Key: "rotated-b\nrotated-a", BaseURL: &server.URL, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
	require.NoError(t, model.DB.Create(ch).Error)
	t.Cleanup(func() { model.DB.Delete(ch) })
	service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor { return &suno.TaskAdaptor{} }
	tasks := map[string]*model.Task{"a": {PrivateData: model.TaskPrivateData{Key: "selected-a"}}, "unknown": {Status: model.TaskStatusNotStart, Quota: 99}, "b": {PrivateData: model.TaskPrivateData{Key: "selected-b"}}}
	require.NoError(t, service.UpdateSunoTasks(context.Background(), map[int][]string{ch.Id: {"a", "unknown", "b"}}, tasks))
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"Bearer selected-a:[a]", "Bearer selected-b:[b]"}, got)
	assert.Equal(t, model.TaskStatusNotStart, tasks["unknown"].Status)
	assert.Equal(t, 99, tasks["unknown"].Quota)
}
