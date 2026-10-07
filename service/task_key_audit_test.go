package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskKeyAuditPersistedSubmissionSettlementRefundAndPrivacy(t *testing.T) {
	for _, funding := range []string{BillingSourceWallet, BillingSourceSubscription} {
		t.Run(funding, func(t *testing.T) {
			truncate(t)
			const userID, channelID, subID = 901, 901, 901
			seedUser(t, userID, 900)
			seedChannel(t, channelID)
			if funding == BillingSourceSubscription {
				seedSubscription(t, subID, userID, 1000, 100)
			}
			task := makeTask(userID, channelID, 100, 0, funding, 0)
			task.PrivateData.AggregateUsageState = ""
			if funding == BillingSourceSubscription {
				task.PrivateData.SubscriptionId = subID
			}
			// Real private JSON roundtrip; key/index describe submission, not current channel.
			raw, err := json.Marshal(task.PrivateData)
			require.NoError(t, err)
			var private map[string]any
			require.NoError(t, json.Unmarshal(raw, &private))
			private["key"] = "private-original-key"
			private["is_multi_key"] = true
			private["key_index"] = 0
			raw, err = json.Marshal(private)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &task.PrivateData))
			require.NoError(t, model.DB.Create(task).Error)
			public, err := json.Marshal(task)
			require.NoError(t, err)
			assert.NotContains(t, string(public), "private-original-key")
			assert.NotContains(t, string(public), "key_index")
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("key", "replacement-key\nprivate-original-key").Error)
			info := &relaycommon.RelayInfo{UserId: userID, UsingGroup: "default", OriginModelName: "test-model", PriceData: types.PriceData{Quota: 100, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID}, TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: "generate"}}
			require.NoError(t, LogTaskConsumption(taskBillingTestContext(), info, task))
			RecalculateTaskQuota(context.Background(), task, 150, "actual result")
			require.True(t, RefundTaskQuota(context.Background(), task, "failed result"))
			require.True(t, RefundTaskQuota(context.Background(), task, "duplicate failed result"))
			if funding == BillingSourceWallet {
				assert.Equal(t, 1000, getUserQuota(t, userID))
			} else {
				var sub model.UserSubscription
				require.NoError(t, model.DB.First(&sub, subID).Error)
				assert.Zero(t, sub.AmountUsed)
			}
			admin, total, err := model.GetAllLogs(model.LogTypeUnknown, 0, 0, "", "", "", 0, 100, 0, "", "", "", model.SiteScopeAll)
			require.NoError(t, err)
			require.Equal(t, int64(3), total)
			for _, log := range admin {
				var other map[string]any
				require.NoError(t, json.Unmarshal([]byte(log.Other), &other))
				require.Contains(t, other, "admin_info")
				audit := other["admin_info"].(map[string]any)
				assert.Equal(t, true, audit["is_multi_key"])
				assert.Equal(t, float64(0), audit["multi_key_index"])
				assert.NotContains(t, log.Other, "private-original-key")
				assert.NotContains(t, log.Other, "replacement-key")
			}
			user, total, err := model.GetUserLogs(userID, model.LogTypeUnknown, 0, 0, "", "", 0, 100, "", "", "")
			require.NoError(t, err)
			require.Equal(t, int64(3), total)
			for _, log := range user {
				assert.NotContains(t, log.Other, "admin_info")
				assert.NotContains(t, log.Other, "private-original-key")
				assert.NotContains(t, log.Other, "multi_key_index")
			}
		})
	}
}

func TestTaskKeyAuditHistoricalUnknownAndSingleDoNotInventIndex(t *testing.T) {
	for _, raw := range []string{`{"key":"private-key"}`, `{"key":"private-key","is_multi_key":false}`, `{"is_multi_key":true}`, `{"is_multi_key":true,"key_index":-1}`} {
		var private model.TaskPrivateData
		require.NoError(t, json.Unmarshal([]byte(raw), &private))
		other := taskBillingOther(&model.Task{PrivateData: private})
		if audit, ok := other["admin_info"].(map[string]any); ok {
			assert.NotContains(t, audit, "multi_key_index")
			if raw == `{"key":"private-key"}` || raw == `{"key":"private-key","is_multi_key":false}` {
				assert.NotContains(t, audit, "is_multi_key")
			}
		}
		encoded, err := json.Marshal(other)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "private-key")
	}
}

func TestTaskKeyAuditMergesAdminMetadataAndKeepsOriginalNonzeroIndex(t *testing.T) {
	var private model.TaskPrivateData
	require.NoError(t, json.Unmarshal([]byte(`{"key":"secret","is_multi_key":true,"key_index":4}`), &private))
	other := map[string]interface{}{"admin_info": map[string]interface{}{"quota_saturation": "retained", "task_source": "retained"}}
	appendTaskKeyAudit(other, &model.Task{PrivateData: private})
	audit := other["admin_info"].(map[string]interface{})
	assert.Equal(t, 4, audit["multi_key_index"])
	assert.Equal(t, "retained", audit["quota_saturation"])
	assert.Equal(t, "retained", audit["task_source"])
	encoded, err := json.Marshal(other)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret")
}
