package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFileSourceDoesNotAliasEqualLengthAttachments(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	t.Cleanup(func() { CleanupFileSources(c) })
	first := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("shared-header-", 16) + "first"))
	second := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("shared-header-", 16) + "other"))
	require.Equal(t, len(first), len(second))
	require.Equal(t, first[:128], second[:128])

	firstData, err := LoadFileSource(c, types.NewBase64FileSource(first, "text/plain"))
	require.NoError(t, err)
	secondData, err := LoadFileSource(c, types.NewBase64FileSource(second, "text/plain"))
	require.NoError(t, err)
	gotFirst, err := firstData.GetBase64Data()
	require.NoError(t, err)
	gotSecond, err := secondData.GetBase64Data()
	require.NoError(t, err)
	assert.Equal(t, first, gotFirst)
	assert.Equal(t, second, gotSecond, "each attachment must retain its own full content")

	repeated, err := LoadFileSource(c, types.NewBase64FileSource(second, "text/plain"))
	require.NoError(t, err)
	assert.Same(t, secondData, repeated, "identical attachments can still share a request-local cache")
}

type failingFetchTransport struct{ err error }

func (t failingFetchTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, t.err }

func TestRemoteFetchLogsAndErrorsDoNotExposeCredentials(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	originalClient, originalProtected := httpClient, ssrfProtectedHTTPClient
	originalWorker, originalWorkerKey := system_setting.WorkerUrl, system_setting.WorkerValidKey
	originalDebug := common.DebugEnabled
	common.DebugEnabled = true
	common.LogWriterMu.Lock()
	originalWriter, originalErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	var logs bytes.Buffer
	gin.DefaultWriter, gin.DefaultErrorWriter = &logs, &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		httpClient, ssrfProtectedHTTPClient = originalClient, originalProtected
		system_setting.WorkerUrl, system_setting.WorkerValidKey = originalWorker, originalWorkerKey
		common.DebugEnabled = originalDebug
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = originalWriter, originalErrorWriter
		common.LogWriterMu.Unlock()
	})

	transportErr := errors.New("fetch transport unavailable")
	httpClient = &http.Client{Transport: failingFetchTransport{err: transportErr}}
	ssrfProtectedHTTPClient = httpClient
	rawURL := "https://download-user:download-password@93.184.216.34/private-object?signature=download-signature#private-fragment"
	for _, worker := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "worker"}[worker], func(t *testing.T) {
			logs.Reset()
			system_setting.WorkerUrl = ""
			if worker {
				system_setting.WorkerUrl = "https://worker-user:worker-password@worker.example/service?token=worker-token"
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			_, err := LoadFileSource(c, types.NewURLFileSource(rawURL))
			require.Error(t, err)
			assert.ErrorIs(t, err, transportErr, "classification must retain the original cause")
			_, mimeErr := GetFileTypeFromUrl(c, rawURL)
			require.Error(t, mimeErr)
			combined := logs.String() + err.Error() + mimeErr.Error()
			for _, secret := range []string{"download-user", "download-password", "download-signature", "private-object", "private-fragment", "worker-user", "worker-password", "worker-token"} {
				assert.NotContains(t, combined, secret)
			}
		})
	}
}

func TestGotifyErrorDoesNotExposeNotificationToken(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	originalProtected, originalWorker := ssrfProtectedHTTPClient, system_setting.WorkerUrl
	t.Cleanup(func() { ssrfProtectedHTTPClient, system_setting.WorkerUrl = originalProtected, originalWorker })
	system_setting.WorkerUrl = ""
	transportErr := errors.New("connection failed")
	ssrfProtectedHTTPClient = &http.Client{Transport: failingFetchTransport{err: transportErr}}
	err := sendGotifyNotify("https://93.184.216.34", "private-notification-token", 1, dto.Notify{})
	require.Error(t, err)
	assert.ErrorIs(t, err, transportErr)
	assert.NotContains(t, err.Error(), "private-notification-token")
}

type cancelledFetchTransport struct {
	started chan struct{}
	guard   context.Context
}

func (t cancelledFetchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	close(t.started)
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-t.guard.Done():
		return nil, errors.New("download did not receive caller cancellation")
	}
}

func TestFileFetchStopsWhenCallerCancels(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	originalClient, originalProtected, originalWorker := httpClient, ssrfProtectedHTTPClient, system_setting.WorkerUrl
	t.Cleanup(func() {
		httpClient, ssrfProtectedHTTPClient, system_setting.WorkerUrl = originalClient, originalProtected, originalWorker
	})
	for _, worker := range []bool{false, true} {
		for _, mimeOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("worker=%t/mime=%t", worker, mimeOnly), func(t *testing.T) {
				guard, stopGuard := context.WithTimeout(context.Background(), 3*time.Second)
				defer stopGuard()
				started := make(chan struct{})
				httpClient = &http.Client{Transport: cancelledFetchTransport{started: started, guard: guard}}
				ssrfProtectedHTTPClient = httpClient
				system_setting.WorkerUrl = ""
				if worker {
					system_setting.WorkerUrl = "https://worker.example/"
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/completions", nil)
				result := make(chan error, 1)
				go func() {
					if mimeOnly {
						_, err := GetFileTypeFromUrl(c, "https://93.184.216.34/image")
						result <- err
						return
					}
					_, err := LoadFileSource(c, types.NewURLFileSource("https://93.184.216.34/image"))
					result <- err
				}()
				select {
				case <-started:
				case <-guard.Done():
					t.Fatal("download did not start")
				}
				cancel()
				assert.ErrorIs(t, <-result, context.Canceled)
			})
		}
	}
}
