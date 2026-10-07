package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyRequiredPreservesDirectAndIndirectParamPricing(t *testing.T) {
	for _, tt := range []struct {
		name string
		expr string
		need bool
		want float64
	}{
		{"tokens", `tier("base", p * 2)`, false, 200},
		{"string is not a call", `v1:tier("param(stream)", p * 2)`, false, 200},
		{"header only", `tier("base", p * 2) * (header("selector") == "param" ? 2 : 1)`, false, 400},
		{"static environment token", `tier("base", $env.p * 2)`, false, 200},
		{"static environment header alias", `let h = $env["header"]; tier("base", h("selector") == "param" ? p * 4 : p * 2)`, false, 400},
		{"versioned request rule", `v1:tier("base", p * 2) * (param("stream") == true ? 2 : 1)`, true, 400},
		{"direct alias", `let f = param; tier("base", f("stream") == true ? p * 4 : p * 2)`, true, 400},
		{"environment member", `tier("base", $env.param("stream") == true ? p * 4 : p * 2)`, true, 400},
		{"environment string alias", `let f = $env["param"]; tier("base", f("stream") == true ? p * 4 : p * 2)`, true, 400},
		{"dynamic environment alias", `let f = $env[header("selector")]; tier("base", f("stream") == true ? p * 4 : p * 2)`, true, 400},
		{"environment escapes via alias", `let e = $env; tier("base", e.param("stream") == true ? p * 4 : p * 2)`, true, 400},
		{"conditional may use body", `p > 50 ? tier("fast", param("stream") == true ? p * 4 : p * 2) : tier("base", p * 2)`, true, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, cached := range []bool{false, true} {
				if !cached {
					InvalidateCache()
				}
				need, err := RequestBodyRequired(tt.expr)
				require.NoError(t, err)
				assert.Equal(t, tt.need, need)
				input := RequestInput{Headers: map[string]string{"selector": "param"}}
				if need {
					input.Body = []byte(`{"stream":true}`)
				}
				cost, _, err := RunExprWithRequest(tt.expr, TokenParams{P: 100}, input)
				require.NoError(t, err)
				assert.Equal(t, tt.want, cost)
			}
		})
	}
}

func TestRequestBodyRequiredIsIndependentOfMutableUsedVars(t *testing.T) {
	t.Cleanup(InvalidateCache)
	expr := `tier("base", param("stream") == true ? p * 4 : p * 2)`
	used := UsedVars(expr)
	require.True(t, used["param"])
	delete(used, "param")

	need, err := RequestBodyRequired(expr)
	require.NoError(t, err)
	assert.True(t, need, "mutating token-normalization metadata must not drop a request multiplier")
}

func TestRequestBodyRequiredReturnsCompileError(t *testing.T) {
	_, err := RequestBodyRequired(`tier("base", p *)`)
	require.ErrorContains(t, err, "expr compile error")
}
