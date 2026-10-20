package common

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/tidwall/gjson"
)

var reservedBillableToolNames = map[string]struct{}{
	dto.BuildInToolWebSearchPreview: {},
	dto.BuildInToolWebSearch:        {},
	dto.BuildInToolFileSearch:       {},
	dto.BuildInToolGoogleSearch:     {},
	dto.BuildInToolImageGeneration:  {},
	"deepseek_search":               {},
	"deepseek_fetch":                {},
}

// CountBillableToolCall is the single entry point for per-call tool billing counts.
// Built-in call types always count; custom function/tool_use names only count when priced.
func (info *RelayInfo) CountBillableToolCall(itemType string, functionName string) {
	if info == nil {
		return
	}
	if info.ResponsesUsageInfo == nil {
		info.ResponsesUsageInfo = &ResponsesUsageInfo{
			BuiltInTools: make(map[string]*BuildInToolInfo),
		}
	}
	if info.ResponsesUsageInfo.BuiltInTools == nil {
		info.ResponsesUsageInfo.BuiltInTools = make(map[string]*BuildInToolInfo)
	}

	switch itemType {
	case dto.BuildInCallWebSearchCall:
		name := info.WebSearchBillingKey
		if name == "" {
			name = resolveWebSearchToolName(info.ResponsesUsageInfo.BuiltInTools)
		}
		info.incrementBillableToolCall(name)
	case dto.BuildInCallFileSearchCall:
		info.incrementBillableToolCall(dto.BuildInToolFileSearch)
	case dto.BuildInCallFunctionCall, dto.BuildInCallToolUse:
		if functionName == "" {
			return
		}
		if _, reserved := reservedBillableToolNames[functionName]; reserved {
			return
		}
		if operation_setting.GetToolPriceForModel(functionName, info.OriginModelName) <= 0 {
			return
		}
		info.incrementBillableToolCall(functionName)
	}
}

func resolveWebSearchToolName(tools map[string]*BuildInToolInfo) string {
	if _, ok := tools[dto.BuildInToolWebSearchPreview]; ok {
		return dto.BuildInToolWebSearchPreview
	}
	if _, ok := tools[dto.BuildInToolWebSearch]; ok {
		return dto.BuildInToolWebSearch
	}
	return dto.BuildInToolWebSearchPreview
}

func (info *RelayInfo) incrementBillableToolCall(name string) {
	if existing, ok := info.ResponsesUsageInfo.BuiltInTools[name]; ok && existing != nil {
		existing.CallCount++
		return
	}
	info.ResponsesUsageInfo.BuiltInTools[name] = &BuildInToolInfo{
		ToolName:  name,
		CallCount: 1,
	}
}

// ImageGenerationCallCounter counts completed Responses image_generation_call
// outputs with stream-safe identity deduplication.
type ImageGenerationCallCounter struct {
	seen  map[string]struct{}
	count int
}

// Observe records one completed final image output when billable.
// outputIndex may be nil; when set and nonnegative it participates in dedup.
func (c *ImageGenerationCallCounter) Observe(item *dto.ResponsesOutput, outputIndex *int) {
	if c == nil || item == nil {
		return
	}
	if item.Type != dto.ResponsesOutputTypeImageGenerationCall {
		return
	}
	if strings.TrimSpace(item.Result) == "" {
		return
	}
	switch strings.ToLower(strings.TrimSpace(item.Status)) {
	case "failed", "cancelled", "canceled", "incomplete", "partial":
		return
	}

	aliases := make([]string, 0, 4)
	if item.ID != "" {
		aliases = append(aliases, "id:"+item.ID)
	}
	if item.CallId != "" {
		aliases = append(aliases, "call:"+item.CallId)
	}
	if outputIndex != nil && *outputIndex >= 0 {
		aliases = append(aliases, fmt.Sprintf("index:%d", *outputIndex))
	}
	sum := sha256.Sum256([]byte(item.Result))
	aliases = append(aliases, "result:"+hex.EncodeToString(sum[:]))

	if c.seen == nil {
		c.seen = make(map[string]struct{})
	}
	for _, alias := range aliases {
		if _, ok := c.seen[alias]; ok {
			return
		}
	}
	for _, alias := range aliases {
		c.seen[alias] = struct{}{}
	}
	c.count++
}

// Reset clears pending observations (used when a terminal response fails).
func (c *ImageGenerationCallCounter) Reset() {
	if c == nil {
		return
	}
	c.seen = nil
	c.count = 0
}

// Count returns the deduplicated completed image output count before commit capping.
func (c *ImageGenerationCallCounter) Count() int {
	if c == nil {
		return 0
	}
	return c.count
}

// Commit writes the capped completed-output count into RelayInfo once.
// Request tool declarations alone must not become billable calls.
func (c *ImageGenerationCallCounter) Commit(info *RelayInfo) {
	if info == nil {
		return
	}
	if info.ResponsesUsageInfo == nil {
		info.ResponsesUsageInfo = &ResponsesUsageInfo{
			BuiltInTools: make(map[string]*BuildInToolInfo),
		}
	}
	if info.ResponsesUsageInfo.BuiltInTools == nil {
		info.ResponsesUsageInfo.BuiltInTools = make(map[string]*BuildInToolInfo)
	}

	count := 0
	if c != nil {
		count = c.count
	}
	if count > dto.MaxImageN {
		count = dto.MaxImageN
	}

	if existing, ok := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration]; ok && existing != nil {
		existing.CallCount = count
		return
	}
	info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration] = &BuildInToolInfo{
		ToolName:  dto.BuildInToolImageGeneration,
		CallCount: count,
	}
}

// IsNonBillableResponsesStatus reports terminal response statuses that must not
// bill pending image_generation observations.
func IsNonBillableResponsesStatus(status []byte) bool {
	if len(status) == 0 {
		return false
	}
	var s string
	if err := common.Unmarshal(status, &s); err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "failed", "cancelled", "canceled", "incomplete":
		return true
	default:
		return false
	}
}

// hostedWebToolCounter keeps observations separate from cumulative usage reports.
// Repeated terminal snapshots cannot charge a completed call twice.
type hostedWebToolCounter struct {
	seen  map[string]struct{}
	count int
}

// ObserveHostedWebToolResult records a completed hosted search or fetch. A stable
// upstream call ID or content/output index is required for duplicate suppression.
func (info *RelayInfo) ObserveHostedWebToolResult(name, id string, index *int) {
	if info == nil || name == "" || (name != info.WebSearchBillingKey && name != info.WebFetchBillingKey) {
		return
	}
	var aliases []string
	if id != "" {
		aliases = append(aliases, "id:"+id)
	}
	if index != nil && *index >= 0 {
		aliases = append(aliases, fmt.Sprintf("index:%d", *index))
	}
	if len(aliases) == 0 {
		return
	}
	if info.hostedWebTools == nil {
		info.hostedWebTools = make(map[string]*hostedWebToolCounter)
	}
	counter := info.hostedWebTools[name]
	if counter == nil {
		counter = &hostedWebToolCounter{seen: make(map[string]struct{})}
		info.hostedWebTools[name] = counter
	}
	duplicate := false
	for _, alias := range aliases {
		if _, exists := counter.seen[alias]; exists {
			duplicate = true
		}
	}
	for _, alias := range aliases {
		counter.seen[alias] = struct{}{}
	}
	if !duplicate && counter.count < math.MaxInt {
		counter.count++
	}
	info.recordHostedWebToolUsage(name, counter.count)
}

// RecordHostedWebToolUsage reconciles cumulative reports with completed results.
// A missing, negative or smaller report cannot erase work already observed.
func (info *RelayInfo) RecordHostedWebToolUsage(searches, fetches int) {
	if info == nil {
		return
	}
	info.recordHostedWebToolUsage(info.WebSearchBillingKey, searches)
	info.recordHostedWebToolUsage(info.WebFetchBillingKey, fetches)
}

func (info *RelayInfo) recordHostedWebToolUsage(name string, count int) {
	if name == "" || count <= 0 {
		return
	}
	if info.ResponsesUsageInfo == nil {
		info.ResponsesUsageInfo = &ResponsesUsageInfo{}
	}
	if info.ResponsesUsageInfo.BuiltInTools == nil {
		info.ResponsesUsageInfo.BuiltInTools = make(map[string]*BuildInToolInfo)
	}
	tool := info.ResponsesUsageInfo.BuiltInTools[name]
	if tool == nil {
		tool = &BuildInToolInfo{ToolName: name}
		info.ResponsesUsageInfo.BuiltInTools[name] = tool
	}
	tool.CallCount = max(tool.CallCount, count)
}

// ObserveResponsesHostedWebTools reads the original upstream action because the
// shared Responses DTO deliberately leaves vendor fields out of conversion.
// DeepSeek open_page actions use the Fetch price; other completed actions use Search.
func (info *RelayInfo) ObserveResponsesHostedWebTools(raw []byte) {
	if info == nil || (info.WebSearchBillingKey == "" && info.WebFetchBillingKey == "") {
		return
	}
	root := gjson.ParseBytes(raw)
	observe := func(item gjson.Result, index *int) {
		if item.Get("type").String() != dto.BuildInCallWebSearchCall || item.Get("status").String() != "completed" {
			return
		}
		name := info.WebSearchBillingKey
		if item.Get("action.type").String() == "open_page" {
			name = info.WebFetchBillingKey
		}
		info.ObserveHostedWebToolResult(name, item.Get("id").String(), index)
	}
	if root.Get("type").String() == dto.ResponsesOutputTypeItemDone {
		var index *int
		if value := root.Get("output_index"); value.Exists() && value.Type == gjson.Number && value.Int() >= 0 && value.Int() <= int64(math.MaxInt) {
			i := int(value.Int())
			index = &i
		}
		observe(root.Get("item"), index)
		return
	}
	switch root.Get("type").String() {
	case "", "response", "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
	default:
		return
	}
	output := root.Get("output")
	if !output.Exists() {
		output = root.Get("response.output")
	}
	if output.IsArray() {
		for i, item := range output.Array() {
			observe(item, &i)
		}
	}
}
