package claude

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleClaudeResponseDataCountsToolUse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	operation_setting.SetToolPriceForTest("lookup_fn", 3.0)
	t.Cleanup(func() {
		operation_setting.DeleteToolPriceForTest("lookup_fn")
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-7-sonnet",
		RelayFormat:     types.RelayFormatClaude,
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}

	data := []byte(`{
		"type":"message",
		"content":[
			{"type":"text","text":"hi"},
			{"type":"tool_use","id":"tu1","name":"lookup_fn","input":{}},
			{"type":"server_tool_use","id":"stu1","name":"web_search","input":{}}
		],
		"usage":{"input_tokens":1,"output_tokens":1}
	}`)

	err := HandleClaudeResponseData(c, info, claudeInfo, nil, data)
	require.Nil(t, err)
	require.NotNil(t, info.ResponsesUsageInfo)
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, "lookup_fn")
	assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools["lookup_fn"].CallCount)
	assert.NotContains(t, info.ResponsesUsageInfo.BuiltInTools, "web_search")
}

func TestCountClaudeStreamBillableToolsSetsWebSearchRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{OriginModelName: "claude-3-7-sonnet"}

	countClaudeStreamBillableTools(c, info, &dto.ClaudeResponse{
		Type: "message_delta",
		Usage: &dto.ClaudeUsage{
			ServerToolUse: &dto.ClaudeServerToolUse{WebSearchRequests: 3},
		},
	})
	assert.Equal(t, 3, c.GetInt("claude_web_search_requests"))

	operation_setting.SetToolPriceForTest("stream_fn", 2.0)
	t.Cleanup(func() {
		operation_setting.DeleteToolPriceForTest("stream_fn")
	})
	countClaudeStreamBillableTools(c, info, &dto.ClaudeResponse{
		Type: "content_block_start",
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: "tool_use",
			Name: "stream_fn",
		},
	})
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, "stream_fn")
	assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools["stream_fn"].CallCount)
}

func TestClaudeHostedWebToolsUseVendorPricesAndCompletedResults(t *testing.T) {
	blocks := `[{"type":"web_search_tool_result","tool_use_id":"search1","content":[]},{"type":"web_search_tool_result","tool_use_id":"search1","content":[]},{"type":"web_search_tool_result","tool_use_id":"error1","content":{"type":"web_search_tool_result_error"}},{"type":"web_fetch_tool_result","tool_use_id":"fetch1","content":{"type":"web_fetch_result","content":{"type":"document"}}},{"type":"web_fetch_tool_result","tool_use_id":"error2","content":{"type":"web_fetch_tool_result_error"}}]`
	for _, stream := range []bool{true, false} {
		for _, report := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("stream=%t report=%d", stream, report), func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				info := &relaycommon.RelayInfo{OriginModelName: "mapped-public-model", RelayFormat: types.RelayFormatClaude, WebSearchBillingKey: "deepseek_search", WebFetchBillingKey: "deepseek_fetch"}
				usage := fmt.Sprintf(`{"input_tokens":1,"output_tokens":1,"server_tool_use":{"web_search_requests":%d,"web_fetch_requests":%d}}`, report, report)
				if stream {
					var content []dto.ClaudeMediaMessage
					require.NoError(t, common.UnmarshalJsonStr(blocks, &content))
					for i := range content {
						countClaudeStreamBillableTools(c, info, &dto.ClaudeResponse{Type: "content_block_start", Index: &i, ContentBlock: &content[i]})
					}
					if report > 0 {
						var u dto.ClaudeUsage
						require.NoError(t, common.UnmarshalJsonStr(usage, &u))
						countClaudeStreamBillableTools(c, info, &dto.ClaudeResponse{Type: "message_delta", Usage: &u})
					}
				} else {
					body := []byte(`{"type":"message","content":` + blocks + `,"usage":` + usage + `}`)
					require.Nil(t, HandleClaudeResponseData(c, info, &ClaudeResponseInfo{Usage: &dto.Usage{}}, nil, body))
				}
				require.NotNil(t, info.ResponsesUsageInfo)
				assert.Equal(t, max(1, report), info.BuiltInTools["deepseek_search"].CallCount)
				assert.Equal(t, max(1, report), info.BuiltInTools["deepseek_fetch"].CallCount)
				assert.Zero(t, c.GetInt("claude_web_search_requests"), "vendor counts must not also bill native Claude search")
			})
		}
	}
}

func TestClaudeStreamHandlerRetainsHostedToolsAcrossConversionAndMissingFinalUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		for _, finalUsage := range []bool{false, true} {
			t.Run(fmt.Sprintf("format=%s finalUsage=%t", format, finalUsage), func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				info := &relaycommon.RelayInfo{
					OriginModelName:     "public-alias",
					RelayFormat:         format,
					DisablePing:         true,
					WebSearchBillingKey: "deepseek_search",
					WebFetchBillingKey:  "deepseek_fetch",
					ChannelMeta: &relaycommon.ChannelMeta{
						UpstreamModelName: "deepseek-flash",
					},
				}
				events := []string{
					`{"type":"message_start","message":{"id":"msg_tools","type":"message","role":"assistant","model":"deepseek-flash","content":[],"usage":{"input_tokens":2,"output_tokens":0}}}`,
					`{"type":"content_block_start","index":0,"content_block":{"type":"web_search_tool_result","tool_use_id":"search1","content":[]}}`,
					`{"type":"content_block_start","index":0,"content_block":{"type":"web_search_tool_result","tool_use_id":"search1","content":[]}}`,
					`{"type":"content_block_start","index":1,"content_block":{"type":"web_fetch_tool_result","tool_use_id":"fetch1","content":{"type":"web_fetch_result","content":{"type":"document"}}}}`,
					`{"type":"content_block_start","index":2,"content_block":{"type":"web_fetch_tool_result","tool_use_id":"failed","content":{"type":"web_fetch_tool_result_error"}}}`,
				}
				if finalUsage {
					events = append(events,
						`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1,"server_tool_use":{"web_search_requests":1,"web_fetch_requests":1}}}`,
						`{"type":"message_stop"}`,
					)
				}
				body := "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
				t.Cleanup(func() { _ = resp.Body.Close() })

				usage, apiErr := ClaudeStreamHandler(c, resp, info)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				require.NotNil(t, info.ResponsesUsageInfo)
				require.Contains(t, info.BuiltInTools, "deepseek_search")
				require.Contains(t, info.BuiltInTools, "deepseek_fetch")
				assert.Equal(t, 1, info.BuiltInTools["deepseek_search"].CallCount)
				assert.Equal(t, 1, info.BuiltInTools["deepseek_fetch"].CallCount)
				assert.Zero(t, c.GetInt("claude_web_search_requests"))
				assert.NotContains(t, info.BuiltInTools, dto.BuildInToolWebSearch)
			})
		}
	}
}
