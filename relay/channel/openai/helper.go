package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// 辅助函数
func HandleStreamFormat(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool, deferClaudeTerminal ...bool) error {
	info.SendResponseCount++

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		return sendStreamData(c, info, data, forceFormat, thinkToContent)
	case types.RelayFormatClaude:
		return handleClaudeFormat(c, data, info, len(deferClaudeTerminal) > 0 && deferClaudeTerminal[0])
	case types.RelayFormatGemini:
		return handleGeminiFormat(c, data, info)
	}
	return nil
}

func handleClaudeFormat(c *gin.Context, data string, info *relaycommon.RelayInfo, deferTerminal bool) error {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(data), &streamResponse); err != nil {
		return err
	}
	streamResponse.Model = info.PublicResponseModelName(streamResponse.Model)

	if deferTerminal {
		// Source finish/usage may precede a fatal frame. Keep content incremental,
		// but finalize the Claude message only after the scanner has succeeded.
		state := info.EnsureClaudeConvertInfo()
		streamResponse.Usage = nil
		for i := range streamResponse.Choices {
			if reason := streamResponse.Choices[i].FinishReason; reason != nil && *reason != "" {
				state.FinishReason = *reason
			}
			streamResponse.Choices[i].FinishReason = nil
		}
	} else if streamResponse.Usage != nil {
		info.EnsureClaudeConvertInfo().Usage = streamResponse.Usage
	}
	result, err := relayconvert.ConvertStreamResponse(c, info, types.RelayFormatClaude, &streamResponse)
	if err != nil {
		return err
	}
	claudeResponses, ok := result.Value.([]*dto.ClaudeResponse)
	if !ok {
		return fmt.Errorf("expected Claude stream responses, got %T", result.Value)
	}
	for _, resp := range claudeResponses {
		if err := writeOpenAIStreamObject(c, resp, resp.Type); err != nil {
			return err
		}
	}
	return nil
}

func handleGeminiFormat(c *gin.Context, data string, info *relaycommon.RelayInfo) error {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(data), &streamResponse); err != nil {
		logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
		return err
	}
	streamResponse.Model = info.PublicResponseModelName(streamResponse.Model)

	result, err := relayconvert.ConvertStreamResponse(c, info, types.RelayFormatGemini, &streamResponse)
	if err != nil {
		return err
	}
	geminiResponse, ok := result.Value.(*dto.GeminiChatResponse)
	if !ok {
		return fmt.Errorf("expected Gemini stream response, got %T", result.Value)
	}

	// 如果返回 nil，表示没有实际内容，跳过发送
	if geminiResponse == nil {
		return nil
	}

	geminiResponseStr, err := common.Marshal(geminiResponse)
	if err != nil {
		logger.LogError(c, "failed to marshal gemini response: "+err.Error())
		return err
	}

	// send gemini format response
	return writeOpenAIStreamData(c, "data: "+string(geminiResponseStr)+"\n\n")
}

func ProcessStreamResponse(streamResponse dto.ChatCompletionsStreamResponse, responseTextBuilder *strings.Builder, toolCount *int) error {
	for _, choice := range streamResponse.Choices {
		responseTextBuilder.WriteString(choice.Delta.GetContentString())
		responseTextBuilder.WriteString(choice.Delta.GetReasoningContent())
		if choice.Delta.ToolCalls != nil {
			if len(choice.Delta.ToolCalls) > *toolCount {
				*toolCount = len(choice.Delta.ToolCalls)
			}
			for _, tool := range choice.Delta.ToolCalls {
				responseTextBuilder.WriteString(tool.Function.Name)
				responseTextBuilder.WriteString(tool.Function.Arguments)
			}
		}
	}
	return nil
}

func processTokenData(relayMode int, data string, responseTextBuilder *strings.Builder, toolCount *int) error {
	switch relayMode {
	case relayconstant.RelayModeChatCompletions:
		var streamResponse dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			return err
		}
		return ProcessStreamResponse(streamResponse, responseTextBuilder, toolCount)
	case relayconstant.RelayModeCompletions:
		var streamResponse dto.CompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			return err
		}
		processCompletionsStreamResponse(streamResponse, responseTextBuilder)
	}
	return nil
}

func processCompletionsStreamResponse(streamResponse dto.CompletionsStreamResponse, responseTextBuilder *strings.Builder) {
	for _, choice := range streamResponse.Choices {
		responseTextBuilder.WriteString(choice.Text)
	}
}

func HandleFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, lastStreamData string,
	responseId string, createAt int64, model string, systemFingerprint string,
	usage *dto.Usage, containStreamUsage bool) {

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if info.ShouldIncludeUsage && !containStreamUsage {
			response := helper.GenerateFinalUsageResponse(responseId, createAt, model, *usage)
			response.SetSystemFingerprint(systemFingerprint)
			helper.ObjectData(c, response)
		}
		helper.Done(c)

	case types.RelayFormatClaude:
		var streamResponse dto.ChatCompletionsStreamResponse
		if err := common.Unmarshal(common.StringToByteSlice(lastStreamData), &streamResponse); err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			return
		}
		streamResponse.Model = info.PublicResponseModelName(streamResponse.Model)

		info.ClaudeConvertInfo.Usage = usage

		result, err := relayconvert.ConvertStreamResponse(c, info, types.RelayFormatClaude, &streamResponse)
		if err != nil {
			common.SysLog("error converting Claude stream response: " + err.Error())
			return
		}
		claudeResponses, ok := result.Value.([]*dto.ClaudeResponse)
		if !ok {
			common.SysLog(fmt.Sprintf("expected Claude stream responses, got %T", result.Value))
			return
		}
		for _, resp := range claudeResponses {
			_ = helper.ClaudeData(c, *resp)
		}
		info.ClaudeConvertInfo.Done = true

	case types.RelayFormatGemini:
		var streamResponse dto.ChatCompletionsStreamResponse
		if err := common.Unmarshal(common.StringToByteSlice(lastStreamData), &streamResponse); err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			return
		}
		streamResponse.Model = info.PublicResponseModelName(streamResponse.Model)

		// 这里处理的是 openai 最后一个流响应，其 delta 为空，有 finish_reason 字段
		// 因此相比较于 google 官方的流响应，由 openai 转换而来会多一个 parts 为空，finishReason 为 STOP 的响应
		// 而包含最后一段文本输出的响应（倒数第二个）的 finishReason 为 null
		// 暂不知是否有程序会不兼容。

		result, err := relayconvert.ConvertStreamResponse(c, info, types.RelayFormatGemini, &streamResponse)
		if err != nil {
			common.SysLog("error converting Gemini stream response: " + err.Error())
			return
		}
		geminiResponse, ok := result.Value.(*dto.GeminiChatResponse)
		if !ok {
			common.SysLog(fmt.Sprintf("expected Gemini stream response, got %T", result.Value))
			return
		}

		// openai 流响应开头的空数据
		if geminiResponse == nil {
			return
		}

		geminiResponseStr, err := common.Marshal(geminiResponse)
		if err != nil {
			common.SysLog("error marshalling gemini response: " + err.Error())
			return
		}

		// 发送最终的 Gemini 响应
		c.Render(-1, common.CustomEvent{Data: "data: " + string(geminiResponseStr)})
		_ = helper.FlushWriter(c)
	}
}

func sendResponsesStreamData(c *gin.Context, streamResponse dto.ResponsesStreamResponse, data string) {
	if data == "" {
		return
	}
	_ = helper.ResponseChunkData(c, streamResponse, data)
}

// Keep each frame at one checked write boundary. Mark transport failures and
// return static errors so the controller can close without another write.
func writeOpenAIStreamData(c *gin.Context, frame string) error {
	if c == nil || c.Writer == nil {
		return fmt.Errorf("invalid stream writer")
	}
	if c.Request != nil {
		if err := c.Request.Context().Err(); err != nil {
			return err
		}
	}
	helper.SetEventStreamHeaders(c)
	if n, err := c.Writer.WriteString(frame); err != nil {
		c.Set("relay_stream_write_failed", true)
		if c.Request != nil {
			if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
				return requestErr
			}
		}
		return fmt.Errorf("downstream stream write failed")
	} else if n != len(frame) {
		c.Set("relay_stream_write_failed", true)
		return io.ErrShortWrite
	}
	if err := helper.FlushWriter(c); err != nil {
		c.Set("relay_stream_write_failed", true)
		if c.Request != nil {
			if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
				return requestErr
			}
		}
		return fmt.Errorf("downstream stream flush failed")
	}
	if strings.HasPrefix(frame, "event: ") {
		event := strings.TrimPrefix(strings.SplitN(frame, "\n", 2)[0], "event: ")
		switch event {
		case "error", "response.error", "response.failed", "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled", "message_stop", "message_delta":
			return nil
		}
	}
	if strings.Contains(frame, "data: ") && strings.TrimSpace(strings.TrimPrefix(frame, "data: ")) != "[DONE]" {
		helper.MarkStreamDataWritten(c)
	}
	return nil
}
func writeOpenAIStreamObject(c *gin.Context, value any, event string) error {
	data, err := common.Marshal(value)
	if err != nil {
		return err
	}
	prefix := ""
	if event != "" {
		prefix = "event: " + event + "\n"
	}
	return writeOpenAIStreamData(c, prefix+"data: "+string(data)+"\n\n")
}

// Usage is observed before this OpenAI-only presentation policy is applied.
// Keep raw extension fields rather than round-tripping through the narrow DTO.
func openAIStreamDataWithoutUsage(data string) (string, bool, error) {
	var frame map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(data, &frame); err != nil {
		return "", false, err
	}
	if _, ok := frame["usage"]; !ok {
		return data, true, nil
	}
	delete(frame, "usage")
	semantic := false
	for key := range frame {
		switch key {
		case "id", "object", "created", "model", "system_fingerprint", "service_tier", "choices", "error":
		default:
			semantic = true
		}
	}
	var choices []map[string]json.RawMessage
	if raw, ok := frame["choices"]; ok {
		if err := common.Unmarshal(raw, &choices); err != nil {
			return "", false, err
		}
	}
	for _, choice := range choices {
		for key, raw := range choice {
			switch key {
			case "index":
			case "finish_reason", "logprobs":
				if string(raw) != "null" && string(raw) != `""` {
					semantic = true
				}
			case "delta":
				var delta map[string]json.RawMessage
				if err := common.Unmarshal(raw, &delta); err != nil {
					return "", false, err
				}
				for field, value := range delta {
					switch field {
					case "role", "content", "reasoning_content", "reasoning", "refusal", "tool_calls", "function_call", "audio":
						if string(value) != "null" && string(value) != `""` && string(value) != "[]" && string(value) != "{}" {
							semantic = true
						}
					default:
						semantic = true
					}
				}
			default:
				semantic = true
			}
		}
	}
	if !semantic {
		return "", false, nil
	}
	encoded, err := common.Marshal(frame)
	return string(encoded), true, err
}

func finalizeObservedOpenAIStream(c *gin.Context, info *relaycommon.RelayInfo, responseID string, created int64, model, fingerprint string, usage *dto.Usage, containedUsage bool) error {
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if info.ShouldIncludeUsage && !containedUsage {
			response := helper.GenerateFinalUsageResponse(responseID, created, model, *usage)
			response.SetSystemFingerprint(fingerprint)
			if err := writeOpenAIStreamObject(c, response, ""); err != nil {
				return err
			}
		}
		return writeOpenAIStreamData(c, "data: [DONE]\n\n")
	case types.RelayFormatClaude:
		info.EnsureClaudeConvertInfo().Usage = usage
		state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatClaude, relayconvert.ResponseStreamOptions{})
		if err != nil {
			return err
		}
		state.SetUsage(usage)
		results, err := relayconvert.FinalizeStreamResponse(c, info, state)
		if err != nil {
			return err
		}
		for _, result := range results {
			response, ok := result.Value.(*dto.ClaudeResponse)
			if !ok {
				return fmt.Errorf("invalid Claude final event")
			}
			if err := writeOpenAIStreamObject(c, response, response.Type); err != nil {
				return err
			}
		}
	case types.RelayFormatGemini:
		if !containedUsage {
			chunk := helper.GenerateFinalUsageResponse(responseID, created, model, *usage)
			raw, err := common.Marshal(chunk)
			if err != nil {
				return err
			}
			return handleGeminiFormat(c, string(raw), info)
		}
	}
	return nil
}
