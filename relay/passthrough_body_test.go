package relay

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func passthroughTestContext(t *testing.T, payload []byte, claude bool, baseURL string, override map[string]any) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	path := "/v1/chat/completions"
	channelType := constant.ChannelTypeCustom
	info := &relaycommon.RelayInfo{OriginModelName: "client-model", RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions, Request: &dto.GeneralOpenAIRequest{Model: "client-model"}}
	if claude {
		path = "/v1/messages"
		channelType = constant.ChannelTypeAnthropic
		info.RelayFormat = types.RelayFormatClaude
		info.Request = &dto.ClaudeRequest{Model: "client-model", MaxTokens: common.GetPointer(uint(1024))}
	}
	info.RequestURLPath = path
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, baseURL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "configured-credential")
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, override)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "client-model")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	return c, info
}

func TestPassthroughHandlersApplyExplicitRawOverride(t *testing.T) {
	oldDisk := common.GetDiskCacheConfig()
	memoryConfig := oldDisk
	memoryConfig.Enabled = false
	common.SetDiskCacheConfig(memoryConfig)
	t.Cleanup(func() { common.SetDiskCacheConfig(oldDisk) })
	oldGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldGlobal })
	service.InitHttpClient()
	original := []byte(" {\n\"model\":\"client-model\",\"messages\":[{\"role\":\"user\",\"content\":[{\"type\":\"vendor_private\",\"data\":{\"large\":9007199254740993,\"literal\":\"a\\r\\nb\"}}]}],\"stream\":false,\"private_ext\":{\"keep\":1.2300,\"remove\":true}} ")
	for _, claude := range []bool{false, true} {
		for _, global := range []bool{false, true} {
			name := "chat/channel"
			if claude {
				name = "claude/channel"
			}
			if global {
				name += "/global"
			}
			t.Run(name, func(t *testing.T) {
				model_setting.GetGlobalSettings().PassThroughRequestEnabled = global
				var got []byte
				var headers http.Header
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got, _ = io.ReadAll(r.Body)
					headers = r.Header.Clone()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(418)
					w.Write([]byte(`{"error":{"message":"fixture stop","type":"invalid_request_error"}}`))
				}))
				defer upstream.Close()
				override := map[string]any{"reasoning_effort": "low", "operations": []any{
					map[string]any{"mode": "delete", "path": "private_ext.remove"},
					map[string]any{"mode": "set", "path": "stream", "value": true, "keep_origin": false},
					map[string]any{"mode": "set", "path": "private_ext.added", "value": "yes", "conditions": []any{map[string]any{"path": "model", "mode": "full", "value": "client-model"}}},
					map[string]any{"mode": "set_header", "path": "X-Override", "value": "applied"},
				}}
				c, info := passthroughTestContext(t, original, claude, upstream.URL, override)
				common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: !global})
				var apiErr *types.NewAPIError
				if claude {
					apiErr = ClaudeHelper(c, info)
				} else {
					apiErr = TextHelper(c, info)
				}
				require.NotNil(t, apiErr)
				require.Equal(t, 418, apiErr.StatusCode, "upstream fixture must be reached")
				assert.Equal(t, "low", gjson.GetBytes(got, "reasoning_effort").String())
				assert.True(t, gjson.GetBytes(got, "stream").Bool())
				assert.False(t, gjson.GetBytes(got, "private_ext.remove").Exists())
				assert.Equal(t, "yes", gjson.GetBytes(got, "private_ext.added").String())
				assert.Equal(t, "applied", headers.Get("X-Override"))
				assert.Equal(t, "9007199254740993", gjson.GetBytes(got, "messages.0.content.0.data.large").Raw)
				assert.Equal(t, "1.2300", gjson.GetBytes(got, "private_ext.keep").Raw)
				assert.Equal(t, "a\r\nb", gjson.GetBytes(got, "messages.0.content.0.data.literal").String())
				storage, err := common.GetBodyStorage(c)
				require.NoError(t, err)
				require.False(t, storage.IsDisk(), "verify immutable memory backing, not only disk copies")
				unchanged, err := storage.Bytes()
				require.NoError(t, err)
				assert.Equal(t, original, unchanged)
			})
		}
	}
}

type passthroughBytesCounter struct {
	common.BodyStorage
	bytesCalls int
}

func (s *passthroughBytesCounter) Bytes() ([]byte, error) {
	s.bytesCalls++
	return s.BodyStorage.Bytes()
}

func TestPassthroughReplayPreservesDiskBodyAndRetries(t *testing.T) {
	oldDisk := common.GetDiskCacheConfig()
	common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 64, Path: t.TempDir()})
	t.Cleanup(func() { common.SetDiskCacheConfig(oldDisk) })
	original := []byte(" {\n\"model\":\"client-model\",\"private\":\"seed\",\"large\":9007199254740993,\"blob\":\"" + strings.Repeat("x", 1<<20) + "\"} ")
	c, info := passthroughTestContext(t, original, false, "http://unused.invalid", nil)
	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	require.True(t, storage.IsDisk())
	counted := &passthroughBytesCounter{BodyStorage: storage}
	c.Set(common.KeyBodyStorage, counted)
	info.InitChannelMeta(c)
	for attempt := 0; attempt < 2; attempt++ {
		body, closer, apiErr := newPassthroughRequestBody(c, info)
		require.Nil(t, apiErr)
		require.Nil(t, closer)
		got, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.Equal(t, original, got)
		first, err := body.NewReader()
		require.NoError(t, err)
		defer first.Close()
		prefix := make([]byte, 7)
		_, err = io.ReadFull(first, prefix)
		require.NoError(t, err)
		second, err := body.NewReader()
		require.NoError(t, err)
		defer second.Close()
		replay, err := io.ReadAll(second)
		require.NoError(t, err)
		assert.Equal(t, original, replay)
		rest, err := io.ReadAll(first)
		require.NoError(t, err)
		assert.Equal(t, original, append(prefix, rest...))
	}
	assert.Zero(t, counted.bytesCalls, "plain passthrough must never materialize the disk payload")
	for _, suffix := range []string{"-first", "-retry"} {
		info.ParamOverride = map[string]any{"operations": []any{map[string]any{"mode": "append", "path": "private", "value": suffix}, map[string]any{"mode": "set", "path": "stream", "value": true}}}
		body, closer, apiErr := newPassthroughRequestBody(c, info)
		require.Nil(t, apiErr)
		require.NotNil(t, closer)
		outputStorage, ok := closer.(common.BodyStorage)
		require.True(t, ok)
		assert.True(t, outputStorage.IsDisk())
		prefix := make([]byte, 19)
		_, err = io.ReadFull(body, prefix)
		require.NoError(t, err)
		partialReplay, err := body.NewReader()
		require.NoError(t, err)
		fromStart, err := io.ReadAll(partialReplay)
		require.NoError(t, err)
		require.NoError(t, partialReplay.Close())
		rest, err := io.ReadAll(body)
		require.NoError(t, err)
		got := append(prefix, rest...)
		assert.Equal(t, got, fromStart, "transport replay starts from zero after a partial write")
		assert.Equal(t, "seed"+suffix, gjson.GetBytes(got, "private").String())
		assert.True(t, gjson.GetBytes(got, "stream").Bool())
		assert.Equal(t, "9007199254740993", gjson.GetBytes(got, "large").Raw)
		replay, err := body.NewReader()
		require.NoError(t, err)
		replayed, err := io.ReadAll(replay)
		require.NoError(t, err)
		require.NoError(t, replay.Close())
		assert.Equal(t, got, replayed)
		require.NoError(t, closer.Close())
		_, err = body.NewReader()
		assert.ErrorIs(t, err, common.ErrStorageClosed)
		unchanged, err := storage.Bytes()
		require.NoError(t, err)
		assert.Equal(t, original, unchanged)
	}
}

func TestPassthroughOverrideRejectsMalformedJSONWithoutLeakingBody(t *testing.T) {
	for _, raw := range []string{`{"private":"SYNTH_PRIVATE",`, `null`, `[]`, `"SYNTH_PRIVATE"`} {
		c, info := passthroughTestContext(t, []byte(raw), false, "http://unused.invalid", nil)
		info.InitChannelMeta(c)
		body, closer, apiErr := newPassthroughRequestBody(c, info)
		require.Nil(t, apiErr)
		require.Nil(t, closer)
		got, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.Equal(t, raw, string(got), "no-override adds no parsing policy")
		info.ParamOverride = map[string]any{"temperature": 0}
		body, closer, apiErr = newPassthroughRequestBody(c, info)
		assert.Nil(t, body)
		assert.Nil(t, closer)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
		assert.True(t, types.IsSkipRetryError(apiErr))
		assert.NotContains(t, apiErr.Error(), "SYNTH_PRIVATE")
	}
}

func TestPassthroughOverrideKeepsExplicitAdminErrorsAndHeaderRules(t *testing.T) {
	c, info := passthroughTestContext(t, []byte(`{"model":"client-model","stream":false}`), false, "http://unused.invalid", nil)
	info.InitChannelMeta(c)
	info.HeadersOverride = map[string]any{"Authorization": "Bearer configured", "X-Keep": "yes", "X-Delete": "old"}
	info.RequestHeaders = map[string]string{"Authorization": "Bearer client-secret"}
	info.ParamOverride = map[string]any{"operations": []any{
		map[string]any{"mode": "set_header", "path": "Authorization", "value": "Bearer admin-override"},
		map[string]any{"mode": "delete_header", "path": "X-Delete"},
		map[string]any{"mode": "set", "path": "stream", "value": true},
	}}
	body, closer, apiErr := newPassthroughRequestBody(c, info)
	require.Nil(t, apiErr)
	defer closer.Close()
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.True(t, gjson.GetBytes(got, "stream").Bool())
	assert.Equal(t, "Bearer admin-override", info.RuntimeHeadersOverride["authorization"])
	assert.Equal(t, "yes", info.RuntimeHeadersOverride["x-keep"])
	assert.NotContains(t, info.RuntimeHeadersOverride, "x-delete")
	assert.NotContains(t, string(got), "client-secret")
	info.ParamOverride = map[string]any{"operations": []any{map[string]any{"mode": "return_error", "value": map[string]any{"message": "Configured administrator diagnostic", "status_code": 422, "code": "operator_rule", "skip_retry": false}}}}
	_, _, apiErr = newPassthroughRequestBody(c, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, 422, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "Configured administrator diagnostic")
	assert.False(t, types.IsSkipRetryError(apiErr))
	info.ParamOverride = map[string]any{"operations": []any{map[string]any{"mode": "regex_replace", "path": "model", "value": map[string]any{"pattern": "[SYNTH_CONFIG_PRIVATE", "replacement": "x"}}}}
	_, _, apiErr = newPassthroughRequestBody(c, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeChannelParamOverrideInvalid, apiErr.GetErrorCode())
	assert.NotContains(t, apiErr.Error(), "SYNTH_CONFIG_PRIVATE")
	assert.True(t, types.IsSkipRetryError(apiErr))
}

func TestPassthroughBodyRetainsExistingRequestLimit(t *testing.T) {
	oldMax := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = oldMax })
	c, info := passthroughTestContext(t, bytes.Repeat([]byte("x"), (1<<20)+1), false, "http://unused.invalid", nil)
	info.InitChannelMeta(c)
	body, closer, apiErr := newPassthroughRequestBody(c, info)
	assert.Nil(t, body)
	assert.Nil(t, closer)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeReadRequestBodyFailed, apiErr.GetErrorCode())
	assert.True(t, types.IsSkipRetryError(apiErr))
}

func TestPassthroughAdditionalJSONHandlersComposeOverrides(t *testing.T) {
	oldGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldGlobal })
	service.InitHttpClient()
	for _, kind := range []string{"responses", "gemini", "rerank", "image"} {
		for _, global := range []bool{false, true} {
			for _, withOverride := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/global_%t/override_%t", kind, global, withOverride), func(t *testing.T) {
					model_setting.GetGlobalSettings().PassThroughRequestEnabled = global
					var prefix string
					var request dto.Request
					var handler func(*gin.Context, *relaycommon.RelayInfo) *types.NewAPIError
					var mode int
					var format types.RelayFormat
					channelType := constant.ChannelTypeOpenAI
					path := "/v1/responses"
					switch kind {
					case "responses":
						prefix = `"model":"client-model","input":"hello"`
						request = &dto.OpenAIResponsesRequest{}
						handler = ResponsesHelper
						mode = relayconstant.RelayModeResponses
						format = types.RelayFormatOpenAIResponses
					case "gemini":
						prefix = `"contents":[{"role":"user","parts":[{"text":"hello"}]}]`
						request = &dto.GeminiChatRequest{}
						handler = GeminiHelper
						mode = relayconstant.RelayModeGemini
						format = types.RelayFormatGemini
						channelType = constant.ChannelTypeGemini
						path = "/v1beta/models/client-model:generateContent"
					case "rerank":
						prefix = `"model":"client-model","query":"hello","documents":["world"]`
						request = &dto.RerankRequest{}
						handler = RerankHelper
						mode = relayconstant.RelayModeRerank
						format = types.RelayFormatRerank
						path = "/v1/rerank"
					case "image":
						prefix = `"model":"client-model","prompt":"hello","n":1`
						request = &dto.ImageRequest{}
						handler = ImageHelper
						mode = relayconstant.RelayModeImagesGenerations
						format = types.RelayFormatOpenAIImage
						path = "/v1/images/generations"
					}
					raw := []byte(" {\n" + prefix + `,"private_ext":{"large":9007199254740993,"fraction":1.2300,"remove":true}} `)
					require.NoError(t, common.Unmarshal(raw, request))
					var got []byte
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						got, _ = io.ReadAll(r.Body)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(418)
						w.Write([]byte(`{"error":{"message":"fixture stop","type":"invalid_request_error"}}`))
					}))
					defer upstream.Close()
					var override map[string]any
					if withOverride {
						override = map[string]any{"private_added": "applied", "operations": []any{map[string]any{"mode": "delete", "path": "private_ext.remove"}}}
					}
					c, info := passthroughTestContext(t, raw, false, upstream.URL, override)
					c.Request.URL.Path = path
					info.RequestURLPath = path
					info.Request = request
					info.RelayMode = mode
					info.RelayFormat = format
					common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
					common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: !global})
					apiErr := handler(c, info)
					require.NotNil(t, apiErr)
					require.Equal(t, 418, apiErr.StatusCode, apiErr.Error())
					if withOverride {
						assert.Equal(t, "applied", gjson.GetBytes(got, "private_added").String())
						assert.False(t, gjson.GetBytes(got, "private_ext.remove").Exists())
					} else {
						assert.Equal(t, raw, got)
					}
					assert.Equal(t, "9007199254740993", gjson.GetBytes(got, "private_ext.large").Raw)
					assert.Equal(t, "1.2300", gjson.GetBytes(got, "private_ext.fraction").Raw)
					storage, err := common.GetBodyStorage(c)
					require.NoError(t, err)
					original, err := storage.Bytes()
					require.NoError(t, err)
					assert.Equal(t, raw, original)
				})
			}
		}
	}
}

func TestPassthroughMultipartImageRetainsOriginalBodyWithJSONOverrideConfigured(t *testing.T) {
	oldGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldGlobal })
	service.InitHttpClient()
	var payload bytes.Buffer
	form := multipart.NewWriter(&payload)
	require.NoError(t, form.WriteField("model", "client-model"))
	file, err := form.CreateFormFile("image", "sample.png")
	require.NoError(t, err)
	_, err = file.Write([]byte{0, 1, 2, 3, 255})
	require.NoError(t, err)
	require.NoError(t, form.Close())
	raw := bytes.Clone(payload.Bytes())
	var got []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(418)
		w.Write([]byte(`{"error":{"message":"fixture stop","type":"invalid_request_error"}}`))
	}))
	defer upstream.Close()
	c, info := passthroughTestContext(t, raw, false, upstream.URL, map[string]any{"model": "must-not-patch-multipart"})
	c.Request.Header.Set("Content-Type", form.FormDataContentType())
	c.Request.URL.Path = "/v1/images/edits"
	info.RequestURLPath = c.Request.URL.Path
	info.RelayMode = relayconstant.RelayModeImagesEdits
	info.RelayFormat = types.RelayFormatOpenAIImage
	info.Request = &dto.ImageRequest{Model: "client-model"}
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: true})
	apiErr := ImageHelper(c, info)
	require.NotNil(t, apiErr)
	require.Equal(t, 418, apiErr.StatusCode, apiErr.Error())
	assert.Equal(t, raw, got)
}
