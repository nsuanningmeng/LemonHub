package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type clientIntegrationRequest struct {
	TokenID  int    `json:"tokenId"`
	Tool     string `json:"tool"`
	Level    string `json:"level"`
	Platform string `json:"platform"`
	Model    string `json:"model"`
}

func GetClientIntegrations(c *gin.Context) {
	common.ApiSuccess(c, setting.GetClientIntegrations())
}

func RenderClientIntegration(c *gin.Context) {
	req, err := decodeClientIntegrationRequest(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	artifact, err := service.RenderClientIntegrationArtifact(service.RenderInput{
		Ctx:       c,
		TokenID:   req.TokenID,
		UserID:    c.GetInt("id"),
		Tool:      req.Tool,
		Platform:  req.Platform,
		Level:     req.Level,
		Model:     req.Model,
		Sensitive: false,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, artifact)
}

func DownloadClientIntegration(c *gin.Context) {
	req, err := decodeClientIntegrationRequest(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	artifact, err := service.RenderClientIntegrationArtifact(service.RenderInput{
		Ctx:       c,
		TokenID:   req.TokenID,
		UserID:    c.GetInt("id"),
		Tool:      req.Tool,
		Platform:  req.Platform,
		Level:     req.Level,
		Model:     req.Model,
		Sensitive: true,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	filename := artifact.Filename
	if filename == "" {
		filename = sanitizeDownloadFilename(req.Tool, req.Level, req.Platform)
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, artifact.MimeType, []byte(artifact.Content))
}

func decodeClientIntegrationRequest(c *gin.Context) (*clientIntegrationRequest, error) {
	var req clientIntegrationRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		return nil, err
	}
	req.Tool = strings.TrimSpace(req.Tool)
	req.Level = strings.TrimSpace(req.Level)
	req.Platform = strings.TrimSpace(req.Platform)
	req.Model = strings.TrimSpace(req.Model)
	if req.TokenID <= 0 {
		if raw := c.Query("tokenId"); raw != "" {
			if id, err := strconv.Atoi(raw); err == nil {
				req.TokenID = id
			}
		}
	}
	if req.TokenID <= 0 {
		return nil, errors.New("tokenId is required")
	}
	if req.Tool == "" || req.Level == "" || req.Platform == "" {
		return nil, errors.New("tool, level, and platform are required")
	}
	return &req, nil
}

func sanitizeDownloadFilename(tool, level, platform string) string {
	base := strings.ToLower(strings.TrimSpace(tool))
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.ReplaceAll(base, "_", "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "client-integration"
	}
	parts := []string{base}
	if level != "" {
		parts = append(parts, strings.ToLower(strings.TrimSpace(level)))
	}
	if platform != "" && platform != "any" {
		parts = append(parts, strings.ToLower(strings.TrimSpace(platform)))
	}
	return strings.Join(parts, "-") + ".txt"
}
