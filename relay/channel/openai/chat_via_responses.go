package openai

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
			return nil, types.NewErrorWithStatusCode(requestErr, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
		}
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if err := common.Unmarshal(body, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	responsesResp.Model = info.PublicResponseModelName(responsesResp.Model)

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, &responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := common.Marshal(responseValue)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	accumulator := relayconvert.NewResponsesBufferedAccumulator()
	var finalResponse *dto.OpenAIResponsesResponse
	var streamErr *types.NewAPIError

	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 6 || line[:5] != "data:" {
			continue
		}
		data := line[5:]
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				break
			}
			continue
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal buffered responses stream event: "+err.Error())
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			break
		}
		accumulator.ProcessEvent(&streamResp)
		switch streamResp.Type {
		case "response.completed", "response.done", "response.incomplete":
			finalResponse = streamResp.Response
			if streamResp.Type == "response.incomplete" {
				if finalResponse == nil {
					finalResponse = &dto.OpenAIResponsesResponse{}
				}
				if len(finalResponse.Status) == 0 {
					finalResponse.Status = []byte(`"incomplete"`)
				}
			}
		case "response.failed", "response.error":
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					break
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if streamErr != nil || finalResponse != nil {
			break
		}
	}
	if streamErr != nil {
		return nil, streamErr
	}
	if err := scanner.Err(); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if finalResponse == nil {
		finalResponse = &dto.OpenAIResponsesResponse{
			ID:        helper.GetResponseID(c),
			CreatedAt: dto.IntValue(time.Now().Unix()),
			Model:     info.PublicResponseModelName(info.UpstreamModelName),
			Status:    []byte(`"completed"`),
		}
	}
	finalResponse.Model = info.PublicResponseModelName(finalResponse.Model)
	accumulator.SupplementResponseOutput(finalResponse)

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, finalResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(finalResponse)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := common.Marshal(responseValue)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAIResponses, info.RelayFormat, relayconvert.ResponseStreamOptions{
		ID:      responseId,
		Model:   info.PublicResponseModelName(info.UpstreamModelName),
		Created: createAt,
	})
	if err != nil {
		info.MarkUpstreamFailureStatus(http.StatusBadGateway)
		return nil, types.NewOpenAIError(fmt.Errorf("invalid Responses conversion route"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	streamErr := (*types.NewAPIError)(nil)
	accumulator := responsesUsageAccumulator{info: info}

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendGeminiResponse := func(geminiResponse *dto.GeminiChatResponse) bool {
		if geminiResponse == nil {
			return true
		}

		if err := helper.ObjectData(c, geminiResponse); err != nil {
			streamErr = responsesStreamWriteError(c, err)
			return false
		}
		return true
	}

	sendStreamResult := func(result relayconvert.ResponseResult) bool {
		switch value := result.Value.(type) {
		case dto.ChatCompletionsStreamResponse:
			if len(value.Choices) == 0 && value.Usage == nil {
				return true
			}
			if err := helper.ObjectData(c, &value); err != nil {
				streamErr = responsesStreamWriteError(c, err)
				return false
			}
			return true
		case *dto.ChatCompletionsStreamResponse:
			if value == nil || (len(value.Choices) == 0 && value.Usage == nil) {
				return true
			}
			if err := helper.ObjectData(c, value); err != nil {
				streamErr = responsesStreamWriteError(c, err)
				return false
			}
			return true
		case dto.ClaudeResponse:
			if err := helper.ClaudeData(c, value); err != nil {
				streamErr = responsesStreamWriteError(c, err)
				return false
			}
			return true
		case *dto.ClaudeResponse:
			if value == nil {
				return true
			}
			if err := helper.ClaudeData(c, *value); err != nil {
				streamErr = responsesStreamWriteError(c, err)
				return false
			}
			return true
		case dto.GeminiChatResponse:
			return sendGeminiResponse(&value)
		case *dto.GeminiChatResponse:
			return sendGeminiResponse(value)
		default:
			info.MarkUpstreamFailureStatus(http.StatusBadGateway)
			streamErr = types.NewOpenAIError(fmt.Errorf("invalid converted stream response type"), types.ErrorCodeBadResponse, http.StatusBadGateway)
			return false
		}
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			streamErr = types.NewOpenAIError(fmt.Errorf("invalid upstream Responses stream event"), types.ErrorCodeBadResponse, http.StatusBadGateway)
			info.MarkUpstreamFailureStatus(streamErr.StatusCode)
			sr.Stop(streamErr)
			return
		}
		completed, otherTerminal, failed := responsesStreamTerminal(&streamResp, data)
		if c.Request.Context().Err() != nil && !completed && !otherTerminal && !failed {
			info.MarkDownstreamCancelled()
			return
		}
		accumulator.observe(&streamResp)
		if failed {
			streamErr = responsesStreamUpstreamError(data)
			info.MarkUpstreamFailureStatus(streamErr.StatusCode)
			sr.Stop(streamErr)
			return
		}
		if completed {
			info.MarkUpstreamCompleted()
			sr.Done()
		} else if otherTerminal {
			info.MarkOtherUpstreamTerminal()
			sr.Stop(nil)
		}
		if c.Request.Context().Err() != nil {
			info.MarkDownstreamCancelled()
			streamErr = responsesStreamWriteError(c, c.Request.Context().Err())
			return
		}
		if streamResp.Response != nil {
			streamResp.Response.Model = info.PublicResponseModelName(streamResp.Response.Model)
		}

		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &streamResp)
		if err != nil {
			streamErr = types.NewOpenAIError(fmt.Errorf("invalid converted Responses stream event"), types.ErrorCodeBadResponse, http.StatusBadGateway)
			info.MarkUpstreamFailureStatus(streamErr.StatusCode)
			sr.Stop(streamErr)
			return
		}
		for _, result := range results {
			if !sendStreamResult(result) {
				if c.Request.Context().Err() != nil {
					info.MarkDownstreamCancelled()
				} else {
					sr.Stop(nil)
				}
				return
			}
		}
		if c.Request.Context().Err() != nil {
			info.MarkDownstreamCancelled()
		}
	})

	observeResponsesStreamEnd(c, info)
	if info.StreamOutcome().UpstreamFailed {
		if streamErr != nil {
			return nil, streamErr
		}
		return nil, responsesStreamEndError()
	}
	if streamErr != nil {
		return responsesConvertedWriteFailure(c, &accumulator, streamErr)
	}
	if usage, cancelled := responsesCancellationUsage(c, &accumulator); cancelled {
		return usage, nil
	}
	usage := accumulator.finish()
	state.SetUsage(usage)
	if c.Request.Context().Err() != nil {
		return responsesConvertedWriteFailure(c, &accumulator, c.Request.Context().Err())
	}

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
		info.ClaudeConvertInfo.Usage = usage
	}
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		info.MarkUpstreamFailureStatus(http.StatusBadGateway)
		return nil, types.NewOpenAIError(fmt.Errorf("invalid finalized Responses stream"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}

	for _, result := range finalResults {
		if !sendStreamResult(result) {
			return responsesConvertedWriteFailure(c, &accumulator, streamErr)
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, createAt, info.PublicResponseModelName(info.UpstreamModelName), *usage)); err != nil {
			return responsesConvertedWriteFailure(c, &accumulator, err)
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI {
		if err := helper.Done(c); err != nil {
			return responsesConvertedWriteFailure(c, &accumulator, err)
		}
	}
	return usage, nil
}
