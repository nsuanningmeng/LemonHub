package oairesponses

import (
	"errors"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// validateCrossProviderMediaSource rejects malformed or provider-local sources
// before the permissive token metadata parser can coerce them into bytes.
func validateCrossProviderMediaSource(part map[string]any) error {
	fields := []map[string]any{part}
	for _, key := range []string{"file", "image_url", "input_audio", "video_url"} {
		if nested, ok := part[key].(map[string]any); ok {
			fields = append(fields, nested)
		}
	}
	for _, field := range fields {
		for _, key := range []string{"file_id", "file_data", "file_url", "url", "data"} {
			value, exists := field[key]
			if !exists || value == nil {
				continue
			}
			text, ok := value.(string)
			if !ok {
				return types.NewErrorWithStatusCode(errors.New("input.content media source fields must be strings"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
			}
			if key == "file_id" && text != "" {
				return types.NewErrorWithStatusCode(errors.New("input.content.file_id cannot be converted across providers"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
			}
		}
	}
	return nil
}
