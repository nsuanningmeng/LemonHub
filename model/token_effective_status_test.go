package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTokenStatusQueriesUseOneExpiryBoundaryAndOwner(t *testing.T) {
	truncateTables(t)
	const now int64 = 1700000000
	entries := []struct {
		name              string
		stored, effective int
		expiry            int64
		quota             int
		unlimited         bool
	}{
		{"before", 1, 3, now - 1, 10, false},
		{"exact", 1, 1, now, 10, false},
		{"after", 1, 1, now + 1, 10, false},
		{"never", 1, 1, -1, 10, false},
		{"zero_epoch", 1, 3, 0, 10, false},
		{"exact_exhausted", 1, 4, now, 0, false},
		{"expired_exhausted", 1, 3, now - 1, 0, false},
		{"unlimited", 1, 1, now, 0, true},
		{"manual", 2, 2, now - 1, 0, false},
		{"legacy_expired", 3, 3, -1, 10, false},
		{"legacy_exhausted", 4, 4, -1, 10, false},
	}
	for _, e := range entries {
		token := Token{UserId: 77, Name: "boundary-" + e.name, Key: "boundary-key-" + e.name, Status: e.stored, ExpiredTime: e.expiry, RemainQuota: e.quota, UnlimitedQuota: e.unlimited}
		require.NoError(t, DB.Create(&token).Error)
		// GORM's default tag would otherwise replace explicit zero on Create.
		require.NoError(t, DB.Model(&token).Update("expired_time", e.expiry).Error)
		token.ExpiredTime = e.expiry
		assert.Equal(t, e.effective, token.EffectiveStatus(now))
	}
	other := Token{UserId: 78, Name: "boundary-other", Key: "boundary-key-other", ExpiredTime: now - 1, UnlimitedQuota: true}
	require.NoError(t, DB.Create(&other).Error)
	for status := common.TokenStatusEnabled; status <= common.TokenStatusExhausted; status++ {
		filter := TokenStatusFilter{Status: status, Now: now}
		expected := map[string]bool{}
		for _, e := range entries {
			if e.effective == status {
				expected["boundary-"+e.name] = true
			}
		}
		total, err := CountUserTokens(77, filter)
		require.NoError(t, err)
		assert.EqualValues(t, len(expected), total)
		got, err := GetAllUserTokens(77, 0, 100, filter)
		require.NoError(t, err)
		assert.Len(t, got, len(expected))
		for _, token := range got {
			assert.Contains(t, expected, token.Name)
			assert.Equal(t, status, token.EffectiveStatus(now))
		}
		searched, count, err := SearchUserTokens(77, "boundary-%", "", 0, 100, filter)
		require.NoError(t, err)
		assert.EqualValues(t, len(expected), count)
		assert.Len(t, searched, len(expected))
		for _, token := range searched {
			assert.Contains(t, expected, token.Name)
		}
	}
	exact, count, err := SearchUserTokens(77, "boundary-exact", "sk-boundary-key-exact", 0, 2, TokenStatusFilter{Status: 1, Now: now})
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	require.Len(t, exact, 1)
	assert.Equal(t, "boundary-exact", exact[0].Name)
	excluded, count, err := SearchUserTokens(77, "boundary-exact", "sk-boundary-key-exact", 0, 2, TokenStatusFilter{Status: 3, Now: now})
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Empty(t, excluded)
}

func TestTokenStatusFiltersMatchNullableLegacyZeroValues(t *testing.T) {
	truncateTables(t)
	const now int64 = 1700000000
	for _, tc := range []struct {
		name, column string
		want         int
	}{
		{"unlimited_null", "unlimited_quota", common.TokenStatusExhausted},
		{"quota_null", "remain_quota", common.TokenStatusExhausted},
		{"expiry_null", "expired_time", common.TokenStatusExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := Token{UserId: 79, Name: tc.name, Key: "nullable-" + tc.name, Status: 1, ExpiredTime: -1, RemainQuota: 0, UnlimitedQuota: false}
			require.NoError(t, DB.Create(&token).Error)
			require.NoError(t, DB.Model(&token).Update(tc.column, nil).Error)
			var actual Token
			require.NoError(t, DB.First(&actual, token.Id).Error)
			require.Equal(t, tc.want, actual.EffectiveStatus(now))
			rows, total, err := SearchUserTokens(79, tc.name, "", 0, 10, TokenStatusFilter{Status: tc.want, Now: now})
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			require.Len(t, rows, 1)
			assert.Equal(t, token.Id, rows[0].Id)
		})
	}
}
