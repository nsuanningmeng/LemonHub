package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelSettingsErrorOverrideText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		settings    ChannelSettings
		wantEnabled bool
		wantText    string
	}{
		{
			name:        "disabled returns nothing even with text",
			settings:    ChannelSettings{ErrorOverrideMessage: "自定义文本"},
			wantEnabled: false,
		},
		{
			name:        "enabled with custom text",
			settings:    ChannelSettings{ErrorOverrideEnabled: true, ErrorOverrideMessage: "  渠道繁忙，请稍后重试  "},
			wantEnabled: true,
			wantText:    "渠道繁忙，请稍后重试",
		},
		{
			name:        "enabled with empty text falls back to default",
			settings:    ChannelSettings{ErrorOverrideEnabled: true, ErrorOverrideMessage: "   "},
			wantEnabled: true,
			wantText:    defaultErrorOverrideMessage,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			text, enabled := tc.settings.ErrorOverrideText()
			require.Equal(t, tc.wantEnabled, enabled)
			assert.Equal(t, tc.wantText, text)
		})
	}
}
