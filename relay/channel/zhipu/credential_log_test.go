package zhipu

import (
	"bytes"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestInvalidKeyDoesNotLeakCredentialToLogs(t *testing.T) {
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	original := gin.DefaultWriter
	gin.DefaultWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = original
		common.LogWriterMu.Unlock()
	})
	key := "incorrectly-formatted-private-provider-credential"
	assert.Empty(t, getZhipuToken(key))
	assert.Contains(t, logs.String(), "invalid zhipu key format")
	assert.NotContains(t, logs.String(), key)
}
