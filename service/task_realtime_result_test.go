package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeTaskResultRefundsOnceAndRejectsStaleTransition(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID = 841, 842, 843
	seedUser(t, userID, 800)
	seedToken(t, tokenID, userID, "realtime-refund-token", 800)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, 200, 1)
	task := makeTask(userID, channelID, 200, tokenID, BillingSourceWallet, 0)
	task.TaskID = "realtime-refund"
	task.PrivateData.UpstreamTaskID = "provider-task"
	require.NoError(t, model.DB.Create(task).Error)
	stale := *task
	body, err := common.Marshal(taskdto.TaskResponse[model.Task]{Code: taskdto.TaskSuccessCode, Data: model.Task{Status: model.TaskStatusFailure, FailReason: "upstream failed"}})
	require.NoError(t, err)
	adaptor := &taskPollingFetchAdaptor{}
	require.NoError(t, ApplyVideoTaskResult(context.Background(), adaptor, task, body))
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	assert.Equal(t, 1000, getUserQuota(t, userID))
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTaskQuota(t, task.ID))
	require.Error(t, ApplyVideoTaskResult(context.Background(), adaptor, &stale, body))
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stale.Status, "failed CAS must restore the caller's original state")
	require.NoError(t, ApplyVideoTaskResult(context.Background(), adaptor, task, body))
	assert.Equal(t, 1000, getUserQuota(t, userID))
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(1), countLogs(t))
	assert.Zero(t, adaptor.fetchCount(), "applying an already fetched result must not send another upstream request")
}

func TestRealtimeTaskResultDoesNotRefundHistoricalTerminalTask(t *testing.T) {
	truncate(t)
	seedUser(t, 844, 800)
	task := makeTask(844, 0, 200, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatusFailure
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, ApplyVideoTaskResult(context.Background(), &taskPollingFetchAdaptor{}, task, []byte(`{}`)))
	assert.Equal(t, 800, getUserQuota(t, 844))
	assert.Equal(t, int64(0), countLogs(t))
}
