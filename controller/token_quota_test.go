package controller

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenQuotaSupportsLargeFiniteBalances(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("large token quotas require a 64-bit server build")
	}

	db := setupTokenControllerTestDB(t)
	require.NoError(t, appI18n.Init())
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	createQuota := int64(5_000_000_000)
	createBody := map[string]any{
		"name":                 "large-quota-token",
		"expired_time":         -1,
		"remain_quota":         createQuota,
		"unlimited_quota":      false,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "default",
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", createBody, 1)
	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var token model.Token
	require.NoError(t, db.Where("name = ?", "large-quota-token").First(&token).Error)
	assert.Equal(t, int(createQuota), token.RemainQuota)

	updateQuota := int64(500_000_000_000_000)
	updateBody := map[string]any{
		"id":                   token.Id,
		"name":                 token.Name,
		"status":               common.TokenStatusEnabled,
		"expired_time":         -1,
		"remain_quota":         updateQuota,
		"unlimited_quota":      false,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "default",
	}
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/", updateBody, 1)
	UpdateToken(ctx)

	response = decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, int(updateQuota), token.RemainQuota)

	const overBusinessLimit int64 = 500_000_000_000_001
	updateBody["remain_quota"] = overBusinessLimit
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/", updateBody, 1)
	UpdateToken(ctx)

	response = decodeAPIResponse(t, recorder)
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "500000000000000")
	assert.NotContains(t, response.Message, "2147483647")
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, int(updateQuota), token.RemainQuota)
}
