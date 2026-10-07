package relayconvert

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesTimestampConversionRoundTrip(t *testing.T) {
	for _, timestamp := range []string{"1786588600.9", strconv.Itoa(int(^uint(0)>>1)) + ".0"} {
		t.Run(timestamp, func(t *testing.T) {
			var response dto.OpenAIResponsesResponse
			require.NoError(t, kitutil.Unmarshal([]byte(`{"id":"resp_1","created_at":`+timestamp+`,"usage":{"input_tokens":12,"output_tokens":48,"total_tokens":60}}`), &response))
			chat, usage, err := ResponsesResponseToChatCompletionsResponse(&response, "chat_1")
			require.NoError(t, err)
			require.NotNil(t, usage)
			assert.Equal(t, int(response.CreatedAt), chat.Created)
			assert.Equal(t, 60, usage.TotalTokens)
			// Chat Created is an interface. Retaining IntValue inside it would
			// bypass the reverse converter's integer case and replace it with now.
			roundTrip, _, err := ChatCompletionsResponseToResponsesResponse(chat, "resp_2")
			require.NoError(t, err)
			assert.Equal(t, response.CreatedAt, roundTrip.CreatedAt)
			wire, err := kitutil.Marshal(roundTrip)
			require.NoError(t, err)
			var decoded struct {
				CreatedAt int `json:"created_at"`
			}
			require.NoError(t, kitutil.Unmarshal(wire, &decoded))
			assert.Equal(t, int(response.CreatedAt), decoded.CreatedAt)
		})
	}
}
