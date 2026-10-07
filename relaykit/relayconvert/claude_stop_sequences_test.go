package relayconvert

import (
	"context"
	"encoding/json"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClaudeStopSequencesRemainArraysOnFinalChatWire(t *testing.T) {
	for _, stops := range []string{`["stop-one"]`, `["stop-one","stop-two"]`, `[]`, `null`, `missing`} {
		t.Run(stops, func(t *testing.T) {
			var req dto.ClaudeRequest
			raw := `{"model":"m","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
			if stops != "missing" {
				raw = raw[:len(raw)-1] + `,"stop_sequences":` + stops + `}`
			}
			require.NoError(t, kitutil.UnmarshalJsonStr(raw, &req))
			result, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, &req)
			require.NoError(t, err)
			wire, err := kitutil.Marshal(result.Value)
			require.NoError(t, err)
			var actual map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire, &actual))
			if stops == `[]` || stops == `null` || stops == "missing" {
				assert.NotContains(t, actual, "stop")
			} else {
				assert.JSONEq(t, stops, string(actual["stop"]))
			}
			if len(req.StopSequences) > 0 {
				chat := result.Value.(*dto.GeneralOpenAIRequest)
				hop, err := ConvertRequest(context.Background(), nil, types.RelayFormatClaude, chat)
				require.NoError(t, err)
				back, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, hop.Value)
				require.NoError(t, err)
				backwire, err := kitutil.Marshal(back.Value)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(backwire, &actual))
				assert.JSONEq(t, stops, string(actual["stop"]))
			}
		})
	}
}
func TestNativeChatStringStopIsStillAccepted(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "m", Stop: "native-stop"}
	result, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, req)
	require.NoError(t, err)
	wire, err := kitutil.Marshal(result.Value)
	require.NoError(t, err)
	assert.Contains(t, string(wire), `"stop":"native-stop"`)
}
