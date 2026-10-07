package relay

import (
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// newPassthroughRequestBody keeps the original replay source immutable across
// attempts. Only explicit overrides require loading and patching the raw JSON;
// otherwise even disk-backed bodies are replayed without materializing Bytes.
func newPassthroughRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (common.ReplayableBody, io.Closer, *types.NewAPIError) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) == 0 {
		return common.NewReplayableBodyReader(storage), nil, nil
	}
	jsonData, err := storage.Bytes()
	if err != nil {
		return nil, nil, types.NewErrorWithStatusCode(errors.New("failed to read passthrough request body"), types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if !gjson.ValidBytes(jsonData) || !gjson.ParseBytes(jsonData).IsObject() {
		return nil, nil, types.NewErrorWithStatusCode(errors.New("passthrough parameter override requires a valid JSON object"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
	if err != nil {
		if _, ok := relaycommon.AsParamOverrideReturnError(err); ok {
			return nil, nil, newAPIErrorFromParamOverride(err)
		}
		return nil, nil, newAPIErrorFromParamOverride(errors.New("invalid passthrough parameter override"))
	}
	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return nil, nil, types.NewError(errors.New("failed to prepare passthrough request body"), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	return body, closer, nil
}
