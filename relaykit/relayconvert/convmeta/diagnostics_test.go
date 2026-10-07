package convmeta

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type diagnosticObserver struct {
	Values
	reports []ConversionDiagnostic
}

func (o *diagnosticObserver) RecordConversionDiagnostic(d ConversionDiagnostic) {
	o.reports = append(o.reports, d)
}

func TestConversionDiagnosticObserverIsOptionalAndNilSafe(t *testing.T) {
	d := ConversionDiagnostic{Code: DiagnosticDeveloperRoleMerged, Source: types.RelayFormatOpenAI, Target: types.RelayFormatGemini, Field: "messages.role", Message: "private prompt"}
	var nilObserver *diagnosticObserver
	require.NotPanics(t, func() { ReportConversionDiagnostic(nilObserver, d) })
	require.NotPanics(t, func() { ReportConversionDiagnostic(nil, d) })
	require.NotPanics(t, func() { ReportConversionDiagnostic(&Values{}, d) })
	observer := &diagnosticObserver{}
	ReportConversionDiagnostic(observer, d)
	require.Len(t, observer.reports, 1)
	assert.NotContains(t, observer.reports[0].Message, "private prompt")
	assert.Contains(t, observer.reports[0].Message, "Gemini has no developer role")
}

func TestConversionDiagnosticRejectsUnregisteredDescriptions(t *testing.T) {
	base := ConversionDiagnostic{Code: DiagnosticDeveloperRoleMerged, Source: types.RelayFormatOpenAI, Target: types.RelayFormatGemini, Field: "messages.role"}
	for _, field := range []string{"code", "source", "target", "field"} {
		t.Run(field, func(t *testing.T) {
			d := base
			switch field {
			case "code":
				d.Code = "private prompt"
			case "source":
				d.Source = "private prompt"
			case "target":
				d.Target = "private prompt"
			case "field":
				d.Field = "private prompt"
			}
			observer := &diagnosticObserver{}
			ReportConversionDiagnostic(observer, d)
			assert.Empty(t, observer.reports)
		})
	}
}

func TestResponsesDeveloperDiagnosticKeepsActualSource(t *testing.T) {
	observer := &diagnosticObserver{}
	d := ConversionDiagnostic{Code: DiagnosticDeveloperRoleMerged, Source: types.RelayFormatOpenAIResponses, Target: types.RelayFormatGemini, Field: "input.role"}
	ReportConversionDiagnostic(observer, d)
	require.Len(t, observer.reports, 1)
	assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), observer.reports[0].Source)
	assert.Equal(t, "input.role", observer.reports[0].Field)
	d.Field = "messages.role"
	ReportConversionDiagnostic(observer, d)
	assert.Len(t, observer.reports, 1, "source/field combinations must remain canonical")
}
