package openai

import (
	"errors"
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
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
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
		if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
			return nil, types.NewErrorWithStatusCode(requestErr, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
		}
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
	forwardedSemantic := false
	sourceTerminal := false
	responseID := helper.GetResponseID(c)
	responseModel := info.PublicResponseModelName(info.UpstreamModelName)
	var responseCreated int64

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			info.MarkUpstreamFailureStatus(http.StatusBadGateway)
			sr.Error(fmt.Errorf("invalid upstream Responses stream event"))
			sr.Stop(nil)
			return
		}
		completed, otherTerminal, failed := responsesStreamTerminal(&streamResponse, data)
		if streamResponse.Response != nil {
			if streamResponse.Response.ID != "" {
				responseID = streamResponse.Response.ID
			}
			if streamResponse.Response.Model != "" {
				responseModel = info.PublicResponseModelName(streamResponse.Response.Model)
			}
			if streamResponse.Response.CreatedAt != 0 {
				responseCreated = int64(streamResponse.Response.CreatedAt)
			}
		}
		if completed || otherTerminal || failed {
			sourceTerminal = true
		}
		if c.Request.Context().Err() != nil && !completed && !otherTerminal && !failed {
			info.MarkDownstreamCancelled()
			return
		}
		accumulator.observe(&streamResponse)
		if failed {
			apiErr := responsesStreamUpstreamError(data)
			info.MarkUpstreamFailureStatus(apiErr.StatusCode)
			common.SetContextKey(c, constant.ContextKeyResponseFailed, true)
			sr.Stop(nil)
			imageCounter.Reset()
			imageCounter.Commit(info)
			imageCommitted = true
			if overrideText, ok := service.ErrorOverrideForChannelError(info.ChannelSetting, data); ok {
				logger.LogError(c, "responses stream error event (masked for user): "+common.LocalLogPreview(data))
				data = maskResponsesErrorEvent(data, overrideText)
			}
		} else if completed {
			info.MarkUpstreamCompleted()
			sr.Done()
		} else if otherTerminal {
			info.MarkOtherUpstreamTerminal()
			sr.Stop(nil)
		}
		publicData, err := info.RewriteModelForPublicResponse(common.StringToByteSlice(data), "model", "response.model")
		if err != nil {
			info.MarkUpstreamFailureStatus(http.StatusBadGateway)
			sr.Error(err)
			sr.Stop(nil)
			return
		}
		data = string(publicData)
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
		if err := helper.ResponseChunkData(c, streamResponse, data); err != nil {
			sr.Stop(nil)
			if c.Request.Context().Err() != nil {
				info.MarkDownstreamCancelled()
			}
		} else {
			if strings.HasPrefix(streamResponse.Type, "response.") {
				forwardedSemantic = true
			}
			if c.Request.Context().Err() != nil {
				info.MarkDownstreamCancelled()
			}
		}
	})
	if !sourceTerminal || (info.StreamStatus != nil && info.StreamStatus.EndReason != relaycommon.StreamEndReasonEOF && info.StreamStatus.EndReason != relaycommon.StreamEndReasonDone && info.StreamStatus.EndReason != relaycommon.StreamEndReasonScannerErr) {
		observeResponsesStreamEnd(c, info)
	}
	if c.Request.Context().Err() != nil {
		outcome := info.StreamOutcome()
		if !outcome.UpstreamFailed && !outcome.UpstreamCompleted && !outcome.OtherUpstreamTerminal && !imageCommitted && imageCounter.Count() > 0 {
			imageCounter.Commit(info)
			imageCommitted = true
			accumulator.generated = true
		}
	}
	if usage, cancelled := responsesCancellationUsage(c, &accumulator); cancelled {
		return usage, nil
	}
	if forwardedSemantic && !sourceTerminal && c.Request.Context().Err() == nil && !c.GetBool("relay_stream_write_failed") {
		info.MarkUpstreamFailure()
		if info.StreamOutcome().UpstreamFailureStatus == 0 {
			info.MarkUpstreamFailureStatus(http.StatusBadGateway)
		}
		common.SetContextKey(c, constant.ContextKeyResponseFailed, true)
		event := relayconvert.BuildResponsesStreamFailure(responseID, responseModel, responseCreated)
		data, err := common.Marshal(event.Payload)
		if err == nil && helper.ResponseChunkData(c, event.Payload, string(data)) == nil {
			c.Set("relay_stream_error_emitted", true)
		}
	}
	// Native failed/timeout streams historically settle observed usage. Preserve
	// that endpoint contract while failure facts independently drive reliability.
	if info.StreamOutcome().UpstreamFailed {
		return accumulator.finish(), nil
	}

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

// Explicit status wins over an inconsistent event name. Missing status is
// accepted for compatible providers, but transport DONE/EOF is not completion.
func responsesStreamTerminal(event *dto.ResponsesStreamResponse, data string) (completed, otherTerminal, failed bool) {
	var envelope struct {
		Error any `json:"error"`
	}
	if common.UnmarshalJsonStr(data, &envelope) == nil && ClassifyOpenAIEmbeddedError(envelope.Error) != nil {
		return false, false, true
	}
	if event.Response != nil && ClassifyOpenAIEmbeddedError(event.Response.Error) != nil {
		return false, false, true
	}
	status := ""
	if event.Response != nil {
		_ = common.Unmarshal(event.Response.Status, &status)
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "failed" || event.Type == "response.failed" || event.Type == "response.error" || event.Type == "error" {
		return false, false, true
	}
	if status == "incomplete" || status == "cancelled" || status == "canceled" {
		return false, true, false
	}
	switch event.Type {
	case "response.completed", "response.done":
		return status == "" || status == "completed", status != "" && status != "completed", false
	case "response.incomplete", "response.cancelled", "response.canceled":
		return false, true, false
	}
	return false, false, false
}
func responsesStreamUpstreamError(data string) *types.NewAPIError {
	var envelope map[string]any
	if common.UnmarshalJsonStr(data, &envelope) == nil {
		if err := ClassifyOpenAIEmbeddedError(envelope["error"]); err != nil {
			return err
		}
		if response, ok := envelope["response"].(map[string]any); ok {
			if err := ClassifyOpenAIEmbeddedError(response["error"]); err != nil {
				return err
			}
		}
		if envelope["type"] == "error" {
			if err := ClassifyOpenAIEmbeddedError(map[string]any{"code": envelope["code"], "message": envelope["message"]}); err != nil {
				return err
			}
		}
	}
	return responsesStreamEndError()
}
