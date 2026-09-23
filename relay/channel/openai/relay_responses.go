package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

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

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	var responseStatus string
	_ = common.Unmarshal(responsesResponse.Status, &responseStatus)
	if strings.EqualFold(strings.TrimSpace(responseStatus), "failed") {
		common.SetContextKey(c, constant.ContextKeyResponseFailed, true)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	responsesResponse.Model = info.PublicResponseModelName(responsesResponse.Model)
	responseBody, err = info.RewriteModelForPublicResponse(responseBody, "model")
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	accumulator := responsesUsageAccumulator{info: info}
	accumulator.observeResponse(&responsesResponse)
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			idx := i
			imageCounter.Observe(&responsesResponse.Output[i], &idx)
		}
	}
	imageCounter.Commit(info)

	return accumulator.finish(), nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	accumulator := responsesUsageAccumulator{info: info}
	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	imageCommitted := false

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		var responseStatus string
		if streamResponse.Response != nil {
			_ = common.Unmarshal(streamResponse.Response.Status, &responseStatus)
		}
		if streamResponse.Type == "response.failed" || streamResponse.Type == "response.error" || streamResponse.Type == "error" || strings.EqualFold(strings.TrimSpace(responseStatus), "failed") {
			common.SetContextKey(c, constant.ContextKeyResponseFailed, true)
		}
		accumulator.observe(&streamResponse)
		switch streamResponse.Type {
		case "response.failed", "response.error", "error":
			// 终止性错误事件仅在命中泄密关键词时替换，其余原样透传
			if overrideText, ok := service.ErrorOverrideForChannelError(info.ChannelSetting, data); ok {
				logger.LogError(c, "responses stream error event (masked for user): "+common.LocalLogPreview(data))
				data = maskResponsesErrorEvent(data, overrideText)
			}
		}
		publicData, err := info.RewriteModelForPublicResponse(common.StringToByteSlice(data), "model", "response.model")
		if err != nil {
			sr.Error(err)
			return
		}
		data = string(publicData)
		sendResponsesStreamData(c, streamResponse, data)
		switch streamResponse.Type {
		case "response.completed", "response.done":
			if streamResponse.Response != nil {
				if !imageCommitted {
					if relaycommon.IsNonBillableResponsesStatus(streamResponse.Response.Status) {
						imageCounter.Reset()
						imageCounter.Commit(info)
						imageCommitted = true
					} else {
						for i := range streamResponse.Response.Output {
							idx := i
							imageCounter.Observe(&streamResponse.Response.Output[i], &idx)
						}
						imageCounter.Commit(info)
						imageCommitted = true
					}
				}
			} else if !imageCommitted {
				imageCounter.Commit(info)
				imageCommitted = true
			}
		case "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			if !imageCommitted {
				imageCounter.Reset()
				imageCounter.Commit(info)
				imageCommitted = true
			}
		case dto.ResponsesOutputTypeItemDone:
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
				case dto.BuildInCallFileSearchCall:
					info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
				case dto.BuildInCallFunctionCall:
					info.CountBillableToolCall(dto.BuildInCallFunctionCall, streamResponse.Item.Name)
				case dto.ResponsesOutputTypeImageGenerationCall:
					if !imageCommitted {
						imageCounter.Observe(streamResponse.Item, streamResponse.OutputIndex)
					}
				}
			}
		}
	})

	return accumulator.finish(), nil
}

// maskResponsesErrorEvent replaces the upstream error text inside a Responses
// stream error event (`error` / `response.failed` / `response.error`) with the
// configured fixed message while preserving the event shape. Upstream error
// codes are neutralized too — they can identify the upstream just like the
// message text.
func maskResponsesErrorEvent(data string, overrideText string) string {
	var event map[string]any
	if err := common.UnmarshalJsonStr(data, &event); err != nil {
		return data
	}
	maskErrorFields := func(m map[string]any) {
		if _, ok := m["message"]; ok {
			m["message"] = overrideText
		}
		if _, ok := m["code"]; ok {
			m["code"] = "upstream_error"
		}
		if _, ok := m["param"]; ok {
			m["param"] = ""
		}
	}
	maskErrorFields(event)
	if resp, ok := event["response"].(map[string]any); ok {
		if errObj, ok := resp["error"].(map[string]any); ok {
			maskErrorFields(errObj)
		}
	}
	masked, err := common.Marshal(event)
	if err != nil {
		return data
	}
	return string(masked)
}
