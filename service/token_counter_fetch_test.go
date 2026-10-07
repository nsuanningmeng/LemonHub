package service

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func configureB17MediaEstimator(t *testing.T) {
	t.Helper()
	configureSSRFTestFetchSetting(t)
	count, media, nonstream := constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream
	maxDownload, worker := constant.MaxFileDownloadMB, system_setting.WorkerUrl
	t.Cleanup(func() {
		constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = count, media, nonstream
		constant.MaxFileDownloadMB, system_setting.WorkerUrl = maxDownload, worker
	})
	constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = true, true, true
	constant.MaxFileDownloadMB, system_setting.WorkerUrl = 1, ""
}

func b17EstimatorContext(t *testing.T, model string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, model)
	t.Cleanup(func() { CleanupFileSources(c) })
	return c
}

func b17EstimatorMeta(fileType types.FileType, rawURL, detail string) *types.TokenCountMeta {
	return &types.TokenCountMeta{
		TokenType: types.TokenTypeTextNumber,
		Files: []*types.FileMeta{{
			FileType: fileType,
			Source:   types.NewURLFileSource(rawURL),
			Detail:   detail,
		}},
	}
}

// Keep production URL, redirect and dial-time checks enabled. Only the final
// socket destination is replaced after the protected dialer validates the public
// address; all HTTP and file loading still run against a real local server.
func b17MediaHTTPFixture(t *testing.T) (*atomic.Int32, *atomic.Int32) {
	t.Helper()
	var pngData bytes.Buffer
	require.NoError(t, png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	requests, dials := &atomic.Int32{}, &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://127.0.0.1/private.png", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData.Bytes())
	}))
	t.Cleanup(server.Close)
	original := ssrfProtectedHTTPClient
	client := newProtectedFetchHTTPClientWithProxy(nil,
		func(ctx context.Context, network, address string) (net.Conn, error) {
			dials.Add(1)
			if address != "93.184.216.34:80" {
				t.Errorf("unexpected protected dial target %q", address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		}, nil, func(*http.Request) (*url.URL, error) { return nil, nil })
	ssrfProtectedHTTPClient = client
	t.Cleanup(func() {
		client.CloseIdleConnections()
		ssrfProtectedHTTPClient = original
	})
	return requests, dials
}

func TestB17MediaEstimatorPrivateURLPolicy(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		fileType  types.FileType
		detail    string
		format    types.RelayFormat
		mediaOff  bool
		streamOff bool
		want      int
		wantError bool
	}{
		{name: "known_qwen_fixed", model: "qwen-vl-plus", fileType: types.FileTypeImage, detail: "high", want: 523},
		{name: "known_claude_fixed", model: "claude-sonnet-4", fileType: types.FileTypeImage, want: 523},
		{name: "known_gemini_fixed", model: "gemini-2.5-pro", fileType: types.FileTypeImage, format: types.RelayFormatGemini, want: 520},
		{name: "known_openai_low", model: "gpt-4o", fileType: types.FileTypeImage, detail: "low", want: 88},
		{name: "known_openai_high_requires_dimensions", model: "gpt-4o", fileType: types.FileTypeImage, detail: "high", wantError: true},
		{name: "patch_low_still_requires_dimensions", model: "gpt-4.1-mini", fileType: types.FileTypeImage, detail: "low", wantError: true},
		{name: "gemini_format_does_not_change_openai_dimensions", model: "gpt-4o", fileType: types.FileTypeImage, format: types.RelayFormatGemini, wantError: true},
		{name: "known_audio_fixed", model: "qwen-audio", fileType: types.FileTypeAudio, want: 259},
		{name: "known_video_fixed", model: "qwen-vl-plus", fileType: types.FileTypeVideo, want: 8195},
		{name: "known_file_fixed", model: "qwen-vl-plus", fileType: types.FileTypeFile, want: 4099},
		{name: "unknown_qwen_requires_detection", model: "qwen-vl-plus", wantError: true},
		{name: "unknown_low_requires_detection", model: "gpt-4o", detail: "low", wantError: true},
		{name: "known_openai_media_off", model: "gpt-4o", fileType: types.FileTypeImage, mediaOff: true, want: 258},
		{name: "known_openai_nonstream_off", model: "gpt-4o", fileType: types.FileTypeImage, streamOff: true, want: 258},
		{name: "unknown_media_off_ignores_detection_failure", model: "qwen-vl-plus", mediaOff: true, want: 4099},
		{name: "unknown_nonstream_off_ignores_detection_failure", model: "qwen-vl-plus", streamOff: true, want: 4099},
		{name: "unknown_gemini_ignores_detection_failure", model: "gemini-2.5-pro", format: types.RelayFormatGemini, want: 4096},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configureB17MediaEstimator(t)
			requests, dials := b17MediaHTTPFixture(t)
			constant.GetMediaToken = !tc.mediaOff
			constant.GetMediaTokenNotStream = !tc.streamOff
			format := tc.format
			if format == "" {
				format = types.RelayFormatOpenAI
			}
			meta := b17EstimatorMeta(tc.fileType, "http://127.0.0.1/private.png", tc.detail)
			got, err := EstimateRequestToken(b17EstimatorContext(t, tc.model), meta, &relaycommon.RelayInfo{RelayFormat: format, IsStream: !tc.streamOff})
			if tc.wantError {
				require.ErrorContains(t, err, "private IP address not allowed")
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
			require.Zero(t, requests.Load())
			require.Zero(t, dials.Load())
			require.False(t, meta.Files[0].Source.HasCache())
		})
	}
}

func TestB17MediaEstimatorLoadsOnlyRequiredPayload(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		fileType  types.FileType
		detail    string
		format    types.RelayFormat
		mediaOff  bool
		streamOff bool
		want      int
		fetches   int32
	}{
		{name: "known_qwen_fixed", model: "qwen-vl-plus", fileType: types.FileTypeImage, detail: "high", want: 523},
		{name: "known_openai_low", model: "gpt-4o", fileType: types.FileTypeImage, detail: "low", want: 88},
		{name: "known_openai_high", model: "gpt-4o", fileType: types.FileTypeImage, detail: "high", want: 768, fetches: 1},
		{name: "known_openai_patch_low", model: "gpt-4.1-mini", fileType: types.FileTypeImage, detail: "low", want: 5, fetches: 1},
		{name: "unknown_qwen_detects_image", model: "qwen-vl-plus", want: 523, fetches: 1},
		{name: "unknown_openai_detects_and_reuses_dimensions", model: "gpt-4o", want: 768, fetches: 1},
		{name: "unknown_media_off_still_detects", model: "qwen-vl-plus", mediaOff: true, want: 523, fetches: 1},
		{name: "unknown_nonstream_off_still_detects", model: "qwen-vl-plus", streamOff: true, want: 523, fetches: 1},
		{name: "unknown_gemini_still_detects", model: "gemini-2.5-pro", format: types.RelayFormatGemini, want: 520, fetches: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configureB17MediaEstimator(t)
			requests, dials := b17MediaHTTPFixture(t)
			constant.GetMediaToken = !tc.mediaOff
			constant.GetMediaTokenNotStream = !tc.streamOff
			format := tc.format
			if format == "" {
				format = types.RelayFormatOpenAI
			}
			meta := b17EstimatorMeta(tc.fileType, "http://93.184.216.34/image.png", tc.detail)
			got, err := EstimateRequestToken(b17EstimatorContext(t, tc.model), meta, &relaycommon.RelayInfo{RelayFormat: format, IsStream: !tc.streamOff})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.fetches, requests.Load())
			require.Equal(t, tc.fetches, dials.Load())
			require.Equal(t, tc.fetches != 0, meta.Files[0].Source.HasCache())
			if tc.fetches != 0 {
				require.Equal(t, types.FileTypeImage, meta.Files[0].FileType)
			}
		})
	}
}

func TestB17MediaEstimatorCacheIsRequestScoped(t *testing.T) {
	configureB17MediaEstimator(t)
	requests, _ := b17MediaHTTPFixture(t)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, IsStream: true}
	newMeta := func() *types.TokenCountMeta {
		meta := b17EstimatorMeta(types.FileTypeImage, "http://93.184.216.34/image.png", "high")
		meta.Files = append(meta.Files, b17EstimatorMeta("", "http://93.184.216.34/image.png", "auto").Files[0])
		return meta
	}
	c, meta := b17EstimatorContext(t, "gpt-4o"), newMeta()
	for i := 0; i < 2; i++ {
		got, err := EstimateRequestToken(c, meta, info)
		require.NoError(t, err)
		require.Equal(t, 1533, got)
		require.EqualValues(t, 1, requests.Load())
		require.Same(t, meta.Files[0].Source.GetCache(), meta.Files[1].Source.GetCache())
	}
	freshMeta := newMeta()
	got, err := EstimateRequestToken(b17EstimatorContext(t, "gpt-4o"), freshMeta, info)
	require.NoError(t, err)
	require.Equal(t, 1533, got)
	require.EqualValues(t, 2, requests.Load())
	require.NotSame(t, meta.Files[0].Source.GetCache(), freshMeta.Files[0].Source.GetCache())
}

func TestB17MediaEstimatorPreservesRedirectRejection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		model    string
		fileType types.FileType
		fetches  int32
	}{
		{name: "known_qwen_needs_no_fetch", model: "qwen-vl-plus", fileType: types.FileTypeImage},
		{name: "known_openai_rejects_redirect", model: "gpt-4o", fileType: types.FileTypeImage, fetches: 1},
		{name: "unknown_qwen_rejects_redirect", model: "qwen-vl-plus", fetches: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureB17MediaEstimator(t)
			requests, dials := b17MediaHTTPFixture(t)
			got, err := EstimateRequestToken(b17EstimatorContext(t, tc.model), b17EstimatorMeta(tc.fileType, "http://93.184.216.34/redirect", "high"), &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, IsStream: true})
			if tc.fetches == 0 {
				require.NoError(t, err)
				require.Equal(t, 523, got)
			} else {
				require.ErrorContains(t, err, "private IP address not allowed")
			}
			require.Equal(t, tc.fetches, requests.Load())
			require.Equal(t, tc.fetches, dials.Load())
		})
	}
}
