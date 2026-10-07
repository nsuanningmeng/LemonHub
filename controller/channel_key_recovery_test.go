package controller

import (
	"context"
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func keyRecoveryPolicy(t *testing.T) {
	// Local notification limiter prevents outbound notification and the global
	// memory limiter's permanent cleanup worker from polluting host drain proofs.
	oldRedis, oldClient, oldLimit := common.RedisEnabled, common.RDB, constant.NotifyLimitCount
	local := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: local.Addr(), MaxRetries: -1})
	common.RedisEnabled, common.RDB, constant.NotifyLimitCount = true, client, 0
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled, common.RDB, constant.NotifyLimitCount = oldRedis, oldClient, oldLimit
	})
	oldEnable, oldDisable := common.AutomaticEnableChannelEnabled, common.AutomaticDisableChannelEnabled
	common.AutomaticEnableChannelEnabled, common.AutomaticDisableChannelEnabled = true, false
	t.Cleanup(func() {
		common.AutomaticEnableChannelEnabled, common.AutomaticDisableChannelEnabled = oldEnable, oldDisable
	})
}
func keyRecoveryChannel(t *testing.T, db *gorm.DB, url string, keys []string, status int, statuses map[int]int) *model.Channel {
	ch := &model.Channel{Id: 7040, Name: "key health recovery", Type: constant.ChannelTypeOpenAI, Key: strings.Join(keys, "\n"), BaseURL: common.GetPointer(url), Models: "gpt-4o", Group: "default", Status: status, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: len(keys), MultiKeyStatusList: statuses, MultiKeyDisabledReason: map[int]string{}, MultiKeyDisabledTime: map[int]int64{}}}
	for index := range statuses {
		ch.ChannelInfo.MultiKeyDisabledReason[index] = "prior failure"
		ch.ChannelInfo.MultiKeyDisabledTime[index] = 123
	}
	require.NoError(t, db.Create(ch).Error)
	require.NoError(t, db.Create(&model.Ability{ChannelId: ch.Id, Group: "default", Model: "gpt-4o", Enabled: status == common.ChannelStatusEnabled}).Error)
	return ch
}
func writeRecoverySuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"id":"health_probe","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"healthy"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
}
func assertRecoveryNoWalletDebit(t *testing.T, db *gorm.DB, user model.User, token model.Token) {
	waitCancellationHostWork(t)
	var actualUser model.User
	var actualToken model.Token
	require.NoError(t, db.First(&actualUser, user.Id).Error)
	require.NoError(t, db.First(&actualToken, token.Id).Error)
	assert.Equal(t, user.Quota, actualUser.Quota)
	assert.Equal(t, user.UsedQuota, actualUser.UsedQuota)
	assert.Equal(t, token.RemainQuota, actualToken.RemainQuota)
	assert.Equal(t, token.UsedQuota, actualToken.UsedQuota)
	var receipts []model.SubscriptionPreConsumeRecord
	require.NoError(t, db.Find(&receipts).Error)
	assert.Empty(t, receipts, "channel health never opens a wallet/subscription billing session")
}

func TestChannelHealthRecoveryProbesOnlyAutoDisabledKeysAndKeepsProductionSelectionSafe(t *testing.T) {
	cases := []struct {
		name    string
		keys    []string
		status  int
		states  map[int]int
		failed  map[string]bool
		calls   []string
		enabled int
	}{
		{"all_auto_disabled", []string{"A", "B"}, common.ChannelStatusAutoDisabled, map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled}, nil, []string{"A", "B"}, 2},
		{"mixed", []string{"enabled", "A", "B", "manual"}, common.ChannelStatusEnabled, map[int]int{1: common.ChannelStatusAutoDisabled, 2: common.ChannelStatusAutoDisabled, 3: common.ChannelStatusManuallyDisabled}, map[string]bool{"B": true}, []string{"enabled"}, 0},
		{"all_failed", []string{"A", "B"}, common.ChannelStatusAutoDisabled, map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled}, map[string]bool{"A": true, "B": true}, []string{"A", "B"}, 0},
		{"manual_channel", []string{"A"}, common.ChannelStatusManuallyDisabled, map[int]int{0: common.ChannelStatusAutoDisabled}, nil, nil, 0},
		{"manual_keys", []string{"manual"}, common.ChannelStatusAutoDisabled, map[int]int{0: common.ChannelStatusManuallyDisabled}, nil, nil, 0},
		{"duplicate_identity", []string{"A", "A"}, common.ChannelStatusAutoDisabled, map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled}, nil, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, user, token, _ := cancellationHostFixture(t, "openai")
			keyRecoveryPolicy(t)
			var mu sync.Mutex
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				mu.Lock()
				calls = append(calls, key)
				mu.Unlock()
				if tc.failed[key] {
					w.WriteHeader(503)
					_, _ = io.WriteString(w, `{"error":{"message":"probe failed","type":"server_error"}}`)
					return
				}
				writeRecoverySuccess(w)
			}))
			defer server.Close()
			ch := keyRecoveryChannel(t, db, server.URL, tc.keys, tc.status, tc.states)
			if tc.status == common.ChannelStatusAutoDisabled {
				_, _, err := ch.GetNextEnabledKey()
				assert.NotNil(t, err, "ordinary selection never probes disabled keys")
			}
			summary := testChannelForHealthCheck(context.Background(), ch, user.Id, true, 100000)
			mu.Lock()
			assert.Equal(t, tc.calls, calls)
			mu.Unlock()
			assert.Equal(t, len(tc.calls), summary.Tested)
			assert.Equal(t, tc.enabled, summary.Enabled)
			current, err := model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			for i, key := range tc.keys {
				old, has := tc.states[i]
				if !has {
					old = common.ChannelStatusEnabled
				}
				actual, has := current.ChannelInfo.MultiKeyStatusList[i]
				if !has {
					actual = common.ChannelStatusEnabled
				}
				expected := old
				if old == common.ChannelStatusAutoDisabled && !tc.failed[key] && tc.enabled > 0 {
					expected = common.ChannelStatusEnabled
				}
				assert.Equal(t, expected, actual)
				if expected == common.ChannelStatusEnabled && old == common.ChannelStatusAutoDisabled {
					assert.NotContains(t, current.ChannelInfo.MultiKeyDisabledReason, i)
					assert.NotContains(t, current.ChannelInfo.MultiKeyDisabledTime, i)
				}
			}
			if tc.enabled > 0 {
				assert.Equal(t, common.ChannelStatusEnabled, current.Status)
			} else {
				assert.Equal(t, tc.status, current.Status)
			}
			assertRecoveryNoWalletDebit(t, db, user, token)
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			for _, log := range logs {
				assert.Equal(t, 18, log.Quota)
				var other map[string]interface{}
				require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
				admin := other["admin_info"].(map[string]interface{})
				assert.Equal(t, true, admin["is_multi_key"])
				assert.NotContains(t, log.Other, "Bearer A")
				assert.NotContains(t, log.Other, "Bearer B")
			}
		})
	}
}

func TestChannelHealthRecoveryRevalidatesIdentityAndManualStateAfterProbe(t *testing.T) {
	for _, edit := range []string{"manual_channel", "manual_key", "reorder", "replace", "cancel"} {
		t.Run(edit, func(t *testing.T) {
			db, user, token, _ := cancellationHostFixture(t, "openai")
			keyRecoveryPolicy(t)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer A", r.Header.Get("Authorization"))
				close(entered)
				select {
				case <-release:
					writeRecoverySuccess(w)
				case <-r.Context().Done():
				}
			}))
			t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
			ch := keyRecoveryChannel(t, db, server.URL, []string{"A", "B"}, common.ChannelStatusAutoDisabled, map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusManuallyDisabled})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan channelTestSummary, 1)
			go func() { done <- testChannelForHealthCheck(ctx, ch, user.Id, true, 100000) }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("health probe did not dispatch auto-disabled credential")
			}
			current, err := model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			current.Name = "admin concurrent configuration"
			switch edit {
			case "manual_channel":
				current.Status = common.ChannelStatusManuallyDisabled
			case "manual_key":
				current.ChannelInfo.MultiKeyStatusList[0] = common.ChannelStatusManuallyDisabled
			case "reorder":
				current.Key = "B\nA"
				current.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusAutoDisabled}
				current.ChannelInfo.MultiKeyDisabledReason = map[int]string{0: "manual B", 1: "auto A"}
			case "replace":
				current.Key = "C\nB"
			case "cancel":
				cancel()
			}
			require.NoError(t, db.Save(current).Error)
			once.Do(func() { close(release) })
			var summary channelTestSummary
			select {
			case summary = <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("gated key recovery did not finish")
			}
			actual, err := model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			assert.Equal(t, "admin concurrent configuration", actual.Name)
			if edit == "reorder" {
				assert.Equal(t, 1, summary.Enabled)
				assert.Equal(t, common.ChannelStatusEnabled, actual.Status)
				assert.Equal(t, common.ChannelStatusManuallyDisabled, actual.ChannelInfo.MultiKeyStatusList[0])
				assert.NotContains(t, actual.ChannelInfo.MultiKeyStatusList, 1)
			} else {
				assert.Zero(t, summary.Enabled)
				assert.Equal(t, current.Status, actual.Status)
				assert.Equal(t, current.Key, actual.Key)
				assert.Equal(t, current.ChannelInfo.MultiKeyStatusList, actual.ChannelInfo.MultiKeyStatusList)
			}
			assertRecoveryNoWalletDebit(t, db, user, token)
		})
	}
}

func TestChannelExplicitRecoveryProbePreservesClaudeStreamDTOAndAuditOnlyFee(t *testing.T) {
	db, user, token, _ := cancellationHostFixture(t, "openai")
	keyRecoveryPolicy(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "A", r.Header.Get("x-api-key"))
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, true, body["stream"])
		assert.Equal(t, "/v1/messages", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range []string{`{"type":"message_start","message":{"id":"msg_probe","type":"message","role":"assistant","model":"gpt-4o","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"healthy"}}`, `{"type":"content_block_stop","index":0}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`, `{"type":"message_stop"}`} {
			_, _ = io.WriteString(w, "data: "+frame+"\n\n")
		}
	}))
	defer server.Close()
	ch := keyRecoveryChannel(t, db, server.URL, []string{"A"}, common.ChannelStatusAutoDisabled, map[int]int{0: common.ChannelStatusAutoDisabled})
	ch.Type = constant.ChannelTypeAnthropic
	require.NoError(t, db.Save(ch).Error)
	key := "A"
	result := testChannelWithKeyProbe(context.Background(), ch, user.Id, "gpt-4o", "", true, &key)
	require.NoError(t, result.localErr)
	require.Nil(t, result.newAPIError)
	assertRecoveryNoWalletDebit(t, db, user, token)
	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.True(t, logs[0].IsStream)
	assert.Equal(t, 10, logs[0].PromptTokens)
	assert.Equal(t, 2, logs[0].CompletionTokens)
	assert.Equal(t, 18, logs[0].Quota)
}
