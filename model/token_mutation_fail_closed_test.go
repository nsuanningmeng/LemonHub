package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRestrictiveMutationsFailClosedWhenCacheCannotBeInvalidated(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)

	token := createReserveTestToken(t, 100)
	_, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	server.Close()

	token.Status = common.TokenStatusDisabled
	err = token.SelectUpdate()
	assert.Error(t, err)
	stored := getTokenFromDB(t, token.Id)
	assert.Equal(t, common.TokenStatusEnabled, stored.Status, "a cached enabled token must not be disabled only in the database")

	err = token.Delete()
	assert.Error(t, err)
	require.NoError(t, DB.First(&stored, token.Id).Error, "the token must remain valid when its cache cannot be revoked")

	deleted, err := BatchDeleteTokens([]int{token.Id}, token.UserId)
	assert.Zero(t, deleted)
	assert.Error(t, err)
	require.NoError(t, DB.First(&stored, token.Id).Error)
}
