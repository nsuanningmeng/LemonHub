package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type credentialGroupAdaptor struct {
	calls  *[]string
	cancel context.CancelFunc
}

func (a *credentialGroupAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *credentialGroupAdaptor) FetchTask(_ string, key string, body map[string]any, _ string) (*http.Response, error) {
	*a.calls = append(*a.calls, key+":"+fmt.Sprint(body["ids"]))
	if a.cancel != nil {
		a.cancel()
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":"success","data":[]}`))}, nil
}

func TestSunoPollingCancellationStopsRemainingCredentialGroups(t *testing.T) {
	oldFactory, oldCache := GetTaskAdaptorFunc, common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { GetTaskAdaptorFunc = oldFactory; common.MemoryCacheEnabled = oldCache })
	base := "http://unused.test"
	ch := &model.Channel{Key: "current-a\ncurrent-b", BaseURL: &base, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
	require.NoError(t, model.DB.Create(ch).Error)
	t.Cleanup(func() { model.DB.Delete(ch) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := []string{}
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor {
		return &credentialGroupAdaptor{calls: &calls, cancel: cancel}
	}
	tasks := map[string]*model.Task{
		"a": {PrivateData: model.TaskPrivateData{Key: "selected-a"}},
		"b": {PrivateData: model.TaskPrivateData{Key: "selected-b"}},
	}
	err := updateSunoTasks(ctx, ch.Id, []string{"a", "b"}, tasks)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"selected-a:[a]"}, calls)
}
func (a *credentialGroupAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}
func (a *credentialGroupAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func TestSunoPollingGroupsCredentialsAndSkipsUnknown(t *testing.T) {
	oldFactory, oldCache := GetTaskAdaptorFunc, common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { GetTaskAdaptorFunc = oldFactory; common.MemoryCacheEnabled = oldCache })
	base := "http://unused.test"
	ch := &model.Channel{Key: "current-a\ncurrent-b", BaseURL: &base, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
	require.NoError(t, model.DB.Create(ch).Error)
	t.Cleanup(func() { model.DB.Delete(ch) })
	calls := []string{}
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return &credentialGroupAdaptor{calls: &calls} }
	tasks := map[string]*model.Task{
		"a1":      {PrivateData: model.TaskPrivateData{Key: "selected-a"}},
		"unknown": {Status: model.TaskStatusNotStart, Quota: 123},
		"b1":      {PrivateData: model.TaskPrivateData{Key: "selected-b"}},
		"a2":      {PrivateData: model.TaskPrivateData{Key: "selected-a"}},
	}
	err := updateSunoTasks(context.Background(), ch.Id, []string{"a1", "unknown", "b1", "a2"}, tasks)
	require.ErrorContains(t, err, "credential provenance unavailable")
	assert.Equal(t, []string{"selected-a:[a1 a2]", "selected-b:[b1]"}, calls)
	assert.Equal(t, model.TaskStatusNotStart, tasks["unknown"].Status)
	assert.Equal(t, 123, tasks["unknown"].Quota)
}
