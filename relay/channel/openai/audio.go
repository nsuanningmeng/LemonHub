package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func OpenaiTTSHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) *dto.Usage {
	// the status code has been judged before, if there is a body reading failure,
	// it should be regarded as a non-recoverable error, so it should not return err for external retry.
	// Analogous to nginx's load balancing, it will only retry if it can't be requested or
	// if the upstream returns a specific status code, once the upstream has already written the header,
	// the subsequent failure of the response body should be regarded as a non-recoverable error,
	// and can be terminated directly.
	if request, ok := info.Request.(*dto.AudioRequest); ok && request.StreamFormat == "audio" {
		return openaiBinaryTTSHandler(c, resp, info, request)
	}
	defer service.CloseResponseBodyGracefully(resp)
	usage := &dto.Usage{}
	usage.PromptTokens = info.GetEstimatePromptTokens()
	usage.TotalTokens = info.GetEstimatePromptTokens()
	for k, v := range resp.Header {
		if !service.ShouldCopyUpstreamHeader(c, k, v) {
			continue
		}
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)

	if info.IsStream {
		helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
			if service.SundaySearch(data, "usage") {
				reported, present, err := speechSSEUsage(data)
				if err != nil {
					sr.Error(errors.New("invalid speech usage"))
				} else if present {
					*usage = *reported
				}
			}
			if err := helper.StringData(c, data); err != nil {
				sr.Error(err)
			}
		})
	} else {
		common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
		// 读取响应体到缓冲区
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("failed to read TTS response body: %v", err))
			c.Writer.WriteHeaderNow()
			return usage
		}

		// 写入响应到客户端
		c.Writer.WriteHeaderNow()
		_, err = c.Writer.Write(bodyBytes)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("failed to write TTS response: %v", err))
		}

		// 计算音频时长并更新 usage
		audioFormat := "mp3" // 默认格式
		if audioReq, ok := info.Request.(*dto.AudioRequest); ok && audioReq.ResponseFormat != "" {
			audioFormat = audioReq.ResponseFormat
		}

		var duration float64
		var durationErr error

		if audioFormat == "pcm" {
			// PCM 格式没有文件头，根据 OpenAI TTS 的 PCM 参数计算时长
			// 采样率: 24000 Hz, 位深度: 16-bit (2 bytes), 声道数: 1
			const sampleRate = 24000
			const bytesPerSample = 2
			const channels = 1
			duration = float64(len(bodyBytes)) / float64(sampleRate*bytesPerSample*channels)
		} else {
			ext := "." + audioFormat
			reader := bytes.NewReader(bodyBytes)
			duration, durationErr = common.GetAudioDuration(c.Request.Context(), reader, ext)
		}

		usage.PromptTokensDetails.TextTokens = usage.PromptTokens

		if durationErr != nil {
			logger.LogWarn(c, fmt.Sprintf("failed to get audio duration: %v", durationErr))
			// 如果无法获取时长，则设置保底的 CompletionTokens，根据body大小计算
			sizeInKB := float64(len(bodyBytes)) / 1000.0
			estimatedTokens := int(math.Ceil(sizeInKB)) // 粗略估算每KB约等于1 token
			usage.CompletionTokens = estimatedTokens
			usage.CompletionTokenDetails.AudioTokens = estimatedTokens
		} else if duration > 0 {
			// 计算 token: ceil(duration) / 60.0 * 1000，即每分钟 1000 tokens。
			// duration 解析自上游返回的音频元数据，饱和转换防止 int 回绕。
			completionTokens := common.QuotaRound(math.Ceil(duration) / 60.0 * 1000)
			usage.CompletionTokens = completionTokens
			usage.CompletionTokenDetails.AudioTokens = completionTokens
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return usage
}

func OpenaiSTTHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, responseFormat string) (*types.NewAPIError, *dto.Usage) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}
	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	var responseData struct {
		Usage *dto.Usage `json:"usage"`
	}
	if err := common.Unmarshal(responseBody, &responseData); err == nil && responseData.Usage != nil {
		if responseData.Usage.TotalTokens > 0 {
			usage := responseData.Usage
			if usage.PromptTokens == 0 {
				usage.PromptTokens = usage.InputTokens
			}
			if usage.CompletionTokens == 0 {
				usage.CompletionTokens = usage.OutputTokens
			}
			return nil, usage
		}
	}

	usage := &dto.Usage{}
	usage.PromptTokens = info.GetEstimatePromptTokens()
	usage.CompletionTokens = 0
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return nil, usage
}

// Distinguish explicitly reported zero from absent/empty usage without
// changing the original SSE frame or treating null counters as authoritative.
func speechSSEUsage(data string) (*dto.Usage, bool, error) {
	var envelope struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := common.UnmarshalJsonStr(data, &envelope); err != nil {
		return nil, false, err
	}
	if len(envelope.Usage) == 0 || strings.TrimSpace(string(envelope.Usage)) == "null" {
		return nil, false, nil
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(envelope.Usage, &fields); err != nil {
		return nil, false, err
	}
	present := func(key string) bool { raw, ok := fields[key]; return ok && strings.TrimSpace(string(raw)) != "null" }
	if !present("input_tokens") && !present("output_tokens") && !present("total_tokens") && !present("prompt_tokens") && !present("completion_tokens") {
		return nil, false, nil
	}
	var reported dto.Usage
	if err := common.Unmarshal(envelope.Usage, &reported); err != nil {
		return nil, false, err
	}
	usage := &dto.Usage{PromptTokens: reported.PromptTokens, CompletionTokens: reported.CompletionTokens, TotalTokens: reported.TotalTokens, InputTokens: reported.InputTokens, OutputTokens: reported.OutputTokens, PromptTokensDetails: reported.PromptTokensDetails, CompletionTokenDetails: reported.CompletionTokenDetails}
	if present("input_tokens") {
		usage.PromptTokens = reported.InputTokens
	}
	if present("output_tokens") {
		usage.CompletionTokens = reported.OutputTokens
	}
	if !present("total_tokens") {
		usage.TotalTokens = common.QuotaRound(float64(usage.PromptTokens) + float64(usage.CompletionTokens))
	}
	if reported.InputTokensDetails != nil {
		details := *reported.InputTokensDetails
		usage.InputTokensDetails = &details
		usage.PromptTokensDetails = details
	}
	if reported.OutputTokensDetails != nil {
		details := *reported.OutputTokensDetails
		usage.OutputTokensDetails = &details
		usage.CompletionTokenDetails = details
	}
	return usage, true, nil
}

const binaryAudioMemoryLimit = 256 << 10
const binaryAudioSpoolLimit = 32 << 20

// Audio parsers require seekable input (AAC also calls ReadAll). Keep that input
// bounded while forwarding arbitrarily long responses without truncation.
type binaryAudioSpool struct {
	memory   bytes.Buffer
	file     *os.File
	size     int64
	disabled bool
}

func (s *binaryAudioSpool) close() {
	if s.file != nil {
		name := s.file.Name()
		_ = s.file.Close()
		_ = os.Remove(name)
		s.file = nil
	}
	s.memory.Reset()
}
func (s *binaryAudioSpool) append(data []byte) {
	if s.disabled {
		return
	}
	if int64(len(data)) > binaryAudioSpoolLimit-s.size {
		s.disabled = true
		s.close()
		return
	}
	if s.file == nil && s.size+int64(len(data)) > binaryAudioMemoryLimit {
		f, err := os.CreateTemp("", "lemonhub-tts-*")
		if err != nil {
			s.disabled = true
			s.close()
			return
		}
		s.file = f
		if _, err = f.Write(s.memory.Bytes()); err != nil {
			s.disabled = true
			s.close()
			return
		}
		s.memory.Reset()
	}
	var err error
	if s.file != nil {
		var n int
		n, err = s.file.Write(data)
		if err == nil && n != len(data) {
			err = io.ErrShortWrite
		}
	} else {
		_, err = s.memory.Write(data)
	}
	if err != nil {
		s.disabled = true
		s.close()
		return
	}
	s.size += int64(len(data))
}
func (s *binaryAudioSpool) reader() (io.ReadSeeker, error) {
	if s.disabled {
		return nil, errors.New("audio duration spool unavailable")
	}
	if s.file != nil {
		_, err := s.file.Seek(0, io.SeekStart)
		return s.file, err
	}
	return bytes.NewReader(s.memory.Bytes()), nil
}
func binaryAudioContentType(upstream, format string) string {
	mediaType, _, err := mime.ParseMediaType(upstream)
	if err == nil && (strings.HasPrefix(mediaType, "audio/") || mediaType == "application/octet-stream" || mediaType == "application/ogg") {
		return upstream
	}
	switch format {
	case "pcm":
		return "audio/pcm"
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	default:
		return "audio/mpeg"
	}
}
func openaiBinaryTTSHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, request *dto.AudioRequest) *dto.Usage {
	info.StreamStatus = relaycommon.NewStreamStatus()
	usage := &dto.Usage{PromptTokens: info.GetEstimatePromptTokens(), TotalTokens: info.GetEstimatePromptTokens()}
	usage.PromptTokensDetails.TextTokens = usage.PromptTokens
	common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	format := request.ResponseFormat
	if format == "" {
		format = "mp3"
	}
	for key, values := range resp.Header {
		if len(values) > 0 && service.ShouldCopyUpstreamHeader(c, key, values) {
			c.Writer.Header().Set(key, values[0])
		}
	}
	c.Writer.Header().Set("Content-Type", binaryAudioContentType(resp.Header.Get("Content-Type"), format))
	c.Writer.WriteHeader(resp.StatusCode)
	var closeOnce sync.Once
	var cancelledBody atomic.Bool
	closeBody := func() { closeOnce.Do(func() { _ = resp.Body.Close() }) }
	watcherDone, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-c.Request.Context().Done():
			cancelledBody.Store(true)
			closeBody()
		case <-finished:
		}
	}()
	defer func() { close(finished); closeBody(); <-watcherDone }()
	var spool binaryAudioSpool
	defer spool.close()
	var observedBytes int64
	markCancelled := func() {
		info.MarkDownstreamCancelled()
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, errors.New("downstream binary audio cancelled"))
	}
	markFailure := func(err error, message string) {
		if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
			markCancelled()
		} else {
			info.MarkUpstreamFailureStatus(http.StatusBadGateway)
			common.SetContextKey(c, constant.ContextKeyResponseFailed, true)
			info.StreamStatus.RecordError(message)
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, errors.New(message))
		}
	}
	buffer := make([]byte, 32<<10)
	for {
		if c.Request.Context().Err() != nil {
			markCancelled()
			break
		}
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if observedBytes > math.MaxInt64-int64(n) {
				observedBytes = math.MaxInt64
			} else {
				observedBytes += int64(n)
			}
			if format != "pcm" {
				spool.append(buffer[:n])
			}
		}
		// Binary HTTP EOF is its protocol terminal. Preserve this evidence and all
		// observed bytes before writing the final chunk to a cancelling client.
		if readErr == io.EOF {
			if n > 0 || !cancelledBody.Load() {
				info.MarkUpstreamCompleted()
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			} else {
				markCancelled()
			}
		} else if readErr != nil {
			if cancelledBody.Load() && c.Request.Context().Err() != nil && (errors.Is(readErr, io.ErrClosedPipe) || errors.Is(readErr, net.ErrClosed)) {
				markCancelled()
			} else {
				markFailure(readErr, "binary audio read failed")
			}
		}
		if n > 0 {
			if requestErr := c.Request.Context().Err(); requestErr != nil {
				markCancelled()
				break
			}
			helper.ExtendWriteDeadline(c)
			written, writeErr := c.Writer.Write(buffer[:n])
			if writeErr == nil && written != n {
				writeErr = io.ErrShortWrite
			}
			if writeErr != nil {
				c.Set("relay_stream_write_failed", true)
				markFailure(writeErr, "binary audio write failed")
				break
			}
			if flushErr := helper.FlushWriter(c); flushErr != nil {
				markFailure(flushErr, "binary audio flush failed")
				break
			}
		}
		if readErr != nil {
			break
		}
	}
	if requestErr := c.Request.Context().Err(); requestErr != nil {
		markCancelled()
	}
	if info.IsPureDownstreamCancellation() && observedBytes == 0 {
		info.MarkCancelledWithoutBillableOutput()
	}
	if observedBytes > 0 {
		var duration float64
		var durationErr error
		if format == "pcm" {
			duration = float64(observedBytes) / 48000.0
		} else {
			var reader io.ReadSeeker
			reader, durationErr = spool.reader()
			if durationErr == nil {
				duration, durationErr = common.GetAudioDuration(c.Request.Context(), reader, "."+format)
			}
		}
		if durationErr != nil {
			usage.CompletionTokens = common.QuotaRound(math.Ceil(float64(observedBytes) / 1000.0))
		} else if duration > 0 {
			usage.CompletionTokens = common.QuotaRound(math.Ceil(duration) / 60.0 * 1000)
		}
		usage.CompletionTokenDetails.AudioTokens = usage.CompletionTokens
	}
	usage.TotalTokens = common.QuotaRound(float64(usage.PromptTokens) + float64(usage.CompletionTokens))
	return usage
}
