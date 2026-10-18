package gemini

import (
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGeminiNullableSchemaAllowsNullThroughEveryConstraint(t *testing.T) {
	var schema any
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"type":"OBJECT","nullable":true,"anyOf":[{"type":"OBJECT","properties":{"value":{"type":"STRING"}}}]}`, &schema))
	got, dropped := OpenAPISchemaToJSONSchema(schema)
	assert.Empty(t, dropped)
	wire, err := kitutil.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":["object","null"],"anyOf":[{"type":"object","properties":{"value":{"type":"string"}}},{"type":"null"}]}`, string(wire))
	original, err := kitutil.Marshal(schema)
	require.NoError(t, err)
	assert.Contains(t, string(original), `"nullable":true`)
}
