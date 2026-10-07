package helper

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// ResolveIncomingBillingExprRequestInput prepares one evaluation without changing
// the canonical capture on info. A body-free evaluation omits even a preloaded
// body; a later parameter-based estimate can still reuse or complete that capture.
func ResolveIncomingBillingExprRequestInput(c *gin.Context, info *relaycommon.RelayInfo, needBody bool) (billingexpr.RequestInput, error) {
	input := billingexpr.RequestInput{}
	if info != nil && info.BillingRequestInput != nil {
		frozen := info.BillingRequestInput
		input.Headers = cloneStringMap(frozen.Headers)
		if !needBody {
			return input, nil
		}
		if frozen.BodyCaptured || frozen.Body != nil {
			input.Body = append([]byte(nil), frozen.Body...)
			input.BodyCaptured = true
			return input, nil
		}
	} else if info != nil && info.RequestHeaders != nil {
		input.Headers = cloneStringMap(info.RequestHeaders)
	} else if c != nil && c.Request != nil {
		input.Headers = make(map[string]string, len(c.Request.Header))
		for key := range c.Request.Header {
			input.Headers[key] = c.Request.Header.Get(key)
		}
	}
	if !needBody {
		return input, nil
	}

	bodyBytes, err := readIncomingBillingExprBody(c, input.Headers)
	if err != nil {
		return billingexpr.RequestInput{}, err
	}
	input.Body = bodyBytes
	input.BodyCaptured = true
	return input, nil
}

func BuildBillingExprRequestInputFromRequest(request dto.Request, headers map[string]string) (billingexpr.RequestInput, error) {
	input := billingexpr.RequestInput{
		Headers:      cloneStringMap(headers),
		BodyCaptured: true,
	}
	if request == nil {
		return input, nil
	}

	bodyBytes, err := common.Marshal(request)
	if err != nil {
		return billingexpr.RequestInput{}, err
	}
	input.Body = bodyBytes
	return input, nil
}

func readIncomingBillingExprBody(c *gin.Context, headers map[string]string) ([]byte, error) {
	contentType := ""
	for key, value := range headers {
		if strings.EqualFold(key, "Content-Type") {
			contentType = value
			break
		}
	}
	if c == nil || c.Request == nil || !isJSONContentType(contentType) {
		return nil, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

func isJSONContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(contentType, "application/json")
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		if strings.TrimSpace(key) == "" {
			continue
		}
		dst[key] = value
	}
	return dst
}
