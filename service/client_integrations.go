package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RenderInput struct {
	Ctx       *gin.Context
	TokenID   int
	UserID    int
	Tool      string
	Platform  string
	Level     string
	Model     string
	Sensitive bool
}

type Artifact struct {
	Filename  string
	MimeType  string
	Content   string
	Sensitive bool
}

func RenderClientIntegrationArtifact(in RenderInput) (*Artifact, error) {
	token, err := model.GetTokenByIds(in.TokenID, in.UserID)
	if err != nil {
		return nil, err
	}
	integration, spec, ok := setting.ResolveClientIntegration(in.Tool, in.Level, in.Platform)
	if !ok {
		return nil, fmt.Errorf("unsupported client integration: %s", in.Tool)
	}

	// Non-sensitive (catalog/render) keeps the {key} placeholder intact so the
	// client substitutes the real key locally. Only /download (Sensitive) injects it.
	keyValue := "{key}"
	if in.Sensitive {
		keyValue = "sk-" + token.GetFullKey()
	}

	content := renderClientIntegrationTemplate(spec.Template, map[string]string{
		"address": strings.TrimRight(GetRequestBaseURL(in.Ctx), "/"),
		"key":     keyValue,
		"model":   strings.TrimSpace(in.Model),
	})

	filename := spec.Filename
	if filename == "" {
		filename = buildIntegrationFilename(integration.Id, spec)
	}
	return &Artifact{
		Filename:  filename,
		MimeType:  mimeTypeOrDefault(spec.ContentType),
		Content:   content,
		Sensitive: in.Sensitive,
	}, nil
}

func renderClientIntegrationTemplate(template string, values map[string]string) string {
	replacer := strings.NewReplacer(
		"{address}", values["address"],
		"{key}", values["key"],
		"{model}", values["model"],
	)
	return replacer.Replace(template)
}

func buildIntegrationFilename(tool string, artifact setting.ClientArtifact) string {
	slug := sanitizeIntegrationSlug(tool)
	ext := filenameExtension(artifact)
	if ext == "" {
		return slug
	}
	return slug + ext
}

func filenameExtension(artifact setting.ClientArtifact) string {
	switch artifact.ContentType {
	case "application/json":
		return ".json"
	case "application/toml":
		return ".toml"
	case "application/yaml":
		return ".yaml"
	case "text/x-shellscript":
		if artifact.Platform == "windows" {
			return ".ps1"
		}
		return ".sh"
	default:
		return ""
	}
}

func sanitizeIntegrationSlug(tool string) string {
	tool = strings.ToLower(strings.TrimSpace(tool))
	if tool == "" {
		return "client-integration"
	}
	tool = strings.ReplaceAll(tool, " ", "-")
	var b strings.Builder
	b.Grow(len(tool))
	lastDash := false
	for _, r := range tool {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "client-integration"
	}
	return slug
}

func mimeTypeOrDefault(contentType string) string {
	if contentType == "" {
		return "text/plain; charset=utf-8"
	}
	if strings.Contains(contentType, "charset=") {
		return contentType
	}
	switch contentType {
	case "text/plain", "text/markdown", "text/x-shellscript":
		return contentType + "; charset=utf-8"
	default:
		return contentType
	}
}
