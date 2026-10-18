package jimeng

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayloadHashDoesNotLogPrivateRequestBody(t *testing.T) {
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	originalWriter, originalErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = &logs, &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = originalWriter, originalErrorWriter
		common.LogWriterMu.Unlock()
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := map[string]string{"prompt": "private customer prompt", "image_url": "https://example.test/private?signature=secret"}
	body, err := common.Marshal(request)
	require.NoError(t, err)
	require.NoError(t, SetPayloadHash(c, request))
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256(body)), getPayloadHash(c))
	assert.NotContains(t, logs.String(), request["prompt"])
	assert.NotContains(t, logs.String(), request["image_url"])
}
