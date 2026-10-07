package oaichat

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

type ChatToResponsesStreamEvent struct {
	Type    string
	Payload dto.ResponsesStreamResponse
}

type ChatToResponsesStreamState struct {
	ID      string
	Model   string
	Created int64
	Usage   *dto.Usage

	status            string
	incompleteDetails *dto.IncompleteDetails
	sentCreated       bool
	textOutputIndex   int
	textStarted       bool
	textDone          bool
	reasoningIndex    int
	reasoningStarted  bool
	reasoningDone     bool
	finalized         bool
	finished          bool
	finishReason      string
	invalid           bool
	choiceIndex       *int
	nextOutputIndex   int
	toolsByIndex      map[int]*chatToResponsesStreamTool
	outputOrder       []chatToResponsesOutputRef
	text              strings.Builder
	reasoning         strings.Builder
}

type chatToResponsesStreamTool struct {
	ChatIndex   int
	OutputIndex int
	ID          string
	Name        string
	Arguments   strings.Builder
	Done        bool
	Published   bool
}

type chatToResponsesOutputRef struct {
	Kind      string
	ToolIndex int
}

func NewChatToResponsesStreamState(id string, model string) *ChatToResponsesStreamState {
	return &ChatToResponsesStreamState{
		ID:              id,
		Model:           model,
		Created:         time.Now().Unix(),
		Usage:           &dto.Usage{},
		status:          "completed",
		textOutputIndex: -1,
		reasoningIndex:  -1,
		toolsByIndex:    make(map[int]*chatToResponsesStreamTool),
	}
}

func ChatCompletionsStreamChunkToResponsesEvents(chunk *dto.ChatCompletionsStreamResponse, state *ChatToResponsesStreamState) ([]ChatToResponsesStreamEvent, error) {
	if chunk == nil || state == nil || state.finalized {
		return nil, nil
	}
	if state.invalid {
		return nil, fmt.Errorf("invalid Chat stream conversion state")
	}
	finished, finishReason := state.finished, state.finishReason
	for _, choice := range chunk.Choices {
		if state.choiceIndex != nil && *state.choiceIndex != choice.Index {
			state.invalid = true
			return nil, fmt.Errorf("multiple Chat choices cannot be represented as one Responses stream")
		}
		if state.choiceIndex == nil {
			index := choice.Index
			state.choiceIndex = &index
		}
		if finished && (choice.Delta.GetReasoningContent() != "" || choice.Delta.GetContentString() != "" || len(choice.Delta.ToolCalls) != 0) {
			state.invalid = true
			return nil, fmt.Errorf("Chat generation continued after finish")
		}
		if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
			reason := strings.TrimSpace(*choice.FinishReason)
			if finished && reason != finishReason {
				state.invalid = true
				return nil, fmt.Errorf("Chat finish reason changed after finish")
			}
			finished, finishReason = true, reason
		}
	}
	if state.ID == "" {
		state.ID = chunk.Id
	}
	if state.Model == "" {
		state.Model = chunk.Model
	}
	if state.Created == 0 {
		state.Created = chunk.Created
	}
	if chunk.Usage != nil {
		state.Usage = UsageFromChatUsage(chunk.Usage)
	}

	events := make([]ChatToResponsesStreamEvent, 0)
	if !state.sentCreated {
		state.sentCreated = true
		events = append(events, responsesStreamEvent(responsesEventCreated, dto.ResponsesStreamResponse{
			Type:     responsesEventCreated,
			Response: state.createdResponse(),
		}))
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.GetReasoningContent() != "" {
			events = append(events, state.appendReasoningDelta(choice.Delta.GetReasoningContent())...)
		}
		if choice.Delta.GetContentString() != "" {
			events = append(events, state.appendTextDelta(choice.Delta.GetContentString())...)
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			toolEvents, err := state.appendToolCallDelta(toolCall)
			if err != nil {
				state.invalid = true
				return nil, err
			}
			events = append(events, toolEvents...)
		}
		if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
			state.finished = true
			state.finishReason = strings.TrimSpace(*choice.FinishReason)
			state.applyFinishReason(*choice.FinishReason)
			events = append(events, state.doneDeltaEvents()...)
		}
	}
	return events, nil
}

func FinalizeChatCompletionsStreamToResponses(state *ChatToResponsesStreamState) []ChatToResponsesStreamEvent {
	if state == nil || state.finalized {
		return nil
	}
	if state.invalid {
		return AbortChatCompletionsStreamToResponses(state)
	}
	events := state.doneDeltaEvents()
	state.finalized = true
	resp := state.finalResponse()
	eventType := responsesEventCompleted
	if state.status == "incomplete" {
		eventType = responsesEventIncomplete
	}
	events = append(events, responsesStreamEvent(eventType, dto.ResponsesStreamResponse{
		Type:     eventType,
		Response: resp,
	}))
	return events
}

// BuildResponsesStreamFailure describes a relay interruption without exposing source errors.
func BuildResponsesStreamFailure(id, model string, created int64) ChatToResponsesStreamEvent {
	return responsesStreamEvent("response.failed", dto.ResponsesStreamResponse{Type: "response.failed", Response: &dto.OpenAIResponsesResponse{ID: id, Object: "response", CreatedAt: dto.IntValue(created), Model: model, Status: []byte(`"failed"`), Output: []dto.ResponsesOutput{}, Error: map[string]string{"code": "server_error", "message": "upstream_stream_interrupted"}}})
}

// AbortChatCompletionsStreamToResponses closes an unfinished conversion without
// manufacturing successful content/tool done events. Finalize after abort is inert.
func AbortChatCompletionsStreamToResponses(state *ChatToResponsesStreamState) []ChatToResponsesStreamEvent {
	if state == nil || state.finalized {
		return nil
	}
	state.finalized = true
	return []ChatToResponsesStreamEvent{BuildResponsesStreamFailure(state.ID, state.Model, state.Created)}
}

func (s *ChatToResponsesStreamState) UsageText() string {
	if s == nil {
		return ""
	}
	return s.text.String()
}

func (s *ChatToResponsesStreamState) appendTextDelta(delta string) []ChatToResponsesStreamEvent {
	events := make([]ChatToResponsesStreamEvent, 0, 2)
	if !s.textStarted {
		s.textStarted = true
		s.textOutputIndex = s.nextIndex("message", -1)
		events = append(events, responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemAdded,
			OutputIndex: intPtr(s.textOutputIndex),
			Item: &dto.ResponsesOutput{
				Type:    responsesOutputTypeMessage,
				ID:      s.messageID(),
				Status:  "in_progress",
				Role:    "assistant",
				Content: []dto.ResponsesOutputContent{},
			},
		}))
		events = append(events, responsesStreamEvent("response.content_part.added", dto.ResponsesStreamResponse{Type: "response.content_part.added", OutputIndex: intPtr(s.textOutputIndex), ContentIndex: intPtr(0), ItemID: s.messageID(), Part: &dto.ResponsesReasoningSummaryPart{Type: "output_text", Text: "", Annotations: kitutil.GetPointer([]interface{}{})}}))
	}
	s.text.WriteString(delta)
	events = append(events, responsesStreamEvent(responsesEventOutputTextDelta, dto.ResponsesStreamResponse{
		Type:         responsesEventOutputTextDelta,
		OutputIndex:  intPtr(s.textOutputIndex),
		ContentIndex: intPtr(0),
		Delta:        delta,
		ItemID:       s.messageID(),
	}))
	return events
}

func (s *ChatToResponsesStreamState) appendReasoningDelta(delta string) []ChatToResponsesStreamEvent {
	events := make([]ChatToResponsesStreamEvent, 0, 2)
	if !s.reasoningStarted {
		s.reasoningStarted = true
		s.reasoningIndex = s.nextIndex("reasoning", -1)
		events = append(events, responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemAdded,
			OutputIndex: intPtr(s.reasoningIndex),
			Item: &dto.ResponsesOutput{
				Type:    responsesOutputTypeReasoning,
				ID:      s.reasoningID(),
				Status:  "in_progress",
				Content: []dto.ResponsesOutputContent{},
			},
		}))
		events = append(events, responsesStreamEvent("response.reasoning_summary_part.added", dto.ResponsesStreamResponse{Type: "response.reasoning_summary_part.added", OutputIndex: intPtr(s.reasoningIndex), SummaryIndex: intPtr(0), ItemID: s.reasoningID(), Part: &dto.ResponsesReasoningSummaryPart{Type: "summary_text", Text: ""}}))
	}
	s.reasoning.WriteString(delta)
	events = append(events, responsesStreamEvent(responsesEventReasoningSummaryDelta, dto.ResponsesStreamResponse{
		Type:         responsesEventReasoningSummaryDelta,
		OutputIndex:  intPtr(s.reasoningIndex),
		SummaryIndex: intPtr(0),
		Delta:        delta,
		ItemID:       s.reasoningID(),
	}))
	return events
}

func (s *ChatToResponsesStreamState) appendToolCallDelta(toolCall dto.ToolCallResponse) ([]ChatToResponsesStreamEvent, error) {
	chatIndex := 0
	if toolCall.Index != nil {
		chatIndex = *toolCall.Index
	}
	tool := s.toolsByIndex[chatIndex]
	if tool == nil {
		tool = &chatToResponsesStreamTool{ChatIndex: chatIndex, OutputIndex: -1}
		s.toolsByIndex[chatIndex] = tool
	}
	id, name := strings.TrimSpace(toolCall.ID), strings.TrimSpace(toolCall.Function.Name)
	if tool.Published && ((id != "" && id != tool.ID) || (name != "" && name != tool.Name)) {
		return nil, fmt.Errorf("published tool identity cannot change")
	}
	if !tool.Published {
		if id != "" {
			tool.ID = id
		}
		if name != "" {
			tool.Name = name
		}
	}
	tool.Arguments.WriteString(toolCall.Function.Arguments)
	if tool.Name == "" || (!tool.Published && tool.ID == "") {
		return nil, nil
	}
	if !tool.Published {
		return s.publishTool(tool), nil
	}
	if toolCall.Function.Arguments == "" {
		return nil, nil
	}
	return []ChatToResponsesStreamEvent{responsesStreamEvent(responsesEventFunctionArgsDelta, dto.ResponsesStreamResponse{Type: responsesEventFunctionArgsDelta, OutputIndex: intPtr(tool.OutputIndex), ItemID: tool.ID, Delta: toolCall.Function.Arguments})}, nil
}

// Tool identity is published only when complete, or at finish using the legacy
// generated call id when the upstream never supplied one.
func (s *ChatToResponsesStreamState) publishTool(tool *chatToResponsesStreamTool) []ChatToResponsesStreamEvent {
	tool.Published = true
	if tool.ID == "" {
		tool.ID = fmt.Sprintf("%s_call_%d", s.ID, tool.ChatIndex)
	}
	tool.OutputIndex = s.nextIndex("tool", tool.ChatIndex)
	events := []ChatToResponsesStreamEvent{responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{Type: responsesEventOutputItemAdded, OutputIndex: intPtr(tool.OutputIndex), ItemID: tool.ID, Item: &dto.ResponsesOutput{Type: responsesOutputTypeFunctionCall, ID: tool.ID, Status: "in_progress", CallId: tool.ID, Name: tool.Name, Arguments: []byte(`""`)}})}
	if args := tool.Arguments.String(); args != "" {
		events = append(events, responsesStreamEvent(responsesEventFunctionArgsDelta, dto.ResponsesStreamResponse{Type: responsesEventFunctionArgsDelta, OutputIndex: intPtr(tool.OutputIndex), ItemID: tool.ID, Delta: args}))
	}
	return events
}

func (s *ChatToResponsesStreamState) doneDeltaEvents() []ChatToResponsesStreamEvent {
	events := make([]ChatToResponsesStreamEvent, 0)
	status := s.outputStatus()
	if s.textStarted && !s.textDone {
		s.textDone = true
		events = append(events, responsesStreamEvent("response.output_text.done", dto.ResponsesStreamResponse{
			Type:         "response.output_text.done",
			Text:         kitutil.GetPointer(s.text.String()),
			OutputIndex:  intPtr(s.textOutputIndex),
			ContentIndex: intPtr(0),
			ItemID:       s.messageID(),
		}))
		events = append(events, responsesStreamEvent("response.content_part.done", dto.ResponsesStreamResponse{Type: "response.content_part.done", OutputIndex: intPtr(s.textOutputIndex), ContentIndex: intPtr(0), ItemID: s.messageID(), Part: &dto.ResponsesReasoningSummaryPart{Type: "output_text", Text: s.text.String(), Annotations: kitutil.GetPointer([]interface{}{})}}))
		events = append(events, responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemDone,
			OutputIndex: intPtr(s.textOutputIndex),
			Item:        s.messageOutput(status),
		}))
	}
	if s.reasoningStarted && !s.reasoningDone {
		s.reasoningDone = true
		events = append(events, responsesStreamEvent(responsesEventReasoningSummaryDone, dto.ResponsesStreamResponse{
			Type:         responsesEventReasoningSummaryDone,
			OutputIndex:  intPtr(s.reasoningIndex),
			SummaryIndex: intPtr(0),
			ItemID:       s.reasoningID(),
			Text:         kitutil.GetPointer(s.reasoning.String()),
		}))
		events = append(events, responsesStreamEvent("response.reasoning_summary_part.done", dto.ResponsesStreamResponse{Type: "response.reasoning_summary_part.done", OutputIndex: intPtr(s.reasoningIndex), SummaryIndex: intPtr(0), ItemID: s.reasoningID(), Part: &dto.ResponsesReasoningSummaryPart{Type: "summary_text", Text: s.reasoning.String()}}))
		events = append(events, responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemDone,
			OutputIndex: intPtr(s.reasoningIndex),
			Item:        s.reasoningOutput(status),
		}))
	}
	for _, tool := range s.sortedTools() {
		if !tool.Published && tool.Name != "" {
			events = append(events, s.publishTool(tool)...)
		}
		if !tool.Published || tool.Done {
			continue
		}
		tool.Done = true
		events = append(events, responsesStreamEvent(responsesEventFunctionArgsDone, dto.ResponsesStreamResponse{
			Type:      responsesEventFunctionArgsDone,
			Arguments: kitutil.GetPointer(tool.Arguments.String()), Name: tool.Name,
			OutputIndex: intPtr(tool.OutputIndex),
			ItemID:      tool.ID,
		}))
		events = append(events, responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemDone,
			OutputIndex: intPtr(tool.OutputIndex),
			Item:        s.toolOutput(tool, status),
		}))
	}
	return events
}

func (s *ChatToResponsesStreamState) applyFinishReason(finishReason string) {
	if status, details := ResponsesStatusFromChatFinishReason(finishReason); status != "" {
		s.status = status
		s.incompleteDetails = details
	}
}

func (s *ChatToResponsesStreamState) finalResponse() *dto.OpenAIResponsesResponse {
	output := make([]dto.ResponsesOutput, 0, len(s.outputOrder))
	status := s.outputStatus()
	for _, ref := range s.outputOrder {
		switch ref.Kind {
		case "message":
			output = append(output, *s.messageOutput(status))
		case "reasoning":
			output = append(output, *s.reasoningOutput(status))
		case "tool":
			if tool := s.toolsByIndex[ref.ToolIndex]; tool != nil {
				output = append(output, *s.toolOutput(tool, status))
			}
		}
	}
	return &dto.OpenAIResponsesResponse{
		ID:                s.ID,
		Object:            "response",
		CreatedAt:         dto.IntValue(s.Created),
		Status:            []byte(fmt.Sprintf("%q", s.status)),
		IncompleteDetails: s.incompleteDetails,
		Model:             s.Model,
		Output:            output,
		Usage:             s.Usage,
	}
}

func (s *ChatToResponsesStreamState) createdResponse() *dto.OpenAIResponsesResponse {
	return &dto.OpenAIResponsesResponse{
		ID:        s.ID,
		Object:    "response",
		CreatedAt: dto.IntValue(s.Created),
		Status:    []byte(`"in_progress"`),
		Model:     s.Model,
		Output:    []dto.ResponsesOutput{},
	}
}

func (s *ChatToResponsesStreamState) nextIndex(kind string, toolIndex int) int {
	index := s.nextOutputIndex
	s.nextOutputIndex++
	s.outputOrder = append(s.outputOrder, chatToResponsesOutputRef{Kind: kind, ToolIndex: toolIndex})
	return index
}

func (s *ChatToResponsesStreamState) sortedTools() []*chatToResponsesStreamTool {
	indexes := make([]int, 0, len(s.toolsByIndex))
	for index := range s.toolsByIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	tools := make([]*chatToResponsesStreamTool, 0, len(indexes))
	for _, index := range indexes {
		tools = append(tools, s.toolsByIndex[index])
	}
	return tools
}

func (s *ChatToResponsesStreamState) outputStatus() string {
	if s.status == "incomplete" {
		return "incomplete"
	}
	return "completed"
}

func (s *ChatToResponsesStreamState) messageID() string {
	return fmt.Sprintf("%s_msg_0", s.ID)
}

func (s *ChatToResponsesStreamState) reasoningID() string {
	return fmt.Sprintf("%s_reasoning_0", s.ID)
}

func (s *ChatToResponsesStreamState) messageOutput(status string) *dto.ResponsesOutput {
	return &dto.ResponsesOutput{
		Type:   responsesOutputTypeMessage,
		ID:     s.messageID(),
		Status: status,
		Role:   "assistant",
		Content: []dto.ResponsesOutputContent{
			{
				Type:        "output_text",
				Text:        s.text.String(),
				Annotations: []interface{}{},
			},
		},
	}
}

func (s *ChatToResponsesStreamState) reasoningOutput(status string) *dto.ResponsesOutput {
	return &dto.ResponsesOutput{
		Type:   responsesOutputTypeReasoning,
		ID:     s.reasoningID(),
		Status: status,
		Summary: []dto.ResponsesReasoningSummaryPart{
			{
				Type: "summary_text",
				Text: s.reasoning.String(),
			},
		},
	}
}

func (s *ChatToResponsesStreamState) toolOutput(tool *chatToResponsesStreamTool, status string) *dto.ResponsesOutput {
	return &dto.ResponsesOutput{
		Type:      responsesOutputTypeFunctionCall,
		ID:        tool.ID,
		Status:    status,
		CallId:    tool.ID,
		Name:      tool.Name,
		Arguments: chatArgumentsRawMessage(tool.Arguments.String()),
	}
}
