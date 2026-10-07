package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type ollamaChatStreamChunk struct {
	Error     string `json:"error"`
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	// chat
	Message *struct {
		Role      string           `json:"role"`
		Content   string           `json:"content"`
		Thinking  json.RawMessage  `json:"thinking"`
		ToolCalls []OllamaToolCall `json:"tool_calls"`
	} `json:"message"`
	// generate
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	EvalCount          int    `json:"eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalDuration       int64  `json:"eval_duration"`
}

func ollamaToolCallsToOpenAI(toolCalls []OllamaToolCall, startIndex int, includeIndex bool) ([]dto.ToolCallResponse, int) {
	if len(toolCalls) == 0 {
		return nil, startIndex
	}
	result := make([]dto.ToolCallResponse, 0, len(toolCalls))
	for _, tc := range toolCalls {
		var argBytes []byte
		var err error
		if tc.Function.Arguments == nil {
			argBytes = []byte("{}")
		} else {
			argBytes, err = common.Marshal(tc.Function.Arguments)
			if err != nil || len(argBytes) == 0 {
				argBytes = []byte("{}")
			}
		}
		toolCallID := tc.ID
		if toolCallID == "" {
			toolCallID = fmt.Sprintf("call_%d", startIndex)
		}
		tr := dto.ToolCallResponse{
			ID:   toolCallID,
			Type: "function",
			Function: dto.FunctionResponse{
				Name:      tc.Function.Name,
				Arguments: string(argBytes),
			},
		}
		if includeIndex {
			tr.SetIndex(startIndex)
		}
		startIndex++
		result = append(result, tr)
	}
	return result, startIndex
}

func buildOllamaStreamDelta(chunk *ollamaChatStreamChunk, responseID string, created int64, model string, toolCallIndex *int) (dto.ChatCompletionsStreamResponse, bool) {
	delta := dto.ChatCompletionsStreamResponse{
		Id:      responseID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index: 0,
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"},
		}},
	}

	var content string
	if chunk.Message != nil {
		content = chunk.Message.Content
	} else {
		content = chunk.Response
	}
	if content != "" {
		delta.Choices[0].Delta.SetContentString(content)
	}

	hasPayload := content != ""
	if chunk.Message != nil && len(chunk.Message.Thinking) > 0 {
		raw := strings.TrimSpace(string(chunk.Message.Thinking))
		if raw != "" && raw != "null" {
			// Unmarshal the JSON string to get the actual content without quotes
			var thinkingContent string
			if err := common.Unmarshal(chunk.Message.Thinking, &thinkingContent); err == nil {
				delta.Choices[0].Delta.SetReasoningContent(thinkingContent)
			} else {
				// Fallback to raw string if it's not a JSON string
				delta.Choices[0].Delta.SetReasoningContent(raw)
			}
			hasPayload = true
		}
	}
	if chunk.Message != nil && len(chunk.Message.ToolCalls) > 0 {
		delta.Choices[0].Delta.ToolCalls, *toolCallIndex = ollamaToolCallsToOpenAI(chunk.Message.ToolCalls, *toolCallIndex, true)
		hasPayload = true
	}

	return delta, hasPayload
}

func toUnix(ts string) int64 {
	if ts == "" {
		return time.Now().Unix()
	}
	// try time.RFC3339 or with nanoseconds
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t2, err2 := time.Parse(time.RFC3339, ts)
		if err2 == nil {
			return t2.Unix()
		}
		return time.Now().Unix()
	}
	return t.Unix()
}

func ollamaStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("empty response"), types.ErrorCodeBadResponse, http.StatusBadRequest)
	}
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	var closeOnce sync.Once
	var cancelledClose atomic.Bool
	closeBody := func() { closeOnce.Do(func() { _ = resp.Body.Close() }) }
	finished, joined := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			cancelledClose.Store(true)
			closeBody()
		case <-finished:
		}
	}()
	defer func() { close(finished); closeBody(); <-joined }()

	if info.StreamStatus == nil {
		info.StreamStatus = relaycommon.NewStreamStatus()
	}
	usage := &dto.Usage{}
	var observed strings.Builder
	generated, authoritative := false, false
	var toolCallIndex int
	// Only a real request cancellation may preserve usage across downstream failure.
	cancelled := func() (*dto.Usage, *types.NewAPIError) {
		info.MarkDownstreamCancelled()
		if info.StreamOutcome().UpstreamCompleted {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
		} else {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, ctx.Err())
		}
		if !authoritative {
			if !generated {
				info.MarkCancelledWithoutBillableOutput()
			} else {
				usage = service.ResponseText2Usage(c, observed.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
				usage.CompletionTokens += toolCallIndex * 7
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			}
		}
		return usage, nil
	}
	fail := func(err error) (*dto.Usage, *types.NewAPIError) {
		if requestErr := ctx.Err(); requestErr != nil && errors.Is(err, requestErr) {
			return cancelled()
		}
		info.MarkUpstreamFailureStatus(http.StatusBadGateway)
		common.SetContextKey(c, constant.ContextKeyResponseFailed, true)
		if info.StreamStatus == nil {
			info.StreamStatus = relaycommon.NewStreamStatus()
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, fmt.Errorf("ollama stream failed"))
		// Ollama historically settled received usage even after transport faults.
		return usage, nil
	}
	send := func(value any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var data string
		if terminal, ok := value.(string); ok {
			data = terminal
		} else {
			encoded, err := common.Marshal(value)
			if err != nil {
				return err
			}
			data = string(encoded)
		}
		helper.SetEventStreamHeaders(c)
		frame := "data: " + data + "\n\n"
		if event, ok := value.(*dto.ClaudeResponse); ok {
			frame = "event: " + event.Type + "\n" + frame
		}
		n, err := c.Writer.WriteString(frame)
		if err != nil {
			c.Set("relay_stream_write_failed", true)
			return err
		}
		if n != len(frame) {
			c.Set("relay_stream_write_failed", true)
			return io.ErrShortWrite
		}
		if err := helper.FlushWriter(c); err != nil {
			return err
		}
		if event, ok := value.(*dto.ClaudeResponse); ok && event.Type != "message_stop" && event.Type != "message_delta" && event.Type != "error" {
			helper.MarkStreamDataWritten(c)
		}
		return nil
	}
	model := info.PublicResponseModelName(info.UpstreamModelName)
	responseID, created := common.GetUUID(), time.Now().Unix()
	var claudeState *relayconvert.ResponseStreamState
	if info.RelayFormat == types.RelayFormatClaude {
		var err error
		claudeState, err = relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatClaude, relayconvert.ResponseStreamOptions{ID: responseID, Model: model, Created: created})
		if err != nil {
			return nil, types.NewErrorWithStatusCode(errors.New("Ollama response conversion failed"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	sendChat := func(chunk *dto.ChatCompletionsStreamResponse) error {
		if claudeState == nil {
			return send(chunk)
		}
		for _, part := range ollamaClaudePayloadChunks(chunk, info) {
			results, err := relayconvert.ConvertStreamResponseChunk(c, info, claudeState, part)
			if err != nil {
				return errors.New("Ollama response conversion failed")
			}
			for _, result := range results {
				event, ok := result.Value.(*dto.ClaudeResponse)
				if !ok || event == nil {
					return errors.New("invalid Ollama Claude response event")
				}
				if err := send(event); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := sendChat(helper.GenerateStartEmptyResponse(responseID, created, model, nil)); err != nil {
		return fail(err)
	}
	scanner := helper.NewStreamScanner(resp.Body)
	for {
		// A completed downstream write may itself cancel the request. Do not start
		// another read and then mistake the bound transport's closure for failure.
		if ctx.Err() != nil {
			return cancelled()
		}
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk ollamaChatStreamChunk
		if err := common.Unmarshal([]byte(line), &chunk); err != nil {
			info.MarkUpstreamFailureStatus(http.StatusInternalServerError)
			return usage, types.NewErrorWithStatusCode(fmt.Errorf("invalid ollama stream frame"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if chunk.Error != "" {
			return fail(fmt.Errorf("ollama upstream error"))
		}
		// A terminal already received from the provider wins a later client close.
		if chunk.Done {
			authoritative = true
			usage.PromptTokens, usage.CompletionTokens = chunk.PromptEvalCount, chunk.EvalCount
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			info.MarkUpstreamCompleted()
		} else if ctx.Err() != nil {
			return cancelled()
		}
		if chunk.Model != "" {
			model = info.PublicResponseModelName(chunk.Model)
		}
		created = toUnix(chunk.CreatedAt)
		delta, hasPayload := buildOllamaStreamDelta(&chunk, responseID, created, model, &toolCallIndex)
		if hasPayload {
			generated = true
			observed.WriteString(delta.Choices[0].Delta.GetContentString())
			observed.WriteString(delta.Choices[0].Delta.GetReasoningContent())
			for _, tool := range delta.Choices[0].Delta.ToolCalls {
				observed.WriteString(tool.Function.Name)
				observed.WriteString(tool.Function.Arguments)
			}
		}
		if !chunk.Done || hasPayload {
			if err := sendChat(&delta); err != nil {
				return fail(err)
			}
		}
		if !chunk.Done {
			continue
		}
		finishReason := chunk.DoneReason
		if finishReason == "" {
			finishReason = "stop"
		}
		if toolCallIndex > 0 {
			finishReason = constant.FinishReasonToolCalls
		}
		if claudeState != nil {
			claudeState.SetUsage(usage)
			claudeInfo := info.EnsureClaudeConvertInfo()
			claudeInfo.Usage, claudeInfo.FinishReason = usage, finishReason
			// Ollama's actual done frame is the only successful terminal. EOF,
			// upstream faults and client cancellation never enter this finalizer.
			results, err := relayconvert.FinalizeStreamResponse(c, info, claudeState)
			if err != nil {
				return fail(errors.New("Ollama response conversion failed"))
			}
			for _, result := range results {
				event, ok := result.Value.(*dto.ClaudeResponse)
				if !ok || event == nil {
					return fail(errors.New("invalid Ollama Claude response event"))
				}
				if err := send(event); err != nil {
					return fail(err)
				}
			}
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			return usage, nil
		}
		if err := send(helper.GenerateStopResponse(responseID, created, model, finishReason)); err != nil {
			return fail(err)
		}
		if err := send(helper.GenerateFinalUsageResponse(responseID, created, model, *usage)); err != nil {
			return fail(err)
		}
		if err := send("[DONE]"); err != nil {
			return fail(err)
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
		return usage, nil
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		if requestErr := ctx.Err(); requestErr != nil && (errors.Is(err, requestErr) || (cancelledClose.Load() && (errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed)))) {
			return cancelled()
		}
		return fail(err)
	}
	if ctx.Err() != nil {
		return cancelled()
	}
	return fail(fmt.Errorf("ollama stream ended before done"))
}

// Ollama can put thinking, text and complete tool calls in the same NDJSON
// record. Feed each content kind to the shared Claude converter in order so
// each obtains the appropriate content-block lifecycle.
func ollamaClaudePayloadChunks(chunk *dto.ChatCompletionsStreamResponse, info *relaycommon.RelayInfo) []*dto.ChatCompletionsStreamResponse {
	if len(chunk.Choices) == 0 {
		return []*dto.ChatCompletionsStreamResponse{chunk}
	}
	delta := chunk.Choices[0].Delta
	parts := make([]*dto.ChatCompletionsStreamResponse, 0, 3)
	for _, kind := range []string{"thinking", "text", "tools"} {
		part := *chunk
		part.Choices = []dto.ChatCompletionsStreamResponseChoice{{Index: chunk.Choices[0].Index}}
		switch kind {
		case "thinking":
			if delta.GetReasoningContent() == "" {
				continue
			}
			part.Choices[0].Delta.SetReasoningContent(delta.GetReasoningContent())
		case "text":
			if delta.GetContentString() == "" {
				continue
			}
			part.Choices[0].Delta.SetContentString(delta.GetContentString())
		case "tools":
			if len(delta.ToolCalls) == 0 {
				continue
			}
			part.Choices[0].Delta.ToolCalls = append([]dto.ToolCallResponse(nil), delta.ToolCalls...)
			// Tool indexes in Chat are global, whereas a newly opened Claude tool
			// run starts from its own base block. Each Ollama call is complete.
			offset := 0
			state := info.EnsureClaudeConvertInfo()
			if delta.GetReasoningContent() == "" && delta.GetContentString() == "" && state.LastMessagesType == relaycommon.LastMessageTypeTools {
				offset = state.ToolCallMaxIndexOffset + 1
			}
			for i := range part.Choices[0].Delta.ToolCalls {
				part.Choices[0].Delta.ToolCalls[i].SetIndex(offset + i)
			}
		}
		parts = append(parts, &part)
	}
	if len(parts) == 0 {
		return []*dto.ChatCompletionsStreamResponse{chunk}
	}
	return parts
}

// non-stream handler for chat/generate
func ollamaChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	raw := string(body)

	lines := strings.Split(raw, "\n")
	strictClaude := info.RelayFormat == types.RelayFormatClaude
	if strictClaude {
		var single ollamaChatStreamChunk
		if common.Unmarshal(body, &single) == nil {
			lines = []string{raw}
		}
	}
	var (
		aggContent       strings.Builder
		reasoningBuilder strings.Builder
		lastChunk        ollamaChatStreamChunk
		parsedAny        bool
		toolCallIndex    int
		toolCalls        []dto.ToolCallResponse
	)
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var ck ollamaChatStreamChunk
		if err := common.Unmarshal([]byte(ln), &ck); err != nil {
			if strictClaude {
				return nil, types.NewErrorWithStatusCode(errors.New("invalid Ollama response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			}
			if len(lines) == 1 {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			continue
		}
		if strictClaude && ck.Error != "" {
			return nil, types.NewErrorWithStatusCode(errors.New("Ollama upstream error"), types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
		parsedAny = true
		lastChunk = ck
		if ck.Message != nil && len(ck.Message.Thinking) > 0 {
			raw := strings.TrimSpace(string(ck.Message.Thinking))
			if raw != "" && raw != "null" {
				// Unmarshal the JSON string to get the actual content without quotes
				var thinkingContent string
				if err := common.Unmarshal(ck.Message.Thinking, &thinkingContent); err == nil {
					reasoningBuilder.WriteString(thinkingContent)
				} else {
					// Fallback to raw string if it's not a JSON string
					reasoningBuilder.WriteString(raw)
				}
			}
		}
		if ck.Message != nil && ck.Message.Content != "" {
			aggContent.WriteString(ck.Message.Content)
		} else if ck.Response != "" {
			aggContent.WriteString(ck.Response)
		}
		if ck.Message != nil && len(ck.Message.ToolCalls) > 0 {
			var converted []dto.ToolCallResponse
			converted, toolCallIndex = ollamaToolCallsToOpenAI(ck.Message.ToolCalls, toolCallIndex, false)
			toolCalls = append(toolCalls, converted...)
		}
	}
	if strictClaude && (!parsedAny || !lastChunk.Done) {
		return nil, types.NewErrorWithStatusCode(errors.New("Ollama response ended before done"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}

	if !parsedAny {
		var single ollamaChatStreamChunk
		if err := common.Unmarshal(body, &single); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		lastChunk = single
		if single.Message != nil {
			if len(single.Message.Thinking) > 0 {
				raw := strings.TrimSpace(string(single.Message.Thinking))
				if raw != "" && raw != "null" {
					// Unmarshal the JSON string to get the actual content without quotes
					var thinkingContent string
					if err := common.Unmarshal(single.Message.Thinking, &thinkingContent); err == nil {
						reasoningBuilder.WriteString(thinkingContent)
					} else {
						// Fallback to raw string if it's not a JSON string
						reasoningBuilder.WriteString(raw)
					}
				}
			}
			aggContent.WriteString(single.Message.Content)
			if len(single.Message.ToolCalls) > 0 {
				var converted []dto.ToolCallResponse
				converted, toolCallIndex = ollamaToolCallsToOpenAI(single.Message.ToolCalls, toolCallIndex, false)
				toolCalls = append(toolCalls, converted...)
			}
		} else {
			aggContent.WriteString(single.Response)
		}
	}

	model := lastChunk.Model
	if model == "" {
		model = info.UpstreamModelName
	}
	model = info.PublicResponseModelName(model)
	created := toUnix(lastChunk.CreatedAt)
	usage := &dto.Usage{PromptTokens: lastChunk.PromptEvalCount, CompletionTokens: lastChunk.EvalCount, TotalTokens: lastChunk.PromptEvalCount + lastChunk.EvalCount}
	content := aggContent.String()
	finishReason := lastChunk.DoneReason
	if finishReason == "" {
		finishReason = "stop"
	}
	if len(toolCalls) > 0 {
		finishReason = constant.FinishReasonToolCalls
	}

	msg := dto.Message{Role: "assistant", Content: contentPtr(content)}
	if len(toolCalls) > 0 {
		if rawToolCalls, err := common.Marshal(toolCalls); err == nil {
			msg.ToolCalls = rawToolCalls
		}
	}
	if rc := reasoningBuilder.String(); rc != "" {
		msg.ReasoningContent = &rc
	}
	full := dto.OpenAITextResponse{
		Id:      common.GetUUID(),
		Model:   model,
		Object:  "chat.completion",
		Created: created,
		Choices: []dto.OpenAITextResponseChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage: *usage,
	}
	var response any = &full
	if strictClaude {
		// The shared converter expects Message.Content's canonical string form;
		// the native Chat path retains its existing nullable representation.
		full.Choices[0].Message.Content = content
		result, err := relayconvert.ConvertResponse(c, info, types.RelayFormatClaude, &full)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(errors.New("Ollama response conversion failed"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		converted, ok := result.Value.(*dto.ClaudeResponse)
		if !ok || converted == nil {
			return nil, types.NewErrorWithStatusCode(errors.New("invalid Ollama Claude response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		response = converted
	}
	out, err := common.Marshal(response)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(errors.New("Ollama response encoding failed"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, out)
	return usage, nil
}

func contentPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
