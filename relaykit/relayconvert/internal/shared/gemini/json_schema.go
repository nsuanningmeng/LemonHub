package gemini

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// FunctionSchema selects an API-supported field without dropping constraints.
// Gemini API v1 only accepts its typed OpenAPI Schema subset.
func FunctionSchema(schema any, supportsJSONSchema bool) (typed, full any, err error) {
	if schema == nil {
		return nil, nil, nil
	}
	if raw, ok := schema.(json.RawMessage); ok {
		if len(raw) == 0 || kitutil.GetJsonType(raw) == "null" {
			return nil, nil, nil
		}
		if supportsJSONSchema {
			return nil, raw, nil
		}
		if err := kitutil.Unmarshal(raw, &schema); err != nil {
			return nil, nil, unsupportedJSONSchema()
		}
	}
	if supportsJSONSchema {
		return nil, schema, nil
	}
	if !fitsTypedSchema(schema, 0) {
		return nil, nil, unsupportedJSONSchema()
	}
	return CleanFunctionParameters(schema), nil, nil
}

func ApplyResponseSchema(config *dto.GeminiChatGenerationConfig, raw json.RawMessage, supportsJSONSchema bool) error {
	var format struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := kitutil.Unmarshal(raw, &format); err != nil {
		return unsupportedJSONSchema()
	}
	if len(format.Schema) == 0 || string(format.Schema) == "null" {
		return nil
	}
	if supportsJSONSchema {
		config.ResponseJsonSchema = format.Schema
		return nil
	}
	var schema any
	if err := kitutil.Unmarshal(format.Schema, &schema); err != nil {
		return unsupportedJSONSchema()
	}
	typed, _, err := FunctionSchema(schema, false)
	if err != nil {
		return err
	}
	config.ResponseSchema = typed
	return nil
}

func unsupportedJSONSchema() error {
	return types.NewErrorWithStatusCode(errors.New("schema cannot be represented without loss by the selected Gemini API version"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
}

func fitsTypedSchema(schema any, depth int) bool {
	if depth >= geminiFunctionSchemaMaxDepth {
		return false
	}
	object, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	for key, value := range object {
		if _, allowed := geminiOpenAPISchemaAllowedFields[key]; !allowed {
			return false
		}
		switch key {
		case "type":
			name, ok := value.(string)
			if !ok {
				return false
			}
			switch strings.ToLower(name) {
			case "object", "array", "string", "integer", "number", "boolean":
			default:
				return false
			}
		case "enum":
			switch values := value.(type) {
			case []string:
			case []any:
				for _, item := range values {
					if _, ok := item.(string); !ok {
						return false
					}
				}
			default:
				return false
			}
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				return false
			}
			for _, property := range properties {
				if !fitsTypedSchema(property, depth+1) {
					return false
				}
			}
		case "items":
			if !fitsTypedSchema(value, depth+1) {
				return false
			}
		case "anyOf":
			variants, ok := value.([]any)
			if !ok {
				return false
			}
			for _, variant := range variants {
				if !fitsTypedSchema(variant, depth+1) {
					return false
				}
			}
		}
	}
	return true
}
