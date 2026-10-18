package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// WorkerRequest Worker请求的数据结构
type WorkerRequest struct {
	URL     string            `json:"url"`
	Key     string            `json:"key"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

// remoteFetchError prevents signed URLs, webhook tokens and basic-auth userinfo
// from escaping through net/http's URL-bearing errors. Keep the cause available
// to errors.Is/As without including it in log or client-facing error text.
type remoteFetchError struct {
	cause error
}

func (e *remoteFetchError) Error() string {
	if strings.Contains(e.cause.Error(), "private IP address not allowed") {
		return "remote request failed: private IP address not allowed"
	}
	return "remote request failed"
}
func (e *remoteFetchError) Unwrap() error { return e.cause }

// DoWorkerRequest 通过Worker发送请求
func DoWorkerRequest(req *WorkerRequest) (*http.Response, error) {
	return doWorkerRequestWithContext(context.Background(), req)
}

func doWorkerRequestWithContext(ctx context.Context, req *WorkerRequest) (*http.Response, error) {
	if !system_setting.EnableWorker() {
		return nil, fmt.Errorf("worker not enabled")
	}
	if !system_setting.WorkerAllowHttpImageRequestEnabled && !strings.HasPrefix(req.URL, "https") {
		return nil, fmt.Errorf("only support https url")
	}

	// SSRF防护：验证请求URL
	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(req.URL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		return nil, fmt.Errorf("request rejected: %w", &remoteFetchError{cause: err})
	}

	workerUrl := system_setting.WorkerUrl
	if !strings.HasSuffix(workerUrl, "/") {
		workerUrl += "/"
	}

	// 序列化worker请求数据
	workerPayload, err := common.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal worker payload: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, workerUrl, bytes.NewBuffer(workerPayload))
	if err != nil {
		return nil, &remoteFetchError{cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := GetHttpClient().Do(httpReq)
	if err != nil {
		return resp, &remoteFetchError{cause: err}
	}
	return resp, nil
}

func DoDownloadRequest(originUrl string, reason ...string) (resp *http.Response, err error) {
	return DoDownloadRequestWithContext(context.Background(), originUrl, reason...)
}

// DoDownloadRequestWithContext binds remote file I/O (including Worker requests)
// to the relay request so disconnected callers do not leave downloads running.
func DoDownloadRequestWithContext(ctx context.Context, originUrl string, reason ...string) (resp *http.Response, err error) {
	if err := ctx.Err(); err != nil {
		return nil, &remoteFetchError{cause: err}
	}
	if system_setting.EnableWorker() {
		common.SysLog(fmt.Sprintf("downloading file through worker, reason: %s", strings.Join(reason, ", ")))
		req := &WorkerRequest{
			URL: originUrl,
			Key: system_setting.WorkerValidKey,
		}
		return doWorkerRequestWithContext(ctx, req)
	} else {
		// SSRF防护：验证请求URL（非Worker模式）
		if err := ValidateSSRFProtectedFetchURL(originUrl); err != nil {
			return nil, fmt.Errorf("request rejected: %w", &remoteFetchError{cause: err})
		}

		common.SysLog(fmt.Sprintf("downloading file from origin, reason: %s", strings.Join(reason, ", ")))
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, originUrl, nil)
		if requestErr != nil {
			return nil, &remoteFetchError{cause: requestErr}
		}
		resp, err = GetSSRFProtectedHTTPClient().Do(req)
		if err != nil {
			return resp, &remoteFetchError{cause: err}
		}
		return resp, nil
	}
}
