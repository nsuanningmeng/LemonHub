package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type headerOverridePollingAdaptor struct {
	taskPollingFetchAdaptor
	info  *relaycommon.RelayInfo
	key   string
	proxy string
}

func (a *headerOverridePollingAdaptor) Init(info *relaycommon.RelayInfo) { a.info = info }
func (a *headerOverridePollingAdaptor) FetchTask(_ string, key string, _ map[string]any, proxy string) (*http.Response, error) {
	a.key = key
	a.proxy = proxy
	return nil, errors.New("stop after polling request initialization")
}

func TestTaskPollingInitializesHeaderOverridesWithSavedCredentials(t *testing.T) {
	baseURL := "https://upstream.example"
	headers := `{"X-Task-Key":"{api_key}","X-Static":"configured"}`
	setting := `{"proxy":"http://proxy.example:8080","http_protocol":"http1"}`
	ch := &model.Channel{Id: 42, Type: constant.ChannelTypeOpenAI, Key: "current-channel-key", BaseURL: &baseURL, HeaderOverride: &headers, Setting: &setting}
	task := &model.Task{ChannelId: ch.Id, TaskID: "task-1", PrivateData: model.TaskPrivateData{Key: "saved-task-key"}}
	for _, mode := range []string{"single", "suno batch"} {
		t.Run(mode, func(t *testing.T) {
			adaptor := &headerOverridePollingAdaptor{}
			var err error
			if mode == "single" {
				err = updateVideoSingleTask(context.Background(), adaptor, ch, "task-1", map[string]*model.Task{"task-1": task})
			} else {
				previous := GetTaskAdaptorFunc
				GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
				t.Cleanup(func() { GetTaskAdaptorFunc = previous })
				err = updateSunoTaskGroup(context.Background(), ch, "saved-task-key", []string{"task-1"}, map[string]*model.Task{"task-1": task})
			}
			require.ErrorContains(t, err, "stop after polling request initialization")
			require.NotNil(t, adaptor.info)
			require.NotNil(t, adaptor.info.ChannelMeta)
			assert.Equal(t, ch.GetHeaderOverride(), adaptor.info.HeadersOverride)
			assert.Equal(t, "saved-task-key", adaptor.info.ApiKey)
			assert.Equal(t, "saved-task-key", adaptor.key)
			assert.Equal(t, ch.GetSetting(), adaptor.info.ChannelSetting)
			assert.Equal(t, "http://proxy.example:8080", adaptor.proxy)
			assert.Equal(t, ch.Id, adaptor.info.ChannelId)
			assert.Equal(t, "current-channel-key", ch.Key)
		})
	}
}
