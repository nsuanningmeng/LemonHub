package service

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversionDiagnosticsPersistOnlyInAdminMetadataAndWarnOnce(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	d := convmeta.ConversionDiagnostic{Code: convmeta.DiagnosticDeveloperRoleMerged, Source: types.RelayFormatOpenAI, Target: types.RelayFormatGemini, Field: "messages.role", Message: "private prompt/schema sentinel"}
	convmeta.ReportConversionDiagnostic(info, d)
	convmeta.ReportConversionDiagnostic(info, d)
	var warnings bytes.Buffer
	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &warnings
	common.LogWriterMu.Unlock()
	t.Cleanup(func() { common.LogWriterMu.Lock(); gin.DefaultErrorWriter = oldWriter; common.LogWriterMu.Unlock() })
	for i := 0; i < 2; i++ {
		other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 0, 1)
		admin, ok := other["admin_info"].(map[string]interface{})
		require.True(t, ok)
		diagnostics, ok := admin["conversion_diagnostics"].([]convmeta.ConversionDiagnostic)
		require.True(t, ok)
		require.Len(t, diagnostics, 1)
		body, err := common.Marshal(other)
		require.NoError(t, err)
		assert.NotContains(t, string(body), "private prompt/schema sentinel")
		assert.NotContains(t, diagnostics[0].Message, "private")
		_, publicKey := other["conversion_diagnostics"]
		assert.False(t, publicKey)
	}
	assert.Equal(t, 1, strings.Count(warnings.String(), "request conversion: code=developer_role_merged"))
	assert.NotContains(t, warnings.String(), "private prompt/schema sentinel")
	info.ResetConversionDiagnostics()
	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 0, 1)
	admin := other["admin_info"].(map[string]interface{})
	_, ok := admin["conversion_diagnostics"]
	assert.False(t, ok, "next attempt must not inherit diagnostics")
	convmeta.ReportConversionDiagnostic(info, d)
	GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 0, 1)
	assert.Equal(t, 2, strings.Count(warnings.String(), "request conversion: code=developer_role_merged"))
}
