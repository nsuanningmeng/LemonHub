package claude

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/toolpolicy"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

func MapOpenAIToolChoice(value any, parallel *bool) (*dto.ClaudeToolChoice, error) {
	choice, err := toolpolicy.OpenAIChoice(value)
	if err != nil {
		return nil, err
	}
	if choice.Subset && (choice.Mode == "auto" || len(choice.Names) != 1) {
		return nil, toolpolicy.Invalid("tool_choice.allowed_tools")
	}
	if choice.Mode == "" && parallel == nil {
		return nil, nil
	}
	mapped := &dto.ClaudeToolChoice{Type: "auto"}
	switch choice.Mode {
	case "none":
		mapped.Type = "none"
	case "required":
		mapped.Type = "any"
	}
	if len(choice.Names) > 0 {
		mapped.Type = "tool"
		mapped.Name = choice.Names[0]
	}
	if parallel != nil && mapped.Type != "none" {
		mapped.DisableParallelToolUse = kitutil.GetPointer(!*parallel)
	}
	return mapped, nil
}
