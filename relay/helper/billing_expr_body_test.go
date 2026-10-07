package helper

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type billingExprCountingStorage struct {
	common.BodyStorage
	bytesCalls int
	bytesError error
}

func (s *billingExprCountingStorage) Bytes() ([]byte, error) {
	s.bytesCalls++
	if s.bytesError != nil {
		return nil, s.bytesError
	}
	return s.BodyStorage.Bytes()
}

func setupBillingExprBodyTest(t *testing.T, expr, body string) (*gin.Context, *relaycommon.RelayInfo, *billingExprCountingStorage) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		gin.SetMode(oldMode)
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"body-test-model":"tiered_expr"}`,
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))
	setBillingExprBodyTestPrice(t, expr)
	rawStorage, err := common.CreateBodyStorage([]byte(body))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rawStorage.Close()) })
	storage := &billingExprCountingStorage{BodyStorage: rawStorage}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Tier", "fast")
	ctx.Set(common.KeyBodyStorage, storage)
	info := &relaycommon.RelayInfo{
		OriginModelName: "body-test-model", UserGroup: "default", UsingGroup: "default",
		RequestHeaders: map[string]string{"Content-Type": "application/json", "X-Tier": "fast"},
	}
	return ctx, info, storage
}

func setBillingExprBodyTestPrice(t *testing.T, expr string) {
	t.Helper()
	data, err := common.Marshal(map[string]string{"body-test-model": expr})
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_expr": string(data),
	}))
}

func TestTieredPriceWithoutParamNeverMaterializesRequestBody(t *testing.T) {
	for _, tt := range []struct {
		name, expr string
		want       int
	}{
		{"tokens", `tier("base", p * 2)`, 100},
		{"header only", `tier("base", p * 2) * (header("X-Tier") == "fast" ? 2 : 1)`, 200},
		{"time only", `hour("UTC") >= 0 ? tier("base", p * 2) : tier("base", 0)`, 100},
		{"param in string only", `v1:tier("param(service_tier)", p * 2)`, 100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, info, storage := setupBillingExprBodyTest(t, tt.expr, `{"model":"client-model"}`)
			storage.bytesError = errors.New("body must remain in storage")

			price, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})

			require.NoError(t, err)
			assert.Equal(t, tt.want, price.QuotaToPreConsume)
			assert.Zero(t, storage.bytesCalls)
			require.NotNil(t, info.BillingRequestInput)
			assert.Nil(t, info.BillingRequestInput.Body)
		})
	}
}

func TestTieredPriceRejectsInvalidExpressionBeforeReadingBody(t *testing.T) {
	ctx, info, storage := setupBillingExprBodyTest(t, `tier("base", p *)`, `{}`)
	storage.bytesError = errors.New("body must remain in storage")

	_, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{})

	require.ErrorContains(t, err, "expr compile error")
	assert.Zero(t, storage.bytesCalls)
	assert.Nil(t, info.BillingRequestInput)
}

func TestTieredPriceLazyCaptureKeepsClientInputAcrossReestimates(t *testing.T) {
	const clientBody = `{"model":"client-model","stream":true}`
	const paramExpr = `tier("base", p * 2) * (param("model") == "client-model" && param("stream") == true && header("X-Tier") == "fast" && header("X-Extra") == "" ? 3 : 1)`
	ctx, info, storage := setupBillingExprBodyTest(t, `tier("base", p * 2)`, clientBody)
	price, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 100, price.QuotaToPreConsume)
	assert.Zero(t, storage.bytesCalls)

	// These are upstream/routing mutations, not changes to the client billing contract.
	ctx.Request.Header.Set("Content-Type", "text/plain")
	ctx.Request.Header.Set("X-Tier", "slow")
	info.RequestHeaders["X-Tier"] = "slow"
	info.RequestHeaders["X-Extra"] = "added-after-snapshot"
	info.Request = &dto.GeneralOpenAIRequest{Model: "upstream-mapped-model"}
	setBillingExprBodyTestPrice(t, paramExpr)
	price, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 300, price.QuotaToPreConsume)
	assert.Equal(t, 1, storage.bytesCalls)
	assert.JSONEq(t, clientBody, string(info.BillingRequestInput.Body))

	// A body-free reestimate must neither replace the frozen body nor reread it.
	storage.bytesError = errors.New("the client body must already be frozen")
	setBillingExprBodyTestPrice(t, `tier("base", p * 2) * (header("X-Tier") == "fast" ? 2 : 1)`)
	price, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 200, price.QuotaToPreConsume)
	setBillingExprBodyTestPrice(t, paramExpr)
	price, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 300, price.QuotaToPreConsume)
	assert.Equal(t, 1, storage.bytesCalls)

	result, err := billingexpr.ComputeTieredQuotaWithRequest(info.TieredBillingSnapshot,
		billingexpr.TokenParams{P: 200, Len: 200}, *info.BillingRequestInput)
	require.NoError(t, err)
	assert.Equal(t, 600, result.ActualQuotaAfterGroup)
	require.Len(t, result.RequestRules, 1)
	assert.Equal(t, float64(3), result.RequestRules[0].Multiplier)
	assert.True(t, result.RequestRules[0].Matched)
}

func TestTieredPriceParamReadFailureDoesNotFreezeMissingBody(t *testing.T) {
	ctx, info, storage := setupBillingExprBodyTest(t, `tier("base", p * 2)`, `{"stream":true}`)
	_, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	setBillingExprBodyTestPrice(t, `tier("base", p * 2) * (param("stream") == true ? 2 : 1)`)
	readError := errors.New("client storage unavailable")
	storage.bytesError = readError

	_, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.ErrorIs(t, err, readError)
	assert.Nil(t, info.BillingRequestInput.Body)

	storage.bytesError = nil
	price, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 200, price.QuotaToPreConsume)
	assert.Equal(t, 2, storage.bytesCalls)
}

func TestBillingExprBodyFreeEvaluationOmitsPreloadedBody(t *testing.T) {
	ctx, info, storage := setupBillingExprBodyTest(t, `tier("base", p * 2)`, `{}`)
	storage.bytesError = errors.New("a preloaded input must remain authoritative")
	preloaded, err := BuildBillingExprRequestInputFromRequest(&dto.GeneralOpenAIRequest{Model: "client-model"}, info.RequestHeaders)
	require.NoError(t, err)
	info.BillingRequestInput = &preloaded

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info, false)

	require.NoError(t, err)
	assert.Nil(t, input.Body, "body-free evaluation must not retain a cloned payload")
	assert.Zero(t, storage.bytesCalls)
	input.Headers["X-Tier"] = "changed-evaluation-copy"
	assert.Equal(t, "fast", preloaded.Headers["X-Tier"])
	_, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Same(t, &preloaded, info.BillingRequestInput, "keep the original capture for a later param estimate")
	setBillingExprBodyTestPrice(t, `tier("base", param("model") == "client-model" ? p * 4 : p * 2)`)
	price, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 200, price.QuotaToPreConsume)
	assert.Zero(t, storage.bytesCalls)
}

func TestBillingExprCapturedEmptyBodyIsNotReadAgain(t *testing.T) {
	ctx, info, storage := setupBillingExprBodyTest(t, `tier("base", param("stream") == nil ? p * 2 : p * 4)`, "")
	price, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 100, price.QuotaToPreConsume)
	assert.Equal(t, 1, storage.bytesCalls)
	storage.bytesError = errors.New("an empty capture is complete")

	price, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})

	require.NoError(t, err)
	assert.Equal(t, 100, price.QuotaToPreConsume)
	assert.Equal(t, 1, storage.bytesCalls)
	// Cloning an empty capture can produce nil; that is still a complete capture.
	price, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 100, price.QuotaToPreConsume)
	assert.Equal(t, 1, storage.bytesCalls)
}

func TestBillingExprNonJSONInputDoesNotReadStorage(t *testing.T) {
	ctx, info, storage := setupBillingExprBodyTest(t, `tier("base", param("stream") == nil ? p * 2 : p * 4)`, "stream=true")
	info.RequestHeaders["Content-Type"] = "multipart/form-data; boundary=example"
	storage.bytesError = errors.New("param only reads original JSON bodies")
	price, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 100, price.QuotaToPreConsume)
	assert.Zero(t, storage.bytesCalls)
	info.RequestHeaders["Content-Type"] = "application/json"
	ctx.Request.Header.Set("Content-Type", "application/json")
	price, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 100, price.QuotaToPreConsume)
	assert.Zero(t, storage.bytesCalls)
}
