package openai

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// ClassifyOpenAIEmbeddedError examines only the decoded top-level error field.
// Numeric provider codes are preserved, but are never interpreted as HTTP status.
func ClassifyOpenAIEmbeddedError(errorField any) *types.NewAPIError {
	if errorField == nil {
		return nil
	}
	malformed := func() *types.NewAPIError {
		return types.NewOpenAIError(errors.New("invalid upstream error envelope"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	var upstream types.OpenAIError
	switch field := errorField.(type) {
	case string:
		if strings.TrimSpace(field) == "" {
			return nil
		}
		upstream.Message = field
	case map[string]any:
		for _, name := range []string{"type", "message"} {
			value := field[name]
			if value == nil {
				continue
			}
			text, ok := value.(string)
			if !ok {
				return malformed()
			}
			if name == "type" {
				upstream.Type = text
			} else {
				upstream.Message = text
			}
		}
		switch code := field["code"].(type) {
		case nil:
		case string:
			if strings.TrimSpace(code) != "" {
				upstream.Code = code
			}
		case float64:
			upstream.Code = code
		case json.Number:
			upstream.Code = code
		default:
			return malformed()
		}
		if strings.TrimSpace(upstream.Type) == "" && strings.TrimSpace(upstream.Message) == "" && upstream.Code == nil {
			return nil
		}
	default:
		return malformed()
	}
	status := http.StatusBadGateway
	for _, label := range []any{upstream.Code, upstream.Type} {
		text, ok := label.(string)
		if !ok {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(text)) {
		case "server_is_overloaded", "overloaded_error":
			status = http.StatusServiceUnavailable
		case "invalid_api_key", "authentication_error", "unauthorized":
			status = http.StatusUnauthorized
		case "permission_error", "permission_denied", "access_denied":
			status = http.StatusForbidden
		case "rate_limit_error", "rate_limit_exceeded", "too_many_requests":
			status = http.StatusTooManyRequests
		case "invalid_request_error", "invalid_request":
			status = http.StatusBadRequest
		default:
			continue
		}
		break
	}
	if strings.TrimSpace(upstream.Message) == "" {
		upstream.Message = "upstream returned an error"
	}
	return types.WithOpenAIError(upstream, status)
}
