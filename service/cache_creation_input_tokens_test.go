package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesCacheCreationAliasSurvivesBillingSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		details string
		write   int
		quota   int
	}{
		{"explicit alias", `"cache_creation_input_tokens":300`, 300, 1005},
		{"aliases do not add", `"cache_creation_input_tokens":300,"cache_write_tokens":250,"cached_creation_tokens":200`, 300, 1005},
		{"native alias larger", `"cache_creation_input_tokens":300,"cache_write_tokens":400`, 400, 1030},
		{"negative alias", `"cache_creation_input_tokens":-300`, 0, 930},
	} {
		for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
			t.Run(tc.name+"/"+string(format), func(t *testing.T) {
				var response dto.OpenAIResponsesResponse
				require.NoError(t, common.Unmarshal([]byte(`{"id":"resp_cache","status":"completed","model":"cache-test","output":[],"usage":{"input_tokens":1000,"output_tokens":20,"total_tokens":1020,"input_tokens_details":{"cached_tokens":100,`+tc.details+`}}}`), &response))
				converted, err := relayconvert.ConvertResponse(nil, &convmeta.Values{}, format, &response)
				require.NoError(t, err)
				require.NotNil(t, converted.Usage)
				require.NotNil(t, converted.Usage.BillingUsage)
				usage := effectiveBillingUsage(converted.Usage)
				assert.Equal(t, 1000, usage.PromptTokens, "the original Responses input is inclusive")
				assert.Equal(t, tc.write, usage.PromptTokensDetails.CacheCreationTokensTotal())
				info := &relaycommon.RelayInfo{
					RelayFormat: format, OriginModelName: "cache-test", StartTime: time.Now(),
					PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, CacheRatio: 0.1, CacheCreationRatio: 1.25, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
				}
				summary := calculateTextQuotaSummary(taskBillingTestContext(), info, usage)
				assert.Equal(t, tc.quota, summary.Quota)
				params := BuildTieredTokenParams(usage, false, map[string]bool{"cr": true, "cc": true})
				assert.Equal(t, float64(tc.write), params.CC)
				assert.Equal(t, float64(900-tc.write), params.P)
			})
		}
	}
}
