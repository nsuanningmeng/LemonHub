package claudemessages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/toolpolicy"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

const (
	webSearchMaxUsesLow    = 1
	webSearchMaxUsesMedium = 5
	webSearchMaxUsesHigh   = 10
)

type openRouterRequestReasoning struct {
	Enabled   bool   `json:"enabled"`
	Effort    string `json:"effort,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	Exclude   bool   `json:"exclude,omitempty"`
}

func ClaudeMessagesRequestToOpenAIChat(claudeRequest dto.ClaudeRequest, info convmeta.Meta) (*dto.GeneralOpenAIRequest, error) {
	return ClaudeMessagesRequestToOpenAIChatWithContext(context.Background(), claudeRequest, info)
}

func ClaudeMessagesRequestToOpenAIChatWithContext(ctx context.Context, claudeRequest dto.ClaudeRequest, info convmeta.Meta) (*dto.GeneralOpenAIRequest, error) {
	openAIRequest := dto.GeneralOpenAIRequest{
		Model:       claudeRequest.Model,
		Temperature: claudeRequest.Temperature,
	}
	if claudeRequest.MaxTokens != nil {
		openAIRequest.MaxTokens = kitutil.GetPointer(*claudeRequest.MaxTokens)
	}
	if claudeRequest.TopP != nil {
		openAIRequest.TopP = kitutil.GetPointer(*claudeRequest.TopP)
	}
	if claudeRequest.TopK != nil {
		openAIRequest.TopK = kitutil.GetPointer(*claudeRequest.TopK)
	}
	if claudeRequest.Stream != nil {
		openAIRequest.Stream = kitutil.GetPointer(*claudeRequest.Stream)
	}

	isOpenRouter := convmeta.OptionsOf(info).OpenRouterDialect
	// Standard OpenAI accepts these explicit shared effort levels. Thinking
	// budgets do not define an equivalent effort and are never bucketed here.
	// OpenRouter retains its existing verbosity/nested-reasoning dialect below.
	if !isOpenRouter && len(claudeRequest.OutputConfig) > 0 {
		var outputConfig struct {
			Effort *string `json:"effort"`
		}
		if err := kitutil.Unmarshal(claudeRequest.OutputConfig, &outputConfig); err != nil {
			return nil, types.NewErrorWithStatusCode(errors.New("output_config.effort: invalid effort configuration"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
		}
		if outputConfig.Effort != nil {
			switch *outputConfig.Effort {
			case "low", "medium", "high":
				openAIRequest.ReasoningEffort = *outputConfig.Effort
			default:
				return nil, types.NewErrorWithStatusCode(errors.New("output_config.effort: unsupported effort for OpenAI conversion"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
			}
		}
	}
	if isOpenRouter {
		if effort := claudeRequest.GetEfforts(); effort != "" {
			effortBytes, _ := kitutil.Marshal(effort)
			openAIRequest.Verbosity = effortBytes
		}
		if claudeRequest.Thinking != nil {
			var reasoningConfig openRouterRequestReasoning
			if claudeRequest.Thinking.Type == "enabled" {
				reasoningConfig = openRouterRequestReasoning{
					Enabled:   true,
					MaxTokens: claudeRequest.Thinking.GetBudgetTokens(),
				}
			} else if claudeRequest.Thinking.Type == "adaptive" {
				reasoningConfig = openRouterRequestReasoning{
					Enabled: true,
				}
			}
			reasoningJSON, err := kitutil.Marshal(reasoningConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal reasoning: %w", err)
			}
			openAIRequest.Reasoning = reasoningJSON
		}
	} else if info != nil {
		thinkingSuffix := "-thinking"
		if strings.HasSuffix(info.GetOriginModelName(), thinkingSuffix) &&
			!strings.HasSuffix(openAIRequest.Model, thinkingSuffix) {
			openAIRequest.Model = openAIRequest.Model + thinkingSuffix
		}
	}

	if len(claudeRequest.StopSequences) > 0 {
		openAIRequest.Stop = claudeRequest.StopSequences
	}

	tools, err := kitutil.Any2Type[[]dto.Tool](claudeRequest.Tools)
	if err != nil {
		return nil, toolpolicy.Invalid("tools")
	}
	if claudeRequest.ToolChoice != nil {
		choice, err := kitutil.Any2Type[dto.ClaudeToolChoice](claudeRequest.ToolChoice)
		if err != nil {
			return nil, toolpolicy.Invalid("tool_choice")
		}
		switch choice.Type {
		case "auto", "none":
			openAIRequest.ToolChoice = choice.Type
		case "any":
			openAIRequest.ToolChoice = "required"
		case "tool":
			if choice.Name == "" {
				return nil, toolpolicy.Invalid("tool_choice.name")
			}
			openAIRequest.ToolChoice = map[string]any{"type": "function", "function": map[string]any{"name": choice.Name}}
		default:
			return nil, toolpolicy.Invalid("tool_choice.type")
		}
		if choice.DisableParallelToolUse != nil {
			openAIRequest.ParallelTooCalls = kitutil.GetPointer(!*choice.DisableParallelToolUse)
		}
	}
	openAITools := make([]dto.ToolCallRequest, 0)
	for _, claudeTool := range tools {
		if err := toolpolicy.ValidateClaudeCallers(claudeTool.AllowedCallers); err != nil {
			return nil, err
		}
		openAITool := dto.ToolCallRequest{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        claudeTool.Name,
				Description: claudeTool.Description,
				Parameters:  claudeTool.InputSchema,
				Strict:      claudeTool.Strict,
			},
		}
		openAITools = append(openAITools, openAITool)
	}
	openAIRequest.Tools = openAITools

	openAIMessages := make([]dto.Message, 0)
	if claudeRequest.System != nil {
		if claudeRequest.IsStringSystem() && claudeRequest.GetStringSystem() != "" {
			openAIMessage := dto.Message{
				Role: "system",
			}
			openAIMessage.SetStringContent(claudeRequest.GetStringSystem())
			openAIMessages = append(openAIMessages, openAIMessage)
		} else {
			systems := claudeRequest.ParseSystem()
			if len(systems) > 0 {
				openAIMessage := dto.Message{
					Role: "system",
				}
				isOpenRouterClaude := isOpenRouter && strings.HasPrefix(convmeta.UpstreamModelName(info), "anthropic/claude")
				if isOpenRouterClaude {
					systemMediaMessages := make([]dto.MediaContent, 0, len(systems))
					for _, system := range systems {
						message := dto.MediaContent{
							Type:         "text",
							Text:         system.GetText(),
							CacheControl: system.CacheControl,
						}
						systemMediaMessages = append(systemMediaMessages, message)
					}
					openAIMessage.SetMediaContent(systemMediaMessages)
				} else {
					systemStr := ""
					for _, system := range systems {
						if system.Text != nil {
							systemStr += *system.Text
						}
					}
					openAIMessage.SetStringContent(systemStr)
				}
				openAIMessages = append(openAIMessages, openAIMessage)
			}
		}
	}

	toolNames := make(map[string]string)
	for _, message := range claudeRequest.Messages {
		if message.IsStringContent() {
			continue
		}
		content, err := message.ParseContent()
		if err != nil {
			return nil, invalidClaudeMedia("messages.content", "malformed content blocks")
		}
		for _, block := range content {
			if block.Type == "tool_use" {
				if _, exists := toolNames[block.Id]; exists {
					continue
				}
				toolNames[block.Id] = block.Name
			}
		}
	}
	var pendingMedia []dto.MediaContent
	flushMedia := func() {
		if len(pendingMedia) > 0 {
			message := dto.Message{Role: "user"}
			message.SetMediaContent(pendingMedia)
			openAIMessages = append(openAIMessages, message)
			pendingMedia = nil
		}
	}
	for messageIndex, claudeMessage := range claudeRequest.Messages {
		if claudeMessage.IsStringContent() {
			flushMedia()
			message := dto.Message{Role: claudeMessage.Role}
			message.SetStringContent(claudeMessage.GetStringContent())
			openAIMessages = append(openAIMessages, message)
			continue
		}
		content, err := claudeMessage.ParseContent()
		if err != nil {
			return nil, invalidClaudeMedia("messages.content", "malformed content blocks")
		}
		hasResults := false
		for _, block := range content {
			if block.Type == "tool_result" {
				hasResults = true
			}
		}
		if claudeMessage.Role != "user" || !hasResults {
			flushMedia()
		}
		message := dto.Message{Role: claudeMessage.Role}
		var parts []dto.MediaContent
		var calls []dto.ToolCallRequest
		for blockIndex, block := range content {
			path := fmt.Sprintf("messages[%d].content[%d]", messageIndex, blockIndex)
			switch block.Type {
			case "tool_use":
				calls = append(calls, dto.ToolCallRequest{ID: block.Id, Type: "function", Function: dto.FunctionRequest{Name: block.Name, Arguments: requestToJSONString(block.Input)}})
			case "tool_result":
				text, mediaParts, err := claudeToolResultContent(ctx, block.Content, path+".content")
				if err != nil {
					return nil, err
				}
				name := block.Name
				if name == "" {
					name = toolNames[block.ToolUseId]
				}
				result := dto.Message{Role: "tool", Name: &name, ToolCallId: block.ToolUseId}
				result.SetStringContent(text)
				openAIMessages = append(openAIMessages, result)
				parts = append(parts, mediaParts...)
			case "thinking", "redacted_thinking":
				// Thinking/signature compatibility is unchanged.
			default:
				part, err := claudeContentPart(ctx, block, path)
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
			}
		}
		if hasResults && claudeMessage.Role == "user" {
			pendingMedia = append(pendingMedia, parts...)
			continue
		}
		if len(calls) > 0 {
			message.SetToolCalls(calls)
		}
		if len(parts) > 0 {
			message.SetMediaContent(parts)
		}
		if len(parts) > 0 || len(calls) > 0 {
			openAIMessages = append(openAIMessages, message)
		}
	}
	flushMedia()

	openAIRequest.Messages = openAIMessages
	return &openAIRequest, nil
}

func requestToJSONString(v interface{}) string {
	b, err := kitutil.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
