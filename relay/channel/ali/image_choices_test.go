package ali

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAliImageChoicesPreserveEveryImageAndPrompt(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		want      []dto.ImageData
		wantRatio float64
	}{
		{
			name:      "one choice two images",
			body:      `{"output":{"choices":[{"message":{"content":[{"image":"https://images.example/first"},{"image":"https://images.example/second"}]}}]},"usage":{"image_count":2}}`,
			want:      []dto.ImageData{{Url: "https://images.example/first"}, {Url: "https://images.example/second"}},
			wantRatio: 2,
		},
		{
			name:      "choice order mixed encodings and shared last text",
			body:      `{"output":{"choices":[{"message":{"content":[{"text":"first prompt"},{"image":"https://images.example/first"},{"image":"Zmlyc3Q="},{"text":"last prompt"}]}},{"message":{"content":[{"text":"no image"}]}},{"message":{"content":[{"text":"next prompt"},{"image":"c2Vjb25k","text":"ignored alongside image"},{"image":"https://images.example/second"},{"text":""}]}}]},"usage":{"image_count":7}}`,
			want:      []dto.ImageData{{Url: "https://images.example/first", RevisedPrompt: "last prompt"}, {B64Json: "Zmlyc3Q=", RevisedPrompt: "last prompt"}, {B64Json: "c2Vjb25k", RevisedPrompt: "next prompt"}, {Url: "https://images.example/second", RevisedPrompt: "next prompt"}},
			wantRatio: 7,
		},
		{
			name:      "text only creates no image",
			body:      `{"output":{"choices":[{"message":{"content":[{"text":"no image"},{}]}},{"message":{"content":[]}}]}}`,
			wantRatio: 9,
		},
		{
			name:      "single image stays compatible",
			body:      `{"output":{"choices":[{"message":{"content":[{"text":"before"},{"image":"https://images.example/one"},{"text":"after"}]}}]},"usage":{"image_count":1}}`,
			want:      []dto.ImageData{{Url: "https://images.example/one", RevisedPrompt: "after"}},
			wantRatio: 1,
		},
		{
			name:      "missing usage falls back to all delivered images",
			body:      `{"output":{"choices":[{"message":{"content":[{"image":"https://images.example/first"},{"image":"https://images.example/second"}]}}]}}`,
			want:      []dto.ImageData{{Url: "https://images.example/first"}, {Url: "https://images.example/second"}},
			wantRatio: 2,
		},
		{
			name:      "legacy results retain precedence and data",
			body:      `{"output":{"results":[{"url":"https://images.example/legacy-first","b64_image":"bGVnYWN5"},{"url":"https://images.example/legacy-second"}],"choices":[{"message":{"content":[{"image":"https://images.example/ignored"}]}}]},"usage":{"image_count":2}}`,
			want:      []dto.ImageData{{Url: "https://images.example/legacy-first", B64Json: "bGVnYWN5"}, {Url: "https://images.example/legacy-second"}},
			wantRatio: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			requestN := uint(9)
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeImagesGenerations, StartTime: time.Unix(123, 0), Request: &dto.ImageRequest{N: &requestN, ResponseFormat: "url"}}
			info.PriceData.AddOtherRatio("n", 9)
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tt.body))}
			apiErr, usage := aliImageHandler(&Adaptor{IsSyncImageModel: true}, c, resp, info)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			var got dto.ImageResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &got))
			assert.Equal(t, tt.want, got.Data)
			assert.Equal(t, int64(123), got.Created)
			assert.JSONEq(t, tt.body, string(got.Metadata))
			assert.Equal(t, tt.wantRatio, info.PriceData.OtherRatios()["n"])
		})
	}
}

func TestAliAsyncImageChoicesPreserveEveryImage(t *testing.T) {
	const completed = `{"output":{"task_id":"local-task","task_status":"SUCCEEDED","choices":[{"message":{"content":[{"image":"https://images.example/async-first"},{"image":"https://images.example/async-second"},{"text":"async prompt"}]}}]},"usage":{"image_count":2}}`
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/tasks/local-task", r.URL.Path)
		assert.Equal(t, "Bearer local-fixture-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, completed)
	}))
	t.Cleanup(server.Close)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestN := uint(2)
	info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeImagesGenerations, StartTime: time.Unix(123, 0), Request: &dto.ImageRequest{N: &requestN, ResponseFormat: "url"}, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "local-fixture-key"}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"output":{"task_id":"local-task","task_status":"PENDING"}}`))}
	apiErr, usage := aliImageHandler(&Adaptor{}, c, resp, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	var got dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &got))
	assert.Equal(t, []dto.ImageData{{Url: "https://images.example/async-first", RevisedPrompt: "async prompt"}, {Url: "https://images.example/async-second", RevisedPrompt: "async prompt"}}, got.Data)
	assert.JSONEq(t, completed, string(got.Metadata))
	assert.Equal(t, float64(2), info.PriceData.OtherRatios()["n"])
	assert.Equal(t, int32(1), polls.Load())
}

func TestAliImageChoicesBase64DeliveryIsCompleteOrFails(t *testing.T) {
	originalDebug := common.DebugEnabled
	common.DebugEnabled = true
	t.Cleanup(func() { common.DebugEnabled = originalDebug })
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/failed":
			w.WriteHeader(http.StatusBadGateway)
		case "/oversized":
			w.Header().Set("Content-Length", "1048577")
		default:
			_, _ = io.WriteString(w, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	*fetchSetting = system_setting.FetchSetting{EnableSSRFProtection: true, AllowPrivateIp: true}
	t.Cleanup(func() { *fetchSetting = originalFetchSetting })
	originalLimit := rootconstant.MaxFileDownloadMB
	rootconstant.MaxFileDownloadMB = 1
	t.Cleanup(func() { rootconstant.MaxFileDownloadMB = originalLimit })
	service.InitHttpClient()
	t.Cleanup(service.GetHttpClient().CloseIdleConnections)
	t.Cleanup(service.GetSSRFProtectedHTTPClient().CloseIdleConnections)

	tests := []struct {
		name          string
		secondPath    string
		blockPrivate  bool
		wantError     bool
		wantDownloads int32
	}{
		{name: "all images", secondPath: "/second", wantDownloads: 2},
		{name: "later download failure cannot silently return first image", secondPath: "/failed", wantError: true, wantDownloads: 2},
		{name: "private image URL remains blocked", secondPath: "/second", blockPrivate: true, wantError: true},
		{name: "size limit remains enforced", secondPath: "/oversized", wantError: true, wantDownloads: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			downloads.Store(0)
			fetchSetting.AllowPrivateIp = !tt.blockPrivate
			firstURL := server.URL + "/first?signature=private-fixture"
			secondURL := server.URL + tt.secondPath + "?signature=private-fixture"
			body := fmt.Sprintf(`{"output":{"choices":[{"message":{"content":[{"text":"shared prompt"},{"image":%q},{"image":"aW5saW5l"},{"image":%q}]}}]},"usage":{"image_count":3}}`, firstURL, secondURL)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeImagesGenerations, StartTime: time.Unix(123, 0), Request: &dto.ImageRequest{ResponseFormat: "b64_json"}}
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			var logs bytes.Buffer
			common.LogWriterMu.Lock()
			originalErrorWriter := gin.DefaultErrorWriter
			gin.DefaultErrorWriter = &logs
			common.LogWriterMu.Unlock()
			t.Cleanup(func() {
				common.LogWriterMu.Lock()
				gin.DefaultErrorWriter = originalErrorWriter
				common.LogWriterMu.Unlock()
			})
			apiErr, usage := aliImageHandler(&Adaptor{IsSyncImageModel: true}, c, resp, info)
			assert.Equal(t, tt.wantDownloads, downloads.Load())
			if tt.wantError {
				require.NotNil(t, apiErr)
				assert.Nil(t, usage)
				assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
				assert.True(t, types.IsSkipRetryError(apiErr), "generation already succeeded upstream; downloading must not trigger another generation")
				assert.NotContains(t, apiErr.Error(), server.URL)
				assert.NotContains(t, apiErr.Error(), "private-fixture")
				assert.NotContains(t, logs.String(), "private-fixture", "debug logging must not expose signed image URLs")
				assert.NotContains(t, logs.String(), "aW5saW5l", "debug logging must not expose image contents")
				assert.Empty(t, recorder.Body.String(), "no partial success body before all image downloads finish")
				return
			}
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			var got dto.ImageResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &got))
			assert.Equal(t, []dto.ImageData{{Url: firstURL, B64Json: base64.StdEncoding.EncodeToString([]byte("/first")), RevisedPrompt: "shared prompt"}, {B64Json: "aW5saW5l", RevisedPrompt: "shared prompt"}, {Url: secondURL, B64Json: base64.StdEncoding.EncodeToString([]byte("/second")), RevisedPrompt: "shared prompt"}}, got.Data)
			assert.Equal(t, float64(3), info.PriceData.OtherRatios()["n"])
		})
	}
}
