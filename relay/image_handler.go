package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	imageReq, ok := info.Request.(*dto.ImageRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.ImageRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(imageReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ImageRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	imageCount := uint(1)
	if request.N != nil && *request.N != 0 {
		imageCount = *request.N
	}
	if imageCount > dto.MaxImageN {
		return types.NewErrorWithStatusCode(fmt.Errorf("n must be an integer between 1 and %d", dto.MaxImageN), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if strings.EqualFold(c.ContentType(), "multipart/form-data") {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		defer form.RemoveAll()
		// Both converted and passthrough multipart bodies forward every value.
		// Reject ambiguous counts instead of trusting the validator's first n
		// while a provider might use its last value.
		imageCount = 1
		for key, values := range form.Value {
			if !strings.EqualFold(key, "n") {
				continue
			}
			if key != "n" || len(values) != 1 {
				return types.NewErrorWithStatusCode(errors.New("ambiguous multipart image count"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			if raw := strings.TrimSpace(values[0]); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil || n < 0 || n > dto.MaxImageN {
					return types.NewErrorWithStatusCode(fmt.Errorf("n must be an integer between 1 and %d", dto.MaxImageN), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				}
				if n != 0 {
					imageCount = uint(n)
				}
			}
		}
		for key := range form.File {
			if strings.EqualFold(key, "n") {
				return types.NewErrorWithStatusCode(errors.New("image count must be a form value"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
		}
	}

	var requestBody io.Reader

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		if strings.EqualFold(c.ContentType(), "multipart/form-data") {
			// Multipart image edits retain the original form/binary body; the
			// parameter override engine only patches JSON requests.
			storage, err := common.GetBodyStorage(c)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			requestBody = common.NewReplayableBodyReader(storage)
		} else {
			body, closer, apiErr := newPassthroughRequestBody(c, info)
			if apiErr != nil {
				return apiErr
			}
			if closer != nil {
				defer closer.Close()
			}
			// Inspect an independent cursor without consuming the outbound body
			// or requesting an additional Bytes copy from its storage.
			reader, err := body.NewReader()
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			imageCount, err = outboundImageCount(reader, info.ApiType)
			_ = reader.Close()
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			requestBody = body
		}
	} else {
		convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		switch convertedRequest.(type) {
		case *bytes.Buffer:
			requestBody = convertedRequest.(io.Reader)
		default:
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			// apply param override
			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if err != nil {
					return newAPIErrorFromParamOverride(err)
				}
			}
			imageCount, err = outboundImageCount(bytes.NewReader(jsonData), info.ApiType)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}

			logger.LogDebug(c, "image request body: %s", jsonData)
			body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			defer closer.Close()
			jsonData = nil
			requestBody = body
		}
	}

	// Fixed image prices use the effective wire quantity, after conversion and
	// overrides. Refresh it on every attempt so retries cannot retain another
	// channel's quantity. Token and expression pricing keep their existing
	// contract, including the frozen original request used by param().
	if info.PriceData.UsePrice && info.TieredBillingSnapshot == nil {
		info.PriceData.AddOtherRatio("n", float64(imageCount))
		quota, err := common.QuotaFromFloatStrict(info.PriceData.ApplyOtherRatiosToFloat(info.PriceData.ModelPrice * common.QuotaPerUnit * info.PriceData.GroupRatioInfo.GroupRatio))
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		info.PriceData.QuotaToPreConsume = quota
		if quota != 0 || info.Billing != nil {
			if billingErr := service.PreConsumeBilling(c, quota, info); billingErr != nil {
				return billingErr
			}
		}
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		if requestErr := c.Request.Context().Err(); requestErr != nil && errors.Is(err, requestErr) {
			return types.NewErrorWithStatusCode(requestErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			if httpResp.StatusCode == http.StatusCreated && info.ApiType == constant.APITypeReplicate {
				// replicate channel returns 201 Created when using Prefer: wait, treat it as success.
				httpResp.StatusCode = http.StatusOK
			} else {
				newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
				// reset status code 重置状态码
				service.ResetStatusCode(newAPIError, statusCodeMappingStr)
				return newAPIError
			}
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	imageN := imageCount
	if count, ok := info.PriceData.OtherRatios()["n"]; ok && info.PriceData.UsePrice && count >= 1 && count <= dto.MaxImageN {
		imageN = uint(count)
	}

	if usage.(*dto.Usage).TotalTokens == 0 {
		usage.(*dto.Usage).TotalTokens = 1
	}
	if usage.(*dto.Usage).PromptTokens == 0 {
		usage.(*dto.Usage).PromptTokens = 1
	}

	quality := request.Quality
	if quality == "" {
		quality = "standard"
	}

	var logContent []string

	if len(request.Size) > 0 {
		logContent = append(logContent, fmt.Sprintf("大小 %s", request.Size))
	}
	if len(quality) > 0 {
		logContent = append(logContent, fmt.Sprintf("品质 %s", quality))
	}
	if imageN > 0 {
		logContent = append(logContent, fmt.Sprintf("生成数量 %d", imageN))
	}

	service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), logContent)
	return nil
}

// outboundImageCount reads only the provider's quantity field from the final
// JSON body. Provider-native fields can override the original OpenAI n even
// without a channel override (for example Replicate extra_fields/extra input).
func outboundImageCount(reader io.Reader, apiType int) (uint, error) {
	quantity := outboundImageQuantity{apiType: apiType}
	if err := common.DecodeJsonBody(reader, &quantity); err != nil {
		return 0, fmt.Errorf("invalid image billing parameters: %w", err)
	}
	return quantity.count, nil
}

type outboundImageQuantity struct {
	apiType int
	count   uint
}

// JSON's usual struct decoder accepts case-insensitive keys and silently uses
// the last duplicate. Providers need not share those semantics. Inspect every
// key along the actual provider path and reject ambiguous billing quantities.
func (q *outboundImageQuantity) UnmarshalJSON(data []byte) error {
	path := []string{"n"}
	requireCount := false
	switch q.apiType {
	case constant.APITypeAli:
		path = []string{"parameters", "n"}
	case constant.APITypeReplicate:
		path = []string{"input", "num_outputs"}
	case constant.APITypeGemini, constant.APITypeVertexAi:
		path = []string{"parameters", "sampleCount"}
		// Imagen deployments can have different server-side defaults. The
		// converter emits an explicit count; do not guess one if it is removed.
		requireCount = true
	case constant.APITypeSiliconFlow:
		path = []string{"batch_size"}
	}
	value := gjson.ParseBytes(data)
	for _, key := range path {
		if !value.IsObject() {
			return fmt.Errorf("%s requires a JSON object", strings.Join(path, "."))
		}
		var next gjson.Result
		var fieldErr error
		found := false
		value.ForEach(func(k, v gjson.Result) bool {
			if !strings.EqualFold(k.Str, key) {
				return true
			}
			if k.Str != key || found {
				fieldErr = fmt.Errorf("ambiguous image quantity field %s", strings.Join(path, "."))
				return false
			}
			found, next = true, v
			return true
		})
		if fieldErr != nil {
			return fieldErr
		}
		if !found || next.Type == gjson.Null {
			if requireCount {
				return fmt.Errorf("%s must be explicitly specified", strings.Join(path, "."))
			}
			q.count = 1
			return nil
		}
		value = next
	}
	n, err := strconv.ParseUint(value.Raw, 10, 64)
	if value.Type != gjson.Number || err != nil || n > dto.MaxImageN || (requireCount || q.apiType == constant.APITypeReplicate) && n == 0 {
		return fmt.Errorf("%s must be an integer between 1 and %d", strings.Join(path, "."), dto.MaxImageN)
	}
	// Preserve legacy zero/default behavior. If an override removes the field,
	// the provider's default is one, not the incoming request's previous count.
	if n == 0 {
		n = 1
	}
	q.count = uint(n)
	return nil
}
