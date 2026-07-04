package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMappingContext(t *testing.T, mapping string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("model_mapping", mapping)
	return c
}

// Model mapping must stay invisible to users: OriginModelName (billing/log
// display) keeps the requested model while UpstreamModelName carries the
// mapped model, and the mapped name is exposed to the error-scrub context key.
func TestModelMappedHelperKeepsOriginModelForUsers(t *testing.T) {
	c := newMappingContext(t, `{"gpt-5":"gpt-5-mini"}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5"},
	}

	require.NoError(t, ModelMappedHelper(c, info, nil))

	assert.True(t, info.IsModelMapped)
	assert.Equal(t, "gpt-5", info.OriginModelName)
	assert.Equal(t, "gpt-5-mini", info.UpstreamModelName)
	assert.Equal(t, "gpt-5-mini", c.GetString(string(constant.ContextKeyUpstreamModelName)))
}

// In ResponsesCompact mode the billing/log model must also be derived from the
// REQUESTED model (+ compact suffix), never from the mapped upstream model.
func TestModelMappedHelperCompactUsesRequestedModelForBilling(t *testing.T) {
	compactModel := ratio_setting.WithCompactModelSuffix("gpt-5")
	c := newMappingContext(t, `{"gpt-5":"gpt-5-mini"}`)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeResponsesCompact,
		OriginModelName: compactModel,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: compactModel},
	}

	require.NoError(t, ModelMappedHelper(c, info, nil))

	assert.True(t, info.IsModelMapped)
	assert.Equal(t, compactModel, info.OriginModelName)
	assert.Equal(t, "gpt-5-mini", info.UpstreamModelName)
}

func TestModelMappedHelperNoMappingClearsContextKey(t *testing.T) {
	c := newMappingContext(t, `{}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5"},
	}

	require.NoError(t, ModelMappedHelper(c, info, nil))

	assert.False(t, info.IsModelMapped)
	assert.Equal(t, "gpt-5", info.UpstreamModelName)
	assert.Empty(t, c.GetString(string(constant.ContextKeyUpstreamModelName)))
}

// A retry can move from a mapped channel to an unmapped one on the same gin
// context. The context key must be overwritten on every attempt, otherwise
// the stale mapped name from the previous channel scrubs the wrong channel's
// error text.
func TestModelMappedHelperRetryOverwritesStaleContextKey(t *testing.T) {
	c := newMappingContext(t, `{"gpt-5-mini":"gpt-5"}`)
	first := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5-mini",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5-mini"},
	}
	require.NoError(t, ModelMappedHelper(c, first, nil))
	require.Equal(t, "gpt-5", c.GetString(string(constant.ContextKeyUpstreamModelName)))

	// Retry lands on a channel without mapping.
	c.Set("model_mapping", "")
	second := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5-mini",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5-mini"},
	}
	require.NoError(t, ModelMappedHelper(c, second, nil))

	assert.False(t, second.IsModelMapped)
	assert.Empty(t, c.GetString(string(constant.ContextKeyUpstreamModelName)))
}
