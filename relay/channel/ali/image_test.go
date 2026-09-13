package ali

import (
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
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAliImageHandlerHonorsRequestResponseFormat(t *testing.T) {
	imageBytes := []byte("ali-image")
	var downloads atomic.Int32
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		w.Header().Set("Content-Type", "image/png")
		if r.URL.Path == "/oversized" {
			w.Header().Set("Content-Length", "1048577")
		}
		_, _ = w.Write(imageBytes)
	}))
	t.Cleanup(imageServer.Close)

	fetchSetting := system_setting.GetFetchSetting()
	require.NotNil(t, fetchSetting)
	originalFetchSetting := *fetchSetting
	*fetchSetting = system_setting.FetchSetting{
		EnableSSRFProtection: true,
		AllowPrivateIp:       true,
	}
	t.Cleanup(func() { *fetchSetting = originalFetchSetting })
	originalMaxFileDownloadMB := rootconstant.MaxFileDownloadMB
	rootconstant.MaxFileDownloadMB = 1
	t.Cleanup(func() { rootconstant.MaxFileDownloadMB = originalMaxFileDownloadMB })
	service.InitHttpClient()
	t.Cleanup(service.GetHttpClient().CloseIdleConnections)
	t.Cleanup(service.GetSSRFProtectedHTTPClient().CloseIdleConnections)

	tests := []struct {
		name           string
		responseFormat string
		choices        bool
		blockPrivate   bool
		oversized      bool
		wantBase64     bool
		wantDownloads  int32
	}{
		{name: "results base64", responseFormat: "b64_json", wantBase64: true, wantDownloads: 1},
		{name: "results url", responseFormat: "url"},
		{name: "results default"},
		{name: "choices base64", responseFormat: "b64_json", choices: true, wantBase64: true, wantDownloads: 1},
		{name: "choices url", responseFormat: "url", choices: true},
		{name: "choices default", choices: true},
		{name: "base64 preserves SSRF protection", responseFormat: "b64_json", blockPrivate: true},
		{name: "base64 preserves download limit", responseFormat: "b64_json", oversized: true, wantDownloads: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			downloads.Store(0)
			fetchSetting.AllowPrivateIp = !tt.blockPrivate
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			info := &relaycommon.RelayInfo{
				RelayMode: constant.RelayModeImagesGenerations,
				StartTime: time.Unix(1, 0),
				Request:   &dto.ImageRequest{ResponseFormat: tt.responseFormat},
			}
			imageURL := imageServer.URL
			if tt.oversized {
				imageURL += "/oversized"
			}
			responseBody := fmt.Sprintf(`{"output":{"results":[{"url":%q}]}}`, imageURL)
			if tt.choices {
				responseBody = fmt.Sprintf(`{"output":{"choices":[{"message":{"content":[{"image":%q}]}}]}}`, imageURL)
			}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}

			newAPIError, usage := aliImageHandler(&Adaptor{IsSyncImageModel: true}, c, resp, info)
			require.Nil(t, newAPIError)
			require.NotNil(t, usage)

			var imageResponse dto.ImageResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &imageResponse))
			assert.Equal(t, tt.wantDownloads, downloads.Load())
			if tt.blockPrivate || tt.oversized {
				assert.Empty(t, imageResponse.Data)
				return
			}
			require.Len(t, imageResponse.Data, 1)
			assert.Equal(t, imageURL, imageResponse.Data[0].Url)
			if tt.wantBase64 {
				assert.Equal(t, base64.StdEncoding.EncodeToString(imageBytes), imageResponse.Data[0].B64Json)
			} else {
				assert.Empty(t, imageResponse.Data[0].B64Json)
			}
		})
	}
}
