package model

import (
	"math/big"

	"github.com/QuantumNous/new-api/common"
)

// Campaign totals are not wallet balances: an unlimited audience may legitimately
// claim more than one wallet's ceiling. Read SQL decimal sums as text. SQLite's
// integer SUM can overflow int64, so that dialect accumulates narrow ledger rows
// with big.Int instead of converting money to floating point.
func populateRewardClaimStatistics(items []RewardCampaign) error {
	ids := make([]int, len(items))
	positions := make(map[int]int, len(items))
	for i := range items {
		ids[i] = items[i].Id
		positions[items[i].Id] = i
		items[i].ClaimedQuotaExact = "0"
		items[i].ClaimedQuota = 0
		items[i].ClaimedCount = 0
	}
	totals := make(map[int]*big.Int, len(items))
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		rows, err := DB.Model(&UserReward{}).Select("campaign_id, quota_awarded").
			Where("campaign_id IN ? AND status = ?", ids, RewardStatusClaimed).Rows()
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var campaignID int
			var amount int64
			if err := rows.Scan(&campaignID, &amount); err != nil {
				return err
			}
			if totals[campaignID] == nil {
				totals[campaignID] = new(big.Int)
			}
			totals[campaignID].Add(totals[campaignID], big.NewInt(amount))
			items[positions[campaignID]].ClaimedCount++
		}
		if err := rows.Err(); err != nil {
			return err
		}
	} else {
		var claims []struct {
			CampaignID int
			Count      int64
			Quota      string
		}
		if err := DB.Model(&UserReward{}).
			Select("campaign_id, COUNT(*) AS count, SUM(quota_awarded) AS quota").
			Where("campaign_id IN ? AND status = ?", ids, RewardStatusClaimed).
			Group("campaign_id").Scan(&claims).Error; err != nil {
			return err
		}
		for _, claim := range claims {
			total, valid := new(big.Int).SetString(claim.Quota, 10)
			if !valid {
				return ErrInvalidRewardCampaign
			}
			totals[claim.CampaignID] = total
			items[positions[claim.CampaignID]].ClaimedCount = claim.Count
		}
	}
	for campaignID, total := range totals {
		if total.Sign() < 0 {
			return ErrInvalidRewardCampaign
		}
		i := positions[campaignID]
		items[i].ClaimedQuotaExact = total.String()
		if total.IsInt64() {
			items[i].ClaimedQuota = total.Int64()
		}
	}
	return nil
}
