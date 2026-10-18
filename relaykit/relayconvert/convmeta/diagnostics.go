package convmeta

import (
	"reflect"

	"github.com/QuantumNous/new-api/relaykit/types"
)

const DiagnosticDeveloperRoleMerged = "developer_role_merged"
const DiagnosticTrailingAssistantOmitted = "trailing_assistant_omitted"
const DiagnosticGeminiSchemaNormalized = "gemini_schema_normalized"

// ConversionDiagnostic describes a compatibility mapping without request data.
// Only registered codes and their canonical protocol fields may be reported.
type ConversionDiagnostic struct {
	Code    string            `json:"code"`
	Source  types.RelayFormat `json:"source"`
	Target  types.RelayFormat `json:"target"`
	Field   string            `json:"field"`
	Message string            `json:"message"`
}

// ConversionDiagnosticObserver is optional; Meta implementations need not
// implement it. Observers must tolerate a nil receiver.
type ConversionDiagnosticObserver interface {
	RecordConversionDiagnostic(ConversionDiagnostic)
}

// NormalizeConversionDiagnostic admits only fixed, redacted compatibility
// descriptions. Caller-supplied messages never enter logs or consume metadata.
func NormalizeConversionDiagnostic(d ConversionDiagnostic) (ConversionDiagnostic, bool) {
	if d.Code == DiagnosticGeminiSchemaNormalized && d.Source == types.RelayFormatGemini && d.Target == types.RelayFormatOpenAI && d.Field == "tools.functionDeclarations.parameters" {
		d.Message = "Gemini presentation-only schema fields were omitted while converting tool parameters to JSON Schema"
		return d, true
	}

	if d.Code == DiagnosticTrailingAssistantOmitted && d.Source == types.RelayFormatOpenAIResponses &&
		(d.Target == types.RelayFormatClaude || d.Target == types.RelayFormatGemini) && d.Field == "input" {
		d.Message = "Completed assistant output after the last user message or tool result was omitted to avoid treating Responses history as an assistant prefill"
		return d, true
	}
	validSourceField := (d.Source == types.RelayFormatOpenAI && d.Field == "messages.role") ||
		(d.Source == types.RelayFormatOpenAIResponses && d.Field == "input.role")
	if d.Code != DiagnosticDeveloperRoleMerged || d.Target != types.RelayFormatGemini || !validSourceField {
		return ConversionDiagnostic{}, false
	}
	d.Message = "Gemini has no developer role; developer messages are merged into systemInstruction using the existing compatibility mapping"
	return d, true
}

// ReportConversionDiagnostic also handles typed-nil third-party Meta values
// without invoking their optional observer method.
func ReportConversionDiagnostic(info Meta, diagnostic ConversionDiagnostic) {
	if info == nil {
		return
	}
	value := reflect.ValueOf(info)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return
		}
	}
	diagnostic, ok := NormalizeConversionDiagnostic(diagnostic)
	if !ok {
		return
	}
	if observer, ok := info.(ConversionDiagnosticObserver); ok {
		observer.RecordConversionDiagnostic(diagnostic)
	}
}
