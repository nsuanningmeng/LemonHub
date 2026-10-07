package helper

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func FlushWriter(c *gin.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flush panic recovered: %v", r)
			if c != nil {
				c.Set("relay_stream_write_failed", true)
			}
		}
	}()

	if c == nil || c.Writer == nil {
		return nil
	}

	if requestContextDone(c) {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	c.Writer.WriteHeaderNow()
	err = http.NewResponseController(c.Writer).Flush()
	if err != nil {
		c.Set("relay_stream_write_failed", true)
	}
	return err
}

func requestContextDone(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.Context().Err() != nil
}

func SetEventStreamHeaders(c *gin.Context) {
	// 检查是否已经设置过头部
	if _, exists := c.Get("event_stream_headers_set"); exists {
		return
	}

	// 设置标志，表示头部已经设置过
	c.Set("event_stream_headers_set", true)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}

func renderStreamData(c *gin.Context, data string) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}
	if requestContextDone(c) {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}
	err := (common.CustomEvent{Data: data}).Render(c.Writer)
	if err != nil {
		c.Set("relay_stream_write_failed", true)
	}
	return err
}

// MarkStreamDataWritten records a validated business frame after its checked
// write and flush succeeded. Headers, comments and terminal sentinels do not
// establish readiness. Protocol handlers that write directly must call this.
func MarkStreamDataWritten(c *gin.Context) {
	if c != nil && !common.GetContextKeyBool(c, constant.ContextKeyResponseFailed) {
		c.Set("relay_stream_data_written", true)
	}
}
func StreamDataWritten(c *gin.Context) bool {
	return c != nil && c.GetBool("relay_stream_data_written")
}

func ClaudeData(c *gin.Context, resp dto.ClaudeResponse) error {
	jsonData, err := common.Marshal(resp)
	if err != nil {
		return fmt.Errorf("error marshalling stream response: %w", err)
	}
	if err := renderStreamData(c, fmt.Sprintf("event: %s\n", resp.Type)); err != nil {
		return err
	}
	if err := renderStreamData(c, "data: "+string(jsonData)); err != nil {
		return err
	}
	if err := FlushWriter(c); err != nil {
		return err
	}
	if resp.Type != "error" && resp.Type != "message_stop" {
		MarkStreamDataWritten(c)
	}
	return nil
}

func ClaudeChunkData(c *gin.Context, resp dto.ClaudeResponse, data string) error {
	if err := renderStreamData(c, fmt.Sprintf("event: %s\n", resp.Type)); err != nil {
		return err
	}
	if err := renderStreamData(c, fmt.Sprintf("data: %s\n", data)); err != nil {
		return err
	}
	if err := FlushWriter(c); err != nil {
		return err
	}
	if resp.Type != "error" && resp.Type != "message_stop" {
		MarkStreamDataWritten(c)
	}
	return nil
}

func ResponseChunkData(c *gin.Context, resp dto.ResponsesStreamResponse, data string) error {
	if err := renderStreamData(c, fmt.Sprintf("event: %s\n", resp.Type)); err != nil {
		return err
	}
	if err := renderStreamData(c, fmt.Sprintf("data: %s", data)); err != nil {
		return err
	}
	if err := FlushWriter(c); err != nil {
		return err
	}
	if resp.Type != "error" && resp.Type != "response.failed" && resp.Type != "response.completed" && resp.Type != "response.incomplete" && resp.Type != "response.cancelled" && resp.Type != "response.canceled" && resp.Type != "response.done" && resp.Type != "response.error" {
		MarkStreamDataWritten(c)
	}
	return nil
}

func StringData(c *gin.Context, str string) error {
	if err := renderStreamData(c, "data: "+str); err != nil {
		return err
	}
	if err := FlushWriter(c); err != nil {
		return err
	}
	if strings.TrimSpace(str) != "" && strings.TrimSpace(str) != "[DONE]" {
		MarkStreamDataWritten(c)
	}
	return nil
}

func PingData(c *gin.Context) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}

	if requestContextDone(c) {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	if n, err := c.Writer.Write([]byte(": PING\n\n")); err != nil {
		c.Set("relay_stream_write_failed", true)
		return fmt.Errorf("write ping data failed: %w", err)
	} else if n != len(": PING\n\n") {
		c.Set("relay_stream_write_failed", true)
		return io.ErrShortWrite
	}
	return FlushWriter(c)
}

func ObjectData(c *gin.Context, object interface{}) error {
	if object == nil {
		return errors.New("object is nil")
	}
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	return StringData(c, string(jsonData))
}

func Done(c *gin.Context) error {
	return StringData(c, "[DONE]")
}

func WssString(c *gin.Context, ws *websocket.Conn, str string) error {
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", str))
	return ws.WriteMessage(1, []byte(str))
}

func WssObject(c *gin.Context, ws *websocket.Conn, object interface{}) error {
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", jsonData))
	return ws.WriteMessage(1, jsonData)
}

func WssError(c *gin.Context, ws *websocket.Conn, openaiError types.OpenAIError) {
	if ws == nil {
		return
	}
	errorObj := &dto.RealtimeEvent{
		Type:    "error",
		EventId: GetLocalRealtimeID(c),
		Error:   &openaiError,
	}
	_ = WssObject(c, ws, errorObj)
}

func GetResponseID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("chatcmpl-%s", logID)
}

func GetLocalRealtimeID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("evt_%s", logID)
}

func GenerateStartEmptyResponse(id string, createAt int64, model string, systemFingerprint *string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: systemFingerprint,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Role:    "assistant",
					Content: common.GetPointer(""),
				},
			},
		},
	}
}

func GenerateStopResponse(id string, createAt int64, model string, finishReason string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				FinishReason: &finishReason,
			},
		},
	}
}

func GenerateFinalUsageResponse(id string, createAt int64, model string, usage dto.Usage) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices:           make([]dto.ChatCompletionsStreamResponseChoice, 0),
		Usage:             &usage,
	}
}
