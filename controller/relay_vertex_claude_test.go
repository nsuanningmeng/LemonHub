package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const vertexClaudeBody = `{"model":"gpt-4o","max_tokens":16,"system":"system text","messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}]},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{"city":"Paris"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"found"}]}],"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]}`
const vertexGeminiResponse = `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK"},{"functionCall":{"name":"lookup","args":{"city":"Paris"}}},{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":10,"cachedContentTokenCount":4,"candidatesTokenCount":3,"totalTokenCount":13}}`

func TestVertexGeminiClaudeProductionRoundTrip(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			db, user, token, _ := mediaConversionBillingFixture(t)
			oldCache, oldCompletion := ratio_setting.CacheRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(oldCache))
				require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
			})
			require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"gpt-4o":0.25}`))
			require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-4o":2}`))
			var wire map[string]any
			var path, authorization, project string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, common.Unmarshal(raw, &wire))
				path = r.URL.RequestURI()
				authorization = r.Header.Get("Authorization")
				project = r.Header.Get("x-goog-user-project")
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"OK\"}]},\"index\":0}]}\n\n")
					_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.Replace(vertexGeminiResponse, `{"text":"OK"},`, "", 1))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, vertexGeminiResponse)
				}
			}))
			t.Cleanup(upstream.Close)
			channel := model.Channel{Id: 671501, Type: constant.ChannelTypeVertexAi, Name: "vertex-local", Key: `{"project_id":"fixture-project","client_email":"fixture@example.invalid"}`, BaseURL: common.GetPointer(upstream.URL), Other: "global", Status: common.ChannelStatusEnabled, ModelMapping: common.GetPointer(`{"gpt-4o":"gemini-2.5-flash"}`)}
			key := fmt.Sprintf("access-token-%d", channel.Id)
			require.False(t, vertex.Cache.SetDefault(key, "fixture-access-token"))
			t.Cleanup(func() { vertex.Cache.DeleteIf(func(k string) bool { return k == key }) })
			require.NoError(t, db.Create(&channel).Error)
			body := vertexClaudeBody
			if stream {
				body = strings.TrimSuffix(body, "}") + `,"stream":true}`
			}
			response, _ := runProtocolConversionRelay(t, user, token, channel, "/v1/messages", body, types.RelayFormatClaude)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			expectedPath := "/v1/projects/fixture-project/locations/global/publishers/google/models/gemini-2.5-flash:generateContent"
			if stream {
				expectedPath = "/v1/projects/fixture-project/locations/global/publishers/google/models/gemini-2.5-flash:streamGenerateContent?alt=sse"
			}
			assert.Equal(t, expectedPath, path)
			assert.Equal(t, "Bearer fixture-access-token", authorization)
			assert.Equal(t, "fixture-project", project)
			assert.NotContains(t, wire, "anthropic_version")
			assert.NotContains(t, wire, "messages")
			assert.NotContains(t, wire, "max_tokens")
			require.Contains(t, wire, "contents")
			assert.Equal(t, float64(16), wire["generationConfig"].(map[string]any)["maxOutputTokens"])
			encoded, err := common.Marshal(wire)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), "inlineData")
			assert.Contains(t, string(encoded), "functionResponse")
			assert.Contains(t, string(encoded), "found")
			assert.NotContains(t, string(encoded), `"id":"call_1"`, "Vertex compatibility strips unsupported functionResponse.id")
			if !stream {
				var msg struct {
					Type, Role, Model, StopReason string
					Content                       []map[string]any
					Usage                         struct {
						InputTokens  int `json:"input_tokens"`
						CacheRead    int `json:"cache_read_input_tokens"`
						OutputTokens int `json:"output_tokens"`
					}
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &msg))
				assert.Equal(t, "message", msg.Type)
				assert.Equal(t, "assistant", msg.Role)
				assert.Equal(t, "gpt-4o", msg.Model)
				assert.Contains(t, response.Body.String(), "lookup")
				assert.Contains(t, response.Body.String(), "Paris")
				assert.Contains(t, response.Body.String(), "aGVsbG8=")
				assert.Equal(t, 6, msg.Usage.InputTokens, "Claude input tokens exclude cached read")
				assert.Equal(t, 4, msg.Usage.CacheRead)
				assert.Equal(t, 3, msg.Usage.OutputTokens)
			} else {
				text := response.Body.String()
				assert.Equal(t, 1, strings.Count(text, "event: message_start\n"))
				assert.Equal(t, 1, strings.Count(text, "event: message_stop\n"))
				assert.Contains(t, text, "event: content_block_stop")
				assert.Contains(t, text, "input_json_delta")
				assert.Contains(t, text, "Paris")
				assert.Contains(t, text, `"output_tokens":3`)
				assert.Contains(t, text, `"cache_read_input_tokens":4`)
				assert.NotContains(t, text, "[DONE]")
			}
			for _, secret := range []string{"fixture-access-token", "fixture-project", "fixture@example.invalid"} {
				assert.NotContains(t, response.Body.String(), secret)
			}
			var log model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).First(&log).Error)
			assert.Equal(t, 10, log.PromptTokens)
			assert.Equal(t, 3, log.CompletionTokens)
			assert.Equal(t, 13, log.Quota, "6 fresh + 4*0.25 cached + 3*2 output")
			assert.Contains(t, log.Other, "billing-usage-gemini")
			assert.Contains(t, log.Other, `"cache_tokens":4`)
		})
	}
}

func TestVertexGeminiClaudeChannelTestUsesProductionDTO(t *testing.T) {
	db, user, _, _ := mediaConversionBillingFixture(t)
	var wire map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, common.Unmarshal(raw, &wire))
		if _, ok := wire["anthropic_version"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"wrong provider protocol"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, vertexGeminiResponse)
	}))
	t.Cleanup(upstream.Close)
	channel := model.Channel{Id: 671502, Type: constant.ChannelTypeVertexAi, Name: "vertex-probe", Key: `{"project_id":"fixture-project"}`, BaseURL: common.GetPointer(upstream.URL), Other: "global", Status: common.ChannelStatusEnabled, ModelMapping: common.GetPointer(`{"gpt-4o":"gemini-2.5-flash"}`)}
	key := fmt.Sprintf("access-token-%d", channel.Id)
	require.False(t, vertex.Cache.SetDefault(key, "fixture-access-token"))
	t.Cleanup(func() { vertex.Cache.DeleteIf(func(k string) bool { return k == key }) })
	require.NoError(t, db.Create(&channel).Error)
	result := testChannel(context.Background(), &channel, user.Id, "gpt-4o", string(constant.EndpointTypeAnthropic), false)
	require.NoError(t, result.localErr)
	require.Nil(t, result.newAPIError)
	assert.Contains(t, wire, "contents")
	assert.NotContains(t, wire, "anthropic_version")
	var final model.User
	require.NoError(t, db.First(&final, user.Id).Error)
	assert.Equal(t, user.Quota, final.Quota, "admin channel test doesn't debit wallet")
}

func TestVertexGeminiClaudeHTTPErrorRefundsPreConsume(t *testing.T) {
	db, user, token, _ := mediaConversionBillingFixture(t)
	common.RetryTimes = 0
	var preUser model.User
	var preToken model.Token
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.NoError(t, db.First(&preUser, user.Id).Error)
		require.NoError(t, db.First(&preToken, token.Id).Error)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":400,"message":"invalid Gemini request","status":"INVALID_ARGUMENT"}}`)
	}))
	t.Cleanup(upstream.Close)
	channel := model.Channel{Id: 671503, Type: constant.ChannelTypeVertexAi, Name: "vertex-error", Key: `{"project_id":"fixture-project"}`, BaseURL: common.GetPointer(upstream.URL), Other: "global", Status: common.ChannelStatusEnabled, ModelMapping: common.GetPointer(`{"gpt-4o":"gemini-2.5-flash"}`)}
	key := fmt.Sprintf("access-token-%d", channel.Id)
	require.False(t, vertex.Cache.SetDefault(key, "fixture-access-token"))
	t.Cleanup(func() { vertex.Cache.DeleteIf(func(k string) bool { return k == key }) })
	require.NoError(t, db.Create(&channel).Error)
	response, _ := runProtocolConversionRelay(t, user, token, channel, "/v1/messages", vertexClaudeBody, types.RelayFormatClaude)
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"type":"error"`)
	assert.Equal(t, int64(1), calls.Load())
	assert.Less(t, preUser.Quota, user.Quota, "upstream is reached after actual wallet pre-consume")
	assert.Less(t, preToken.RemainQuota, token.RemainQuota)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, service.WaitBillingRefunds(ctx))
	var finalUser model.User
	var finalToken model.Token
	require.NoError(t, db.First(&finalUser, user.Id).Error)
	require.NoError(t, db.First(&finalToken, token.Id).Error)
	assert.Equal(t, user.Quota, finalUser.Quota)
	assert.Equal(t, token.RemainQuota, finalToken.RemainQuota)
	var logs int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&logs).Error)
	assert.Zero(t, logs)
}

func localVertexServiceAccountChannel(t *testing.T, id int, baseURL string) model.Channel {
	t.Helper()
	channel := model.Channel{Id: id, Type: constant.ChannelTypeVertexAi, Name: "vertex-local", Key: `{"project_id":"fixture-project"}`, BaseURL: common.GetPointer(baseURL), Other: "global", Status: common.ChannelStatusEnabled, ModelMapping: common.GetPointer(`{"gpt-4o":"gemini-2.5-flash"}`)}
	key := fmt.Sprintf("access-token-%d", id)
	require.False(t, vertex.Cache.SetDefault(key, "fixture-access-token"))
	t.Cleanup(func() { vertex.Cache.DeleteIf(func(k string) bool { return k == key }) })
	return channel
}

func TestVertexGeminiClaudeFirstStreamToolRetainsArguments(t *testing.T) {
	db, user, token, _ := mediaConversionBillingFixture(t)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"functionCall\":{\"name\":\"lookup\",\"args\":{\"city\":\"Paris\"}}}]},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2,\"totalTokenCount\":3}}\n\n")
	}))
	t.Cleanup(upstream.Close)
	channel := localVertexServiceAccountChannel(t, 671504, upstream.URL)
	require.NoError(t, db.Create(&channel).Error)
	body := strings.TrimSuffix(vertexClaudeBody, "}") + `,"stream":true}`
	response, _ := runProtocolConversionRelay(t, user, token, channel, "/v1/messages", body, types.RelayFormatClaude)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "Paris", "first function-call response must not discard its argument JSON")
	assert.Equal(t, 1, strings.Count(response.Body.String(), `"type":"tool_use"`))
	assert.Equal(t, 1, strings.Count(response.Body.String(), "event: message_stop\n"))
}

func TestVertexGeminiClaudeInBandRefusalKeepsGeminiBilling(t *testing.T) {
	db, user, token, _ := mediaConversionBillingFixture(t)
	old := ratio_setting.GetCacheRatioCopy()
	require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"gpt-4o":0.25}`))
	t.Cleanup(func() { raw, _ := common.Marshal(old); _ = ratio_setting.UpdateCacheRatioByJSONString(string(raw)) })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":10,"cachedContentTokenCount":4,"candidatesTokenCount":0,"totalTokenCount":10}}`)
	}))
	t.Cleanup(upstream.Close)
	channel := localVertexServiceAccountChannel(t, 671506, upstream.URL)
	require.NoError(t, db.Create(&channel).Error)
	response, _ := runProtocolConversionRelay(t, user, token, channel, "/v1/messages", vertexClaudeBody, types.RelayFormatClaude)
	assert.Contains(t, response.Body.String(), `"type":"error"`)
	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0].Other, "billing-usage-gemini")
	assert.Equal(t, 7, logs[0].Quota)
}
