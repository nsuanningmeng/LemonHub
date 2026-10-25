package controller

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetRewardSummary(c *gin.Context) {
	summary, err := model.GetUserRewardSummary(c.GetInt("id"))
	if errors.Is(err, model.ErrRewardUserUnavailable) {
		common.ApiSuccess(c, model.RewardSummary{})
		return
	}
	if err != nil {
		rewardAPIError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func GetUserRewards(c *gin.Context) {
	page := common.GetPageQuery(c)
	items, total, err := model.ListUserRewards(c.GetInt("id"), c.Query("status"), c.Query("type"), page.GetPage(), page.GetPageSize())
	if errors.Is(err, model.ErrRewardUserUnavailable) {
		common.ApiSuccess(c, gin.H{"items": []model.UserReward{}, "total": 0})
		return
	}
	if err != nil {
		rewardAPIError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": total})
}

func ClaimReward(c *gin.Context) {
	rewardID, err := strconv.Atoi(c.Param("id"))
	if err != nil || rewardID <= 0 {
		common.ApiErrorMsg(c, "Invalid reward ID")
		return
	}
	userID := c.GetInt("id")
	reward, claimed, err := model.ClaimUserReward(userID, rewardID)
	if err != nil {
		rewardAPIError(c, err)
		return
	}
	if claimed {
		model.RecordLog(userID, model.LogTypeSystem, fmt.Sprintf("领取奖励 %s（活动 #%d，奖励 #%d），获得额度 %s", reward.Title, reward.CampaignID, reward.Id, logger.LogQuota(reward.QuotaAwarded)))
	}
	// Read the committed wallet value. A retry after a response failure returns
	// this same reward without crediting the account a second time.
	quota, err := model.GetUserQuota(userID, true)
	if err != nil {
		rewardAPIError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"reward": reward, "quota": quota})
}

func PreviewRewardCampaign(c *gin.Context) {
	var filter model.RewardAudienceFilter
	if err := c.ShouldBindJSON(&filter); err != nil {
		common.ApiErrorMsg(c, "Invalid reward audience")
		return
	}
	preview, err := model.PreviewRewardRecipients(filter)
	if err != nil {
		rewardAPIError(c, err)
		return
	}
	common.ApiSuccess(c, preview)
}

func CreateRewardCampaign(c *gin.Context) {
	var input model.RewardCampaignInput
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorMsg(c, "Invalid reward campaign")
		return
	}
	campaign, err := model.CreateRewardCampaign(input, c.GetInt("id"))
	if err != nil {
		rewardAPIError(c, err)
		return
	}
	if campaign.Continuous {
		service.WakeRewardDistribution()
	}
	common.ApiSuccess(c, campaign)
}

func GetRewardCampaigns(c *gin.Context) {
	page := common.GetPageQuery(c)
	items, total, err := model.ListRewardCampaigns(page.GetPage(), page.GetPageSize())
	if err != nil {
		rewardAPIError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": total})
}

func rewardAPIError(c *gin.Context, err error) {
	for _, expected := range []error{
		model.ErrInvalidRewardCampaign, model.ErrInvalidRewardAudience,
		model.ErrRewardRequestConflict, model.ErrRewardRecipientsEmpty,
		model.ErrRewardRecipientsLimit, model.ErrRewardNotFound,
		model.ErrRewardAudienceChanged,
		model.ErrRewardExpired,
		model.ErrRewardUserUnavailable, model.ErrInvalidRewardStatus,
		model.ErrTopUpQuotaLimitExceeded, model.ErrInvalidTopUpQuota,
	} {
		if errors.Is(err, expected) {
			common.ApiError(c, expected)
			return
		}
	}
	common.SysError("reward operation failed: " + err.Error())
	common.ApiErrorMsg(c, "Unable to process rewards. Please try again.")
}
