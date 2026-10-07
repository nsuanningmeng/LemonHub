package oairesponses

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/toolpolicy"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

const (
	responsesInputTypeFunctionCall       = "function_call"
	responsesInputTypeFunctionCallOutput = "function_call_output"
	responsesInputTypeCustomToolCall     = "custom_tool_call"
	responsesInputTypeCustomToolOutput   = "custom_tool_call_output"
)

const (
	ResponsesInputTypeFunctionCall       = responsesInputTypeFunctionCall
	ResponsesInputTypeFunctionCallOutput = responsesInputTypeFunctionCallOutput
	ResponsesInputTypeCustomToolCall     = responsesInputTypeCustomToolCall
	ResponsesInputTypeCustomToolOutput   = responsesInputTypeCustomToolOutput
)

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}
	if err := validateResponsesRequestChatUnsupportedFields(req); err != nil {
		return nil, err
	}

	messages, err := responsesRequestMessagesToChat(req)
	if err != nil {
		return nil, err
	}

	tools, err := responsesRequestToolsToChat(req.Tools)
	if err != nil {
		return nil, err
	}

	toolChoice, err := responsesRequestToolChoiceToChat(req.ToolChoice)
	if err != nil {
		return nil, err
	}

	responseFormat, err := responsesRequestTextToChatResponseFormat(req.Text)
	if err != nil {
		return nil, err
	}

	out := &dto.GeneralOpenAIRequest{
		Model:                req.Model,
		Messages:             messages,
		Stream:               req.Stream,
		StreamOptions:        req.StreamOptions,
		MaxCompletionTokens:  req.MaxOutputTokens,
		Temperature:          req.Temperature,
		TopP:                 req.TopP,
		TopLogProbs:          req.TopLogProbs,
		ResponseFormat:       responseFormat,
		Tools:                tools,
		ToolChoice:           toolChoice,
		User:                 req.User,
		Store:                req.Store,
		Metadata:             req.Metadata,
		SafetyIdentifier:     req.SafetyIdentifier,
		PromptCacheRetention: req.PromptCacheRetention,
		EnableThinking:       req.EnableThinking,
		ThinkingBudget:       req.ThinkingBudget,
	}

	out.FrequencyPenalty, err = responsesRawFloat(req.FrequencyPenalty)
	if err != nil {
		return nil, fmt.Errorf("invalid frequency_penalty: %w", err)
	}
	out.PresencePenalty, err = responsesRawFloat(req.PresencePenalty)
	if err != nil {
		return nil, fmt.Errorf("invalid presence_penalty: %w", err)
	}

	if req.Reasoning != nil {
		out.ReasoningEffort = req.Reasoning.Effort
	}
	if req.ServiceTier != "" {
		out.ServiceTier, _ = kitutil.Marshal(req.ServiceTier)
	}
	if len(req.ParallelToolCalls) > 0 && kitutil.GetJsonType(req.ParallelToolCalls) == "boolean" {
		var parallelToolCalls bool
		if err := kitutil.Unmarshal(req.ParallelToolCalls, &parallelToolCalls); err == nil {
			out.ParallelTooCalls = &parallelToolCalls
		}
	}
	if len(req.PromptCacheKey) > 0 && kitutil.GetJsonType(req.PromptCacheKey) == "string" {
		var promptCacheKey string
		if err := kitutil.Unmarshal(req.PromptCacheKey, &promptCacheKey); err == nil {
			out.PromptCacheKey = promptCacheKey
		}
	}

	return out, nil
}

func validateResponsesRequestChatUnsupportedFields(req *dto.OpenAIResponsesRequest) error {
	unsupported := make([]string, 0, 4)
	if rawJSONPresent(req.Conversation) {
		unsupported = append(unsupported, "conversation")
	}
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		unsupported = append(unsupported, "previous_response_id")
	}
	if rawJSONPresent(req.Prompt) {
		unsupported = append(unsupported, "prompt")
	}
	if rawJSONPresent(req.ContextManagement) {
		unsupported = append(unsupported, "context_management")
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("responses to chat conversion does not support stateful fields: %s", strings.Join(unsupported, ", "))
	}
	return nil
}

func ValidateRequestChatUnsupportedFields(req *dto.OpenAIResponsesRequest) error {
	return validateResponsesRequestChatUnsupportedFields(req)
}

func responsesRequestMessagesToChat(req *dto.OpenAIResponsesRequest) ([]dto.Message, error) {
	messages := make([]dto.Message, 0)
	if rawJSONPresent(req.Instructions) {
		instructions, err := responsesJSONString(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("invalid instructions: %w", err)
		}
		if strings.TrimSpace(instructions) != "" {
			messages = append(messages, dto.Message{Role: "system", Content: instructions})
		}
	}

	if !rawJSONPresent(req.Input) {
		return messages, nil
	}

	switch kitutil.GetJsonType(req.Input) {
	case "string":
		input, err := responsesJSONString(req.Input)
		if err != nil {
			return nil, fmt.Errorf("invalid input string: %w", err)
		}
		messages = append(messages, dto.Message{Role: "user", Content: input})
		return messages, nil
	case "array":
		var items []map[string]any
		if err := kitutil.Unmarshal(req.Input, &items); err != nil {
			return nil, fmt.Errorf("invalid input array: %w", err)
		}
		// Consecutive function/custom tool-call items collapse onto a single trailing
		// assistant message. Accumulate them in a structured slice and marshal that
		// message's tool_calls exactly once when the run ends, instead of re-parsing
		// and re-marshaling the growing slice on every item: the latter is O(N^2) in
		// the client-controlled, uncapped number of tool-call items and is a
		// single-request CPU/allocation DoS.
		var runCalls []dto.ToolCallRequest
		runIdx := -1
		flushToolCallRun := func() {
			if runIdx >= 0 {
				messages[runIdx].SetToolCalls(runCalls)
			}
			runCalls = nil
			runIdx = -1
		}
		// Chat tool results cannot contain media. Keep all results in their batch
		// consecutive, then emit their media in one following user message.
		var toolMedia []any
		flushToolMedia := func() error {
			if len(toolMedia) == 0 {
				return nil
			}
			content, err := responsesContentPartsToChatContent(toolMedia)
			if err != nil {
				return err
			}
			messages = append(messages, dto.Message{Role: "user", Content: content})
			toolMedia = nil
			return nil
		}
		for _, item := range items {
			itemType := strings.TrimSpace(kitutil.Interface2String(item["type"]))
			if itemType != responsesInputTypeFunctionCallOutput {
				if err := flushToolMedia(); err != nil {
					return nil, err
				}
			}
			if itemType == responsesInputTypeFunctionCall || itemType == responsesInputTypeCustomToolCall {
				var toolCall dto.ToolCallRequest
				var err error
				if itemType == responsesInputTypeFunctionCall {
					toolCall, err = responsesFunctionCallItemToChatToolCall(item)
				} else {
					toolCall, err = responsesCustomToolCallItemToChatToolCall(item)
				}
				if err != nil {
					return nil, err
				}
				if runIdx < 0 {
					if len(messages) == 0 || messages[len(messages)-1].Role != "assistant" {
						messages = append(messages, dto.Message{Role: "assistant"})
					}
					runIdx = len(messages) - 1
				}
				runCalls = append(runCalls, toolCall)
				continue
			}
			flushToolCallRun()
			if itemType == responsesInputTypeFunctionCallOutput {
				content, media, err := responsesToolOutputToChat(item["output"])
				if err != nil {
					return nil, err
				}
				callID := strings.TrimSpace(kitutil.Interface2String(item["call_id"]))
				messages = append(messages, dto.Message{Role: "tool", ToolCallId: callID, Content: content})
				toolMedia = append(toolMedia, media...)
				continue
			}
			nextMessages, err := responsesInputItemToChatMessages(item, messages)
			if err != nil {
				return nil, err
			}
			messages = nextMessages
		}
		flushToolCallRun()
		if err := flushToolMedia(); err != nil {
			return nil, err
		}
		return messages, nil
	default:
		return nil, fmt.Errorf("unsupported responses input type %q", kitutil.GetJsonType(req.Input))
	}
}

// responsesInputItemToChatMessages converts an ordinary message. Tool calls and
// function outputs are handled by the caller's batching loop.
func responsesInputItemToChatMessages(item map[string]any, messages []dto.Message) ([]dto.Message, error) {
	role := strings.TrimSpace(kitutil.Interface2String(item["role"]))
	if role == "" {
		role = "user"
	}
	content, err := responsesInputContentToChatContent(item["content"])
	if err != nil {
		return nil, err
	}
	return append(messages, dto.Message{Role: role, Content: content}), nil
}

func responsesInputContentToChatContent(content any) (any, error) {
	if content == nil {
		return "", nil
	}

	switch value := content.(type) {
	case string:
		return value, nil
	case []any:
		return responsesContentPartsToChatContent(value)
	case []map[string]any:
		parts := make([]any, 0, len(value))
		for _, part := range value {
			parts = append(parts, part)
		}
		return responsesContentPartsToChatContent(parts)
	default:
		return content, nil
	}
}

func responsesContentPartsToChatContent(parts []any) (any, error) {
	chatParts := make([]any, 0, len(parts))
	var textOnly strings.Builder
	onlyText := true

	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			onlyText = false
			chatParts = append(chatParts, rawPart)
			continue
		}

		partType := strings.TrimSpace(kitutil.Interface2String(part["type"]))
		switch partType {
		case "input_text", "output_text", "text":
			text := kitutil.Interface2String(part["text"])
			textOnly.WriteString(text)
			chatParts = append(chatParts, map[string]any{
				"type": dto.ContentTypeText,
				"text": text,
			})
		case "input_image":
			imageURL, err := responsesImagePartToChatImageURL(part)
			if err != nil {
				return nil, err
			}
			onlyText = false
			chatParts = append(chatParts, map[string]any{
				"type":      dto.ContentTypeImageURL,
				"image_url": imageURL,
			})
		case "input_file":
			file, err := responsesFilePartToChatFile(part)
			if err != nil {
				return nil, err
			}
			onlyText = false
			chatParts = append(chatParts, map[string]any{
				"type": dto.ContentTypeFile,
				"file": file,
			})
		case "input_audio":
			audio, ok := responsesPartPayload(part, "input_audio").(map[string]any)
			if !ok {
				return nil, errors.New("input_audio must contain an object")
			}
			for _, key := range []string{"data", "format"} {
				value, ok := audio[key].(string)
				if !ok || strings.TrimSpace(value) == "" {
					return nil, fmt.Errorf("input_audio %s must be a non-empty string", key)
				}
			}
			onlyText = false
			chatParts = append(chatParts, map[string]any{
				"type":        dto.ContentTypeInputAudio,
				"input_audio": audio,
			})
		case "input_video":
			videoURL, err := responsesVideoPartToChatVideoURL(part)
			if err != nil {
				return nil, err
			}
			onlyText = false
			chatParts = append(chatParts, map[string]any{
				"type":      dto.ContentTypeVideoUrl,
				"video_url": videoURL,
			})
		default:
			onlyText = false
			chatParts = append(chatParts, part)
		}
	}

	if onlyText {
		return textOnly.String(), nil
	}
	return chatParts, nil
}

func responsesFunctionCallItemToChatToolCall(item map[string]any) (dto.ToolCallRequest, error) {
	name := strings.TrimSpace(kitutil.Interface2String(item["name"]))
	if name == "" {
		return dto.ToolCallRequest{}, errors.New("function_call item is missing name")
	}
	return dto.ToolCallRequest{
		ID:   responsesCallID(item),
		Type: "function",
		Function: dto.FunctionRequest{
			Name:      name,
			Arguments: responsesArgumentsString(item["arguments"]),
		},
	}, nil
}

func responsesCustomToolCallItemToChatToolCall(item map[string]any) (dto.ToolCallRequest, error) {
	raw, err := kitutil.Marshal(item)
	if err != nil {
		return dto.ToolCallRequest{}, err
	}
	return dto.ToolCallRequest{
		ID:     responsesCallID(item),
		Type:   dto.CustomType,
		Custom: raw,
		Function: dto.FunctionRequest{
			Name:      strings.TrimSpace(kitutil.Interface2String(item["name"])),
			Arguments: responsesArgumentsString(item["input"]),
		},
	}, nil
}

func responsesRequestToolsToChat(raw json.RawMessage) ([]dto.ToolCallRequest, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}

	var tools []map[string]any
	if err := kitutil.Unmarshal(raw, &tools); err != nil {
		return nil, fmt.Errorf("invalid tools: %w", err)
	}

	out := make([]dto.ToolCallRequest, 0, len(tools))
	for _, tool := range tools {
		toolType := strings.TrimSpace(kitutil.Interface2String(tool["type"]))
		if toolType == "function" {
			var strict *bool
			if value, exists := tool["strict"]; exists && value != nil {
				flag, ok := value.(bool)
				if !ok {
					return nil, toolpolicy.Invalid("tools.strict")
				}
				strict = &flag
			}
			out = append(out, dto.ToolCallRequest{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        strings.TrimSpace(kitutil.Interface2String(tool["name"])),
					Description: kitutil.Interface2String(tool["description"]),
					Parameters:  tool["parameters"],
					Strict:      strict,
				},
			})
			continue
		}

		rawTool, err := kitutil.Marshal(tool)
		if err != nil {
			return nil, err
		}
		out = append(out, dto.ToolCallRequest{
			Type:   toolType,
			Custom: rawTool,
		})
	}
	return out, nil
}

func responsesRequestToolChoiceToChat(raw json.RawMessage) (any, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}
	if kitutil.GetJsonType(raw) == "string" {
		var choice string
		if err := kitutil.Unmarshal(raw, &choice); err != nil {
			return nil, fmt.Errorf("invalid tool_choice: %w", err)
		}
		return choice, nil
	}

	var choice map[string]any
	if err := kitutil.Unmarshal(raw, &choice); err != nil {
		return nil, fmt.Errorf("invalid tool_choice: %w", err)
	}
	if kitutil.Interface2String(choice["type"]) == "allowed_tools" {
		parsed, err := toolpolicy.OpenAIChoice(choice)
		if err != nil {
			return nil, err
		}
		tools := make([]map[string]any, 0, len(parsed.Names))
		for _, name := range parsed.Names {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": name}})
		}
		return map[string]any{"type": "allowed_tools", "allowed_tools": map[string]any{"mode": parsed.Mode, "tools": tools}}, nil
	}
	if kitutil.Interface2String(choice["type"]) == "function" {
		name := strings.TrimSpace(kitutil.Interface2String(choice["name"]))
		if name != "" {
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": name,
				},
			}, nil
		}
	}
	return choice, nil
}

func RequestToolChoiceToChat(raw json.RawMessage) (any, error) {
	return responsesRequestToolChoiceToChat(raw)
}

func responsesRequestTextToChatResponseFormat(raw json.RawMessage) (*dto.ResponseFormat, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}

	var textConfig map[string]any
	if err := kitutil.Unmarshal(raw, &textConfig); err != nil {
		return nil, fmt.Errorf("invalid text config: %w", err)
	}
	format, ok := textConfig["format"].(map[string]any)
	if !ok {
		return nil, nil
	}

	formatType := strings.TrimSpace(kitutil.Interface2String(format["type"]))
	if formatType == "" {
		return nil, nil
	}

	out := &dto.ResponseFormat{Type: formatType}
	if formatType == "json_schema" {
		schemaRaw, err := kitutil.Marshal(format)
		if err != nil {
			return nil, err
		}
		out.JsonSchema = schemaRaw
	}
	return out, nil
}

func RequestTextToChatResponseFormat(raw json.RawMessage) (*dto.ResponseFormat, error) {
	return responsesRequestTextToChatResponseFormat(raw)
}

func responsesImagePartToChatImageURL(part map[string]any) (map[string]any, error) {
	image := map[string]any{}
	if value, exists := part["image_url"]; exists && value != nil {
		switch value := value.(type) {
		case string:
			image["url"] = value
		case map[string]any:
			for key, v := range value {
				image[key] = v
			}
		default:
			return nil, errors.New("input_image image_url must be a string or object")
		}
	} else {
		for _, key := range []string{"url", "file_id", "detail"} {
			if value, ok := part[key]; ok {
				image[key] = value
			}
		}
	}
	for _, key := range []string{"file_id", "detail"} {
		if image[key] == nil && part[key] != nil {
			image[key] = part[key]
		}
	}
	hasSource := false
	for _, key := range []string{"url", "file_id"} {
		if value, exists := image[key]; exists {
			if value == nil {
				delete(image, key)
				continue
			}
			source, ok := value.(string)
			if !ok || strings.TrimSpace(source) == "" {
				return nil, fmt.Errorf("input_image %s must be a non-empty string", key)
			}
			hasSource = true
		}
	}
	if !hasSource {
		return nil, errors.New("input_image is missing url or file_id")
	}
	if detail, exists := image["detail"]; exists {
		if detail == nil {
			delete(image, "detail")
		} else if _, ok := detail.(string); !ok {
			return nil, errors.New("input_image detail must be a string")
		}
	}
	return image, nil
}

func responsesFilePartToChatFile(part map[string]any) (map[string]any, error) {
	file := map[string]any{}
	if value, exists := part["file"]; exists && value != nil {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("input_file file must be an object")
		}
		for key, value := range object {
			file[key] = value
		}
	} else {
		for _, key := range []string{"file_id", "file_data", "filename", "file_url"} {
			if value, ok := part[key]; ok {
				file[key] = value
			}
		}
	}
	hasSource := false
	for _, key := range []string{"file_id", "file_data", "file_url"} {
		if value, exists := file[key]; exists {
			if value == nil {
				delete(file, key)
				continue
			}
			source, ok := value.(string)
			if !ok || strings.TrimSpace(source) == "" {
				return nil, fmt.Errorf("input_file %s must be a non-empty string", key)
			}
			hasSource = true
		}
	}
	if !hasSource {
		return nil, errors.New("input_file is missing file_id, file_data or file_url")
	}
	if filename, exists := file["filename"]; exists {
		if filename == nil {
			delete(file, "filename")
		} else if _, ok := filename.(string); !ok {
			return nil, errors.New("input_file filename must be a string")
		}
	}
	return file, nil
}

func responsesVideoPartToChatVideoURL(part map[string]any) (string, error) {
	value, exists := part["video_url"]
	if !exists || value == nil {
		value = part["url"]
	}
	if object, ok := value.(map[string]any); ok {
		value = object["url"]
	}
	url, ok := value.(string)
	if !ok || strings.TrimSpace(url) == "" {
		return "", errors.New("input_video url must be a non-empty string")
	}
	return url, nil
}

func responsesPartPayload(part map[string]any, key string) any {
	if value, ok := part[key]; ok {
		return value
	}
	payload := make(map[string]any, len(part))
	for k, value := range part {
		if k == "type" {
			continue
		}
		payload[k] = value
	}
	return payload
}

func responsesCallID(item map[string]any) string {
	callID := strings.TrimSpace(kitutil.Interface2String(item["call_id"]))
	if callID != "" {
		return callID
	}
	return strings.TrimSpace(kitutil.Interface2String(item["id"]))
}

func CallID(item map[string]any) string {
	return responsesCallID(item)
}

func responsesArgumentsString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		raw, err := kitutil.Marshal(v)
		if err != nil {
			return kitutil.Interface2String(v)
		}
		return string(raw)
	}
}

// responsesToolOutputToChat extracts only known media. Unknown values remain JSON
// text in their original order; outputs without media keep their legacy encoding.
func responsesToolOutputToChat(value any) (string, []any, error) {
	parts, ok := value.([]any)
	if !ok {
		return responseToolOutputToChatContent(value), nil, nil
	}
	hasMedia := false
	for _, rawPart := range parts {
		if part, ok := rawPart.(map[string]any); ok {
			switch strings.TrimSpace(kitutil.Interface2String(part["type"])) {
			case "input_image", "input_file", "input_audio", "input_video":
				hasMedia = true
			}
		}
		if hasMedia {
			break
		}
	}
	if !hasMedia {
		return responseToolOutputToChatContent(value), nil, nil
	}
	var media []any
	var textParts, placeholders []string
	for _, rawPart := range parts {
		part, isObject := rawPart.(map[string]any)
		if isObject {
			partType := strings.TrimSpace(kitutil.Interface2String(part["type"]))
			switch partType {
			case "input_image", "input_file", "input_audio", "input_video":
				media = append(media, part)
				placeholders = append(placeholders, "["+strings.TrimPrefix(partType, "input_")+"]")
				continue
			case "input_text", "output_text", "text":
				if text, ok := part["text"].(string); ok {
					textParts = append(textParts, text)
					continue
				}
			}
		}
		encoded, err := kitutil.Marshal(rawPart)
		if err != nil {
			return "", nil, fmt.Errorf("invalid function_call_output part: %w", err)
		}
		textParts = append(textParts, string(encoded))
	}
	if len(textParts) == 0 {
		textParts = placeholders
	}
	return strings.Join(textParts, "\n"), media, nil
}

func responseToolOutputToChatContent(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		raw, err := kitutil.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(raw)
	}
}

func responsesRawFloat(raw json.RawMessage) (*float64, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}
	var value float64
	if err := kitutil.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func responsesJSONString(raw json.RawMessage) (string, error) {
	if kitutil.GetJsonType(raw) != "string" {
		return string(raw), nil
	}
	var value string
	if err := kitutil.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func rawJSONPresent(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	return kitutil.GetJsonType(raw) != "null"
}

func JSONString(raw json.RawMessage) (string, error) {
	return responsesJSONString(raw)
}

func RawJSONPresent(raw json.RawMessage) bool {
	return rawJSONPresent(raw)
}
