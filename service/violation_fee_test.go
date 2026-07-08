package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NormalizeViolationFeeError re-wraps violation errors into new NewAPIError
// objects (in the retry loop AND again in the refund defer). The channel
// user-message-override tag must survive both re-wraps, or the raw upstream
// violation text reaches the user despite override being enabled.
func TestNormalizeViolationFeeErrorPreservesUserMessageOverride(t *testing.T) {
	t.Parallel()

	err := types.WithOpenAIError(types.OpenAIError{
		Message: "Failed check: SAFETY_CHECK_TYPE csam detected in prompt",
		Type:    "invalid_request_error",
	}, http.StatusBadRequest)
	err.SetUserMessageOverride("渠道固定文案")

	// First normalize: CSAM marker path re-wraps into a new error object.
	wrapped := NormalizeViolationFeeError(err)
	require.NotSame(t, err, wrapped)
	require.Equal(t, types.ErrorCodeViolationFeeGrokCSAM, wrapped.GetErrorCode())
	overrideText, ok := wrapped.UserMessageOverride()
	require.True(t, ok, "override tag must survive CSAM re-wrap")
	assert.Equal(t, "渠道固定文案", overrideText)

	// Second normalize (refund defer path): violation-fee code branch re-wraps again.
	wrapped2 := NormalizeViolationFeeError(wrapped)
	overrideText2, ok := wrapped2.UserMessageOverride()
	require.True(t, ok, "override tag must survive violation-code re-wrap")
	assert.Equal(t, "渠道固定文案", overrideText2)
}
