package claude

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// ClaudeResponsesStreamHandler keeps the request's Responses format while
// translating Claude events through the registered Claude→Chat→Responses chain.
func ClaudeResponsesStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("invalid Claude stream response"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatClaude, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{ID: helper.GetResponseID(c), Model: info.PublicResponseModelName(info.UpstreamModelName)})
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponse)
	}
	claudeInfo := &ClaudeResponseInfo{ResponseId: helper.GetResponseID(c), Created: common.GetTimestamp(), Model: info.UpstreamModelName, ResponseText: strings.Builder{}, Usage: &dto.Usage{}}
	var streamErr *types.NewAPIError
	sawStart, sawStop := false, false
	sendEvents := func(results []relayconvert.ResponseResult) bool {
		for _, result := range results {
			event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
			if !ok {
				streamErr = types.NewOpenAIError(errors.New("invalid Responses stream event"), types.ErrorCodeBadResponse, http.StatusBadGateway)
				return false
			}
			data, marshalErr := common.Marshal(event.Payload)
			if marshalErr != nil {
				streamErr = types.NewError(marshalErr, types.ErrorCodeJsonMarshalFailed)
				return false
			}
			var writeErr error
			if c.Request.Context().Err() != nil {
				writeErr = c.Request.Context().Err()
			} else {
				// Write one complete event and stop conversion on write or flush failure.
				helper.SetEventStreamHeaders(c)
				frame := "event: " + event.Type + "\ndata: " + string(data) + "\n\n"
				n, err := c.Writer.WriteString(frame)
				writeErr = err
				if writeErr == nil && n != len(frame) {
					writeErr = io.ErrShortWrite
				}
				if writeErr == nil {
					writeErr = helper.FlushWriter(c)
				}
			}
			if writeErr != nil || len(c.Errors) != 0 {
				if c.Request.Context().Err() == nil {
					c.Set("relay_stream_write_failed", true)
				}
				streamErr = types.NewOpenAIError(errors.New("Responses stream write failed"), types.ErrorCodeBadResponse, http.StatusBadGateway)
				return false
			}
			switch event.Type {
			case "response.completed", "response.failed", "response.incomplete", "response.cancelled", "error", "[DONE]":
			default:
				helper.MarkStreamDataWritten(c)
			}
		}
		return true
	}
	abort := func() {
		if sawStop || !helper.StreamDataWritten(c) || c.Request.Context().Err() != nil || c.GetBool("relay_stream_write_failed") || c.GetBool("relay_stream_error_emitted") {
			return
		}
		results, abortErr := relayconvert.AbortStreamResponse(state)
		if abortErr == nil && len(results) > 0 && sendEvents(results) {
			c.Set("relay_stream_error_emitted", true)
		}
	}
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		var event dto.ClaudeResponse
		if err := common.UnmarshalJsonStr(data, &event); err != nil {
			streamErr = types.NewOpenAIError(errors.New("invalid Claude stream event"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		if upstreamError := event.GetClaudeError(); upstreamError != nil && upstreamError.Type != "" {
			streamErr = types.WithClaudeError(*upstreamError, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		if event.Type == "message_start" && event.Message == nil {
			streamErr = types.NewOpenAIError(errors.New("invalid Claude message_start event"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		if event.Type == "content_block_delta" {
			invalid := event.Delta == nil
			if event.Delta != nil {
				switch event.Delta.Type {
				case "input_json_delta":
					invalid = event.Delta.PartialJson == nil
				case "text_delta":
					invalid = event.Delta.Text == nil
				case "thinking_delta":
					invalid = event.Delta.Thinking == nil
				}
			}
			if invalid {
				streamErr = types.NewOpenAIError(errors.New("invalid Claude content block delta"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
		}
		claudeInfo.ObserveRefusalStreamEvent(&event, data)
		if event.Delta != nil && event.Delta.StopReason != nil {
			maybeMarkClaudeRefusal(c, *event.Delta.StopReason)
		}
		FormatClaudeResponseInfo(&event, nil, claudeInfo)
		countClaudeStreamBillableTools(c, info, &event)
		switch event.Type {
		case "message_start":
			sawStart = true
			if event.Message != nil {
				event.Message.Model = info.PublicResponseModelName(event.Message.Model)
			}
		case "message_delta":
			// Claude emits output-only usage deltas. Supply its accumulated native usage
			// before mapping so the Responses terminal usage includes input/cache too.
			event.Usage = buildMessageDeltaPatchUsage(&event, claudeInfo)
		case "message_stop":
			sawStop = true
			sr.Done()
			return
		}
		event.Model = info.PublicResponseModelName(info.UpstreamModelName)
		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &event)
		if err != nil {
			streamErr = types.NewError(err, types.ErrorCodeBadResponseBody)
			sr.Stop(streamErr)
			return
		}
		if !sendEvents(results) {
			sr.Stop(streamErr)
		}
	})
	if streamErr != nil {
		original := streamErr
		abort()
		return nil, original
	}
	// Scanner may observe a trailing reset before the consumer processes an
	// already queued message_stop. After join, validated source completion
	// survives that trailing transport error; timeout/ping/panic still fail.
	abnormalEnd := info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd()
	if abnormalEnd && sawStart && sawStop && info.StreamStatus.EndReason == relaycommon.StreamEndReasonScannerErr {
		abnormalEnd = false
	}
	if c.Request.Context().Err() != nil || !sawStart || !sawStop || abnormalEnd {
		abort()
		return nil, types.NewOpenAIError(errors.New("Claude stream ended before message_stop"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	HandleStreamFinalResponse(c, info, claudeInfo)
	state.SetUsage(relayconvert.UsageFromClaudeUsage(claudeInfo.Usage))
	results, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponse)
	}
	if !sendEvents(results) {
		return nil, streamErr
	}
	return claudeInfo.Usage, nil
}
