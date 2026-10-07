package claudemessages

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func invalidClaudeMedia(path, reason string) error {
	return types.NewErrorWithStatusCode(fmt.Errorf("%s: %s", path, reason), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
}

// claudeContentPart maps ordinary and tool-result media through the same contract.
func claudeContentPart(ctx context.Context, block dto.ClaudeMediaMessage, path string) (dto.MediaContent, error) {
	part := dto.MediaContent{CacheControl: append([]byte(nil), block.CacheControl...)}
	switch block.Type {
	case "text", "input_text":
		if block.Text == nil {
			return part, invalidClaudeMedia(path, "text must be a string")
		}
		part.Type = "text"
		part.Text = *block.Text
		return part, nil
	case "image", "document":
	default:
		return part, invalidClaudeMedia(path, "unsupported content block")
	}
	source := block.Source
	if source == nil {
		return part, invalidClaudeMedia(path+".source", "source is required")
	}
	data, mime := "", source.MediaType
	switch source.Type {
	case "url":
		parsed, err := url.Parse(source.Url)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
			return part, invalidClaudeMedia(path+".source.url", "expected HTTP media URL")
		}
		if source.Data != nil {
			return part, invalidClaudeMedia(path+".source", "conflicting media sources")
		}
		if block.Type == "image" {
			part.Type = "image_url"
			part.ImageUrl = &dto.MessageImageUrl{Url: source.Url}
			return part, nil
		}
		declaredMIME := mime
		var errResolve error
		data, mime, errResolve = media.ResolveBase64Data(ctx, types.NewURLFileSource(source.Url), "claude document conversion")
		if errResolve != nil {
			var typed *types.NewAPIError
			if errors.As(errResolve, &typed) {
				return part, types.NewErrorWithStatusCode(errors.New("messages.content.source: document media resolution failed"), typed.GetErrorCode(), typed.StatusCode, types.ErrOptionWithSkipRetry())
			}
			return part, errors.New("messages.content.source: document media resolution failed")
		}
		if declaredMIME != "" && declaredMIME != mime {
			return part, invalidClaudeMedia(path+".source.media_type", "document MIME conflict")
		}
	case "text":
		text, ok := source.Data.(string)
		if block.Type != "document" || !ok || text == "" || source.Url != "" || (mime != "" && mime != "text/plain") {
			return part, invalidClaudeMedia(path+".source", "expected text document source")
		}
		part.Type = "text"
		part.Text = text
		return part, nil
	case "base64":
		var ok bool
		data, ok = source.Data.(string)
		if !ok || data == "" || source.Url != "" {
			return part, invalidClaudeMedia(path+".source.data", "expected nonempty base64 string")
		}
	default:
		return part, invalidClaudeMedia(path+".source.type", "unsupported media source")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(decoded) == 0 {
		return part, invalidClaudeMedia(path+".source.data", "invalid base64 media")
	}
	if block.Type == "image" {
		detected := http.DetectContentType(decoded)
		if detected == "application/pdf" || (strings.HasPrefix(detected, "image/") && detected != mime) {
			return part, invalidClaudeMedia(path+".source.media_type", "image MIME conflict")
		}
		switch mime {
		case "image/jpeg", "image/png", "image/gif", "image/webp":
		default:
			return part, invalidClaudeMedia(path+".source.media_type", "unsupported image MIME type")
		}
		part.Type = "image_url"
		part.ImageUrl = &dto.MessageImageUrl{Url: "data:" + mime + ";base64," + data}
		return part, nil
	}
	switch mime {
	case "application/pdf":
		if !strings.HasPrefix(string(decoded), "%PDF-") {
			return part, invalidClaudeMedia(path+".source.data", "document MIME does not match content")
		}
		part.Type = "file"
		part.File = &dto.MessageFile{FileName: "document.pdf", FileData: "data:application/pdf;base64," + data}
	case "text/plain":
		if !utf8.Valid(decoded) {
			return part, invalidClaudeMedia(path+".source.data", "text document must be UTF-8")
		}
		part.Type = "text"
		part.Text = string(decoded)
	default:
		return part, invalidClaudeMedia(path+".source.media_type", "unsupported document MIME type")
	}
	return part, nil
}

// Tool strings remain strings. Unknown JSON remains text, while recognized blocks
// are validated before media is moved to a following user message.
func claudeToolResultContent(ctx context.Context, value any, path string) (string, []dto.MediaContent, error) {
	if text, ok := value.(string); ok {
		return text, nil, nil
	}
	raw, err := kitutil.Marshal(value)
	if err != nil {
		return "", nil, invalidClaudeMedia(path, "invalid tool result content")
	}
	var entries []any
	if err := kitutil.Unmarshal(raw, &entries); err != nil || string(raw) == "null" {
		return string(raw), nil, nil
	}
	texts := make([]string, 0, len(entries))
	var parts []dto.MediaContent
	for i, entry := range entries {
		object, ok := entry.(map[string]any)
		kind, _ := object["type"].(string)
		if !ok || kind == "" {
			bytes, _ := kitutil.Marshal(entry)
			texts = append(texts, string(bytes))
			continue
		}
		switch kind {
		case "text", "input_text", "image", "document", "audio", "input_audio", "video", "container_upload":
			block, err := kitutil.Any2Type[dto.ClaudeMediaMessage](entry)
			if err != nil {
				return "", nil, invalidClaudeMedia(path, "malformed tool result block")
			}
			part, err := claudeContentPart(ctx, block, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return "", nil, err
			}
			if part.Type == "text" {
				texts = append(texts, part.Text)
			} else {
				parts = append(parts, part)
			}
		default:
			bytes, _ := kitutil.Marshal(entry)
			texts = append(texts, string(bytes))
		}
	}
	// Entirely unknown arrays retain their original JSON representation.
	if len(parts) == 0 {
		recognized := false
		for _, entry := range entries {
			if object, ok := entry.(map[string]any); ok {
				kind, _ := object["type"].(string)
				if kind == "text" || kind == "input_text" || kind == "document" {
					recognized = true
				}
			}
		}
		if !recognized {
			return string(raw), nil, nil
		}
	}
	return strings.Join(texts, "\n"), parts, nil
}
