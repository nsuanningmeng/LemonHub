package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// DeepSeekToolsHelper relays the DeepSeek Search and Fetch APIs like
// /v1/alpha/search: the raw body is forwarded, the response is returned
// unchanged, and the call is billed at the routing name's request price with
// no tokens.
func DeepSeekToolsHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	switch info.ChannelType {
	case constant.ChannelTypeDeepSeek,
		constant.ChannelTypeNewAPI,
		constant.ChannelTypeSub2API,
		constant.ChannelTypeAdvancedCustom:
	default:
		// Allow retry onto another channel that may support this endpoint.
		return types.NewError(
			fmt.Errorf("channel does not support %s", c.Request.URL.Path),
			types.ErrorCodeInvalidRequest,
		)
	}

	request, ok := info.Request.(*dto.DeepSeekToolsRequest)
	if !ok {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected *dto.DeepSeekToolsRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	err := helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	jsonData := request.RawBody

	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return newAPIErrorFromParamOverride(err)
		}
	}

	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	resp, err := adaptor.DoRequest(c, info, body)
	if err != nil {
		if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
			return types.NewErrorWithStatusCode(requestErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewOpenAIError(errors.New("invalid http response"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		// Tool failures can echo the user's query or the fetched page in either
		// an error message or an HTML response. Keep those bodies out of both
		// relay error logs and the public error envelope.
		newAPIError = types.NewOpenAIError(
			fmt.Errorf("tool upstream returned HTTP %d", httpResp.StatusCode),
			types.ErrorCodeBadResponseStatusCode,
			httpResp.StatusCode,
		)
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	// Search results and fetched pages are bounded before buffering. An
	// oversized or malformed response cannot become evidence for a charge.
	const maxToolResponseBytes = 16 << 20
	responseBody, err := io.ReadAll(io.LimitReader(httpResp.Body, maxToolResponseBytes+1))
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	if len(responseBody) > maxToolResponseBytes || !gjson.ValidBytes(responseBody) {
		return types.NewOpenAIError(errors.New("invalid or oversized tool response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	service.IOCopyBytesGracefully(c, httpResp, responseBody)

	// DeepSeek bills a successful request only when it returns something: a
	// non-empty results array for Search, page content for Fetch. A billable
	// call settles at the routing name's request price; any other response
	// releases the reservation.
	var billable bool
	switch info.RelayMode {
	case relayconstant.RelayModeToolsFetch:
		content := gjson.GetBytes(responseBody, "content")
		billable = content.Type == gjson.String && strings.TrimSpace(content.Str) != ""
	default:
		results := gjson.GetBytes(responseBody, "results")
		billable = results.IsArray() && len(results.Array()) > 0
	}
	if !billable {
		if info.Billing != nil {
			info.Billing.Refund(c)
		}
		return nil
	}

	info.BillableToolResult = true
	service.PostTextConsumeQuota(c, info, &dto.Usage{}, nil)
	return nil
}
