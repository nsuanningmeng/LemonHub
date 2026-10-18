package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"

	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
)

func TestRelayInfoConvOptionsUsesNormalizedGeminiSafetySettings(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	original := settings.SafetySettings
	t.Cleanup(func() {
		settings.SafetySettings = original
	})
	settings.SafetySettings = map[string]string{
		"HARM_CATEGORY_HATE_SPEECH":       "",
		"HARM_CATEGORY_DANGEROUS_CONTENT": "BLOCK_ONLY_HIGH",
	}

	options := (&RelayInfo{}).ConvOptions()

	assert.Equal(t, "OFF", options.Gemini.SafetySetting("HARM_CATEGORY_HATE_SPEECH"))
	assert.Equal(t, "BLOCK_ONLY_HIGH", options.Gemini.SafetySetting("HARM_CATEGORY_DANGEROUS_CONTENT"))
}

func TestRelayInfoConvOptionsGeminiJSONSchemaCapability(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	original := settings.VersionSettings
	t.Cleanup(func() { settings.VersionSettings = original })
	settings.VersionSettings = map[string]string{"default": "v1beta", "gemini-v1-fixture": "v1"}
	for _, tc := range []struct {
		name        string
		channelType int
		model       string
		want        bool
	}{
		{"Gemini beta", constant.ChannelTypeGemini, "gemini-fixture", true},
		{"Gemini v1", constant.ChannelTypeGemini, "gemini-v1-fixture", false},
		{"Vertex ignores Gemini API version", constant.ChannelTypeVertexAi, "gemini-v1-fixture", true},
		{"custom Gemini v1", constant.ChannelTypeAdvancedCustom, "gemini-v1-fixture", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: tc.channelType}}
			assert.Equal(t, tc.want, info.ConvOptions().Gemini.SupportsJSONSchema(tc.model))
		})
	}
}
