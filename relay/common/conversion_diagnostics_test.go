package common

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversionDiagnosticsRemainWithinOneAttempt(t *testing.T) {
	info := &RelayInfo{}
	d := convmeta.ConversionDiagnostic{Code: convmeta.DiagnosticDeveloperRoleMerged, Source: types.RelayFormatOpenAI, Target: types.RelayFormatGemini, Field: "messages.role", Message: "private prompt"}
	info.RecordConversionDiagnostic(d)
	info.RecordConversionDiagnostic(d)
	require.Len(t, info.ConversionDiagnostics(), 1)
	assert.NotContains(t, info.ConversionDiagnostics()[0].Message, "private prompt")
	require.Len(t, info.TakeConversionDiagnosticsForLogging(), 1)
	assert.Empty(t, info.TakeConversionDiagnosticsForLogging())
	snapshot := info.ConversionDiagnostics()
	snapshot[0].Message = "tampered"
	assert.NotEqual(t, "tampered", info.ConversionDiagnostics()[0].Message)
	info.ResetConversionDiagnostics()
	assert.Empty(t, info.ConversionDiagnostics())
	assert.Empty(t, info.TakeConversionDiagnosticsForLogging())
	info.RecordConversionDiagnostic(d)
	require.Len(t, info.TakeConversionDiagnosticsForLogging(), 1)
}

func TestConversionDiagnosticsRejectUnsafeFieldsAndNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.NotPanics(t, func() {
		info.RecordConversionDiagnostic(convmeta.ConversionDiagnostic{})
		info.ResetConversionDiagnostics()
	})
	assert.Empty(t, info.ConversionDiagnostics())
	assert.Empty(t, info.TakeConversionDiagnosticsForLogging())
	info = &RelayInfo{}
	info.RecordConversionDiagnostic(convmeta.ConversionDiagnostic{Code: "private schema"})
	assert.Empty(t, info.ConversionDiagnostics())
}
