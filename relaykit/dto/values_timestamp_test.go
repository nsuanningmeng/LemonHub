package dto

import (
	"math/big"
	"strconv"
	"strings"
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesCreatedAtBoundsNumericLiteralLength(t *testing.T) {
	response := OpenAIResponsesResponse{CreatedAt: 77}
	// Finite decimals must not cause unbounded arbitrary-precision mantissa work.
	tooLong := "1." + strings.Repeat("1", 255)
	require.Error(t, kitutil.Unmarshal([]byte(`{"created_at":`+tooLong+`}`), &response))
	assert.Equal(t, IntValue(77), response.CreatedAt)

	accepted := "1." + strings.Repeat("1", 254)
	require.NoError(t, kitutil.Unmarshal([]byte(`{"created_at":`+accepted+`}`), &response))
	assert.Equal(t, IntValue(1), response.CreatedAt)
}

func TestResponsesCreatedAtAcceptsNumericTimestamps(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	maxText, minText := strconv.Itoa(maxInt), strconv.Itoa(minInt)
	tests := []struct {
		input string
		want  int
	}{
		{input: "1786588600", want: 1786588600},
		{input: "1786588600.0", want: 1786588600},
		{input: "1786588600.9", want: 1786588600},
		{input: "1.7865886009e9", want: 1786588600},
		{input: "178658860000E-2", want: 1786588600},
		{input: "-1.9", want: -1},
		{input: "1e-400", want: 0},
		{input: "-1e-1000000000", want: 0},
		{input: "0e1000000000", want: 0},
		{input: "null", want: 0},
		{input: `"1786588600"`, want: 1786588600},
		{input: maxText, want: maxInt},
		{input: minText, want: minInt},
		{input: maxText + ".0", want: maxInt},
		{input: minText + ".0", want: minInt},
		// Bounds apply after truncation toward zero, including negative fractions.
		{input: maxText + ".9", want: maxInt},
		{input: minText + ".9", want: minInt},
		{input: maxText + "e0", want: maxInt},
		{input: `"` + maxText + `"`, want: maxInt},
	}
	if strconv.IntSize == 64 {
		exactInteger := int64(9007199254740993)
		tests = append(tests, struct {
			input string
			want  int
		}{input: "9007199254740993", want: int(exactInteger)})
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var response OpenAIResponsesResponse
			require.NoError(t, kitutil.Unmarshal([]byte(`{"created_at":`+tt.input+`}`), &response))
			assert.Equal(t, tt.want, int(response.CreatedAt))
			encoded, err := kitutil.Marshal(response)
			require.NoError(t, err)
			var wire struct {
				CreatedAt int `json:"created_at"`
			}
			require.NoError(t, kitutil.Unmarshal(encoded, &wire), "timestamp must serialize as an integer")
			assert.Equal(t, tt.want, wire.CreatedAt)
		})
	}
	var missing OpenAIResponsesResponse
	require.NoError(t, kitutil.Unmarshal([]byte(`{"id":"resp_1"}`), &missing))
	assert.Zero(t, missing.CreatedAt)
}

func TestResponsesCreatedAtRejectsInvalidOrOverflowingTimestamps(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	aboveMax := new(big.Int).Add(big.NewInt(int64(maxInt)), big.NewInt(1)).String()
	belowMin := new(big.Int).Sub(big.NewInt(int64(minInt)), big.NewInt(1)).String()
	for _, input := range []string{
		aboveMax, belowMin, aboveMax + ".0", belowMin + ".0", aboveMax + "e0", belowMin + "e0",
		"1e19", "1e1000000000", "-1e1000000000", "true", "false", "{}", "[]", `"NaN"`, `"Infinity"`, `"abc"`, `"1.5"`,
	} {
		t.Run(input, func(t *testing.T) {
			response := OpenAIResponsesResponse{CreatedAt: 77}
			require.Error(t, kitutil.Unmarshal([]byte(`{"created_at":`+input+`}`), &response))
			assert.Equal(t, IntValue(77), response.CreatedAt, "failed parsing must not wrap or overwrite a timestamp")
		})
	}
}

func TestResponsesTimestampToleranceDoesNotRelaxUsageOrIndices(t *testing.T) {
	for _, input := range []string{
		`{"type":"response.completed","output_index":1.5,"response":{"created_at":1786588600.0}}`,
		`{"type":"response.completed","response":{"created_at":1786588600.0,"usage":{"input_tokens":12.5,"output_tokens":48,"total_tokens":60}}}`,
	} {
		var event ResponsesStreamResponse
		require.Error(t, kitutil.Unmarshal([]byte(input), &event))
	}
}
