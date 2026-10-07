package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	if data == "" {
		return nil
	}
	publicData, err := info.RewriteModelForPublicResponse(common.StringToByteSlice(data), "model")
	if err != nil {
		return err
	}
	data = string(publicData)

	if !forceFormat && !thinkToContent {
		return writeOpenAIStreamData(c, "data: "+data+"\n\n")
	}

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &lastStreamResponse); err != nil {
		return err
	}

	if !thinkToContent {
		return writeOpenAIChatObject(c, info, lastStreamResponse)
	}

	hasThinkingContent := false
	hasContent := false
	var thinkingContent strings.Builder
	for _, choice := range lastStreamResponse.Choices {
		if len(choice.Delta.GetReasoningContent()) > 0 {
			hasThinkingContent = true
			thinkingContent.WriteString(choice.Delta.GetReasoningContent())
		}
		if len(choice.Delta.GetContentString()) > 0 {
			hasContent = true
		}
	}

	// Handle think to content conversion
	if info.ThinkingContentInfo.IsFirstThinkingContent {
		if hasThinkingContent {
			response := lastStreamResponse.Copy()
			for i := range response.Choices {
				// send `think` tag with thinking content
				response.Choices[i].Delta.SetContentString("<think>\n" + thinkingContent.String())
				response.Choices[i].Delta.ReasoningContent = nil
				response.Choices[i].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.IsFirstThinkingContent = false
			info.ThinkingContentInfo.HasSentThinkingContent = true
			return writeOpenAIChatObject(c, info, response)
		}
	}

	if lastStreamResponse.Choices == nil || len(lastStreamResponse.Choices) == 0 {
		return writeOpenAIChatObject(c, info, lastStreamResponse)
	}

	// Process each choice
	for i, choice := range lastStreamResponse.Choices {
		// Handle transition from thinking to content
		// only send `</think>` tag when previous thinking content has been sent
		if hasContent && !info.ThinkingContentInfo.SendLastThinkingContent && info.ThinkingContentInfo.HasSentThinkingContent {
			response := lastStreamResponse.Copy()
			for j := range response.Choices {
				response.Choices[j].Delta.SetContentString("\n</think>\n")
				response.Choices[j].Delta.ReasoningContent = nil
				response.Choices[j].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.SendLastThinkingContent = true
			if err := writeOpenAIChatObject(c, info, response); err != nil {
				return err
			}
		}

		// Convert reasoning content to regular content if any
		if len(choice.Delta.GetReasoningContent()) > 0 {
			lastStreamResponse.Choices[i].Delta.SetContentString(choice.Delta.GetReasoningContent())
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		} else if !hasThinkingContent && !hasContent {
			// flush thinking content
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		}
	}

	return writeOpenAIChatObject(c, info, lastStreamResponse)
}

func OaiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	model := info.UpstreamModelName
	var responseId string
	var createAt int64 = 0
	var systemFingerprint string
	var containStreamUsage bool
	var responseTextBuilder strings.Builder
	var toolCount int
	var usage = &dto.Usage{}

	var usageStreamData, lastObservedData string
	seenStreamToolCalls := make(map[string]struct{})
	var streamFunctionCallNames []string
	var fatal *types.NewAPIError
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		fail := func(err *types.NewAPIError) { fatal = err; sr.Stop(err) }
		var envelope map[string]any
		if err := common.UnmarshalJsonStr(data, &envelope); err != nil || envelope == nil {
			fail(types.NewOpenAIError(fmt.Errorf("invalid upstream stream frame"), types.ErrorCodeBadResponse, http.StatusBadGateway))
			return
		}
		if embedded := ClassifyOpenAIEmbeddedError(envelope["error"]); embedded != nil {
			fail(embedded)
			return
		}
		lastObservedData = data
		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			fail(types.NewOpenAIError(fmt.Errorf("invalid upstream stream frame"), types.ErrorCodeBadResponse, http.StatusBadGateway))
			return
		}
		if chunk.Id != "" {
			responseId = chunk.Id
		}
		if chunk.Created != 0 {
			createAt = chunk.Created
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.GetSystemFingerprint() != "" {
			systemFingerprint = chunk.GetSystemFingerprint()
		}
		if service.ValidUsage(chunk.Usage) {
			usage = chunk.Usage
			containStreamUsage = true
			usageStreamData = data
		}
		collectStreamFunctionCallNames(data, seenStreamToolCalls, &streamFunctionCallNames)
		if err := processTokenData(info.RelayMode, data, &responseTextBuilder, &toolCount); err != nil {
			fail(types.NewOpenAIError(fmt.Errorf("invalid upstream stream frame"), types.ErrorCodeBadResponse, http.StatusBadGateway))
			return
		}
		output := data
		if info.RelayFormat == types.RelayFormatOpenAI && !info.ShouldIncludeUsage {
			filtered, emit, err := openAIStreamDataWithoutUsage(data)
			if err != nil {
				fail(types.NewOpenAIError(fmt.Errorf("invalid upstream stream frame"), types.ErrorCodeBadResponse, http.StatusBadGateway))
				return
			}
			if !emit {
				return
			}
			output = filtered
		}
		if err := HandleStreamFormat(c, info, output, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent, true); err != nil {
			if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
				fail(types.NewErrorWithStatusCode(requestErr, types.ErrorCodeBadResponse, http.StatusBadGateway))
			} else {
				fail(types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway))
			}
			return
		}
	})
	// A callback fatal wins even if the scanner recorded DONE while it was queued.
	if fatal != nil {
		return nil, fatal
	}
	if info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone {
		if requestErr := c.Request.Context().Err(); requestErr != nil {
			return nil, types.NewErrorWithStatusCode(requestErr, types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
	}
	if requestErr := c.Request.Context().Err(); requestErr != nil && info.IsPureDownstreamCancellation() {
		status := info.StreamStatus
		if status == nil || (status.EndReason != relaycommon.StreamEndReasonTimeout && status.EndReason != relaycommon.StreamEndReasonPanic && (status.EndError == nil || errors.Is(status.EndError, requestErr))) {
			return nil, types.NewErrorWithStatusCode(requestErr, types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
	}
	if info.StreamStatus != nil && (!info.StreamStatus.IsNormalEnd() || info.StreamStatus.HasErrors()) {
		return nil, types.NewOpenAIError(fmt.Errorf("upstream stream ended unsuccessfully"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	if !containStreamUsage {
		usage = service.ResponseText2Usage(c, responseTextBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage.CompletionTokens += toolCount * 7
	}
	if usageStreamData == "" {
		usageStreamData = lastObservedData
	}
	applyUsagePostProcessing(info, usage, common.StringToByteSlice(usageStreamData))
	if err := finalizeObservedOpenAIStream(c, info, responseId, createAt, info.PublicResponseModelName(model), systemFingerprint, usage, containStreamUsage); err != nil {
		if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
			return nil, types.NewErrorWithStatusCode(requestErr, types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	for _, name := range streamFunctionCallNames {
		info.CountBillableToolCall(dto.BuildInCallFunctionCall, name)
	}

	return usage, nil
}

func collectStreamFunctionCallNames(data string, seen map[string]struct{}, names *[]string) {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
		return
	}
	for _, choice := range streamResponse.Choices {
		for i, tc := range choice.Delta.ToolCalls {
			name := tc.Function.Name
			if name == "" {
				continue
			}
			toolIdx := i
			if tc.Index != nil {
				toolIdx = *tc.Index
			}
			key := fmt.Sprintf("%d-%d", choice.Index, toolIdx)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			*names = append(*names, name)
		}
	}
}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	var simpleResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
			return nil, types.NewErrorWithStatusCode(requestErr, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
		}
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	logger.LogDebug(c, "upstream response body: %s", responseBody)
	// Unmarshal to simpleResponse
	if info.ChannelType == constant.ChannelTypeOpenRouter && info.ChannelOtherSettings.IsOpenRouterEnterprise() {
		// 尝试解析为 openrouter enterprise
		var enterpriseResponse openrouter.OpenRouterEnterpriseResponse
		err = common.Unmarshal(responseBody, &enterpriseResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if enterpriseResponse.Success {
			responseBody = enterpriseResponse.Data
		} else {
			logger.LogError(c, fmt.Sprintf("openrouter enterprise response success=false, data: %s", enterpriseResponse.Data))
			return nil, types.NewOpenAIError(fmt.Errorf("openrouter response success=false"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	var envelope map[string]any
	if err := common.Unmarshal(responseBody, &envelope); err == nil {
		if embedded := ClassifyOpenAIEmbeddedError(envelope["error"]); embedded != nil {
			return nil, embedded
		}
	}
	err = common.Unmarshal(responseBody, &simpleResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	simpleResponse.Model = info.PublicResponseModelName(simpleResponse.Model)
	responseBody, err = info.RewriteModelForPublicResponse(responseBody, "model")
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	for _, choice := range simpleResponse.Choices {
		if choice.FinishReason == constant.FinishReasonContentFilter {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "openai_finish_reason=content_filter")
			break
		}
	}

	for _, choice := range simpleResponse.Choices {
		for _, tc := range choice.Message.ParseToolCalls() {
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, tc.Function.Name)
		}
	}

	forceFormat := false
	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	usageModified := false
	usageHasTokenFields := false
	if rawUsage, ok := envelope["usage"].(map[string]any); ok {
		for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens"} {
			if value, present := rawUsage[key]; present && value != nil {
				usageHasTokenFields = true
				break
			}
		}
	}
	if !usageHasTokenFields && simpleResponse.Usage.PromptTokens == 0 {
		completionTokens := simpleResponse.Usage.CompletionTokens
		if completionTokens == 0 {
			for _, choice := range simpleResponse.Choices {
				ctkm := service.CountTextToken(choice.Message.StringContent()+choice.Message.GetReasoningContent(), info.UpstreamModelName)
				completionTokens += ctkm
			}
		}
		simpleResponse.Usage = dto.Usage{
			PromptTokens:     info.GetEstimatePromptTokens(),
			CompletionTokens: completionTokens,
			TotalTokens:      info.GetEstimatePromptTokens() + completionTokens,
		}
		usageModified = true
	}

	applyUsagePostProcessing(info, &simpleResponse.Usage, responseBody)

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if usageModified {
			var bodyMap map[string]interface{}
			err = common.Unmarshal(responseBody, &bodyMap)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			bodyMap["usage"] = simpleResponse.Usage
			responseBody, _ = common.Marshal(bodyMap)
		}
		if forceFormat {
			responseBody, err = common.Marshal(simpleResponse)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		} else {
			break
		}
	case types.RelayFormatClaude:
		convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatClaude, &simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		claudeRespStr, err := common.Marshal(convertResult.Value)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	case types.RelayFormatGemini:
		convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatGemini, &simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		geminiRespStr, err := common.Marshal(convertResult.Value)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = geminiRespStr
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &simpleResponse.Usage, nil
}

func writeOpenAIChatObject(c *gin.Context, info *relaycommon.RelayInfo, value any) error {
	data, err := common.Marshal(value)
	if err != nil {
		return err
	}
	if !info.ShouldIncludeUsage {
		var frame map[string]json.RawMessage
		if err := common.Unmarshal(data, &frame); err != nil {
			return err
		}
		delete(frame, "usage")
		data, err = common.Marshal(frame)
		if err != nil {
			return err
		}
	}
	return writeOpenAIStreamData(c, "data: "+string(data)+"\n\n")
}
