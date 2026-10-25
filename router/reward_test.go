package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRewardRouter(t *testing.T) (*gin.Engine, model.User) {
	t.Helper()
	engine := setupLoginOriginRouter(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RewardCampaign{}, &model.UserReward{}, &model.TopUp{}))
	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "attacker").First(&user).Error)
	user.SetAccessToken("reward-router-member")
	user.Quota = 100
	require.NoError(t, model.DB.Save(&user).Error)
	return engine, user
}

func TestRewardRoutesRequireAuthenticationAndAdminRole(t *testing.T) {
	engine, user := setupRewardRouter(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/rewards"},
		{http.MethodGet, "/api/rewards/summary"},
		{http.MethodPost, "/api/rewards/1/claim"},
		{http.MethodGet, "/api/reward-campaigns"},
		{http.MethodPost, "/api/reward-campaigns/preview"},
		{http.MethodPost, "/api/reward-campaigns"},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, route.path, nil)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, http.StatusUnauthorized, response.Code)
			if strings.HasPrefix(route.path, "/api/reward-campaigns") {
				request = httptest.NewRequest(route.method, route.path, nil)
				request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
				response = httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				assert.Equal(t, http.StatusForbidden, response.Code)
			}
		})
	}
}

func TestRewardClaimIgnoresClientAmountAndReturnsBalanceOnlyToOwner(t *testing.T) {
	engine, user := setupRewardRouter(t)
	reward := model.UserReward{CampaignID: 1, UserID: user.Id, Title: "Welcome", Type: model.RewardTypeFixed,
		MinQuota: 25, MaxQuota: 25, Status: model.RewardStatusPending}
	other := model.UserReward{CampaignID: 1, UserID: user.Id + 1, Title: "Private", Type: model.RewardTypeRandom,
		MinQuota: 1, MaxQuota: 50, Status: model.RewardStatusPending}
	require.NoError(t, model.DB.Create(&reward).Error)
	require.NoError(t, model.DB.Create(&other).Error)

	for _, test := range []struct {
		name string
		id   string
		want bool
	}{
		{"other owner", fmt.Sprint(other.Id), false},
		{"invalid id", "invalid", false},
		{"first claim", fmt.Sprint(reward.Id), true},
		{"retry", fmt.Sprint(reward.Id), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/rewards/"+test.id+"/claim", strings.NewReader(`{"quota_awarded":999999,"user_id":999999}`))
			request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var body struct {
				Success bool `json:"success"`
				Data    struct {
					Reward model.UserReward `json:"reward"`
					Quota  int              `json:"quota"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, test.want, body.Success, response.Body.String())
			if test.want {
				assert.Equal(t, 125, body.Data.Quota)
				assert.Equal(t, 25, body.Data.Reward.QuotaAwarded)
				assert.Equal(t, user.Id, body.Data.Reward.UserID)
				assert.Equal(t, model.RewardStatusClaimed, body.Data.Reward.Status)
			}
		})
	}
	var logs int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeSystem).Count(&logs).Error)
	assert.EqualValues(t, 1, logs, "retry must not duplicate the successful claim audit")

	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/rewards?status=claimed&type=fixed&user_id=%d&p=1&page_size=1", other.UserID), nil)
	request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	var page struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.UserReward `json:"items"`
			Total int                `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &page))
	require.True(t, page.Success, response.Body.String())
	require.Len(t, page.Data.Items, 1)
	assert.Equal(t, reward.Id, page.Data.Items[0].Id)
	assert.Equal(t, 1, page.Data.Total)
	assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
}

func TestRewardAdminPreviewIssuanceAndIdempotentRetry(t *testing.T) {
	engine, user := setupRewardRouter(t)
	require.NoError(t, model.DB.Model(&user).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: user.Id, Status: common.TopUpStatusSuccess,
		TradeNo: "reward-success", CompleteTime: 150}).Error)
	body := `{"title":"Customer reward","description":"Thank you","type":"fixed","min_quota":25,"max_quota":25,"audience":"topup","start_time":100,"end_time":200,"expires_in_days":3,"request_id":"router-campaign","expected_recipient_count":1}`
	var campaignID int
	var expiresAt int64
	for _, path := range []string{"/api/reward-campaigns/preview", "/api/reward-campaigns", "/api/reward-campaigns"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		var result struct {
			Success bool                 `json:"success"`
			Data    model.RewardCampaign `json:"data"`
		}
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())
		assert.Equal(t, 1, result.Data.RecipientCount)
		if path == "/api/reward-campaigns" {
			assert.Equal(t, 3, result.Data.ExpiresInDays)
			assert.Equal(t, result.Data.CreatedAt+3*24*60*60, result.Data.ExpiresAt)
			if campaignID != 0 {
				assert.Equal(t, campaignID, result.Data.Id)
				assert.Equal(t, expiresAt, result.Data.ExpiresAt, "retry must preserve the original deadline")
			}
			campaignID = result.Data.Id
			expiresAt = result.Data.ExpiresAt
		}
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.UserReward{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	var reward model.UserReward
	require.NoError(t, model.DB.First(&reward).Error)
	assert.Equal(t, expiresAt, reward.ExpiresAt)
	quota, err := model.GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 100, quota, "issuance must not credit the wallet until the user claims")
}

func TestRewardExpiredClaimReturnsActionableErrorWithoutWalletOrAuditCredit(t *testing.T) {
	engine, user := setupRewardRouter(t)
	reward := model.UserReward{CampaignID: 1, UserID: user.Id, Title: "Expired packet", Type: model.RewardTypeRandom,
		MinQuota: 25, MaxQuota: 50, Status: model.RewardStatusPending, ExpiresAt: common.GetTimestamp() - 60}
	require.NoError(t, model.DB.Create(&reward).Error)
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/rewards/%d/claim", reward.Id),
		strings.NewReader(`{"expires_at":0,"status":"pending","quota_awarded":999}`))
	request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.Equal(t, http.StatusOK, response.Code)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.False(t, result.Success)
	assert.Equal(t, "reward expired", result.Message)
	quota, err := model.GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 100, quota)
	var logs int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeSystem).Count(&logs).Error)
	assert.Zero(t, logs, "an expired claim must not emit a successful credit audit")
	for _, status := range []string{"pending", "expired"} {
		request = httptest.NewRequest(http.MethodGet, "/api/rewards?status="+status, nil)
		request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
		response = httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		var page struct {
			Success bool `json:"success"`
			Data    struct {
				Items []model.UserReward `json:"items"`
				Total int                `json:"total"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &page))
		require.True(t, page.Success, response.Body.String())
		if status == "pending" {
			assert.Zero(t, page.Data.Total)
			assert.Empty(t, page.Data.Items)
		} else {
			assert.Equal(t, 1, page.Data.Total)
			require.Len(t, page.Data.Items, 1)
			assert.Equal(t, "expired", page.Data.Items[0].Status)
			assert.Equal(t, reward.ExpiresAt, page.Data.Items[0].ExpiresAt)
		}
	}
}

func TestRewardIssuanceRejectsMissingOrInvalidExpiryDays(t *testing.T) {
	engine, admin := setupRewardRouter(t)
	require.NoError(t, model.DB.Model(&admin).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: admin.Id, Status: common.TopUpStatusSuccess,
		TradeNo: "expiry-validation", CompleteTime: 150}).Error)
	for _, field := range []string{"", `,"expires_in_days":0`, `,"expires_in_days":1.5`, `,"expires_in_days":3651`} {
		t.Run(field, func(t *testing.T) {
			body := `{"title":"Limited reward","type":"fixed","min_quota":25,"max_quota":25,"audience":"topup","start_time":100,"end_time":200,"request_id":"expiry-invalid"` + field + `}`
			request := httptest.NewRequest(http.MethodPost, "/api/reward-campaigns", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+admin.GetAccessToken())
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			var result struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.False(t, result.Success, response.Body.String())
		})
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.RewardCampaign{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRewardPreviewSupportsFourAudiencesAndExcludesDisabledUsers(t *testing.T) {
	engine, admin := setupRewardRouter(t)
	require.NoError(t, model.DB.Model(&admin).Update("role", common.RoleAdminUser).Error)
	users := []model.User{
		{Username: "topup-only", AffCode: "topup-only", Status: common.UserStatusEnabled},
		{Username: "usage-only", AffCode: "usage-only", Status: common.UserStatusEnabled},
		{Username: "both", AffCode: "both", Status: common.UserStatusEnabled},
		{Username: "disabled-both", AffCode: "disabled-both", Status: common.UserStatusDisabled},
	}
	require.NoError(t, model.DB.Create(&users).Error)
	for _, i := range []int{0, 2, 3} {
		require.NoError(t, model.DB.Create(&model.TopUp{UserId: users[i].Id,
			TradeNo: users[i].Username, CompleteTime: 150, Status: common.TopUpStatusSuccess}).Error)
	}
	for _, i := range []int{1, 2, 3} {
		require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: users[i].Id, Type: model.LogTypeConsume, CreatedAt: 150}).Error)
	}
	for _, test := range []struct {
		audience string
		count    int
	}{
		{"topup", 2}, {"usage", 2}, {"both", 1}, {"either", 3},
	} {
		t.Run(test.audience, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/reward-campaigns/preview",
				strings.NewReader(fmt.Sprintf(`{"audience":%q,"start_time":100,"end_time":200}`, test.audience)))
			request.Header.Set("Authorization", "Bearer "+admin.GetAccessToken())
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			var result struct {
				Success bool `json:"success"`
				Data    struct {
					RecipientCount int `json:"recipient_count"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			require.True(t, result.Success, response.Body.String())
			assert.Equal(t, test.count, result.Data.RecipientCount)
		})
	}
}

func TestRewardCampaignListReportsCommittedClaimProgressWithoutCountingRetries(t *testing.T) {
	engine, user := setupRewardRouter(t)
	require.NoError(t, model.DB.Model(&user).Update("role", common.RoleAdminUser).Error)
	campaign := model.RewardCampaign{Title: "Claim progress", Type: model.RewardTypeFixed,
		MinQuota: 25, MaxQuota: 25, RecipientCount: 1, RequestID: "progress", CreatedBy: user.Id}
	require.NoError(t, model.DB.Create(&campaign).Error)
	reward := model.UserReward{CampaignID: campaign.Id, UserID: user.Id, Title: campaign.Title,
		Type: model.RewardTypeFixed, MinQuota: 25, MaxQuota: 25, Status: model.RewardStatusPending}
	require.NoError(t, model.DB.Create(&reward).Error)
	for _, stage := range []struct {
		name         string
		claim        bool
		count, quota int64
	}{
		{"unclaimed", false, 0, 0}, {"claimed", true, 1, 25}, {"retried", true, 1, 25},
	} {
		t.Run(stage.name, func(t *testing.T) {
			if stage.claim {
				request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/rewards/%d/claim", reward.Id), nil)
				request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				var claim struct {
					Success bool `json:"success"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &claim))
				require.True(t, claim.Success, response.Body.String())
			}
			request := httptest.NewRequest(http.MethodGet, "/api/reward-campaigns?p=1&page_size=10", nil)
			request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			var result struct {
				Success bool `json:"success"`
				Data    struct {
					Items []struct {
						Id           int    `json:"id"`
						ClaimedCount *int64 `json:"claimed_count"`
						ClaimedQuota *int64 `json:"claimed_quota"`
					} `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			require.True(t, result.Success, response.Body.String())
			require.Len(t, result.Data.Items, 1)
			row := result.Data.Items[0]
			assert.Equal(t, campaign.Id, row.Id)
			require.NotNil(t, row.ClaimedCount, "zero claim counts must also be serialized")
			require.NotNil(t, row.ClaimedQuota, "zero claimed amounts must also be serialized")
			assert.Equal(t, stage.count, *row.ClaimedCount)
			assert.Equal(t, stage.quota, *row.ClaimedQuota)
		})
	}
}
