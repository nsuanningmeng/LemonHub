package types

import (
	"errors"
	"io"
	"net/url"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHiddenErrorDoesNotPrintOriginalCredentialInDebugMode(t *testing.T) {
	previousDebug := kitutil.Debug.Load()
	kitutil.Debug.Store(true)
	t.Cleanup(func() { kitutil.Debug.Store(previousDebug) })
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	previousStdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = previousStdout; _ = writer.Close() })

	upstreamErr := &url.Error{Op: "Post", URL: "https://provider.test/request?custom_auth=private-provider-key", Err: errors.New("dial failed")}
	wrapped := NewError(upstreamErr, ErrorCodeDoRequestFailed, ErrOptionWithHideErrMsg("upstream request failed"))
	require.NoError(t, writer.Close())
	os.Stdout = previousStdout
	printed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.NotContains(t, string(printed), "private-provider-key")
	assert.NotContains(t, wrapped.Error(), "private-provider-key")
	assert.Contains(t, wrapped.Error(), "upstream request failed")
}
