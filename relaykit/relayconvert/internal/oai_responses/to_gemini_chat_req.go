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
	sharedgemini "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/gemini"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/toolpolicy"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

func convertOpenAIResponsesRequestToGeminiChat(c context.Context, info convmeta.Meta, request any) (any, error) {
	responsesRequest, err := OpenAIResponsesRequestFromAny(request)
	if err != nil {
		return nil, err
	}

	prepared, err := PrepareOpenAIResponsesRequest(*responsesRequest)
	if err != nil {
		return nil, err
	}
	return OpenAIResponsesRequestToGeminiChat(c, &prepared, info)
}

func OpenAIResponsesRequestToGeminiChat(c context.Context, req *dto.OpenAIResponsesRequest, info convmeta.Meta) (*dto.GeminiChatRequest, error) {
	opts := convmeta.OptionsOf(info)
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if err := ValidateRequestChatUnsupportedFields(req); err != nil {
		return nil, err
	}

	geminiRequest := &dto.GeminiChatRequest{
		GenerationConfig: dto.GeminiChatGenerationConfig{
			Temperature: req.Temperature,
		},
	}
	if req.TopP != nil && *req.TopP > 0 {
		geminiRequest.GenerationConfig.TopP = kitutil.GetPointer(*req.TopP)
	}
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		geminiRequest.GenerationConfig.MaxOutputTokens = kitutil.GetPointer(*req.MaxOutputTokens)
	}

	upstreamModelName := req.Model
	if modelName := convmeta.UpstreamModelName(info); modelName != "" {
		upstreamModelName = modelName
	}
	if opts.Gemini.SupportsImagineModel(upstreamModelName) {
		geminiRequest.GenerationConfig.ResponseModalities = []string{"TEXT", "IMAGE"}
	}
	if err := applyResponsesTextToGemini(req.Text, geminiRequest, opts.Gemini.SupportsJSONSchemaFor(upstreamModelName)); err != nil {
		return nil, err
	}
	sharedgemini.ApplyThinkingConfig(geminiRequest, info, dto.GeneralOpenAIRequest{
		Model:               req.Model,
		MaxCompletionTokens: req.MaxOutputTokens,
		ReasoningEffort:     ReasoningEffort(req),
	})

	var safetySettings []dto.GeminiChatSafetySettings
	for _, category := range sharedgemini.SafetySettingCategories {
		threshold := opts.Gemini.SafetySettingFor(category)
		if threshold == "" {
			continue
		}
		safetySettings = append(safetySettings, dto.GeminiChatSafetySettings{
			Category:  category,
			Threshold: threshold,
		})
	}
	if len(safetySettings) > 0 {
		geminiRequest.SafetySettings = safetySettings
	}

	functions, err := RequestFunctionDeclarations(req.Tools)
	if err != nil {
		return nil, err
	}
	declarations := make([]dto.GeminiFunctionDeclaration, 0, len(functions))
	for _, function := range functions {
		typed, full, err := sharedgemini.FunctionSchema(function.Parameters, opts.Gemini.SupportsJSONSchemaFor(upstreamModelName))
		if err != nil {
			return nil, err
		}
		declarations = append(declarations, dto.GeminiFunctionDeclaration{Name: function.Name, Description: function.Description, Parameters: typed, ParametersJSONSchema: full})
	}
	if len(declarations) > 0 {
		geminiRequest.SetTools([]dto.GeminiChatTool{{FunctionDeclarations: declarations}})
	}

	toolChoice, err := RequestToolChoiceToChat(req.ToolChoice)
	if err != nil {
		return nil, err
	}
	var parallel *bool
	if len(req.ParallelToolCalls) > 0 {
		if err := kitutil.Unmarshal(req.ParallelToolCalls, &parallel); err != nil {
			return nil, toolpolicy.Invalid("parallel_tool_calls")
		}
	}
	policyRequest := dto.GeneralOpenAIRequest{ToolChoice: toolChoice, ParallelTooCalls: parallel}
	for _, function := range functions {
		policyRequest.Tools = append(policyRequest.Tools, dto.ToolCallRequest{Type: "function", Function: function})
	}
	config, err := toolpolicy.GeminiConfig(policyRequest)
	if err != nil {
		return nil, err
	}
	geminiRequest.ToolConfig = config

	systemTexts := make([]string, 0)
	if RawJSONPresent(req.Instructions) {
		instructions, err := JSONString(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("invalid instructions: %w", err)
		}
		if strings.TrimSpace(instructions) != "" {
			systemTexts = append(systemTexts, instructions)
		}
	}

	inputItems, err := InputItems(req.Input)
	if err != nil {
		return nil, err
	}
	callNames := make(map[string]string)
	var pendingToolMedia []dto.GeminiPart
	flushToolMedia := func() {
		for _, part := range pendingToolMedia {
			appendGeminiContentPart(geminiRequest, "user", part)
		}
		pendingToolMedia = nil
	}
	for _, item := range inputItems {
		itemType := strings.TrimSpace(kitutil.Interface2String(item["type"]))
		if itemType != ResponsesInputTypeFunctionCallOutput {
			flushToolMedia()
		}
		switch itemType {
		case ResponsesInputTypeFunctionCall:
			part, callID, err := responsesFunctionCallItemToGeminiPart(item)
			if err != nil {
				return nil, err
			}
			sharedgemini.AttachFunctionCallThoughtSignature(opts, &part)
			if callID != "" {
				callNames[callID] = part.FunctionCall.FunctionName
			}
			appendGeminiContentPart(geminiRequest, "model", part)
		case ResponsesInputTypeFunctionCallOutput:
			part, media, err := responsesFunctionOutputItemToGeminiPart(c, item, callNames, upstreamModelName)
			if err != nil {
				return nil, err
			}
			appendGeminiContentPart(geminiRequest, "user", part)
			pendingToolMedia = append(pendingToolMedia, media...)
		default:
			role := responsesGeminiRole(item)
			if sourceRole, _ := item["role"].(string); strings.TrimSpace(sourceRole) == "developer" {
				convmeta.ReportConversionDiagnostic(info, convmeta.ConversionDiagnostic{Code: convmeta.DiagnosticDeveloperRoleMerged, Source: types.RelayFormatOpenAIResponses, Target: types.RelayFormatGemini, Field: "input.role"})
			}
			parts, err := responsesInputContentToGeminiParts(c, item["content"])
			if err != nil {
				return nil, err
			}
			if role == "system" {
				for _, part := range parts {
					if part.Text != "" {
						systemTexts = append(systemTexts, part.Text)
					}
				}
				continue
			}
			if len(parts) > 0 {
				geminiRequest.Contents = append(geminiRequest.Contents, dto.GeminiChatContent{
					Role:  role,
					Parts: parts,
				})
			}
		}
	}

	flushToolMedia()
	if len(systemTexts) > 0 {
		geminiRequest.SystemInstructions = &dto.GeminiChatContent{
			Parts: []dto.GeminiPart{{Text: strings.Join(systemTexts, "\n")}},
		}
	}

	geminiRequest.Contents = omitTrailingAssistantText(info, types.RelayFormatGemini, geminiRequest.Contents,
		func(turn dto.GeminiChatContent) bool { return turn.Role == "user" },
		func(turn dto.GeminiChatContent) bool {
			if turn.Role != "model" {
				return false
			}
			for _, part := range turn.Parts {
				if part.FunctionCall != nil {
					return false
				}
			}
			return true
		})
	return geminiRequest, nil
}

func applyResponsesTextToGemini(raw []byte, geminiRequest *dto.GeminiChatRequest, supportsJSONSchema bool) error {
	responseFormat, err := RequestTextToChatResponseFormat(raw)
	if err != nil {
		return err
	}
	if responseFormat == nil || (responseFormat.Type != "json_schema" && responseFormat.Type != "json_object") {
		return nil
	}

	geminiRequest.GenerationConfig.ResponseMimeType = "application/json"
	if len(responseFormat.JsonSchema) == 0 {
		return nil
	}

	return sharedgemini.ApplyResponseSchema(&geminiRequest.GenerationConfig, responseFormat.JsonSchema, supportsJSONSchema)
}

func responsesInputContentToGeminiParts(c context.Context, content any) ([]dto.GeminiPart, error) {
	contentParts, err := ContentParts(content)
	if err != nil {
		return nil, err
	}

	parts := make([]dto.GeminiPart, 0, len(contentParts))
	for _, contentPart := range contentParts {
		nextParts, err := responsesContentPartToGeminiParts(c, contentPart)
		if err != nil {
			return nil, err
		}
		parts = append(parts, nextParts...)
	}
	return parts, nil
}

func responsesContentPartToGeminiParts(c context.Context, part map[string]any) ([]dto.GeminiPart, error) {
	partType := strings.TrimSpace(kitutil.Interface2String(part["type"]))
	switch partType {
	case "input_text", "output_text", "text":
		text := kitutil.Interface2String(part["text"])
		if text == "" {
			return nil, nil
		}
		return []dto.GeminiPart{{Text: text}}, nil
	case "input_image", "input_file", "input_audio", "input_video":
		if err := validateCrossProviderMediaSource(part); err != nil {
			return nil, err
		}
		source := ContentPartToFileSource(part)
		if source == nil {
			return nil, types.NewErrorWithStatusCode(errors.New("input.content media source is missing or invalid"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
		}
		base64Data, mimeType, err := relaymedia.ResolveBase64Data(c, source, "formatting Responses input for Gemini")
		if err != nil {
			var typed *types.NewAPIError
			if errors.As(err, &typed) {
				return nil, types.NewErrorWithStatusCode(errors.New("content media resolution failed"), typed.GetErrorCode(), typed.StatusCode, types.ErrOptionWithSkipRetry())
			}
			return nil, errors.New("content media resolution failed")
		}
		if _, ok := sharedgemini.SupportedMimeTypes[strings.ToLower(mimeType)]; !ok {
			return nil, types.NewErrorWithStatusCode(errors.New("input.content media MIME type is not supported by Gemini"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
		}
		return []dto.GeminiPart{
			{
				InlineData: &dto.GeminiInlineData{
					MimeType: mimeType,
					Data:     base64Data,
				},
			},
		}, nil
	default:
		return nil, nil
	}
}

func responsesFunctionCallItemToGeminiPart(item map[string]any) (dto.GeminiPart, string, error) {
	name := strings.TrimSpace(kitutil.Interface2String(item["name"]))
	if name == "" {
		return dto.GeminiPart{}, "", fmt.Errorf("function_call item is missing name")
	}
	callID := CallID(item)
	return dto.GeminiPart{
		FunctionCall: &dto.FunctionCall{
			FunctionName: name,
			Arguments:    ObjectValue(item["arguments"], "arguments"),
		},
	}, callID, nil
}

// Tool results with attachments must carry binary data as media, never as JSON
// text. Older Gemini models receive attachments after the whole result batch.
func responsesFunctionOutputItemToGeminiPart(c context.Context, item map[string]any, callNames map[string]string, model string) (dto.GeminiPart, []dto.GeminiPart, error) {
	callID := CallID(item)
	name := strings.TrimSpace(kitutil.Interface2String(item["name"]))
	if name == "" {
		name = callNames[callID]
	}
	response := &dto.GeminiFunctionResponse{Name: name, Response: GeminiResponseMap(item["output"])}
	text, attachments, err := responsesToolOutputToChat(item["output"])
	if err != nil {
		return dto.GeminiPart{}, nil, err
	}
	if len(attachments) == 0 {
		return dto.GeminiPart{FunctionResponse: response}, nil, nil
	}
	parts, err := responsesInputContentToGeminiParts(c, attachments)
	if err != nil {
		return dto.GeminiPart{}, nil, err
	}
	response.Response = map[string]any{"content": text}
	// Match explicit Gemini versions even when a Vertex publisher path precedes
	// the model name. Unknown targets use ordinary user media for compatibility.
	model = strings.ToLower(model)
	if index := strings.LastIndex(model, "/"); index >= 0 {
		model = model[index+1:]
	}
	if strings.HasPrefix(model, "gemini-3") {
		for _, part := range parts {
			mimeType := strings.ToLower(part.InlineData.MimeType)
			if strings.HasPrefix(mimeType, "audio/") || strings.HasPrefix(mimeType, "video/") {
				return dto.GeminiPart{}, nil, types.NewErrorWithStatusCode(errors.New("Gemini function response does not support audio or video media"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
			}
		}
		response.Parts, err = kitutil.Marshal(parts)
		if err != nil {
			return dto.GeminiPart{}, nil, err
		}
		parts = nil
	}
	return dto.GeminiPart{FunctionResponse: response}, parts, nil
}

func appendGeminiContentPart(req *dto.GeminiChatRequest, role string, part dto.GeminiPart) {
	if len(req.Contents) > 0 && req.Contents[len(req.Contents)-1].Role == role {
		if role == "model" && part.FunctionCall != nil {
			parts := req.Contents[len(req.Contents)-1].Parts
			// Function-call parts form a prefix of the block; a new one is
			// inserted at the end of that prefix. Scan back over the (small)
			// trailing non-function-call suffix instead of forward over the
			// growing function-call prefix, so N adjacent calls stay O(N).
			insertAt := len(parts)
			for insertAt > 0 && parts[insertAt-1].FunctionCall == nil {
				insertAt--
			}
			parts = append(parts, dto.GeminiPart{})
			copy(parts[insertAt+1:], parts[insertAt:])
			parts[insertAt] = part
			req.Contents[len(req.Contents)-1].Parts = parts
			return
		}
		req.Contents[len(req.Contents)-1].Parts = append(req.Contents[len(req.Contents)-1].Parts, part)
		return
	}
	req.Contents = append(req.Contents, dto.GeminiChatContent{
		Role:  role,
		Parts: []dto.GeminiPart{part},
	})
}

func responsesGeminiRole(item map[string]any) string {
	switch strings.TrimSpace(kitutil.Interface2String(item["role"])) {
	case "assistant":
		return "model"
	case "system", "developer":
		return "system"
	case "model":
		return "model"
	default:
		return "user"
	}
}
