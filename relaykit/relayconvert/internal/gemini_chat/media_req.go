package geminichat

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// Gemini media must reach Chat as its actual modality. Unrepresentable parts
// fail conversion instead of being discarded or mislabeled as images.
func geminiMediaToChatContent(inline *dto.GeminiInlineData, file *dto.GeminiFileData) (dto.MediaContent, error) {
	invalid := func() (dto.MediaContent, error) {
		return dto.MediaContent{}, types.NewErrorWithStatusCode(errors.New("Gemini media cannot be represented by Chat Completions"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
	}
	if inline != nil && file != nil {
		return invalid()
	}
	mimeType := ""
	if inline != nil {
		mimeType = inline.MimeType
	} else if file != nil {
		mimeType = file.MimeType
	} else {
		return invalid()
	}
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if mimeType == "image/jpg" {
		mimeType = "image/jpeg"
	}
	isImage := mimeType == "image/png" || mimeType == "image/jpeg" || mimeType == "image/webp" || mimeType == "image/gif"
	if file != nil {
		parsed, err := url.Parse(file.FileUri)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || (!isImage && mimeType != "") {
			return invalid()
		}
		return dto.MediaContent{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: file.FileUri, Detail: "auto", MimeType: mimeType}}, nil
	}
	data, err := base64.StdEncoding.DecodeString(inline.Data)
	if err != nil || len(data) == 0 {
		return invalid()
	}
	if isImage {
		return dto.MediaContent{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: "data:" + mimeType + ";base64," + inline.Data, Detail: "auto", MimeType: mimeType}}, nil
	}
	switch {
	case mimeType == "application/pdf":
		return dto.MediaContent{Type: dto.ContentTypeFile, File: &dto.MessageFile{FileName: "document.pdf", FileData: "data:application/pdf;base64," + inline.Data}}, nil
	case strings.HasPrefix(mimeType, "text/") || mimeType == "application/json":
		if !utf8.Valid(data) {
			return invalid()
		}
		return dto.MediaContent{Type: dto.ContentTypeText, Text: string(data)}, nil
	case mimeType == "audio/wav" || mimeType == "audio/x-wav" || mimeType == "audio/wave":
		return dto.MediaContent{Type: dto.ContentTypeInputAudio, InputAudio: &dto.MessageInputAudio{Data: inline.Data, Format: "wav"}}, nil
	case mimeType == "audio/mp3" || mimeType == "audio/mpeg":
		return dto.MediaContent{Type: dto.ContentTypeInputAudio, InputAudio: &dto.MessageInputAudio{Data: inline.Data, Format: "mp3"}}, nil
	default:
		return invalid()
	}
}
