// Package toolpolicy checks cross-protocol tool constraints before dispatch.
package toolpolicy

import (
	"encoding/json"
	"errors"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

type Choice struct {
	Mode   string
	Names  []string
	Subset bool
}

func Invalid(field string) error {
	return types.NewErrorWithStatusCode(errors.New(field+": tool constraint cannot be represented by target protocol"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
}

func OpenAIChoice(value any) (Choice, error) {
	if value == nil {
		return Choice{}, nil
	}
	if mode, ok := value.(string); ok {
		switch mode {
		case "auto", "none", "required":
			return Choice{Mode: mode}, nil
		}
		return Choice{}, Invalid("tool_choice")
	}
	object, err := kitutil.Any2Type[map[string]any](value)
	if err != nil || object == nil {
		return Choice{}, Invalid("tool_choice")
	}
	kind, _ := object["type"].(string)
	switch kind {
	case "function":
		name, _ := object["name"].(string)
		if function, ok := object["function"].(map[string]any); ok {
			name, _ = function["name"].(string)
		}
		if name == "" {
			return Choice{}, Invalid("tool_choice.function")
		}
		return Choice{Mode: "required", Names: []string{name}}, nil
	case "allowed_tools":
		allowed, ok := object["allowed_tools"].(map[string]any)
		if !ok {
			allowed = object // Responses uses mode/tools at the top level.
		}
		mode, _ := allowed["mode"].(string)
		if mode != "auto" && mode != "required" {
			return Choice{}, Invalid("tool_choice.allowed_tools.mode")
		}
		tools, ok := allowed["tools"].([]any)
		if !ok || len(tools) == 0 {
			return Choice{}, Invalid("tool_choice.allowed_tools.tools")
		}
		choice := Choice{Mode: mode, Subset: true}
		seen := make(map[string]bool)
		for _, tool := range tools {
			entry, ok := tool.(map[string]any)
			if !ok || entry["type"] != "function" {
				return Choice{}, Invalid("tool_choice.allowed_tools.tools")
			}
			name, _ := entry["name"].(string)
			if function, ok := entry["function"].(map[string]any); ok {
				name, _ = function["name"].(string)
			}
			if name == "" {
				return Choice{}, Invalid("tool_choice.allowed_tools.tools")
			}
			if !seen[name] {
				choice.Names = append(choice.Names, name)
				seen[name] = true
			}
		}
		return choice, nil
	default:
		return Choice{}, Invalid("tool_choice")
	}
}

func ValidateChoiceTools(choice Choice, tools []dto.ToolCallRequest) error {
	if len(choice.Names) == 0 {
		return nil
	}
	declared := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if tool.Type == "function" {
			declared[tool.Function.Name] = true
		}
	}
	for _, name := range choice.Names {
		if !declared[name] {
			return Invalid("tool_choice.tools")
		}
	}
	return nil
}

// Gemini's ANY mode supports allowedFunctionNames; AUTO has no equivalent
// callable subset, and parallel=false/strict=true cannot be silently dropped.
func GeminiConfig(req dto.GeneralOpenAIRequest) (*dto.ToolConfig, error) {
	choice, err := OpenAIChoice(req.ToolChoice)
	if err != nil {
		return nil, err
	}
	if err := ValidateChoiceTools(choice, req.Tools); err != nil {
		return nil, err
	}
	if choice.Subset && choice.Mode == "auto" {
		return nil, Invalid("tool_choice.allowed_tools")
	}
	if choice.Mode != "none" && len(req.Tools) > 0 {
		if req.ParallelTooCalls != nil && !*req.ParallelTooCalls {
			return nil, Invalid("parallel_tool_calls")
		}
		for _, tool := range req.Tools {
			if tool.Function.Strict != nil && *tool.Function.Strict {
				return nil, Invalid("tools.function.strict")
			}
		}
	}
	if choice.Mode == "" {
		return nil, nil
	}
	mode := map[string]string{"auto": "AUTO", "none": "NONE", "required": "ANY"}[choice.Mode]
	return &dto.ToolConfig{FunctionCallingConfig: &dto.FunctionCallingConfig{Mode: dto.FunctionCallingConfigMode(mode), AllowedFunctionNames: append([]string(nil), choice.Names...)}}, nil
}

// Only direct caller restrictions are equivalent to ordinary model-selected
// function calls. Opaque programmatic execution contexts cannot cross providers.
func ValidateClaudeCallers(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var callers []string
	if err := kitutil.Unmarshal(raw, &callers); err != nil || len(callers) != 1 || callers[0] != "direct" {
		return Invalid("tools.allowed_callers")
	}
	return nil
}
