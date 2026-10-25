package model

import (
	"bytes"
	cryptorand "crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The database transactions are real; only the external clock and entropy
// sources are controlled. This exercises a deadline crossing without sleeps.
func TestRewardClaimExpiryBoundaries(t *testing.T) {
	for _, database := range []string{"sqlite", "mysql"} {
		t.Run(database, func(t *testing.T) {
			if database == "mysql" {
				db := setupRewardMySQLTestDB(t)
				require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &Log{}, &RewardCampaign{}, &UserReward{}))
			} else {
				setupRewardTestDB(t)
			}

			const deadline int64 = 1790000000
			for _, rewardType := range []string{RewardTypeFixed, RewardTypeRandom} {
				t.Run(rewardType, func(t *testing.T) {
					for index, tc := range []struct {
						name  string
						times []int64
						claim bool
					}{
						{"before_deadline", []int64{deadline - 1}, true},
						{"at_deadline_after_locks", []int64{deadline}, false},
						{"deadline_before_recording_claim", []int64{deadline - 1, deadline}, false},
						{"deadline_after_wallet_write", []int64{deadline - 1, deadline - 1, deadline}, false},
					} {
						t.Run(tc.name, func(t *testing.T) {
							user := createReserveTestUser(t, 100)
							minimum, maximum := 25, 25
							if rewardType == RewardTypeRandom {
								minimum, maximum = 7, 11
							}
							campaign := RewardCampaign{
								Title: "Deadline reward", Type: rewardType, MinQuota: minimum, MaxQuota: maximum,
								RequestID: fmt.Sprintf("deadline-%s-%d", rewardType, index), CreatedBy: user.Id,
								CreatedAt: deadline - 86400, ExpiresInDays: 1, ExpiresAt: deadline, RecipientCount: 1,
							}
							require.NoError(t, DB.Create(&campaign).Error)
							reward := UserReward{
								CampaignID: campaign.Id, UserID: user.Id, Title: campaign.Title, Type: rewardType,
								MinQuota: minimum, MaxQuota: maximum, Status: RewardStatusPending,
								CreatedAt: campaign.CreatedAt, ExpiresAt: deadline,
							}
							require.NoError(t, DB.Create(&reward).Error)
							originalRandom := cryptorand.Reader
							cryptorand.Reader = bytes.NewReader([]byte{0})
							t.Cleanup(func() { cryptorand.Reader = originalRandom })
							nextTime := 0
							clock := func() int64 {
								timestamp := tc.times[min(nextTime, len(tc.times)-1)]
								nextTime++
								return timestamp
							}

							claimed, fresh, err := claimUserReward(user.Id, reward.Id, clock)
							var persisted UserReward
							require.NoError(t, DB.First(&persisted, reward.Id).Error)
							if tc.claim {
								require.NoError(t, err)
								require.NotNil(t, claimed)
								assert.True(t, fresh)
								assert.Equal(t, RewardStatusClaimed, persisted.Status)
								assert.Equal(t, deadline-1, persisted.ClaimedAt)
								assert.Equal(t, minimum, persisted.QuotaAwarded)
								assert.Equal(t, 100+minimum, getUserQuotaFromDB(t, user.Id))
								retried, newClaim, retryErr := claimUserReward(user.Id, reward.Id, func() int64 { return deadline + 1 })
								require.NoError(t, retryErr)
								assert.False(t, newClaim)
								assert.Equal(t, claimed, retried, "a committed reward remains idempotent after the deadline")
								assert.Equal(t, 100+minimum, getUserQuotaFromDB(t, user.Id))
							} else {
								assert.ErrorIs(t, err, ErrRewardExpired)
								assert.Nil(t, claimed)
								assert.False(t, fresh)
								assert.Equal(t, reward, persisted, "expiry must roll back both the claim and its awarded amount")
								assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
							}
							campaigns, _, err := ListRewardCampaigns(1, 1)
							require.NoError(t, err)
							require.Len(t, campaigns, 1)
							assert.Equal(t, campaign.Id, campaigns[0].Id)
							assert.Equal(t, 1, campaigns[0].RecipientCount)
							if tc.claim {
								assert.EqualValues(t, 1, campaigns[0].ClaimedCount)
								assert.EqualValues(t, minimum, campaigns[0].ClaimedQuota)
							} else {
								assert.Zero(t, campaigns[0].ClaimedCount)
								assert.Zero(t, campaigns[0].ClaimedQuota)
							}
						})
					}
				})
			}
		})
	}
}
