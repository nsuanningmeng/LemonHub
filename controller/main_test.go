package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Upstream-model fetch tests dial httptest servers on loopback ephemeral ports,
// which the fork's default-on SSRF guard (ValidateRelayTargetURL) rejects. The
// guard itself is covered by the fetch-setting validator tests; disable it here
// the same way relay/channel/advancedcustom does in its TestMain.
func TestMain(m *testing.M) {
	fs := system_setting.GetFetchSetting()
	fs.EnableSSRFProtection, fs.AllowPrivateIp = false, true
	m.Run()
}
