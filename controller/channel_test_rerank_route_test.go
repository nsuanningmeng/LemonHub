package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelProbeRerankRouteMatchesActualWire(t *testing.T) {
	for _, tc := range []struct{ name, model, endpoint, path string }{
		{"bge reranker", "bge-reranker-v2-m3", "", "/v1/rerank"},
		{"Qwen reranker", "Qwen3-Reranker-8B", "", "/v1/rerank"},
		{"bge embedding", "bge-m3", "", "/v1/embeddings"},
		{"explicit embedding", "bge-reranker-v2-m3", string(constant.EndpointTypeEmbeddings), "/v1/embeddings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user, token, _ := cancellationHostFixture(t, "openai")
			oldRatios := ratio_setting.ModelRatio2JSONString()
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios)) })
			ratios, _ := json.Marshal(map[string]float64{tc.model: 1})
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratios)))
			bodyCh := make(chan []byte, 1)
			pathCh := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				bodyCh <- body
				pathCh <- r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/rerank" {
					_, _ = io.WriteString(w, `{"results":[{"index":0,"relevance_score":0.9}],"usage":{"total_tokens":10}}`)
				} else {
					_, _ = io.WriteString(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1]}],"usage":{"prompt_tokens":10,"total_tokens":10}}`)
				}
			}))
			defer upstream.Close()
			channel := &model.Channel{Id: 101, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "local-test-key", Models: tc.model, Group: "default", BaseURL: &upstream.URL}
			result := testChannel(context.Background(), channel, user.Id, tc.model, tc.endpoint, false)
			require.Nil(t, result.newAPIError)
			require.NoError(t, result.localErr)
			assert.Equal(t, tc.path, <-pathCh)
			wire := <-bodyCh
			if tc.path == "/v1/rerank" {
				var req dto.RerankRequest
				require.NoError(t, json.Unmarshal(wire, &req))
				assert.Equal(t, tc.model, req.Model)
				assert.NotEmpty(t, req.Query)
				assert.Len(t, req.Documents, 2)
				assert.NotContains(t, string(wire), `"input"`)
			} else {
				var req dto.EmbeddingRequest
				require.NoError(t, json.Unmarshal(wire, &req))
				assert.Equal(t, tc.model, req.Model)
				assert.NotEmpty(t, req.Input)
				assert.NotContains(t, string(wire), `"documents"`)
			}
			assertRecoveryNoWalletDebit(t, db, user, token)
		})
	}
}
