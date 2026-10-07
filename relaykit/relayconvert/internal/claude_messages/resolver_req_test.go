package claudemessages

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeDocumentResolverFailureContracts(t *testing.T) {
	block := dto.ClaudeMediaMessage{Type: "document", Source: &dto.ClaudeMessageSource{Type: "url", Url: "https://example.com/private-signature"}}
	t.Cleanup(func() { media.SetMediaResolver(media.MediaResolver{}) })
	for _, tc := range []struct {
		name     string
		resolver media.MediaResolver
		status   int
	}{
		{name: "missing"},
		{name: "network", resolver: media.MediaResolver{GetBase64Data: func(context.Context, types.FileSource, ...string) (string, string, error) {
			return "", "", errors.New("private-signature")
		}}},
		{name: "typed", status: 400, resolver: media.MediaResolver{GetBase64Data: func(context.Context, types.FileSource, ...string) (string, string, error) {
			return "", "", types.NewErrorWithStatusCode(errors.New("private-signature"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
		}}},
		{name: "mime-conflict", status: 400, resolver: media.MediaResolver{GetBase64Data: func(context.Context, types.FileSource, ...string) (string, string, error) {
			return "aGVsbG8=", "text/plain", nil
		}}},
		{name: "unsupported-mime", status: 400, resolver: media.MediaResolver{GetBase64Data: func(context.Context, types.FileSource, ...string) (string, string, error) {
			return "aGVsbG8=", "audio/wav", nil
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media.SetMediaResolver(tc.resolver)
			copy := block
			if tc.name == "mime-conflict" {
				source := *block.Source
				source.MediaType = "application/pdf"
				copy.Source = &source
			}
			_, err := claudeContentPart(context.Background(), copy, "messages[0].content[0]")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-signature")
			wrapped := types.NewError(err, types.ErrorCodeConvertRequestFailed)
			assert.Equal(t, map[bool]int{true: 400, false: 500}[tc.status == 400], wrapped.StatusCode)
			if tc.status == 400 {
				assert.True(t, types.IsSkipRetryError(wrapped))
			}
		})
	}
	media.SetMediaResolver(media.MediaResolver{})
	_, err := ClaudeMessagesRequestToOpenAIChat(dto.ClaudeRequest{Model: "m", Messages: []dto.ClaudeMessage{{Role: "user", Content: []dto.ClaudeMediaMessage{block}}}}, nil)
	require.Error(t, err)
}
