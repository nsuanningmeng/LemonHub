package openai

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBatch9EmbeddedErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		body    string
		status  int
		code    any
		message string
	}{
		{`{"error":{"code":"server_is_overloaded","message":"overloaded"}}`, 503, "server_is_overloaded", "overloaded"},
		{`{"error":{"type":"authentication_error","message":"auth"}}`, 401, nil, "auth"},
		{`{"error":{"type":"overloaded_error"}}`, 503, nil, "upstream returned an error"},
		{`{"error":{"code":"server_is_overloaded"}}`, 503, "server_is_overloaded", "upstream returned an error"},
		{`{"error":{"code":"invalid_api_key","message":"auth"}}`, 401, "invalid_api_key", "auth"},
		{`{"error":{"type":"permission_error","message":"denied"}}`, 403, nil, "denied"},
		{`{"error":{"type":"rate_limit_error","message":"rate"}}`, 429, nil, "rate"},
		{`{"error":{"type":"invalid_request_error","message":"invalid"}}`, 400, nil, "invalid"},
		{`{"error":{"message":"unknown"}}`, 502, nil, "unknown"},
		{`{"error":{"code":429,"message":"numeric"}}`, 502, float64(429), "numeric"},
		{`{"error":"string failure"}`, 502, nil, "string failure"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			var raw map[string]any
			require.NoError(t, common.Unmarshal([]byte(tc.body), &raw))
			err := ClassifyOpenAIEmbeddedError(raw["error"])
			require.NotNil(t, err)
			assert.Equal(t, tc.status, err.StatusCode)
			e := err.ToOpenAIError()
			assert.Equal(t, tc.code, e.Code)
			assert.Equal(t, tc.message, e.Message)
		})
	}
}
func TestBatch9EmbeddedErrorNullAndMalformed(t *testing.T) {
	for _, body := range []string{`{}`, `{"error":null}`, `{"error":{}}`, `{"error":""}`, `{"error":{"code":null,"message":"","type":""}}`, `{"choices":[{"delta":{"content":"{\"error\":{\"message\":\"literal\"}}","tool_calls":[{"function":{"arguments":"{\"error\":true}"}}]}}]}`} {
		var raw map[string]any
		require.NoError(t, common.Unmarshal([]byte(body), &raw))
		assert.Nil(t, ClassifyOpenAIEmbeddedError(raw["error"]), body)
	}
	for _, body := range []string{`{"error":["private-sentinel"]}`, `{"error":true}`, `{"error":42}`, `{"error":{"message":{"secret":"private-sentinel"}}}`, `{"error":{"code":{"secret":"private-sentinel"}}}`, `{"error":{"type":42}}`} {
		var raw map[string]any
		require.NoError(t, common.Unmarshal([]byte(body), &raw))
		err := ClassifyOpenAIEmbeddedError(raw["error"])
		require.NotNil(t, err, body)
		assert.Equal(t, 502, err.StatusCode)
		assert.NotContains(t, err.Error(), "private-sentinel")
	}
}
