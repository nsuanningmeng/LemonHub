package openai

import (
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/constant"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func OaiChatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var chatResp dto.OpenAITextResponse
	if err := common.Unmarshal(body, &chatResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	chatResp.Model = info.PublicResponseModelName(chatResp.Model)

	if responseID := helper.GetResponseID(c); responseID != "" {
		chatResp.Id = responseID
	}
	convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, &chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	responsesResp, ok := convertResult.Value.(*dto.OpenAIResponsesResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI responses response, got %T", convertResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	usage := convertResult.Usage
	usageHasTokenFields := false
	var envelope map[string]any
	if common.Unmarshal(body, &envelope) == nil {
		if rawUsage, ok := envelope["usage"].(map[string]any); ok {
			for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens"} {
				if value, present := rawUsage[key]; present && value != nil {
					usageHasTokenFields = true
					break
				}
			}
		}
	}
	if !usageHasTokenFields && (usage == nil || usage.TotalTokens == 0) {
		text := service.ExtractOutputTextFromResponses(responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		responsesResp.Usage = relayconvert.UsageFromChatUsage(usage)
	}

	responseBody, err := common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiChatToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseID := helper.GetResponseID(c)
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{
		ID:    responseID,
		Model: info.PublicResponseModelName(info.UpstreamModelName),
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	var streamErr *types.NewAPIError
	forwardedSemantic := false
	choiceFinished := make(map[int]bool)
	observedGeneration := false
	usageObserved := false
	writeFailed := false

	sendEvent := func(event relayconvert.ChatToResponsesStreamEvent) bool {
		data, err := common.Marshal(event.Payload)
		if err != nil {
			streamErr = types.NewOpenAIError(fmt.Errorf("invalid converted Responses event"), types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := helper.ResponseChunkData(c, event.Payload, string(data)); err != nil {
			writeFailed = true
			if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
				info.MarkDownstreamCancelled()
			} else {
				info.MarkUpstreamFailure()
				if info.StreamOutcome().UpstreamFailureStatus == 0 {
					info.MarkUpstreamFailureStatus(http.StatusBadGateway)
				}
			}
			return false
		}
		forwardedSemantic = true
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var envelope map[string]any
		if err := common.UnmarshalJsonStr(data, &envelope); err == nil {
			if apiErr := ClassifyOpenAIEmbeddedError(envelope["error"]); apiErr != nil {
				streamErr = apiErr
				info.MarkUpstreamFailureStatus(apiErr.StatusCode)
				sr.Stop(apiErr)
				return
			}
			if rawUsage, ok := envelope["usage"].(map[string]any); ok {
				for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens"} {
					if value, present := rawUsage[key]; present && value != nil {
						usageObserved = true
						break
					}
				}
			}
		}
		if streamErr != nil || writeFailed {
			sr.Stop(streamErr)
			return
		}
		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			info.MarkUpstreamFailureStatus(http.StatusBadGateway)
			sr.Error(fmt.Errorf("invalid upstream chat stream event"))
			sr.Stop(nil)
			return
		}
		for _, choice := range chunk.Choices {
			if _, seen := choiceFinished[choice.Index]; !seen {
				choiceFinished[choice.Index] = false
			}
			if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
				choiceFinished[choice.Index] = true
			}
			if choice.Delta.GetContentString() != "" || choice.Delta.GetReasoningContent() != "" || len(choice.Delta.ToolCalls) != 0 {
				observedGeneration = true
			}
		}
		if usageObserved && chunk.Usage != nil {
			state.SetUsage(chunk.Usage)
		}
		if c.Request.Context().Err() != nil {
			info.MarkDownstreamCancelled()
			return
		}
		chunk.Model = info.PublicResponseModelName(chunk.Model)
		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &chunk)
		if err != nil {
			streamErr = types.NewOpenAIError(fmt.Errorf("invalid upstream chat stream event"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			info.MarkUpstreamFailureStatus(http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		for _, result := range results {
			event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
			if !ok {
				streamErr = types.NewOpenAIError(fmt.Errorf("invalid converted Responses event"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
				info.MarkUpstreamFailureStatus(http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
			if !sendEvent(event) {
				sr.Stop(streamErr)
				return
			}
		}
	})
	allChoicesFinished := len(choiceFinished) != 0
	for _, finished := range choiceFinished {
		if !finished {
			allChoicesFinished = false
			break
		}
	}
	validChatTerminal := allChoicesFinished || (info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone)
	// A completed Chat choice/[DONE] can precede a harmless transport reset.
	// Callback failures and actual timeout/panic/ping failures still win.
	if !validChatTerminal || (info.StreamStatus != nil && info.StreamStatus.EndReason != relaycommon.StreamEndReasonEOF && info.StreamStatus.EndReason != relaycommon.StreamEndReasonDone && info.StreamStatus.EndReason != relaycommon.StreamEndReasonScannerErr) {
		observeResponsesStreamEnd(c, info)
	}
	if validChatTerminal && streamErr == nil && !info.StreamOutcome().UpstreamFailed {
		info.MarkUpstreamCompleted()
	}

	usage := state.Usage()
	if !usageObserved || usage == nil {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}
	if c.Request.Context().Err() != nil && !info.StreamOutcome().UpstreamFailed && streamErr == nil {
		info.MarkDownstreamCancelled()
		if info.IsPureDownstreamCancellation() && !observedGeneration && !usageObserved {
			info.MarkCancelledWithoutBillableOutput()
		}
		return usage, nil
	}
	if writeFailed {
		return usage, streamErr
	}

	// EOF without a source completion is an interrupted stream. Keep this
	// endpoint's existing observed-usage settlement while emitting failure once.
	interrupted := streamErr != nil || info.StreamOutcome().UpstreamFailed || !validChatTerminal
	if interrupted {
		info.MarkUpstreamFailure()
		if info.StreamOutcome().UpstreamFailureStatus == 0 {
			info.MarkUpstreamFailureStatus(http.StatusBadGateway)
		}
		common.SetContextKey(c, constant.ContextKeyResponseFailed, true)
		if forwardedSemantic && c.Request.Context().Err() == nil && !c.GetBool("relay_stream_write_failed") {
			results, err := relayconvert.AbortStreamResponse(state)
			if err == nil {
				for _, result := range results {
					if event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent); ok && sendEvent(event) {
						c.Set("relay_stream_error_emitted", true)
					}
				}
			}
		}
		if streamErr != nil {
			return nil, streamErr
		}
		return usage, nil
	}

	info.MarkUpstreamCompleted()
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
		if !ok {
			return nil, types.NewOpenAIError(fmt.Errorf("invalid converted Responses event"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if !sendEvent(event) {
			return usage, streamErr
		}
	}
	return usage, nil
}
