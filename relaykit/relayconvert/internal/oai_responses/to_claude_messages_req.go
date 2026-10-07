package oairesponses

import (
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/relaykit/types"
	"strings"

	"context"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	sharedclaude "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/claude"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/toolpolicy"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

func convertOpenAIResponsesRequestToClaudeMessages(c context.Context, info convmeta.Meta, request any) (any, error) {
	responsesRequest, err := OpenAIResponsesRequestFromAny(request)
	if err != nil {
		return nil, err
	}
	return OpenAIResponsesRequestToClaudeMessages(c, info, responsesRequest)
}

func OpenAIResponsesRequestToClaudeMessages(c context.Context, info convmeta.Meta, req *dto.OpenAIResponsesRequest) (*dto.ClaudeRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if err := ValidateRequestChatUnsupportedFields(req); err != nil {
		return nil, err
	}

	claudeRequest := &dto.ClaudeRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}
	if req.MaxOutputTokens != nil {
		claudeRequest.MaxTokens = kitutil.GetPointer(*req.MaxOutputTokens)
	}
	if claudeRequest.MaxTokens == nil {
		if defaultMaxTokens, configured := convmeta.OptionsOf(info).Claude.DefaultMaxTokensFor(req.Model); configured {
			value := uint(defaultMaxTokens)
			claudeRequest.MaxTokens = &value
		}
	}

	functions, err := RequestFunctionDeclarations(req.Tools)
	if err != nil {
		return nil, err
	}
	if len(functions) > 0 {
		claudeRequest.Tools = responsesFunctionDeclarationsToClaudeTools(functions)
	}

	toolChoice, err := RequestToolChoiceToChat(req.ToolChoice)
	if err != nil {
		return nil, err
	}
	if toolChoice != nil || RawJSONPresent(req.ParallelToolCalls) {
		choice, err := toolpolicy.OpenAIChoice(toolChoice)
		if err != nil {
			return nil, err
		}
		tools := make([]dto.ToolCallRequest, 0, len(functions))
		for _, function := range functions {
			tools = append(tools, dto.ToolCallRequest{Type: "function", Function: function})
		}
		if err := toolpolicy.ValidateChoiceTools(choice, tools); err != nil {
			return nil, err
		}
		mapped, err := sharedclaude.MapOpenAIToolChoice(toolChoice, ParallelToolCalls(req.ParallelToolCalls))
		if err != nil {
			return nil, err
		}
		if mapped != nil {
			claudeRequest.ToolChoice = mapped
		}
	}
	applyResponsesReasoningToClaude(req, claudeRequest)

	systemMessages := make([]dto.ClaudeMediaMessage, 0)
	if RawJSONPresent(req.Instructions) {
		instructions, err := JSONString(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("invalid instructions: %w", err)
		}
		if strings.TrimSpace(instructions) != "" {
			systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
				Type: "text",
				Text: kitutil.GetPointer(instructions),
			})
		}
	}

	inputItems, err := InputItems(req.Input)
	if err != nil {
		return nil, err
	}
	for _, item := range inputItems {
		itemType := strings.TrimSpace(kitutil.Interface2String(item["type"]))
		switch itemType {
		case ResponsesInputTypeFunctionCall:
			claudeRequest.Messages = appendClaudeToolUse(claudeRequest.Messages, responsesFunctionCallItemToClaudeToolUse(item, "arguments"))
		case ResponsesInputTypeCustomToolCall:
			claudeRequest.Messages = appendClaudeToolUse(claudeRequest.Messages, responsesFunctionCallItemToClaudeToolUse(item, "input"))
		case ResponsesInputTypeFunctionCallOutput, ResponsesInputTypeCustomToolOutput:
			result, err := responsesFunctionOutputItemToClaudeToolResult(c, item)
			if err != nil {
				return nil, err
			}
			claudeRequest.Messages = appendClaudeToolResult(claudeRequest.Messages, result)
		default:
			role := responsesClaudeRole(item)
			parts, err := responsesInputContentToClaudeMediaMessages(c, item["content"])
			if err != nil {
				return nil, err
			}
			if role == "system" {
				systemMessages = append(systemMessages, parts...)
				continue
			}
			if len(parts) == 0 {
				parts = []dto.ClaudeMediaMessage{
					{
						Type: "text",
						Text: kitutil.GetPointer("..."),
					},
				}
			}
			claudeRequest.Messages = append(claudeRequest.Messages, dto.ClaudeMessage{
				Role:    role,
				Content: parts,
			})
		}
	}

	if len(systemMessages) > 0 {
		claudeRequest.System = systemMessages
	}
	claudeRequest.Messages = ensureClaudeMessagesStartWithUser(claudeRequest.Messages)
	// Checked last so every injection path has had its chance to satisfy the
	// required field.
	if claudeRequest.MaxTokens == nil {
		return nil, sharedclaude.ErrMissingMaxTokens
	}
	return claudeRequest, nil
}

func responsesFunctionDeclarationsToClaudeTools(functions []dto.FunctionRequest) []any {
	tools := make([]any, 0, len(functions))
	for _, function := range functions {
		tools = append(tools, &dto.Tool{
			Name:        function.Name,
			Description: function.Description,
			InputSchema: sharedclaude.FunctionParametersToInputSchema(function.Parameters),
			Strict:      function.Strict,
		})
	}
	return tools
}

func applyResponsesReasoningToClaude(req *dto.OpenAIResponsesRequest, claudeRequest *dto.ClaudeRequest) {
	effort := ReasoningEffort(req)
	switch effort {
	case "low":
		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: kitutil.GetPointer(1280),
		}
	case "medium":
		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: kitutil.GetPointer(2048),
		}
	case "high":
		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: kitutil.GetPointer(4096),
		}
	}
}

func responsesInputContentToClaudeMediaMessages(c context.Context, content any) ([]dto.ClaudeMediaMessage, error) {
	contentParts, err := ContentParts(content)
	if err != nil {
		return nil, err
	}

	parts := make([]dto.ClaudeMediaMessage, 0, len(contentParts))
	for _, contentPart := range contentParts {
		partType := strings.TrimSpace(kitutil.Interface2String(contentPart["type"]))
		switch partType {
		case "input_text", "output_text", "text":
			text := kitutil.Interface2String(contentPart["text"])
			if text != "" {
				parts = append(parts, dto.ClaudeMediaMessage{
					Type: "text",
					Text: kitutil.GetPointer(text),
				})
			}
		case "input_image", "input_file", "input_audio", "input_video":
			if err := validateCrossProviderMediaSource(contentPart); err != nil {
				return nil, err
			}
			if partType == "input_audio" || partType == "input_video" {
				return nil, types.NewErrorWithStatusCode(errors.New("input.content media type is not supported by Claude"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
			}
			source := ContentPartToFileSource(contentPart)
			if source == nil {
				return nil, types.NewErrorWithStatusCode(errors.New("input.content media source is missing or invalid"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
			}
			base64Data, mimeType, err := relaymedia.ResolveBase64Data(c, source, "formatting Responses input for Claude")
			if err != nil {
				var typed *types.NewAPIError
				if errors.As(err, &typed) {
					return nil, types.NewErrorWithStatusCode(errors.New("content media resolution failed"), typed.GetErrorCode(), typed.StatusCode, types.ErrOptionWithSkipRetry())
				}
				return nil, errors.New("content media resolution failed")
			}
			claudePart := dto.ClaudeMediaMessage{
				Source: &dto.ClaudeMessageSource{
					Type:      "base64",
					MediaType: mimeType,
					Data:      base64Data,
				},
			}
			if mimeType == "application/pdf" {
				claudePart.Type = "document"
			} else if mimeType == "image/jpeg" || mimeType == "image/png" || mimeType == "image/gif" || mimeType == "image/webp" {
				claudePart.Type = "image"
			} else {
				return nil, types.NewErrorWithStatusCode(errors.New("input.content media MIME type is not supported by Claude"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
			}
			parts = append(parts, claudePart)
		}
	}
	return parts, nil
}

func responsesFunctionCallItemToClaudeToolUse(item map[string]any, inputKey string) dto.ClaudeMediaMessage {
	return dto.ClaudeMediaMessage{
		Type:  "tool_use",
		Id:    CallID(item),
		Name:  strings.TrimSpace(kitutil.Interface2String(item["name"])),
		Input: ObjectValue(item[inputKey], inputKey),
	}
}

func responsesFunctionOutputItemToClaudeToolResult(c context.Context, item map[string]any) (dto.ClaudeMediaMessage, error) {
	result := dto.ClaudeMediaMessage{Type: "tool_result", ToolUseId: CallID(item)}
	value := item["output"]
	parts, ok := value.([]any)
	if !ok {
		result.Content = responseToolOutputToChatContent(value)
		return result, nil
	}
	recognized := false
	for _, raw := range parts {
		if part, ok := raw.(map[string]any); ok {
			partType, _ := part["type"].(string)
			switch strings.TrimSpace(partType) {
			case "input_text", "output_text", "text", "input_image", "input_file", "input_audio", "input_video":
				recognized = true
			}
		}
	}
	if !recognized {
		result.Content = responseToolOutputToChatContent(value)
		return result, nil
	}
	blocks := make([]dto.ClaudeMediaMessage, 0, len(parts))
	for _, raw := range parts {
		if part, ok := raw.(map[string]any); ok {
			partType, _ := part["type"].(string)
			switch strings.TrimSpace(partType) {
			case "input_text", "output_text", "text":
				text, ok := part["text"].(string)
				if !ok {
					return dto.ClaudeMediaMessage{}, types.NewErrorWithStatusCode(errors.New("input.function_call_output text must be a string"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
				}
				blocks = append(blocks, dto.ClaudeMediaMessage{Type: "text", Text: kitutil.GetPointer(text)})
				continue
			case "input_image", "input_file", "input_audio", "input_video":
				media, err := responsesInputContentToClaudeMediaMessages(c, []any{part})
				if err != nil {
					return dto.ClaudeMediaMessage{}, err
				}
				blocks = append(blocks, media...)
				continue
			}
		}
		blocks = append(blocks, dto.ClaudeMediaMessage{Type: "text", Text: kitutil.GetPointer(responseToolOutputToChatContent(raw))})
	}
	result.Content = blocks
	return result, nil
}

func appendClaudeToolUse(messages []dto.ClaudeMessage, toolUse dto.ClaudeMediaMessage) []dto.ClaudeMessage {
	if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
		last := messages[len(messages)-1]
		parts := claudeMessageContentParts(last.Content)
		parts = append(parts, toolUse)
		last.Content = parts
		messages[len(messages)-1] = last
		return messages
	}
	return append(messages, dto.ClaudeMessage{
		Role:    "assistant",
		Content: []dto.ClaudeMediaMessage{toolUse},
	})
}

func appendClaudeToolResult(messages []dto.ClaudeMessage, toolResult dto.ClaudeMediaMessage) []dto.ClaudeMessage {
	if len(messages) > 0 && messages[len(messages)-1].Role == "user" {
		last := messages[len(messages)-1]
		parts := claudeMessageContentParts(last.Content)
		parts = append(parts, toolResult)
		last.Content = parts
		messages[len(messages)-1] = last
		return messages
	}
	return append(messages, dto.ClaudeMessage{
		Role:    "user",
		Content: []dto.ClaudeMediaMessage{toolResult},
	})
}

func claudeMessageContentParts(content any) []dto.ClaudeMediaMessage {
	switch typed := content.(type) {
	case []dto.ClaudeMediaMessage:
		return typed
	case string:
		if typed == "" {
			return nil
		}
		return []dto.ClaudeMediaMessage{
			{
				Type: "text",
				Text: kitutil.GetPointer(typed),
			},
		}
	default:
		parts, _ := kitutil.Any2Type[[]dto.ClaudeMediaMessage](content)
		return parts
	}
}

func responsesClaudeRole(item map[string]any) string {
	switch strings.TrimSpace(kitutil.Interface2String(item["role"])) {
	case "assistant":
		return "assistant"
	case "system", "developer":
		return "system"
	default:
		return "user"
	}
}

func ensureClaudeMessagesStartWithUser(messages []dto.ClaudeMessage) []dto.ClaudeMessage {
	if len(messages) == 0 || messages[0].Role == "user" {
		return messages
	}
	return append([]dto.ClaudeMessage{
		{
			Role: "user",
			Content: []dto.ClaudeMediaMessage{
				{
					Type: "text",
					Text: kitutil.GetPointer("..."),
				},
			},
		},
	}, messages...)
}
