package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTokenValidationPreservesStoredIntentWithAndWithoutRedis(t *testing.T) {
	for _, withRedis := range []bool{false, true} {
		t.Run(fmt.Sprintf("redis_%t", withRedis), func(t *testing.T) {
			truncateTables(t)
			if withRedis {
				useUserCacheMiniRedis(t)
			}
			for _, tc := range []struct {
				name             string
				status           int
				expiry           int64
				quota            int
				unlimited, valid bool
			}{
				{"expired", 1, common.GetTimestamp() - 100, 10, false, false},
				{"exhausted", 1, -1, 0, false, false},
				{"manual", 2, -1, 10, false, false},
				{"legacy_expired_repaired", 3, -1, 10, false, false},
				{"legacy_exhausted_repaired", 4, -1, 10, false, false},
				{"unlimited", 1, -1, 0, true, true},
				{"valid", 1, -1, 10, false, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					token := Token{UserId: 1, Key: fmt.Sprintf("intent-%t-%s", withRedis, tc.name), Status: tc.status, ExpiredTime: tc.expiry, RemainQuota: tc.quota, UnlimitedQuota: tc.unlimited}
					require.NoError(t, DB.Create(&token).Error)
					if withRedis {
						require.NoError(t, cacheSetTokenForTest(token))
					}
					for attempt := 0; attempt < 2; attempt++ {
						got, err := ValidateUserToken(token.Key)
						if tc.valid {
							require.NoError(t, err)
						} else {
							require.ErrorIs(t, err, ErrTokenInvalid)
						}
						require.NotNil(t, got)
						assert.Equal(t, tc.status, got.Status)
					}
					var stored Token
					require.NoError(t, DB.First(&stored, token.Id).Error)
					assert.Equal(t, tc.status, stored.Status)
					if withRedis {
						cached, err := cacheGetTokenByKey(token.Key)
						require.NoError(t, err)
						assert.Equal(t, tc.status, cached.Status)
					}
				})
			}
		})
	}
}
