package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageAndCompactionCacheCreationAliasReachesBilling(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	const usageJSON = `"usage":{"input_tokens":1000,"output_tokens":20,"total_tokens":1020,"input_tokens_details":{"cached_tokens":100,"cache_write_tokens":250,"cache_creation_input_tokens":300}}`
	check := func(t *testing.T, usage *dto.Usage, write int) {
		t.Helper()
		require.NotNil(t, usage)
		assert.Equal(t, 1000, usage.PromptTokens)
		assert.Equal(t, write, usage.PromptTokensDetails.CacheCreationTokensTotal())
		params := service.BuildTieredTokenParams(usage, false, map[string]bool{"cr": true, "cc": true})
		assert.Equal(t, float64(write), params.CC)
		assert.Equal(t, float64(900-write), params.P)
	}
	for _, tc := range []struct {
		name        string
		stream      bool
		contentType string
	}{
		{"image JSON", false, "application/json"},
		{"image SSE", true, "text/event-stream"},
		{"image JSON for streaming client", true, "application/json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"data":[{"b64_json":"image"}],` + usageJSON + `}`
			if tc.contentType == "text/event-stream" {
				body = `data: {"type":"image_generation.completed","b64_json":"image",` + usageJSON + "}\n\ndata: [DONE]\n\n"
			}
			c, recorder, resp, info := newImageTestContext(t, body, tc.contentType, tc.stream)
			info.RelayMode = relayconstant.RelayModeImagesGenerations
			result, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			usage, ok := result.(*dto.Usage)
			require.True(t, ok)
			check(t, usage, 300)
			assert.Contains(t, recorder.Body.String(), `"cache_creation_input_tokens":300`)
		})
	}
	for _, tc := range []struct {
		name  string
		usage string
		write int
	}{
		{"explicit alias larger", usageJSON, 300},
		{"legacy alias larger", strings.Replace(usageJSON, `"cache_write_tokens":250`, `"cache_write_tokens":250,"cached_creation_tokens":400`, 1), 400},
	} {
		t.Run("Responses compaction/"+tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
			body := "{" + tc.usage + "}"
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			usage, apiErr := OaiResponsesCompactionHandler(c, resp)
			require.Nil(t, apiErr)
			check(t, usage, tc.write)
			assert.Equal(t, body, recorder.Body.String())
		})
	}
}

func TestImageCacheCreationAliasPreservesCanonicalUsage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		native string
	}{
		{"missing native alias", ""},
		{"conflicting native alias", `,"cache_creation_input_tokens":400`},
		{"negative native alias", `,"cache_creation_input_tokens":-400`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"data":[{"b64_json":"image"}],"usage":{"input_tokens":1000,"output_tokens":20,"total_tokens":1020,"input_tokens_details":{"cached_tokens":100` + tc.native + `},"prompt_tokens_details":{"cache_creation_input_tokens":300}}}`
			c, _, resp, info := newImageTestContext(t, body, "application/json", false)
			info.RelayMode = relayconstant.RelayModeImagesGenerations
			result, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			usage, ok := result.(*dto.Usage)
			require.True(t, ok)
			assert.Equal(t, 300, usage.PromptTokensDetails.CacheCreationTokensTotal())
			params := service.BuildTieredTokenParams(usage, false, map[string]bool{"cr": true, "cc": true})
			assert.Equal(t, float64(300), params.CC)
			assert.Equal(t, float64(600), params.P)
		})
	}
}
