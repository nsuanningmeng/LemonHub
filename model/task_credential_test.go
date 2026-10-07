package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTaskSnapshotsSelectedCredential(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAli, ApiKey: "selected-private-key", ChannelIsMultiKey: true, ChannelMultiKeyIndex: 0}}
	task := InitTask(constant.TaskPlatform("ali"), info)
	assert.Equal(t, "selected-private-key", task.PrivateData.Key)
	require.NotNil(t, task.PrivateData.IsMultiKey)
	assert.True(t, *task.PrivateData.IsMultiKey)
	require.NotNil(t, task.PrivateData.KeyIndex)
	assert.Zero(t, *task.PrivateData.KeyIndex)
}

func TestTaskCredentialPrivateRoundTripAndLegacyPolicy(t *testing.T) {
	multi := true
	zero := 0
	original := TaskPrivateData{
		Key: "selected\nprivate", IsMultiKey: &multi, KeyIndex: &zero,
		NodeName: "submission-node", TokenId: 91,
		BillingSource: "subscription", SubscriptionId: 17,
		AggregateUsageState: TaskAggregateUsageAccounted,
		SubmissionBilling: &TaskSubmissionBilling{
			PreConsumedQuota: 12, TargetQuota: 9, FundingQuota: 9, TokenQuota: 9,
			FundingUncertain: true, UpdatedAt: 123,
		},
		BillingOperation: &TaskBillingOperation{
			Operation: "settle", PreQuota: 12, TargetQuota: 9, Delta: -3,
			FundingStage: TaskBillingStageStateApplied, TokenStage: TaskBillingStageStatePending,
			FinalizeStage: TaskBillingStageStatePending, UpdatedAt: 456,
		},
		BillingContext: &TaskBillingContext{
			ModelPrice: 0.5, GroupRatio: 2, OriginModelName: "video-model",
			OtherRatios: map[string]float64{"duration": 3},
		},
	}
	value, err := original.Value()
	require.NoError(t, err)
	var decoded TaskPrivateData
	require.NoError(t, decoded.Scan(value))
	assert.Equal(t, original, decoded, "private provenance must preserve the existing billing journal")
	task := &Task{TaskID: GenerateTaskID(), PrivateData: decoded}
	require.NoError(t, DB.Create(task).Error)
	t.Cleanup(func() { DB.Delete(task) })
	var stored Task
	require.NoError(t, DB.First(&stored, task.ID).Error)
	assert.Equal(t, original, stored.PrivateData)
	ch := &Channel{Key: "rotated-a\nrotated-b", ChannelInfo: ChannelInfo{IsMultiKey: true}}
	key, err := ResolveTaskCredential(&stored, ch)
	require.NoError(t, err)
	assert.Equal(t, original.Key, key, "submission identity must survive storage and rotation")
	public, err := common.Marshal(stored)
	require.NoError(t, err)
	assert.NotContains(t, string(public), "selected")
	assert.NotContains(t, string(public), "key_index")
	assert.NotContains(t, string(public), "is_multi_key")
	legacy := &Task{}
	_, err = ResolveTaskCredential(legacy, ch)
	require.ErrorContains(t, err, "credential provenance unavailable")
	ch.ChannelInfo.IsMultiKey = false
	ch.Key = "{\n  \"private_key\": \"single-json\"\n}"
	key, err = ResolveTaskCredential(legacy, ch)
	require.NoError(t, err)
	assert.Equal(t, ch.Key, key, "single Vertex JSON credential must remain byte-exact")
}
